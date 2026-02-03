package web

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/andrewcostello/forecast/internal/config"
	"github.com/andrewcostello/forecast/internal/storage"
)

// Server represents the web server
type Server struct {
	config   *config.Config
	store    *storage.JSONStorage
	handlers *Handlers
	server   *http.Server
	devMode  bool
	staticFS fs.FS
}

// embeddedStatic holds embedded static files (populated at build time)
//
//go:embed static/*
var embeddedStatic embed.FS

// NewServer creates a new web server instance
func NewServer(cfg *config.Config, store *storage.JSONStorage, devMode bool) *Server {
	handlers := NewHandlers(cfg, store)

	var staticFS fs.FS
	if devMode {
		// In dev mode, serve from disk for hot reloading
		staticFS = os.DirFS("web/public")
	} else {
		// In production, use embedded files
		var err error
		staticFS, err = fs.Sub(embeddedStatic, "static")
		if err != nil {
			log.Printf("Warning: failed to access embedded static files: %v", err)
			staticFS = os.DirFS("web/public")
		}
	}

	return &Server{
		config:   cfg,
		store:    store,
		handlers: handlers,
		devMode:  devMode,
		staticFS: staticFS,
	}
}

// Start starts the web server
func (s *Server) Start(port string) error {
	mux := http.NewServeMux()

	// Register API routes
	RegisterRoutes(mux, s.handlers)

	// Serve static files
	fileServer := http.FileServer(http.FS(s.staticFS))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// For SPA routing, serve index.html for non-file requests
		if r.URL.Path != "/" && !hasFileExtension(r.URL.Path) {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	}))

	// Apply middleware
	handler := ChainMiddleware(
		mux,
		RecoverMiddleware,
		LoggingMiddleware,
		CORSMiddleware,
	)

	s.server = &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Channel to listen for errors coming from the listener
	serverErrors := make(chan error, 1)

	// Start the server
	go func() {
		log.Printf("Starting server on http://localhost:%s", port)
		if s.devMode {
			log.Println("Running in development mode - static files served from disk")
		}
		serverErrors <- s.server.ListenAndServe()
	}()

	// Channel to listen for interrupt/terminate signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Block until we receive a signal or an error
	select {
	case err := <-serverErrors:
		return fmt.Errorf("server error: %w", err)

	case sig := <-shutdown:
		log.Printf("Received signal %v, starting graceful shutdown", sig)

		// Give outstanding requests 30 seconds to complete
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := s.server.Shutdown(ctx); err != nil {
			s.server.Close()
			return fmt.Errorf("graceful shutdown failed: %w", err)
		}
	}

	return nil
}

// Stop gracefully stops the server
func (s *Server) Stop() error {
	if s.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return s.server.Shutdown(ctx)
}

// hasFileExtension checks if a path has a file extension
func hasFileExtension(path string) bool {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return false
		}
		if path[i] == '.' {
			return true
		}
	}
	return false
}

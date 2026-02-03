package web

import (
	"net/http"
)

// RegisterRoutes registers all API routes
func RegisterRoutes(mux *http.ServeMux, h *Handlers) {
	// Health check
	mux.HandleFunc("/api/health", h.HealthHandler())

	// Projects endpoints
	mux.HandleFunc("/api/projects", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/projects" || r.URL.Path == "/api/projects/" {
			h.ListProjectsHandler()(w, r)
			return
		}
		// Handle /api/projects/{key} and sub-paths
		h.GetProjectHandler()(w, r)
	})

	// Catch-all for project sub-paths
	mux.HandleFunc("/api/projects/", h.GetProjectHandler())
}

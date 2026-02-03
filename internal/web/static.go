package web

// This file documents the embedded static assets structure.
// Static files are embedded from internal/web/static/ directory.
//
// Directory structure:
//   internal/web/static/
//   ├── index.html      # Dashboard entry point
//   ├── css/
//   │   └── style.css   # Minimal styles
//   └── js/
//       ├── app.js      # Main application
//       └── charts.js   # D2 diagram integration
//
// In development mode (--dev flag), files are served from web/public/
// for hot reloading support.

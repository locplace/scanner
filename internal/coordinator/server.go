// Package coordinator provides the coordination server implementation.
package coordinator

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/locplace/scanner/frontend"
	"github.com/locplace/scanner/internal/coordinator/db"
	"github.com/locplace/scanner/internal/coordinator/handlers"
	"github.com/locplace/scanner/internal/coordinator/middleware"
)

// Config holds server configuration.
type Config struct {
	AdminAPIKey      string
	HeartbeatTimeout time.Duration
}

// NewServer creates a new HTTP server with all routes configured.
func NewServer(database *db.DB, cfg Config) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)

	// Initialize handlers
	adminHandlers := &handlers.AdminHandlers{
		DB:               database,
		HeartbeatTimeout: cfg.HeartbeatTimeout,
	}
	scannerHandlers := &handlers.ScannerHandlers{
		DB: database,
	}
	publicHandlers := &handlers.PublicHandlers{
		DB:               database,
		HeartbeatTimeout: cfg.HeartbeatTimeout,
	}

	// Admin routes (authenticated with API key)
	r.Route("/api/admin", func(r chi.Router) {
		r.Use(middleware.AdminAuth(cfg.AdminAPIKey))
		r.Post("/clients", adminHandlers.RegisterClient)
		r.Get("/clients", adminHandlers.ListClients)
		r.Delete("/clients/{id}", adminHandlers.DeleteClient)
		r.Post("/discover-files", adminHandlers.DiscoverFiles)
		r.Post("/reset-scan", adminHandlers.ResetScan)
	})

	// Scanner routes (authenticated with bearer token)
	r.Route("/api/scanner", func(r chi.Router) {
		r.Use(middleware.ScannerAuth(database))
		r.Post("/jobs", scannerHandlers.GetJobs)
		r.Post("/heartbeat", scannerHandlers.Heartbeat)
		r.Post("/results", scannerHandlers.SubmitResults)
	})

	// Public routes (no authentication)
	r.Route("/api/public", func(r chi.Router) {
		r.Get("/records", publicHandlers.ListRecords)
		r.Get("/records.geojson", publicHandlers.GetRecordsGeoJSON)
		r.Get("/stats", publicHandlers.GetStats)
	})

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok")) // Error is client disconnect, can't recover
	})

	// Serve frontend (must be last to not override API routes)
	r.Handle("/*", frontend.Handler())

	return r
}

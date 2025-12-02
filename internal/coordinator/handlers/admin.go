// Package handlers provides HTTP handlers for the coordinator API.
package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/locplace/scanner/internal/coordinator/db"
	"github.com/locplace/scanner/internal/coordinator/feeder"
	"github.com/locplace/scanner/pkg/api"
)

// AdminHandlers contains handlers for admin endpoints.
type AdminHandlers struct {
	DB               *db.DB
	HeartbeatTimeout time.Duration
}

// RegisterClient handles POST /api/admin/clients.
func (h *AdminHandlers) RegisterClient(w http.ResponseWriter, r *http.Request) {
	var req api.RegisterClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		writeError(w, "name is required", http.StatusBadRequest)
		return
	}

	id, token, err := h.DB.CreateClient(r.Context(), req.Name)
	if err != nil {
		writeError(w, "failed to create client", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, api.RegisterClientResponse{
		ID:    id,
		Name:  req.Name,
		Token: token,
	})
}

// ListClients handles GET /api/admin/clients.
func (h *AdminHandlers) ListClients(w http.ResponseWriter, r *http.Request) {
	clients, err := h.DB.ListClients(r.Context())
	if err != nil {
		writeError(w, "failed to list clients", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	resp := api.ListClientsResponse{
		Clients: make([]api.ClientInfo, 0, len(clients)),
	}

	for _, c := range clients {
		isAlive := c.LastHeartbeat != nil && now.Sub(*c.LastHeartbeat) < h.HeartbeatTimeout
		resp.Clients = append(resp.Clients, api.ClientInfo{
			ID:            c.ID,
			Name:          c.Name,
			CreatedAt:     c.CreatedAt,
			LastHeartbeat: c.LastHeartbeat,
			ActiveBatches: c.ActiveBatches,
			IsAlive:       isAlive,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// DeleteClient handles DELETE /api/admin/clients/{id}.
func (h *AdminHandlers) DeleteClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "client id is required", http.StatusBadRequest)
		return
	}

	err := h.DB.DeleteClient(r.Context(), id)
	if err != nil {
		writeError(w, "client not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DiscoverFiles handles POST /api/admin/discover-files.
// Fetches the domain file list from GitHub and updates the database.
func (h *AdminHandlers) DiscoverFiles(w http.ResponseWriter, r *http.Request) {
	count, err := feeder.DiscoverAndInsertFiles(r.Context(), h.DB)
	if err != nil {
		writeError(w, "failed to discover files: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, api.DiscoverFilesResponse{
		FilesDiscovered: count,
	})
}

// ResetScan handles POST /api/admin/reset-scan.
// Resets all files to pending status for a full re-scan.
func (h *AdminHandlers) ResetScan(w http.ResponseWriter, r *http.Request) {
	// First, get the count of files
	fileStats, err := h.DB.GetDomainFileStats(r.Context())
	if err != nil {
		writeError(w, "failed to get file stats", http.StatusInternalServerError)
		return
	}

	// Reset all files
	if err := h.DB.ResetAllFiles(r.Context()); err != nil {
		writeError(w, "failed to reset files", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, api.ResetScanResponse{
		FilesReset: fileStats.Total,
	})
}

// ManualScan handles POST /api/admin/manual-scan.
// Queues a list of domains for scanning as a single batch.
func (h *AdminHandlers) ManualScan(w http.ResponseWriter, r *http.Request) {
	var req api.ManualScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Domains) == 0 {
		writeError(w, "at least one domain is required", http.StatusBadRequest)
		return
	}

	// Clean up domains: trim whitespace, skip empty lines
	var cleanDomains []string
	for _, d := range req.Domains {
		d = strings.TrimSpace(d)
		if d != "" && !strings.HasPrefix(d, "#") {
			cleanDomains = append(cleanDomains, d)
		}
	}

	if len(cleanDomains) == 0 {
		writeError(w, "no valid domains provided", http.StatusBadRequest)
		return
	}

	// Create the batch
	domainsStr := strings.Join(cleanDomains, "\n")
	if err := h.DB.CreateManualBatch(r.Context(), domainsStr); err != nil {
		writeError(w, "failed to queue domains: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, api.ManualScanResponse{
		DomainsQueued: len(cleanDomains),
	})
}

// Helper functions

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // Error is client disconnect, can't recover
}

func writeError(w http.ResponseWriter, message string, status int) {
	writeJSON(w, status, api.ErrorResponse{Error: message})
}

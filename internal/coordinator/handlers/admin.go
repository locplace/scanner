// Package handlers provides HTTP handlers for the coordinator API.
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/locplace/scanner/internal/coordinator/db"
	"github.com/locplace/scanner/pkg/api"
)

// AdminHandlers contains handlers for admin endpoints.
type AdminHandlers struct {
	DB               *db.DB
	HeartbeatTimeout time.Duration
}

// AddDomains handles POST /api/admin/domains.
func (h *AdminHandlers) AddDomains(w http.ResponseWriter, r *http.Request) {
	var req api.AddDomainsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Domains) == 0 {
		writeError(w, "domains array is required", http.StatusBadRequest)
		return
	}

	inserted, duplicates, err := h.DB.InsertDomains(r.Context(), req.Domains)
	if err != nil {
		writeError(w, "failed to insert domains", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, api.AddDomainsResponse{
		Inserted:   inserted,
		Duplicates: duplicates,
	})
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
			ActiveDomains: c.ActiveDomains,
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

// Helper functions

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // Error is client disconnect, can't recover
}

func writeError(w http.ResponseWriter, message string, status int) {
	writeJSON(w, status, api.ErrorResponse{Error: message})
}

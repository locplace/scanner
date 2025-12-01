package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"golang.org/x/net/publicsuffix"

	"github.com/locplace/scanner/internal/coordinator/db"
	"github.com/locplace/scanner/internal/coordinator/metrics"
	"github.com/locplace/scanner/internal/coordinator/middleware"
	"github.com/locplace/scanner/pkg/api"
)

// ScannerHandlers contains handlers for scanner endpoints.
type ScannerHandlers struct {
	DB *db.DB
}

// GetJobs handles POST /api/scanner/jobs.
// Claims a batch of domains for the scanner to process.
func (h *ScannerHandlers) GetJobs(w http.ResponseWriter, r *http.Request) {
	client := middleware.GetClient(r.Context())
	if client == nil {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req api.GetBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Update client's session_id and heartbeat
	if err := h.DB.UpdateSessionID(r.Context(), client.ID, req.SessionID); err != nil {
		writeError(w, "failed to update session", http.StatusInternalServerError)
		return
	}

	// Claim a batch
	batch, err := h.DB.ClaimBatch(r.Context(), client.ID)
	if err != nil {
		writeError(w, "failed to claim batch", http.StatusInternalServerError)
		return
	}

	// No batches available
	if batch == nil {
		writeJSON(w, http.StatusOK, api.GetBatchResponse{
			Domains: []string{},
		})
		return
	}

	// Parse domains from newline-separated string
	domains := strings.Split(batch.Domains, "\n")
	// Filter empty strings
	filtered := make([]string, 0, len(domains))
	for _, d := range domains {
		d = strings.TrimSpace(d)
		if d != "" {
			filtered = append(filtered, d)
		}
	}

	writeJSON(w, http.StatusOK, api.GetBatchResponse{
		BatchID: batch.ID,
		Domains: filtered,
	})
}

// Heartbeat handles POST /api/scanner/heartbeat.
func (h *ScannerHandlers) Heartbeat(w http.ResponseWriter, r *http.Request) {
	client := middleware.GetClient(r.Context())
	if client == nil {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req api.HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.DB.UpdateHeartbeat(r.Context(), client.ID, req.SessionID); err != nil {
		writeError(w, "failed to update heartbeat", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, api.HeartbeatResponse{OK: true})
}

// SubmitResults handles POST /api/scanner/results.
// Stores LOC records and marks the batch as complete.
func (h *ScannerHandlers) SubmitResults(w http.ResponseWriter, r *http.Request) {
	client := middleware.GetClient(r.Context())
	if client == nil {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req api.SubmitBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.BatchID == 0 {
		writeError(w, "batch_id is required", http.StatusBadRequest)
		return
	}

	// Store LOC records
	accepted := 0
	for _, loc := range req.LOCRecords {
		// Extract root domain from FQDN
		rootDomain, err := publicsuffix.EffectiveTLDPlusOne(loc.FQDN)
		if err != nil {
			// If we can't parse it, use the FQDN as-is
			rootDomain = loc.FQDN
		}

		if err := h.DB.UpsertLOCRecord(r.Context(), rootDomain, loc); err != nil {
			continue
		}
		accepted++
	}

	// Mark batch as complete
	fileID, err := h.DB.CompleteBatch(r.Context(), req.BatchID)
	if err != nil {
		writeError(w, "failed to complete batch", http.StatusInternalServerError)
		return
	}

	// Check if the file is now complete (all batches done)
	completed, err := h.DB.CheckAndMarkFileComplete(r.Context(), fileID)
	if err != nil {
		// Log but don't fail - the batch is already completed
		// The file will be marked complete on next check
		_ = err
	}
	_ = completed // Log this if needed

	// Update metrics
	metrics.ScanCompletionsTotal.Inc()
	metrics.DomainsCheckedTotal.Add(float64(req.DomainsChecked))
	metrics.LOCDiscoveriesTotal.Add(float64(accepted))

	writeJSON(w, http.StatusOK, api.SubmitBatchResponse{Accepted: accepted})
}

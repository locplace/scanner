package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/boet/loc-scanner/internal/coordinator/db"
	"github.com/boet/loc-scanner/internal/coordinator/middleware"
	"github.com/boet/loc-scanner/pkg/api"
)

// ScannerHandlers contains handlers for scanner endpoints.
type ScannerHandlers struct {
	DB *db.DB
}

// GetJobs handles POST /api/scanner/jobs.
func (h *ScannerHandlers) GetJobs(w http.ResponseWriter, r *http.Request) {
	client := middleware.GetClient(r.Context())
	if client == nil {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req api.GetJobsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Count <= 0 {
		req.Count = 3 // Default batch size
	}
	if req.Count > 100 {
		req.Count = 100 // Max batch size
	}

	domains, err := h.DB.GetDomainsToScan(r.Context(), client.ID, req.Count)
	if err != nil {
		writeError(w, "failed to get domains", http.StatusInternalServerError)
		return
	}

	resp := api.GetJobsResponse{
		Domains: make([]api.DomainJob, 0, len(domains)),
	}
	for _, d := range domains {
		resp.Domains = append(resp.Domains, api.DomainJob{Domain: d})
	}

	writeJSON(w, http.StatusOK, resp)
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

	if err := h.DB.UpdateHeartbeat(r.Context(), client.ID); err != nil {
		writeError(w, "failed to update heartbeat", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, api.HeartbeatResponse{OK: true})
}

// SubmitResults handles POST /api/scanner/results.
func (h *ScannerHandlers) SubmitResults(w http.ResponseWriter, r *http.Request) {
	client := middleware.GetClient(r.Context())
	if client == nil {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req api.SubmitResultsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	accepted := 0
	for _, result := range req.Results {
		// Get the root domain ID
		domain, err := h.DB.GetDomainByName(r.Context(), result.Domain)
		if err != nil {
			continue
		}
		if domain == nil {
			continue
		}

		// Store LOC records
		for _, loc := range result.LOCRecords {
			if err := h.DB.UpsertLOCRecord(r.Context(), domain.ID, loc); err != nil {
				continue
			}
		}

		// Mark domain as scanned and update subdomain count
		if err := h.DB.MarkDomainScanned(r.Context(), result.Domain, result.SubdomainsScanned); err != nil {
			continue
		}

		// Release from active scans
		if err := h.DB.ReleaseDomain(r.Context(), result.Domain); err != nil {
			continue
		}

		accepted++
	}

	writeJSON(w, http.StatusOK, api.SubmitResultsResponse{Accepted: accepted})
}

package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/locplace/scanner/internal/coordinator/db"
	"github.com/locplace/scanner/internal/coordinator/metrics"
	"github.com/locplace/scanner/internal/coordinator/middleware"
	"github.com/locplace/scanner/pkg/api"
)

// ScannerHandlers contains handlers for scanner endpoints.
type ScannerHandlers struct {
	DB             *db.DB
	RescanInterval time.Duration
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

	// Update client's session_id
	if err := h.DB.UpdateSessionID(r.Context(), client.ID, req.SessionID); err != nil {
		writeError(w, "failed to update session", http.StatusInternalServerError)
		return
	}

	domains, err := h.DB.GetDomainsToScan(r.Context(), client.ID, req.SessionID, req.Count, h.RescanInterval)
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

	if err := h.DB.UpdateHeartbeat(r.Context(), client.ID, req.SessionID); err != nil {
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

		// Deduplicate LOC records: if root domain has a LOC record,
		// skip subdomains with the same raw record value
		var rootLOCRecord string
		for _, loc := range result.LOCRecords {
			if loc.FQDN == result.Domain {
				rootLOCRecord = loc.RawRecord
				break
			}
		}

		// Store LOC records with deduplication
		locCount := 0
		for _, loc := range result.LOCRecords {
			// Skip subdomain records that match the root domain's LOC
			if rootLOCRecord != "" && loc.FQDN != result.Domain && loc.RawRecord == rootLOCRecord {
				continue
			}
			if err := h.DB.UpsertLOCRecord(r.Context(), domain.ID, loc); err != nil {
				continue
			}
			locCount++
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

		// Update metrics counters
		metrics.ScanCompletionsTotal.Inc()
		metrics.SubdomainsCheckedTotal.Add(float64(result.SubdomainsScanned))
		metrics.LOCDiscoveriesTotal.Add(float64(locCount))
	}

	writeJSON(w, http.StatusOK, api.SubmitResultsResponse{Accepted: accepted})
}

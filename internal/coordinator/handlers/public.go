package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/locplace/scanner/internal/coordinator/db"
	"github.com/locplace/scanner/pkg/api"
)

// PublicHandlers contains handlers for public endpoints.
type PublicHandlers struct {
	DB               *db.DB
	HeartbeatTimeout time.Duration
}

// ListRecords handles GET /api/public/records.
func (h *PublicHandlers) ListRecords(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 100)
	offset := parseIntParam(r, "offset", 0)
	domain := r.URL.Query().Get("domain")

	if limit > 1000 {
		limit = 1000
	}

	records, total, err := h.DB.ListLOCRecords(r.Context(), limit, offset, domain)
	if err != nil {
		writeError(w, "failed to list records", http.StatusInternalServerError)
		return
	}

	if records == nil {
		records = []api.PublicLOCRecord{}
	}

	writeJSON(w, http.StatusOK, api.ListRecordsResponse{
		Records: records,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	})
}

// GetStats handles GET /api/public/stats.
func (h *PublicHandlers) GetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	domainStats, err := h.DB.GetDomainStats(ctx)
	if err != nil {
		writeError(w, "failed to get domain stats", http.StatusInternalServerError)
		return
	}

	inProgress, err := h.DB.CountInProgressDomains(ctx)
	if err != nil {
		writeError(w, "failed to get in-progress count", http.StatusInternalServerError)
		return
	}

	activeClients, err := h.DB.CountActiveClients(ctx, h.HeartbeatTimeout)
	if err != nil {
		writeError(w, "failed to get active clients", http.StatusInternalServerError)
		return
	}

	locCount, err := h.DB.CountLOCRecords(ctx)
	if err != nil {
		writeError(w, "failed to get LOC record count", http.StatusInternalServerError)
		return
	}

	uniqueWithLOC, err := h.DB.CountUniqueRootDomainsWithLOC(ctx)
	if err != nil {
		writeError(w, "failed to get unique domains with LOC", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, api.StatsResponse{
		TotalRootDomains:         domainStats.Total,
		ScannedRootDomains:       domainStats.Scanned,
		PendingRootDomains:       domainStats.Pending,
		InProgressRootDomains:    inProgress,
		TotalSubdomainsScanned:   domainStats.TotalSubdomainsScanned,
		ActiveScanners:           activeClients,
		TotalLOCRecords:          locCount,
		UniqueRootDomainsWithLOC: uniqueWithLOC,
	})
}

func parseIntParam(r *http.Request, name string, defaultVal int) int {
	s := r.URL.Query().Get(name)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return defaultVal
	}
	return v
}

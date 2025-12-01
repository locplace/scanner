package handlers

import (
	"encoding/json"
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

// GetRecordsGeoJSON handles GET /api/public/records.geojson.
// Returns LOC records aggregated by location as a GeoJSON FeatureCollection.
// Multiple FQDNs at the same coordinates are combined into a single feature.
func (h *PublicHandlers) GetRecordsGeoJSON(w http.ResponseWriter, r *http.Request) {
	locations, err := h.DB.GetAggregatedLocationsForGeoJSON(r.Context())
	if err != nil {
		writeError(w, "failed to get records", http.StatusInternalServerError)
		return
	}

	features := make([]api.GeoJSONFeature, 0, len(locations))
	for _, loc := range locations {
		feature := api.GeoJSONFeature{
			Type: "Feature",
			Geometry: api.GeoJSONPoint{
				Type:        "Point",
				Coordinates: []float64{loc.Longitude, loc.Latitude},
			},
			Properties: map[string]any{
				"fqdns":        loc.FQDNs,
				"root_domains": loc.RootDomains,
				"raw_record":   loc.RawRecord,
				"altitude_m":   loc.AltitudeM,
				"count":        loc.Count,
				"first_seen":   loc.FirstSeenAt,
				"last_seen":    loc.LastSeenAt,
			},
		}
		features = append(features, feature)
	}

	fc := api.GeoJSONFeatureCollection{
		Type:     "FeatureCollection",
		Features: features,
	}

	data, err := json.Marshal(fc)
	if err != nil {
		writeError(w, "failed to encode geojson", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/geo+json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// GetStats handles GET /api/public/stats.
func (h *PublicHandlers) GetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// LOC record stats
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

	// Scanner stats - count active sessions (individual scanner instances)
	activeSessions, err := h.DB.CountActiveSessions(ctx, h.HeartbeatTimeout)
	if err != nil {
		// Fall back to counting active clients if sessions table doesn't exist yet
		activeSessions, err = h.DB.CountActiveClients(ctx, h.HeartbeatTimeout)
		if err != nil {
			writeError(w, "failed to get active scanners", http.StatusInternalServerError)
			return
		}
	}

	// File stats
	fileStats, err := h.DB.GetDomainFileStats(ctx)
	if err != nil {
		writeError(w, "failed to get file stats", http.StatusInternalServerError)
		return
	}

	// Batch stats
	batchStats, err := h.DB.GetBatchStats(ctx)
	if err != nil {
		writeError(w, "failed to get batch stats", http.StatusInternalServerError)
		return
	}

	// Current file progress
	var currentFile *api.CurrentFileProgress
	processingFile, err := h.DB.GetCurrentProcessingFile(ctx)
	if err != nil {
		writeError(w, "failed to get current file", http.StatusInternalServerError)
		return
	}
	if processingFile != nil {
		progressPct := 0.0
		if processingFile.BatchesCreated > 0 {
			progressPct = float64(processingFile.BatchesCompleted) / float64(processingFile.BatchesCreated) * 100
		}
		currentFile = &api.CurrentFileProgress{
			Filename:         processingFile.Filename,
			ProcessedLines:   processingFile.ProcessedLines,
			BatchesCreated:   processingFile.BatchesCreated,
			BatchesCompleted: processingFile.BatchesCompleted,
			ProgressPct:      progressPct,
		}
	}

	writeJSON(w, http.StatusOK, api.StatsResponse{
		TotalLOCRecords:          locCount,
		UniqueRootDomainsWithLOC: uniqueWithLOC,
		ActiveScanners:           activeSessions,
		DomainFiles: api.DomainFileStats{
			Total:      fileStats.Total,
			Pending:    fileStats.Pending,
			Processing: fileStats.Processing,
			Complete:   fileStats.Complete,
		},
		BatchQueue: api.BatchQueueStats{
			Pending:  batchStats.Pending,
			InFlight: batchStats.InFlight,
		},
		CurrentFile: currentFile,
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

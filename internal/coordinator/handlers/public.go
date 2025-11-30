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

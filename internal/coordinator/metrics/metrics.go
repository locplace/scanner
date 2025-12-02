// Package metrics provides Prometheus metrics for the coordinator.
//
// # Metric Types
//
// This package exposes two categories of metrics:
//
// ## Gauges (Database State)
//
// These metrics reflect the current state from the database, updated periodically
// (default: every 15 seconds). They show "how many unique X exist" at a point in time.
//
// Use these for dashboards showing current progress and state.
//
// ## Counters (Events/Work Done)
//
// These metrics increment on each event, regardless of whether it's a new or repeated
// action. They track "how much work has been done" and are useful for rate calculations.
//
// Use rate(counter[5m]) to derive throughput metrics like "batches completed per second".
package metrics

import (
	"net/url"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// Build information, set at compile time.
var (
	Version = "dev"
	Commit  = "unknown"
)

// ========================================
// GAUGES - Database State (periodic snapshot)
// ========================================

var (
	// DomainFilesTotal is the total number of domain files.
	DomainFilesTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "locplace_domain_files_total",
		Help: "Total number of domain files in the database (gauge, from DB).",
	})

	// DomainFilesPending is the number of domain files pending processing.
	DomainFilesPending = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "locplace_domain_files_pending",
		Help: "Number of domain files waiting to be processed (gauge, from DB).",
	})

	// DomainFilesProcessing is the number of domain files currently being processed.
	DomainFilesProcessing = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "locplace_domain_files_processing",
		Help: "Number of domain files currently being processed (gauge, from DB).",
	})

	// DomainFilesComplete is the number of domain files that have been fully processed.
	DomainFilesComplete = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "locplace_domain_files_complete",
		Help: "Number of domain files that have been fully processed (gauge, from DB).",
	})

	// BatchesPending is the number of batches waiting to be claimed.
	BatchesPending = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "locplace_batches_pending",
		Help: "Number of batches waiting to be claimed by scanners (gauge, from DB).",
	})

	// BatchesInFlight is the number of batches currently being processed by scanners.
	BatchesInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "locplace_batches_in_flight",
		Help: "Number of batches currently assigned to scanners (gauge, from DB).",
	})

	// LOCRecordsTotal is the number of unique LOC records in the database.
	LOCRecordsTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "locplace_loc_records_total",
		Help: "Number of unique LOC records in the database (gauge, from DB). For discovery rate, use rate(locplace_loc_discoveries_total[5m]) instead.",
	})

	// DomainsWithLOC is the number of root domains with at least one LOC record.
	DomainsWithLOC = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "locplace_domains_with_loc",
		Help: "Number of unique root domains that have at least one LOC record (gauge, from DB).",
	})

	// ScannersTotal is the total number of registered scanner clients.
	ScannersTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "locplace_scanners_total",
		Help: "Total number of registered scanner clients (gauge, from DB).",
	})

	// ScannersActive is the number of scanners with a recent heartbeat.
	ScannersActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "locplace_scanners_active",
		Help: "Number of scanner clients with a heartbeat within the timeout period (gauge, from DB).",
	})
)

// Database pool metrics.
var (
	DBPoolTotalConns = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "locplace_db_pool_total_conns",
		Help: "Total number of connections in the database pool.",
	})

	DBPoolAcquiredConns = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "locplace_db_pool_acquired_conns",
		Help: "Number of currently acquired database connections.",
	})

	DBPoolIdleConns = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "locplace_db_pool_idle_conns",
		Help: "Number of idle database connections in the pool.",
	})

	DBPoolMaxConns = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "locplace_db_pool_max_conns",
		Help: "Maximum number of connections allowed in the pool.",
	})
)

// ========================================
// COUNTERS - Events/Work Done (real-time)
// ========================================

var (
	// ScanCompletionsTotal increments each time a batch is completed.
	ScanCompletionsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "locplace_scan_completions_total",
		Help: "Total number of batch completions (counter). Use rate() for batches/second.",
	})

	// BatchProcessingDuration tracks time from batch claim to completion.
	BatchProcessingDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "locplace_batch_processing_duration_seconds",
		Help:    "Time between batch claim and result submission in seconds.",
		Buckets: []float64{1, 2, 5, 10, 15, 20, 30, 45, 60, 120},
	})

	// DomainsCheckedTotal increments by the number of domains checked per batch.
	DomainsCheckedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "locplace_domains_checked_total",
		Help: "Total number of FQDNs/domains checked (counter). Use rate() for domains/second throughput.",
	})

	// LOCDiscoveriesTotal increments each time a LOC record is found.
	LOCDiscoveriesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "locplace_loc_discoveries_total",
		Help: "Total number of LOC record discoveries (counter). Increments on every discovery including rediscoveries. Use rate() for LOC/second.",
	})

	// ReaperRunsTotal counts reaper execution cycles.
	ReaperRunsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "locplace_reaper_runs_total",
		Help: "Total number of reaper execution cycles (counter).",
	})

	// ReaperBatchesReleasedTotal counts batches released by the reaper.
	ReaperBatchesReleasedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "locplace_reaper_batches_released_total",
		Help: "Total number of batches released by the reaper due to timeout (counter).",
	})
)

// ========================================
// HTTP Metrics
// ========================================

var (
	// HTTPRequestsTotal counts HTTP requests by method, path, and status.
	HTTPRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "locplace_http_requests_total",
		Help: "Total number of HTTP requests by method, path, and status code.",
	}, []string{"method", "path", "status"})

	// HTTPRequestDuration tracks request latency by method and path.
	HTTPRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "locplace_http_request_duration_seconds",
		Help:    "HTTP request duration in seconds.",
		Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}, []string{"method", "path"})

	// HTTPRequestsInFlight tracks concurrent request count.
	HTTPRequestsInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "locplace_http_requests_in_flight",
		Help: "Number of HTTP requests currently being processed.",
	})

	// HTTPReferrerRequests counts requests by referrer domain.
	HTTPReferrerRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "locplace_http_referrer_requests_total",
		Help: "Total number of HTTP requests by referrer domain (direct if no referrer).",
	}, []string{"referrer"})
)

// ========================================
// Build Info
// ========================================

var (
	// BuildInfo exports build information as a metric.
	BuildInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "locplace_build_info",
		Help: "Build information with version and commit labels. Value is always 1.",
	}, []string{"version", "commit"})
)

// Register registers all metrics with the default Prometheus registry.
func Register() {
	// Gauges - File/Batch progress
	prometheus.MustRegister(DomainFilesTotal)
	prometheus.MustRegister(DomainFilesPending)
	prometheus.MustRegister(DomainFilesProcessing)
	prometheus.MustRegister(DomainFilesComplete)
	prometheus.MustRegister(BatchesPending)
	prometheus.MustRegister(BatchesInFlight)

	// Gauges - Results
	prometheus.MustRegister(LOCRecordsTotal)
	prometheus.MustRegister(DomainsWithLOC)
	prometheus.MustRegister(ScannersTotal)
	prometheus.MustRegister(ScannersActive)

	// DB pool
	prometheus.MustRegister(DBPoolTotalConns)
	prometheus.MustRegister(DBPoolAcquiredConns)
	prometheus.MustRegister(DBPoolIdleConns)
	prometheus.MustRegister(DBPoolMaxConns)

	// Counters
	prometheus.MustRegister(ScanCompletionsTotal)
	prometheus.MustRegister(BatchProcessingDuration)
	prometheus.MustRegister(DomainsCheckedTotal)
	prometheus.MustRegister(LOCDiscoveriesTotal)
	prometheus.MustRegister(ReaperRunsTotal)
	prometheus.MustRegister(ReaperBatchesReleasedTotal)

	// HTTP
	prometheus.MustRegister(HTTPRequestsTotal)
	prometheus.MustRegister(HTTPRequestDuration)
	prometheus.MustRegister(HTTPRequestsInFlight)
	prometheus.MustRegister(HTTPReferrerRequests)

	// Build info
	prometheus.MustRegister(BuildInfo)
	BuildInfo.WithLabelValues(Version, Commit).Set(1)
}

// NormalizePath normalizes URL paths for metric labels to avoid high cardinality.
// Replaces UUIDs and other IDs with :id placeholder.
func NormalizePath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		// Replace UUID-like strings (8-4-4-4-12 hex pattern)
		if len(part) == 36 && strings.Count(part, "-") == 4 {
			parts[i] = ":id"
			continue
		}
		// Replace any segment that looks like an ID (long hex string)
		if len(part) >= 32 && isHex(part) {
			parts[i] = ":id"
		}
	}
	return strings.Join(parts, "/")
}

func isHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// ExtractReferrerDomain extracts the domain from a Referer header value.
// Returns "direct" if the referrer is empty or invalid.
func ExtractReferrerDomain(referer string) string {
	if referer == "" {
		return "direct"
	}
	u, err := url.Parse(referer)
	if err != nil || u.Host == "" {
		return "direct"
	}
	return u.Host
}

package scanner

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds all scanner Prometheus metrics.
type Metrics struct {
	// Phase durations
	GetJobsDuration *prometheus.HistogramVec
	DNSDuration     *prometheus.HistogramVec
	SubmitDuration  *prometheus.HistogramVec
	DomainDuration  *prometheus.HistogramVec

	// Distribution metrics
	LOCRecordsFound prometheus.Histogram

	// Counters
	DomainsProcessed     prometheus.Counter
	LOCRecordsFoundTotal prometheus.Counter
	SubmitRetries        prometheus.Counter
	SubmitFailures       prometheus.Counter
}

// NewMetrics creates and registers scanner metrics.
func NewMetrics(registry prometheus.Registerer) *Metrics {
	m := &Metrics{
		GetJobsDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "scanner_getjobs_duration_seconds",
			Help:    "Time spent fetching batch from coordinator.",
			Buckets: []float64{.01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}, []string{"result"}), // result: "success", "empty", "error"

		DNSDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "scanner_dns_duration_seconds",
			Help:    "Time spent on DNS LOC record lookups for a batch.",
			Buckets: []float64{.1, .25, .5, 1, 2.5, 5, 10, 30, 60, 120},
		}, []string{"batch_size"}), // batch_size: "0", "1-10", "11-100", "101-1000", "1001-5000", "5000+"

		SubmitDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "scanner_submit_duration_seconds",
			Help:    "Time spent submitting results to coordinator.",
			Buckets: []float64{.01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}, []string{"result", "loc_found"}), // result: "success", "error"; loc_found: "yes", "no"

		DomainDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "scanner_batch_duration_seconds",
			Help:    "Total time to process a batch (DNS + submit).",
			Buckets: []float64{1, 2.5, 5, 10, 15, 30, 60, 120, 300, 600},
		}, []string{"loc_found"}), // loc_found: "yes", "no"

		LOCRecordsFound: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "scanner_loc_records_found_per_batch",
			Help:    "Distribution of LOC records found per batch.",
			Buckets: []float64{0, 1, 2, 5, 10, 25, 50, 100},
		}),

		DomainsProcessed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "scanner_fqdns_processed_total",
			Help: "Total number of FQDNs processed by this scanner.",
		}),

		LOCRecordsFoundTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "scanner_loc_records_found_total",
			Help: "Total number of LOC records discovered by this scanner.",
		}),

		SubmitRetries: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "scanner_submit_retries_total",
			Help: "Total number of submit retry attempts.",
		}),

		SubmitFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "scanner_submit_failures_total",
			Help: "Total number of failed submissions (after all retries).",
		}),
	}

	registry.MustRegister(
		m.GetJobsDuration,
		m.DNSDuration,
		m.SubmitDuration,
		m.DomainDuration,
		m.LOCRecordsFound,
		m.DomainsProcessed,
		m.LOCRecordsFoundTotal,
		m.SubmitRetries,
		m.SubmitFailures,
	)

	return m
}

// BucketCount returns a label value for count buckets.
func BucketCount(n int) string {
	switch {
	case n == 0:
		return "0"
	case n <= 10:
		return "1-10"
	case n <= 100:
		return "11-100"
	case n <= 1000:
		return "101-1000"
	case n <= 5000:
		return "1001-5000"
	default:
		return "5000+"
	}
}

// BoolLabel returns "yes" or "no" for boolean labels.
func BoolLabel(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

package scanner

import (
	"context"
	"log"
	"math"
	"math/rand/v2"
	"time"

	"github.com/locplace/scanner/pkg/api"
)

// WorkerConfig holds configuration for a scanner worker.
type WorkerConfig struct {
	DNSConfig       DNSConfig
	RetryDelay      time.Duration
	EmptyQueueDelay time.Duration
	MaxBackoff      time.Duration
}

// DefaultWorkerConfig returns the default worker configuration.
func DefaultWorkerConfig() WorkerConfig {
	return WorkerConfig{
		DNSConfig:       DefaultDNSConfig(),
		RetryDelay:      5 * time.Second,
		EmptyQueueDelay: 30 * time.Second,
		MaxBackoff:      5 * time.Minute,
	}
}

// Worker processes batches of FQDNs in a loop.
type Worker struct {
	ID          int
	Config      WorkerConfig
	Coordinator *CoordinatorClient
	DNS         *DNSScanner
	ShutdownCh  <-chan struct{}
	Metrics     *Metrics

	// Circuit breaker state
	consecutiveErrors int
}

// NewWorker creates a new worker.
func NewWorker(id int, config WorkerConfig, coordinator *CoordinatorClient, shutdownCh <-chan struct{}, metrics *Metrics) *Worker {
	return &Worker{
		ID:          id,
		Config:      config,
		Coordinator: coordinator,
		DNS:         NewDNSScanner(config.DNSConfig),
		ShutdownCh:  shutdownCh,
		Metrics:     metrics,
	}
}

// backoffDelay calculates exponential backoff delay based on consecutive errors.
func (w *Worker) backoffDelay() time.Duration {
	if w.consecutiveErrors == 0 {
		return 0
	}
	// Exponential backoff: baseDelay * 2^(errors-1), capped at maxBackoff
	delay := float64(w.Config.RetryDelay) * math.Pow(2, float64(w.consecutiveErrors-1))
	if delay > float64(w.Config.MaxBackoff) {
		delay = float64(w.Config.MaxBackoff)
	}
	return time.Duration(delay)
}

// recordError increments the consecutive error count.
// Returns true if this is the first error (entering error state).
func (w *Worker) recordError() bool {
	w.consecutiveErrors++
	return w.consecutiveErrors == 1
}

// resetErrors resets the consecutive error count.
// Returns the previous error count (0 if we weren't in error state).
func (w *Worker) resetErrors() int {
	prev := w.consecutiveErrors
	w.consecutiveErrors = 0
	return prev
}

// Run starts the worker loop. It blocks until the context is canceled.
func (w *Worker) Run(ctx context.Context) {
	log.Printf("[Worker %d] Started", w.ID)

	for {
		// Check if we should stop getting new jobs (graceful shutdown or context canceled)
		select {
		case <-w.ShutdownCh:
			log.Printf("[Worker %d] Shutdown signal received, exiting", w.ID)
			return
		case <-ctx.Done():
			log.Printf("[Worker %d] Stopped", w.ID)
			return
		default:
		}

		// Apply backoff if we have consecutive errors
		if backoff := w.backoffDelay(); backoff > 0 {
			log.Printf("[Worker %d] Backing off for %v after %d consecutive errors",
				w.ID, backoff, w.consecutiveErrors)
			select {
			case <-w.ShutdownCh:
				log.Printf("[Worker %d] Shutdown signal received during backoff, exiting", w.ID)
				return
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		}

		// Get a batch of FQDNs to scan
		getBatchStart := time.Now()
		batch, err := w.Coordinator.GetBatch(ctx)
		getBatchDuration := time.Since(getBatchStart).Seconds()

		if err != nil {
			if w.Metrics != nil {
				w.Metrics.GetJobsDuration.WithLabelValues("error").Observe(getBatchDuration)
			}
			if w.recordError() {
				log.Printf("[Worker %d] Connection error: %v (entering backoff)", w.ID, err)
			}
			continue
		}

		if batch == nil || len(batch.Domains) == 0 {
			if w.Metrics != nil {
				w.Metrics.GetJobsDuration.WithLabelValues("empty").Observe(getBatchDuration)
			}
			// Empty queue is not an error, reset backoff
			if prev := w.resetErrors(); prev > 0 {
				log.Printf("[Worker %d] Connection recovered after %d errors", w.ID, prev)
			}
			// Add jitter (0.5x to 1.5x) to avoid thundering herd
			jitter := 0.5 + rand.Float64()
			delay := time.Duration(float64(w.Config.EmptyQueueDelay) * jitter)
			log.Printf("[Worker %d] No batches available, waiting %s...", w.ID, delay.Round(time.Second))
			select {
			case <-w.ShutdownCh:
				log.Printf("[Worker %d] Shutdown signal received, exiting", w.ID)
				return
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			continue
		}

		// Got a batch successfully
		if w.Metrics != nil {
			w.Metrics.GetJobsDuration.WithLabelValues("success").Observe(getBatchDuration)
		}

		// Process the batch
		batchStart := time.Now()
		locRecords := w.processBatch(ctx, batch.Domains)
		batchDuration := time.Since(batchStart).Seconds()

		hasLOC := len(locRecords) > 0

		// Submit results with retries
		submitted := false
		var submitDuration float64
		for attempt := 1; attempt <= 3; attempt++ {
			submitStart := time.Now()
			err := w.Coordinator.SubmitBatch(ctx, batch.ID, len(batch.Domains), locRecords)
			submitDuration = time.Since(submitStart).Seconds()

			if err == nil {
				if prev := w.resetErrors(); prev > 0 {
					log.Printf("[Worker %d] Connection recovered after %d errors", w.ID, prev)
				}
				log.Printf("[Worker %d] Submitted batch %d: %d FQDNs checked, %d LOC records found",
					w.ID, batch.ID, len(batch.Domains), len(locRecords))
				submitted = true
				if w.Metrics != nil {
					w.Metrics.SubmitDuration.WithLabelValues("success", BoolLabel(hasLOC)).Observe(submitDuration)
				}
				break
			}

			if attempt < 3 {
				if w.Metrics != nil {
					w.Metrics.SubmitRetries.Inc()
				}
				retryDelay := time.Duration(attempt) * 5 * time.Second
				log.Printf("[Worker %d] Submit failed for batch %d (attempt %d/3): %v, retrying in %s",
					w.ID, batch.ID, attempt, err, retryDelay)
				select {
				case <-ctx.Done():
					return
				case <-time.After(retryDelay):
				}
			} else {
				if w.Metrics != nil {
					w.Metrics.SubmitDuration.WithLabelValues("error", BoolLabel(hasLOC)).Observe(submitDuration)
					w.Metrics.SubmitFailures.Inc()
				}
				if w.recordError() {
					log.Printf("[Worker %d] Submit failed for batch %d after 3 attempts: %v (entering backoff)",
						w.ID, batch.ID, err)
				}
			}
		}

		if !submitted {
			log.Printf("[Worker %d] WARNING: Lost results for batch %d (%d LOC records)",
				w.ID, batch.ID, len(locRecords))
		}

		// Record batch-level metrics
		if w.Metrics != nil {
			w.Metrics.DomainDuration.WithLabelValues(BoolLabel(hasLOC)).Observe(batchDuration)
			w.Metrics.DomainsProcessed.Add(float64(len(batch.Domains)))
			w.Metrics.LOCRecordsFoundTotal.Add(float64(len(locRecords)))
		}
	}
}

// processBatch scans all FQDNs in the batch for LOC records.
func (w *Worker) processBatch(ctx context.Context, fqdns []string) []api.LOCRecord {
	log.Printf("[Worker %d] Processing batch of %d FQDNs", w.ID, len(fqdns))

	// Scan all FQDNs for LOC records
	dnsStart := time.Now()
	locResults := w.DNS.LookupLOCBatch(ctx, fqdns)
	dnsDuration := time.Since(dnsStart).Seconds()

	// Record DNS metrics
	if w.Metrics != nil {
		w.Metrics.DNSDuration.WithLabelValues(BucketCount(len(fqdns))).Observe(dnsDuration)
	}

	// Collect LOC records
	var locRecords []api.LOCRecord
	for _, locResult := range locResults {
		if locResult.Error != nil {
			continue
		}
		if !locResult.HasLOC {
			continue
		}

		// Parse the LOC record
		locRecord, err := ParseLOCRecordLenient(locResult.FQDN, locResult.RawRecord)
		if err != nil {
			log.Printf("[Worker %d] Failed to parse LOC for %s: %v", w.ID, locResult.FQDN, err)
			continue
		}

		locRecords = append(locRecords, *locRecord)
		log.Printf("[Worker %d] Found LOC record: %s -> %s", w.ID, locResult.FQDN, locResult.RawRecord)
	}

	// Record LOC records found distribution
	if w.Metrics != nil {
		w.Metrics.LOCRecordsFound.Observe(float64(len(locRecords)))
	}

	return locRecords
}

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
	BatchSize       int
	SubfinderConfig SubfinderConfig
	DNSConfig       DNSConfig
	RetryDelay      time.Duration
	EmptyQueueDelay time.Duration
	MaxBackoff      time.Duration
}

// DefaultWorkerConfig returns the default worker configuration.
func DefaultWorkerConfig() WorkerConfig {
	return WorkerConfig{
		BatchSize:       3,
		SubfinderConfig: DefaultSubfinderConfig(),
		DNSConfig:       DefaultDNSConfig(),
		RetryDelay:      5 * time.Second,
		EmptyQueueDelay: 30 * time.Second,
		MaxBackoff:      5 * time.Minute,
	}
}

// Worker processes domains in a loop.
type Worker struct {
	ID          int
	Config      WorkerConfig
	Coordinator *CoordinatorClient
	Tracker     *DomainTracker
	Subfinder   *Subfinder
	DNS         *DNSScanner
	ShutdownCh  <-chan struct{}

	// Circuit breaker state
	consecutiveErrors int
}

// NewWorker creates a new worker.
func NewWorker(id int, config WorkerConfig, coordinator *CoordinatorClient, tracker *DomainTracker, shutdownCh <-chan struct{}) *Worker {
	return &Worker{
		ID:          id,
		Config:      config,
		Coordinator: coordinator,
		Tracker:     tracker,
		Subfinder:   NewSubfinder(config.SubfinderConfig),
		DNS:         NewDNSScanner(config.DNSConfig),
		ShutdownCh:  shutdownCh,
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

		// Get domains to scan
		domains, err := w.Coordinator.GetJobs(ctx, w.Config.BatchSize)
		if err != nil {
			if w.recordError() {
				// First error - log it, subsequent errors will just backoff silently
				log.Printf("[Worker %d] Connection error: %v (entering backoff)", w.ID, err)
			}
			continue
		}

		if len(domains) == 0 {
			// Empty queue is not an error, reset backoff
			if prev := w.resetErrors(); prev > 0 {
				log.Printf("[Worker %d] Connection recovered after %d errors", w.ID, prev)
			}
			// Add jitter (0.5x to 1.5x) to avoid thundering herd
			jitter := 0.5 + rand.Float64()
			delay := time.Duration(float64(w.Config.EmptyQueueDelay) * jitter)
			log.Printf("[Worker %d] No domains available, waiting %s...", w.ID, delay.Round(time.Second))
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

		// Register domains in tracker
		w.Tracker.Add(domains...)

		// Process each domain
		for _, domain := range domains {
			select {
			case <-ctx.Done():
				w.Tracker.Remove(domains...)
				return
			default:
			}

			result := w.processDomain(ctx, domain)

			// Submit result with retries to avoid losing data
			submitted := false
			for attempt := 1; attempt <= 3; attempt++ {
				err := w.Coordinator.SubmitResults(ctx, []api.DomainResult{result})
				if err == nil {
					if prev := w.resetErrors(); prev > 0 {
						log.Printf("[Worker %d] Connection recovered after %d errors", w.ID, prev)
					}
					log.Printf("[Worker %d] Submitted results for %s: %d subdomains, %d LOC records",
						w.ID, domain, result.SubdomainsScanned, len(result.LOCRecords))
					submitted = true
					break
				}

				if attempt < 3 {
					// Wait before retry with exponential backoff
					retryDelay := time.Duration(attempt) * 5 * time.Second
					log.Printf("[Worker %d] Submit failed for %s (attempt %d/3): %v, retrying in %s",
						w.ID, domain, attempt, err, retryDelay)
					select {
					case <-ctx.Done():
						return
					case <-time.After(retryDelay):
					}
				} else {
					// Final attempt failed
					if w.recordError() {
						log.Printf("[Worker %d] Submit failed for %s after 3 attempts: %v (entering backoff)",
							w.ID, domain, err)
					}
				}
			}

			if !submitted {
				log.Printf("[Worker %d] WARNING: Lost results for %s (%d LOC records)",
					w.ID, domain, len(result.LOCRecords))
			}

			// Remove from tracker
			w.Tracker.Remove(domain)
		}
	}
}

// processDomain scans a single domain for LOC records.
func (w *Worker) processDomain(ctx context.Context, domain string) api.DomainResult {
	result := api.DomainResult{
		Domain:     domain,
		LOCRecords: []api.LOCRecord{},
	}

	log.Printf("[Worker %d] Processing %s", w.ID, domain)

	// Discover subdomains
	subdomains, err := w.Subfinder.EnumerateSubdomains(ctx, domain)
	if err != nil {
		log.Printf("[Worker %d] Subfinder error for %s: %v", w.ID, domain, err)
		// Continue with just the root domain
		subdomains = []string{}
	}

	// Build list of FQDNs to scan (root domain + subdomains)
	fqdns := make([]string, 0, len(subdomains)+1)
	fqdns = append(fqdns, domain) // Always include root domain
	fqdns = append(fqdns, subdomains...)

	result.SubdomainsScanned = len(fqdns)
	log.Printf("[Worker %d] Scanning %d FQDNs for %s", w.ID, len(fqdns), domain)

	// Scan all FQDNs for LOC records
	locResults := w.DNS.LookupLOCBatch(ctx, fqdns)

	// Collect LOC records
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

		result.LOCRecords = append(result.LOCRecords, *locRecord)
		log.Printf("[Worker %d] Found LOC record: %s -> %s", w.ID, locResult.FQDN, locResult.RawRecord)
	}

	return result
}

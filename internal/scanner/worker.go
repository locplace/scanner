package scanner

import (
	"context"
	"log"
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
}

// DefaultWorkerConfig returns the default worker configuration.
func DefaultWorkerConfig() WorkerConfig {
	return WorkerConfig{
		BatchSize:       3,
		SubfinderConfig: DefaultSubfinderConfig(),
		DNSConfig:       DefaultDNSConfig(),
		RetryDelay:      5 * time.Second,
		EmptyQueueDelay: 30 * time.Second,
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
}

// NewWorker creates a new worker.
func NewWorker(id int, config WorkerConfig, coordinator *CoordinatorClient, tracker *DomainTracker) *Worker {
	return &Worker{
		ID:          id,
		Config:      config,
		Coordinator: coordinator,
		Tracker:     tracker,
		Subfinder:   NewSubfinder(config.SubfinderConfig),
		DNS:         NewDNSScanner(config.DNSConfig),
	}
}

// Run starts the worker loop. It blocks until the context is canceled.
func (w *Worker) Run(ctx context.Context) {
	log.Printf("[Worker %d] Started", w.ID)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Worker %d] Stopped", w.ID)
			return
		default:
		}

		// Get domains to scan
		domains, err := w.Coordinator.GetJobs(ctx, w.Config.BatchSize)
		if err != nil {
			log.Printf("[Worker %d] Failed to get jobs: %v", w.ID, err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(w.Config.RetryDelay):
			}
			continue
		}

		if len(domains) == 0 {
			log.Printf("[Worker %d] No domains available, waiting...", w.ID)
			select {
			case <-ctx.Done():
				return
			case <-time.After(w.Config.EmptyQueueDelay):
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

			// Submit result immediately
			if err := w.Coordinator.SubmitResults(ctx, []api.DomainResult{result}); err != nil {
				log.Printf("[Worker %d] Failed to submit results for %s: %v", w.ID, domain, err)
			} else {
				log.Printf("[Worker %d] Submitted results for %s: %d subdomains, %d LOC records",
					w.ID, domain, result.SubdomainsScanned, len(result.LOCRecords))
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

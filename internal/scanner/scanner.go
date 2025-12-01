package scanner

import (
	"context"
	"log"
	"sync"
	"time"
)

// Config holds the scanner configuration.
type Config struct {
	CoordinatorURL    string
	Token             string
	WorkerCount       int
	BatchSize         int
	HeartbeatInterval time.Duration
	SubfinderConfig   SubfinderConfig
	DNSConfig         DNSConfig
}

// DefaultConfig returns the default scanner configuration.
func DefaultConfig() Config {
	return Config{
		CoordinatorURL:    "http://localhost:8080",
		Token:             "",
		WorkerCount:       4,
		BatchSize:         1,
		HeartbeatInterval: 30 * time.Second,
		SubfinderConfig:   DefaultSubfinderConfig(),
		DNSConfig:         DefaultDNSConfig(),
	}
}

// Scanner orchestrates multiple workers and heartbeat.
type Scanner struct {
	config      Config
	coordinator *CoordinatorClient
	tracker     *DomainTracker
}

// New creates a new scanner.
func New(config Config) *Scanner {
	return &Scanner{
		config:      config,
		coordinator: NewCoordinatorClient(config.CoordinatorURL, config.Token),
		tracker:     NewDomainTracker(),
	}
}

// Run starts the scanner. It blocks until the context is canceled.
func (s *Scanner) Run(ctx context.Context) error {
	log.Printf("Starting scanner with %d workers", s.config.WorkerCount)
	log.Printf("Session ID: %s", s.coordinator.SessionID)
	log.Printf("Coordinator: %s", s.config.CoordinatorURL)
	log.Printf("Batch size: %d, Heartbeat interval: %s", s.config.BatchSize, s.config.HeartbeatInterval)

	// Start heartbeat goroutine
	heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
	defer cancelHeartbeat()
	go s.runHeartbeat(heartbeatCtx)

	// Start workers
	var wg sync.WaitGroup
	workerConfig := WorkerConfig{
		BatchSize:       s.config.BatchSize,
		SubfinderConfig: s.config.SubfinderConfig,
		DNSConfig:       s.config.DNSConfig,
		RetryDelay:      5 * time.Second,
		EmptyQueueDelay: 30 * time.Second,
	}

	for i := 0; i < s.config.WorkerCount; i++ {
		wg.Add(1)
		worker := NewWorker(i+1, workerConfig, s.coordinator, s.tracker)
		go func() {
			defer wg.Done()
			worker.Run(ctx)
		}()
	}

	// Wait for all workers to finish
	wg.Wait()
	log.Println("Scanner stopped")
	return nil
}

// runHeartbeat sends periodic heartbeats to the coordinator.
func (s *Scanner) runHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(s.config.HeartbeatInterval)
	defer ticker.Stop()

	log.Printf("Heartbeat started: interval=%s", s.config.HeartbeatInterval)

	var consecutiveErrors int
	var lastErrorLogged time.Time

	for {
		select {
		case <-ctx.Done():
			log.Println("Heartbeat stopped")
			return
		case <-ticker.C:
			activeDomains := s.tracker.List()
			if err := s.coordinator.Heartbeat(ctx, activeDomains); err != nil {
				consecutiveErrors++
				// Log on first error, then rate limit to every 30 seconds
				if consecutiveErrors == 1 || time.Since(lastErrorLogged) > 30*time.Second {
					log.Printf("Heartbeat error: %v (consecutive errors: %d)", err, consecutiveErrors)
					lastErrorLogged = time.Now()
				}
			} else {
				if consecutiveErrors > 0 {
					log.Printf("Heartbeat recovered after %d consecutive errors", consecutiveErrors)
				}
				consecutiveErrors = 0
				log.Printf("Heartbeat sent: %d active domains", len(activeDomains))
			}
		}
	}
}

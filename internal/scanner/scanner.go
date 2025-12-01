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
	HeartbeatInterval time.Duration
	DNSConfig         DNSConfig
}

// DefaultConfig returns the default scanner configuration.
func DefaultConfig() Config {
	return Config{
		CoordinatorURL:    "http://localhost:8080",
		Token:             "",
		WorkerCount:       4,
		HeartbeatInterval: 30 * time.Second,
		DNSConfig:         DefaultDNSConfig(),
	}
}

// Scanner orchestrates multiple workers and heartbeat.
type Scanner struct {
	config      Config
	coordinator *CoordinatorClient
	metrics     *Metrics

	// Graceful shutdown
	shutdownCh   chan struct{}
	shutdownOnce sync.Once
}

// New creates a new scanner.
func New(config Config) *Scanner {
	return &Scanner{
		config:      config,
		coordinator: NewCoordinatorClient(config.CoordinatorURL, config.Token),
		shutdownCh:  make(chan struct{}),
	}
}

// InitiateShutdown signals workers to stop fetching new jobs.
// Workers will finish their current batch before exiting.
func (s *Scanner) InitiateShutdown() {
	s.shutdownOnce.Do(func() {
		close(s.shutdownCh)
	})
}

// SetMetrics sets the metrics instance for the scanner.
func (s *Scanner) SetMetrics(m *Metrics) {
	s.metrics = m
}

// Run starts the scanner. It blocks until the context is canceled.
func (s *Scanner) Run(ctx context.Context) error {
	log.Printf("Starting scanner with %d workers", s.config.WorkerCount)
	log.Printf("Session ID: %s", s.coordinator.SessionID)
	log.Printf("Coordinator: %s", s.config.CoordinatorURL)
	log.Printf("Heartbeat interval: %s", s.config.HeartbeatInterval)

	// Start heartbeat goroutine
	heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
	defer cancelHeartbeat()
	go s.runHeartbeat(heartbeatCtx)

	// Start workers
	var wg sync.WaitGroup
	workerConfig := WorkerConfig{
		DNSConfig:       s.config.DNSConfig,
		RetryDelay:      5 * time.Second,
		EmptyQueueDelay: 30 * time.Second,
	}

	for i := 0; i < s.config.WorkerCount; i++ {
		wg.Add(1)
		worker := NewWorker(i+1, workerConfig, s.coordinator, s.shutdownCh, s.metrics)
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

	for {
		select {
		case <-ctx.Done():
			log.Println("Heartbeat stopped")
			return
		case <-ticker.C:
			if err := s.coordinator.Heartbeat(ctx); err != nil {
				consecutiveErrors++
				if consecutiveErrors == 1 {
					log.Printf("Heartbeat error: %v (entering backoff)", err)
				}
			} else {
				if consecutiveErrors > 0 {
					log.Printf("Heartbeat recovered after %d errors", consecutiveErrors)
				}
				consecutiveErrors = 0
				log.Println("Heartbeat sent")
			}
		}
	}
}

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/locplace/scanner/internal/scanner"
)

func main() {
	// Configuration from environment
	config := scanner.DefaultConfig()

	if url := os.Getenv("COORDINATOR_URL"); url != "" {
		config.CoordinatorURL = url
	}

	config.Token = os.Getenv("SCANNER_TOKEN")
	if config.Token == "" {
		log.Fatal("SCANNER_TOKEN environment variable is required")
	}

	if v := os.Getenv("WORKER_COUNT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			config.WorkerCount = n
		}
	}

	if v := os.Getenv("BATCH_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			config.BatchSize = n
		}
	}

	if v := os.Getenv("HEARTBEAT_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			config.HeartbeatInterval = d
		}
	}

	// DNS configuration
	if v := os.Getenv("DNS_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			config.DNSConfig.Workers = n
		}
	}

	if v := os.Getenv("DNS_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			config.DNSConfig.Timeout = d
		}
	}

	// Subfinder configuration
	if v := os.Getenv("SUBFINDER_THREADS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			config.SubfinderConfig.Threads = n
		}
	}

	if v := os.Getenv("SUBFINDER_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			config.SubfinderConfig.Timeout = n
		}
	}

	if v := os.Getenv("SUBFINDER_MAX_TIME"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			config.SubfinderConfig.MaxEnumerationTime = n
		}
	}

	// Create scanner
	s := scanner.New(config)

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-stop
		log.Println("Shutting down...")
		cancel()
	}()

	// Run scanner
	if err := s.Run(ctx); err != nil {
		log.Fatalf("Scanner error: %v", err)
	}
}

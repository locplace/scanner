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

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Run scanner in background
	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	// Wait for signal or scanner completion
	select {
	case sig := <-sigChan:
		log.Printf("Received %v signal, initiating graceful shutdown...", sig)
		s.InitiateShutdown() // Signal workers to stop fetching new jobs

		// Wait for scanner to finish with timeout
		select {
		case <-done:
			log.Println("Scanner stopped gracefully")
		case <-time.After(30 * time.Second):
			log.Println("Shutdown timeout exceeded, forcing exit")
			cancel() // Force cancel context
		case sig := <-sigChan:
			log.Printf("Received second %v signal, forcing exit", sig)
			cancel() // Force cancel context
		}

	case err := <-done:
		if err != nil {
			log.Fatalf("Scanner error: %v", err)
		}
	}
}

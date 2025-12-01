package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

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

	// Create scanner
	s := scanner.New(config)

	// Set up Prometheus metrics
	registry := prometheus.NewRegistry()
	metrics := scanner.NewMetrics(registry)
	s.SetMetrics(metrics)

	// Start metrics HTTP server
	metricsAddr := os.Getenv("METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = ":9090"
	}
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
		log.Printf("Metrics server listening on %s", metricsAddr)
		if err := http.ListenAndServe(metricsAddr, mux); err != nil && err != http.ErrServerClosed {
			log.Printf("Metrics server error: %v", err)
		}
	}()

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

package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/locplace/scanner/internal/coordinator"
	"github.com/locplace/scanner/internal/coordinator/db"
	"github.com/locplace/scanner/internal/coordinator/metrics"
	"github.com/locplace/scanner/internal/coordinator/reaper"
	"github.com/locplace/scanner/migrations"
)

func main() {
	// Configuration from environment
	databaseURL := getEnv("DATABASE_URL", "postgres://localhost:5432/locscanner?sslmode=disable")
	adminAPIKey := os.Getenv("ADMIN_API_KEY")
	listenAddr := getEnv("LISTEN_ADDR", ":8080")
	metricsAddr := getEnv("METRICS_ADDR", ":9090")
	metricsInterval := parseDuration("METRICS_INTERVAL", 15*time.Second)
	jobTimeout := parseDuration("JOB_TIMEOUT", 10*time.Minute)
	heartbeatTimeout := parseDuration("HEARTBEAT_TIMEOUT", 2*time.Minute)
	reaperInterval := parseDuration("REAPER_INTERVAL", 60*time.Second)
	rescanInterval := parseDuration("RESCAN_INTERVAL", 0) // 0 = scan each domain only once

	if adminAPIKey == "" {
		log.Fatal("ADMIN_API_KEY environment variable is required")
	}

	// Register Prometheus metrics
	metrics.Register()

	if rescanInterval > 0 {
		log.Printf("Rescan interval: %s (domains will be re-scanned after this time)", rescanInterval)
	} else {
		log.Println("Rescan interval: disabled (domains will only be scanned once)")
	}

	// Connect to database
	ctx := context.Background()
	database, err := db.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()
	log.Println("Connected to database")

	// Run migrations
	if err := runMigrations(databaseURL); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Create server
	cfg := coordinator.Config{
		AdminAPIKey:      adminAPIKey,
		HeartbeatTimeout: heartbeatTimeout,
		RescanInterval:   rescanInterval,
	}
	handler := coordinator.NewServer(database, cfg)

	// Wrap with metrics middleware
	server := &http.Server{
		Addr:         listenAddr,
		Handler:      metrics.Middleware(handler),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Create background context for all goroutines
	bgCtx, cancelBg := context.WithCancel(context.Background())
	defer cancelBg()

	// Start metrics updater
	metricsUpdater := metrics.NewUpdater(database, metrics.UpdaterConfig{
		Interval:         metricsInterval,
		HeartbeatTimeout: heartbeatTimeout,
	})
	go metricsUpdater.Run(bgCtx)

	// Start metrics HTTP server
	metricsServer := &http.Server{
		Addr:    metricsAddr,
		Handler: promhttp.Handler(),
	}
	go func() {
		log.Printf("Metrics server listening on %s", metricsAddr)
		if err := metricsServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("Metrics server error: %v", err)
		}
	}()

	// Start reaper
	r := &reaper.Reaper{
		DB:               database,
		Interval:         reaperInterval,
		JobTimeout:       jobTimeout,
		HeartbeatTimeout: heartbeatTimeout,
	}
	go r.Run(bgCtx)

	// Start main server
	go func() {
		log.Printf("Coordinator listening on %s", listenAddr)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down...")
	cancelBg() // Stop all background goroutines

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Shutdown both servers
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Metrics server shutdown error: %v", err)
	}
	log.Println("Goodbye")
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func parseDuration(key string, defaultVal time.Duration) time.Duration {
	s := os.Getenv(key)
	if s == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		log.Printf("Invalid duration for %s: %v, using default", key, err)
		return defaultVal
	}
	return d
}

func runMigrations(databaseURL string) error {
	// Create migration source from embedded files
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return err
	}

	// Create migrator using database URL
	m, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	if err != nil {
		return err
	}
	defer m.Close() //nolint:errcheck // Close error not actionable

	// Run migrations
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	log.Println("Migrations completed")
	return nil
}

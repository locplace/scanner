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

	"github.com/locplace/scanner/internal/coordinator"
	"github.com/locplace/scanner/internal/coordinator/db"
	"github.com/locplace/scanner/internal/coordinator/reaper"
	"github.com/locplace/scanner/migrations"
)

func main() {
	// Configuration from environment
	databaseURL := getEnv("DATABASE_URL", "postgres://localhost:5432/locscanner?sslmode=disable")
	adminAPIKey := getEnv("ADMIN_API_KEY", "changeme")
	listenAddr := getEnv("LISTEN_ADDR", ":8080")
	jobTimeout := parseDuration("JOB_TIMEOUT", 10*time.Minute)
	heartbeatTimeout := parseDuration("HEARTBEAT_TIMEOUT", 2*time.Minute)
	reaperInterval := parseDuration("REAPER_INTERVAL", 60*time.Second)
	rescanInterval := parseDuration("RESCAN_INTERVAL", 0) // 0 = no minimum, rescan immediately

	if adminAPIKey == "changeme" {
		log.Println("WARNING: Using default admin API key. Set ADMIN_API_KEY in production!")
	}

	if rescanInterval > 0 {
		log.Printf("Rescan interval: %s (domains won't be re-scanned until this time passes)", rescanInterval)
	} else {
		log.Println("Rescan interval: disabled (domains can be re-scanned immediately)")
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

	server := &http.Server{
		Addr:         listenAddr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Start reaper in background
	reaperCtx, cancelReaper := context.WithCancel(context.Background())
	defer cancelReaper()

	r := &reaper.Reaper{
		DB:               database,
		Interval:         reaperInterval,
		JobTimeout:       jobTimeout,
		HeartbeatTimeout: heartbeatTimeout,
	}
	go r.Run(reaperCtx)

	// Start server
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
	cancelReaper()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Shutdown error: %v", err)
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
	defer m.Close()

	// Run migrations
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	log.Println("Migrations completed")
	return nil
}

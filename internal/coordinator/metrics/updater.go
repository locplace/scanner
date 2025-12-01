package metrics

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/locplace/scanner/internal/coordinator/db"
)

// UpdaterConfig holds configuration for the metrics updater.
type UpdaterConfig struct {
	Interval         time.Duration
	HeartbeatTimeout time.Duration
}

// Updater periodically updates gauge metrics from the database.
type Updater struct {
	db     *db.DB
	pool   *pgxpool.Pool
	config UpdaterConfig
}

// NewUpdater creates a new metrics updater.
func NewUpdater(database *db.DB, config UpdaterConfig) *Updater {
	return &Updater{
		db:     database,
		pool:   database.Pool,
		config: config,
	}
}

// Run starts the updater loop. It blocks until the context is canceled.
func (u *Updater) Run(ctx context.Context) {
	log.Printf("Metrics updater started: interval=%s", u.config.Interval)

	// Update immediately on start
	u.update(ctx)

	ticker := time.NewTicker(u.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Metrics updater stopped")
			return
		case <-ticker.C:
			u.update(ctx)
		}
	}
}

func (u *Updater) update(ctx context.Context) {
	// Get metrics snapshot from database
	snapshot, err := u.db.GetMetricsSnapshot(ctx, u.config.HeartbeatTimeout)
	if err != nil {
		log.Printf("Metrics updater: failed to get snapshot: %v", err)
		return
	}

	// Update file/batch gauges
	DomainFilesTotal.Set(float64(snapshot.FilesTotal))
	DomainFilesPending.Set(float64(snapshot.FilesPending))
	DomainFilesProcessing.Set(float64(snapshot.FilesProcessing))
	DomainFilesComplete.Set(float64(snapshot.FilesComplete))
	BatchesPending.Set(float64(snapshot.BatchesPending))
	BatchesInFlight.Set(float64(snapshot.BatchesInFlight))

	// Update LOC/scanner gauges
	LOCRecordsTotal.Set(float64(snapshot.LOCRecordsTotal))
	DomainsWithLOC.Set(float64(snapshot.DomainsWithLOC))
	ScannersTotal.Set(float64(snapshot.ScannersTotal))
	ScannersActive.Set(float64(snapshot.ScannersActive))

	// Update pool stats
	poolStats := u.pool.Stat()
	DBPoolTotalConns.Set(float64(poolStats.TotalConns()))
	DBPoolAcquiredConns.Set(float64(poolStats.AcquiredConns()))
	DBPoolIdleConns.Set(float64(poolStats.IdleConns()))
	DBPoolMaxConns.Set(float64(poolStats.MaxConns()))
}

// Package reaper provides background job cleanup for stale scans.
package reaper

import (
	"context"
	"log"
	"time"

	"github.com/locplace/scanner/internal/coordinator/db"
	"github.com/locplace/scanner/internal/coordinator/metrics"
)

// Reaper periodically releases stale scan assignments.
type Reaper struct {
	DB               *db.DB
	Interval         time.Duration
	JobTimeout       time.Duration
	HeartbeatTimeout time.Duration
}

// Run starts the reaper loop. It blocks until the context is canceled.
func (r *Reaper) Run(ctx context.Context) {
	ticker := time.NewTicker(r.Interval)
	defer ticker.Stop()

	log.Printf("Reaper started: interval=%s, job_timeout=%s, heartbeat_timeout=%s",
		r.Interval, r.JobTimeout, r.HeartbeatTimeout)

	// Run immediately on startup, then on each tick
	for {
		r.runOnce(ctx)

		select {
		case <-ctx.Done():
			log.Println("Reaper stopped")
			return
		case <-ticker.C:
		}
	}
}

func (r *Reaper) runOnce(ctx context.Context) {
	metrics.ReaperRunsTotal.Inc()
	released, err := r.DB.ReleaseStaleScans(ctx, r.JobTimeout, r.HeartbeatTimeout)
	if err != nil {
		log.Printf("Reaper error: %v", err)
		return
	}
	if released > 0 {
		metrics.ReaperDomainsReleasedTotal.Add(float64(released))
		log.Printf("Reaper released %d stale scans", released)
	}
}

// Package reaper provides background job cleanup for stale batches.
package reaper

import (
	"context"
	"log"
	"time"

	"github.com/locplace/scanner/internal/coordinator/db"
	"github.com/locplace/scanner/internal/coordinator/metrics"
)

// Reaper periodically releases stale batch assignments.
type Reaper struct {
	DB               *db.DB
	Interval         time.Duration
	BatchTimeout     time.Duration
	HeartbeatTimeout time.Duration
}

// Run starts the reaper loop. It blocks until the context is canceled.
func (r *Reaper) Run(ctx context.Context) {
	ticker := time.NewTicker(r.Interval)
	defer ticker.Stop()

	log.Printf("Reaper started: interval=%s, batch_timeout=%s, heartbeat_timeout=%s",
		r.Interval, r.BatchTimeout, r.HeartbeatTimeout)

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

	// Reset batches from dead sessions (sessions that haven't heartbeated)
	// This is the primary mechanism for reclaiming batches from crashed scanners
	releasedFromDeadSessions, err := r.DB.ResetBatchesFromDeadSessions(ctx, r.HeartbeatTimeout)
	if err != nil {
		log.Printf("Reaper error resetting batches from dead sessions: %v", err)
	} else if releasedFromDeadSessions > 0 {
		metrics.ReaperBatchesReleasedTotal.Add(float64(releasedFromDeadSessions))
		log.Printf("Reaper reset %d batches from dead sessions", releasedFromDeadSessions)
	}

	// Reset stale batches without session_id (backwards compat for old batches)
	released, err := r.DB.ResetStaleBatches(ctx, r.BatchTimeout)
	if err != nil {
		log.Printf("Reaper error resetting stale batches: %v", err)
	} else if released > 0 {
		metrics.ReaperBatchesReleasedTotal.Add(float64(released))
		log.Printf("Reaper reset %d stale batches (no session)", released)
	}
}

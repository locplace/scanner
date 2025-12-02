package db

import (
	"context"
	"time"
)

// ScanBatch represents a batch of domains to scan.
type ScanBatch struct {
	ID         int64
	FileID     int
	LineStart  int64
	LineEnd    int64
	Domains    string // Newline-separated FQDNs
	Status     string
	AssignedAt *time.Time
	ScannerID  *string // Client ID (for backwards compat)
	SessionID  *string // Session ID (for multi-scanner support)
}

// BatchStats holds aggregate statistics for batches.
type BatchStats struct {
	Pending  int
	InFlight int
}

// GetPendingBatchCount returns the number of pending batches.
func (db *DB) GetPendingBatchCount(ctx context.Context) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM scan_batches WHERE status = 'pending'`).Scan(&count)
	return count, err
}

// GetBatchStats returns aggregate statistics for batches.
func (db *DB) GetBatchStats(ctx context.Context) (*BatchStats, error) {
	var stats BatchStats
	err := db.Pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'pending') as pending,
			COUNT(*) FILTER (WHERE status = 'in_flight') as in_flight
		FROM scan_batches
	`).Scan(&stats.Pending, &stats.InFlight)
	return &stats, err
}

// CreateBatch creates a new batch of domains to scan.
func (db *DB) CreateBatch(ctx context.Context, fileID int, lineStart, lineEnd int64, domains string) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO scan_batches (file_id, line_start, line_end, domains)
		VALUES ($1, $2, $3, $4)
	`, fileID, lineStart, lineEnd, domains)
	return err
}

// CreateBatchAndUpdateProgress creates a batch and updates file progress atomically.
func (db *DB) CreateBatchAndUpdateProgress(ctx context.Context, fileID int, lineStart, lineEnd int64, domains string) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Create batch
	_, err = tx.Exec(ctx, `
		INSERT INTO scan_batches (file_id, line_start, line_end, domains)
		VALUES ($1, $2, $3, $4)
	`, fileID, lineStart, lineEnd, domains)
	if err != nil {
		return err
	}

	// Update file progress
	_, err = tx.Exec(ctx, `
		UPDATE domain_files
		SET processed_lines = $2, batches_created = batches_created + 1
		WHERE id = $1
	`, fileID, lineEnd)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ClaimBatch claims a pending batch for a scanner session.
// scannerID is the client ID (for backwards compat), sessionID is the unique session.
// Returns nil if no batches are available.
func (db *DB) ClaimBatch(ctx context.Context, scannerID, sessionID string) (*ScanBatch, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var b ScanBatch
	err = tx.QueryRow(ctx, `
		SELECT id, file_id, line_start, line_end, domains
		FROM scan_batches
		WHERE status = 'pending'
		ORDER BY id
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`).Scan(&b.ID, &b.FileID, &b.LineStart, &b.LineEnd, &b.Domains)

	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, err
	}

	// Update to in_flight with both scanner_id (backwards compat) and session_id
	_, err = tx.Exec(ctx, `
		UPDATE scan_batches
		SET status = 'in_flight', assigned_at = NOW(), scanner_id = $2, session_id = $3
		WHERE id = $1
	`, b.ID, scannerID, sessionID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	b.Status = "in_flight"
	return &b, nil
}

// CompleteBatch marks a batch as complete (deletes it) and increments file counter.
// Returns the file ID and the time the batch was assigned (for duration tracking).
func (db *DB) CompleteBatch(ctx context.Context, batchID int64) (int, *time.Time, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Get file_id and assigned_at before deleting
	var fileID int
	var assignedAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT file_id, assigned_at FROM scan_batches WHERE id = $1
	`, batchID).Scan(&fileID, &assignedAt)
	if err != nil {
		return 0, nil, err
	}

	// Delete batch
	_, err = tx.Exec(ctx, `DELETE FROM scan_batches WHERE id = $1`, batchID)
	if err != nil {
		return 0, nil, err
	}

	// Increment file counter
	_, err = tx.Exec(ctx, `
		UPDATE domain_files
		SET batches_completed = batches_completed + 1
		WHERE id = $1
	`, fileID)
	if err != nil {
		return 0, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, nil, err
	}

	return fileID, assignedAt, nil
}

// ResetStaleBatches resets batches that have been in_flight too long.
// This is for backwards compatibility with batches that don't have session_id.
func (db *DB) ResetStaleBatches(ctx context.Context, timeout time.Duration) (int, error) {
	result, err := db.Pool.Exec(ctx, `
		UPDATE scan_batches
		SET status = 'pending', assigned_at = NULL, scanner_id = NULL, session_id = NULL
		WHERE status = 'in_flight'
		AND session_id IS NULL
		AND assigned_at < NOW() - $1::interval
	`, timeout.String())
	if err != nil {
		return 0, err
	}
	return int(result.RowsAffected()), nil
}

// ResetBatchesFromDeadSessions resets batches from sessions that haven't heartbeated.
// This is more accurate than time-based reset because it only releases batches
// from scanners that are actually dead (not heartbeating), not just slow.
func (db *DB) ResetBatchesFromDeadSessions(ctx context.Context, heartbeatTimeout time.Duration) (int, error) {
	result, err := db.Pool.Exec(ctx, `
		UPDATE scan_batches b
		SET status = 'pending', assigned_at = NULL, scanner_id = NULL, session_id = NULL
		FROM scanner_sessions s
		WHERE b.session_id = s.id
		AND b.status = 'in_flight'
		AND s.last_heartbeat < NOW() - $1::interval
	`, heartbeatTimeout.String())
	if err != nil {
		return 0, err
	}
	return int(result.RowsAffected()), nil
}

// DeleteBatchesForFile deletes all batches for a file.
func (db *DB) DeleteBatchesForFile(ctx context.Context, fileID int) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM scan_batches WHERE file_id = $1`, fileID)
	return err
}

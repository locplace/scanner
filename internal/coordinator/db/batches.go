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
	ScannerID  *string
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

// ClaimBatch claims a pending batch for a scanner.
// Returns nil if no batches are available.
func (db *DB) ClaimBatch(ctx context.Context, scannerID string) (*ScanBatch, error) {
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

	// Update to in_flight
	_, err = tx.Exec(ctx, `
		UPDATE scan_batches
		SET status = 'in_flight', assigned_at = NOW(), scanner_id = $2
		WHERE id = $1
	`, b.ID, scannerID)
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
func (db *DB) CompleteBatch(ctx context.Context, batchID int64) (int, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Get file_id before deleting
	var fileID int
	err = tx.QueryRow(ctx, `
		SELECT file_id FROM scan_batches WHERE id = $1
	`, batchID).Scan(&fileID)
	if err != nil {
		return 0, err
	}

	// Delete batch
	_, err = tx.Exec(ctx, `DELETE FROM scan_batches WHERE id = $1`, batchID)
	if err != nil {
		return 0, err
	}

	// Increment file counter
	_, err = tx.Exec(ctx, `
		UPDATE domain_files
		SET batches_completed = batches_completed + 1
		WHERE id = $1
	`, fileID)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}

	return fileID, nil
}

// ResetStaleBatches resets batches that have been in_flight too long.
func (db *DB) ResetStaleBatches(ctx context.Context, timeout time.Duration) (int, error) {
	result, err := db.Pool.Exec(ctx, `
		UPDATE scan_batches
		SET status = 'pending', assigned_at = NULL, scanner_id = NULL
		WHERE status = 'in_flight'
		AND assigned_at < NOW() - $1::interval
	`, timeout.String())
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

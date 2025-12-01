package db

import (
	"context"
	"time"
)

// DomainFile represents a .xz file from the domains project.
type DomainFile struct {
	ID               int
	Filename         string
	URL              string
	SizeBytes        *int64
	ProcessedLines   int64
	BatchesCreated   int
	BatchesCompleted int
	FeedingComplete  bool
	Status           string
	StartedAt        *time.Time
	CompletedAt      *time.Time
}

// DomainFileStats holds aggregate statistics for domain files.
type DomainFileStats struct {
	Total      int
	Pending    int
	Processing int
	Complete   int
}

// GetDomainFileCount returns the total number of domain files.
func (db *DB) GetDomainFileCount(ctx context.Context) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM domain_files`).Scan(&count)
	return count, err
}

// GetDomainFileStats returns aggregate statistics for domain files.
func (db *DB) GetDomainFileStats(ctx context.Context) (*DomainFileStats, error) {
	var stats DomainFileStats
	err := db.Pool.QueryRow(ctx, `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE status = 'pending') as pending,
			COUNT(*) FILTER (WHERE status = 'processing') as processing,
			COUNT(*) FILTER (WHERE status = 'complete') as complete
		FROM domain_files
	`).Scan(&stats.Total, &stats.Pending, &stats.Processing, &stats.Complete)
	return &stats, err
}

// GetNextFileToProcess returns the next file to process.
// Prefers files already in 'processing' status (resume), then 'pending'.
// Excludes files that are fully fed but waiting for batches to complete.
func (db *DB) GetNextFileToProcess(ctx context.Context) (*DomainFile, error) {
	var f DomainFile
	err := db.Pool.QueryRow(ctx, `
		SELECT id, filename, url, size_bytes, processed_lines, batches_created, batches_completed, feeding_complete, status, started_at, completed_at
		FROM domain_files
		WHERE status IN ('processing', 'pending')
		-- Exclude files that are done feeding but still have pending batches
		AND NOT (feeding_complete = true AND batches_completed < batches_created)
		ORDER BY
			CASE status WHEN 'processing' THEN 0 ELSE 1 END,
			filename
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`).Scan(&f.ID, &f.Filename, &f.URL, &f.SizeBytes, &f.ProcessedLines, &f.BatchesCreated, &f.BatchesCompleted, &f.FeedingComplete, &f.Status, &f.StartedAt, &f.CompletedAt)

	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, err
	}

	// Mark as processing if pending
	if f.Status == "pending" {
		_, err = db.Pool.Exec(ctx, `
			UPDATE domain_files SET status = 'processing', started_at = NOW()
			WHERE id = $1
		`, f.ID)
		if err != nil {
			return nil, err
		}
		f.Status = "processing"
	}

	return &f, nil
}

// GetCurrentProcessingFile returns the file currently being processed, if any.
func (db *DB) GetCurrentProcessingFile(ctx context.Context) (*DomainFile, error) {
	var f DomainFile
	err := db.Pool.QueryRow(ctx, `
		SELECT id, filename, url, size_bytes, processed_lines, batches_created, batches_completed, feeding_complete, status, started_at, completed_at
		FROM domain_files
		WHERE status = 'processing'
		ORDER BY started_at
		LIMIT 1
	`).Scan(&f.ID, &f.Filename, &f.URL, &f.SizeBytes, &f.ProcessedLines, &f.BatchesCreated, &f.BatchesCompleted, &f.FeedingComplete, &f.Status, &f.StartedAt, &f.CompletedAt)

	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return &f, nil
}

// UpdateFileProgress updates the progress tracking for a file.
func (db *DB) UpdateFileProgress(ctx context.Context, fileID int, processedLines int64, batchesCreated int) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE domain_files
		SET processed_lines = $2, batches_created = $3
		WHERE id = $1
	`, fileID, processedLines, batchesCreated)
	return err
}

// IncrementBatchesCompleted increments the batches_completed counter for a file.
func (db *DB) IncrementBatchesCompleted(ctx context.Context, fileID int) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE domain_files
		SET batches_completed = batches_completed + 1
		WHERE id = $1
	`, fileID)
	return err
}

// MarkFeedingComplete marks a file as done reading all lines.
// The file stays in 'processing' status until all batches complete.
func (db *DB) MarkFeedingComplete(ctx context.Context, fileID int) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE domain_files
		SET feeding_complete = true
		WHERE id = $1
	`, fileID)
	return err
}

// MarkFileComplete marks a file as complete.
func (db *DB) MarkFileComplete(ctx context.Context, fileID int) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE domain_files
		SET status = 'complete', completed_at = NOW()
		WHERE id = $1
	`, fileID)
	return err
}

// CheckAndMarkFileComplete checks if all batches are completed and marks the file complete.
// Returns true if the file was marked complete.
// Note: batches_created = 0 is valid for empty files (all comments/blank lines).
func (db *DB) CheckAndMarkFileComplete(ctx context.Context, fileID int) (bool, error) {
	result, err := db.Pool.Exec(ctx, `
		UPDATE domain_files
		SET status = 'complete', completed_at = NOW()
		WHERE id = $1
		AND feeding_complete = true
		AND batches_created = batches_completed
		AND status = 'processing'
	`, fileID)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() > 0, nil
}

// UpsertDomainFile inserts or updates a domain file record.
func (db *DB) UpsertDomainFile(ctx context.Context, filename, url string, sizeBytes int64) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO domain_files (filename, url, size_bytes)
		VALUES ($1, $2, $3)
		ON CONFLICT (filename) DO UPDATE SET
			url = EXCLUDED.url,
			size_bytes = EXCLUDED.size_bytes
	`, filename, url, sizeBytes)
	return err
}

// ResetAllFiles resets all files to pending status (for re-scanning).
func (db *DB) ResetAllFiles(ctx context.Context) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE domain_files
		SET status = 'pending',
		    processed_lines = 0,
		    batches_created = 0,
		    batches_completed = 0,
		    feeding_complete = false,
		    started_at = NULL,
		    completed_at = NULL
	`)
	return err
}

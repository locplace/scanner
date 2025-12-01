package db

import (
	"context"
	"time"
)

// MetricsSnapshot holds all metrics data from the database.
type MetricsSnapshot struct {
	// File stats
	FilesTotal      int
	FilesPending    int
	FilesProcessing int
	FilesComplete   int

	// Batch stats
	BatchesPending  int
	BatchesInFlight int

	// LOC stats
	LOCRecordsTotal int
	DomainsWithLOC  int

	// Scanner stats
	ScannersTotal  int
	ScannersActive int
}

// GetMetricsSnapshot returns all metrics data in a single efficient query.
func (db *DB) GetMetricsSnapshot(ctx context.Context, heartbeatTimeout time.Duration) (*MetricsSnapshot, error) {
	var m MetricsSnapshot

	// Use a single query with subqueries for efficiency
	err := db.Pool.QueryRow(ctx, `
		SELECT
			-- File stats
			(SELECT COUNT(*) FROM domain_files) as files_total,
			(SELECT COUNT(*) FROM domain_files WHERE status = 'pending') as files_pending,
			(SELECT COUNT(*) FROM domain_files WHERE status = 'processing') as files_processing,
			(SELECT COUNT(*) FROM domain_files WHERE status = 'complete') as files_complete,
			-- Batch stats
			(SELECT COUNT(*) FROM scan_batches WHERE status = 'pending') as batches_pending,
			(SELECT COUNT(*) FROM scan_batches WHERE status = 'in_flight') as batches_in_flight,
			-- LOC stats
			(SELECT COUNT(*) FROM loc_records) as loc_records_total,
			(SELECT COUNT(DISTINCT root_domain) FROM loc_records) as domains_with_loc,
			-- Scanner stats
			(SELECT COUNT(*) FROM scanner_clients) as scanners_total,
			(SELECT COUNT(*) FROM scanner_clients WHERE last_heartbeat > NOW() - $1::interval) as scanners_active
	`, heartbeatTimeout.String()).Scan(
		&m.FilesTotal,
		&m.FilesPending,
		&m.FilesProcessing,
		&m.FilesComplete,
		&m.BatchesPending,
		&m.BatchesInFlight,
		&m.LOCRecordsTotal,
		&m.DomainsWithLOC,
		&m.ScannersTotal,
		&m.ScannersActive,
	)

	return &m, err
}

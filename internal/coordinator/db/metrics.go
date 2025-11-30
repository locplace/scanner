package db

import (
	"context"
	"time"
)

// MetricsSnapshot holds all metrics data from the database.
type MetricsSnapshot struct {
	// Domain stats
	DomainsTotal      int
	DomainsScanned    int
	DomainsPending    int
	DomainsInProgress int
	SubdomainsTotal   int64

	// LOC stats
	LOCRecordsTotal int
	DomainsWithLOC  int

	// Scanner stats
	ScannersTotal  int
	ScannersActive int

	// Domain sets
	DomainSetsTotal int
}

// GetMetricsSnapshot returns all metrics data in a single efficient query.
func (db *DB) GetMetricsSnapshot(ctx context.Context, heartbeatTimeout time.Duration) (*MetricsSnapshot, error) {
	var m MetricsSnapshot

	// Use a single query with subqueries for efficiency
	err := db.Pool.QueryRow(ctx, `
		SELECT
			-- Domain stats
			(SELECT COUNT(*) FROM root_domains) as domains_total,
			(SELECT COUNT(*) FROM root_domains WHERE last_scanned_at IS NOT NULL) as domains_scanned,
			(SELECT COUNT(*) FROM root_domains WHERE last_scanned_at IS NULL) as domains_pending,
			(SELECT COUNT(*) FROM active_scans) as domains_in_progress,
			(SELECT COALESCE(SUM(subdomains_scanned), 0) FROM root_domains) as subdomains_total,
			-- LOC stats
			(SELECT COUNT(*) FROM loc_records) as loc_records_total,
			(SELECT COUNT(DISTINCT root_domain_id) FROM loc_records) as domains_with_loc,
			-- Scanner stats
			(SELECT COUNT(*) FROM scanner_clients) as scanners_total,
			(SELECT COUNT(*) FROM scanner_clients WHERE last_heartbeat > NOW() - $1::interval) as scanners_active,
			-- Domain sets
			(SELECT COUNT(*) FROM domain_sets) as domain_sets_total
	`, heartbeatTimeout.String()).Scan(
		&m.DomainsTotal,
		&m.DomainsScanned,
		&m.DomainsPending,
		&m.DomainsInProgress,
		&m.SubdomainsTotal,
		&m.LOCRecordsTotal,
		&m.DomainsWithLOC,
		&m.ScannersTotal,
		&m.ScannersActive,
		&m.DomainSetsTotal,
	)

	return &m, err
}

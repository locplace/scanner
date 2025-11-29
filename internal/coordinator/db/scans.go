package db

import (
	"context"
	"time"
)

// ActiveScan represents an in-progress scan assignment.
type ActiveScan struct {
	RootDomainID string
	ClientID     string
	AssignedAt   time.Time
}

// GetActiveScansForClient returns all active scans for a client.
func (db *DB) GetActiveScansForClient(ctx context.Context, clientID string) ([]string, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT rd.domain
		FROM active_scans s
		JOIN root_domains rd ON rd.id = s.root_domain_id
		WHERE s.client_id = $1
	`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var domains []string
	for rows.Next() {
		var domain string
		if err := rows.Scan(&domain); err != nil {
			return nil, err
		}
		domains = append(domains, domain)
	}
	return domains, rows.Err()
}

// ReleaseStaleScans releases scans that have been assigned for too long
// and whose clients haven't sent a heartbeat recently.
func (db *DB) ReleaseStaleScans(ctx context.Context, jobTimeout, heartbeatTimeout time.Duration) (int, error) {
	tag, err := db.Pool.Exec(ctx, `
		DELETE FROM active_scans s
		WHERE s.assigned_at < NOW() - $1::interval
		AND EXISTS (
			SELECT 1 FROM scanner_clients c
			WHERE c.id = s.client_id
			AND (c.last_heartbeat IS NULL OR c.last_heartbeat < NOW() - $2::interval)
		)
	`, jobTimeout.String(), heartbeatTimeout.String())
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// CountInProgressDomains returns the number of domains currently being scanned.
func (db *DB) CountInProgressDomains(ctx context.Context) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM active_scans`).Scan(&count)
	return count, err
}

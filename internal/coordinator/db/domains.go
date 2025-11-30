package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// RootDomain represents a root domain in the database.
type RootDomain struct {
	ID                string
	Domain            string
	CreatedAt         time.Time
	LastScannedAt     *time.Time
	SubdomainsScanned int64
}

// GetDomainsToScan returns domains that are not currently being scanned,
// ordered by last_scanned_at (NULL first, then oldest).
// If rescanInterval == 0, only returns never-scanned domains.
// If rescanInterval > 0, also returns domains not scanned within that duration.
// The sessionID is stored with the active scan assignment to detect orphaned jobs.
func (db *DB) GetDomainsToScan(ctx context.Context, clientID, sessionID string, count int, rescanInterval time.Duration) ([]string, error) {
	// Use a transaction to atomically select and assign domains
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // Rollback after commit returns error, which is expected

	// Build query based on rescan interval
	var rows pgx.Rows
	if rescanInterval > 0 {
		// Include domains not scanned within the rescan interval
		rows, err = tx.Query(ctx, `
			SELECT rd.id, rd.domain
			FROM root_domains rd
			WHERE NOT EXISTS (
				SELECT 1 FROM active_scans s WHERE s.root_domain_id = rd.id
			)
			AND (rd.last_scanned_at IS NULL OR rd.last_scanned_at < NOW() - $2::interval)
			ORDER BY rd.last_scanned_at NULLS FIRST, rd.created_at
			LIMIT $1
			FOR UPDATE OF rd SKIP LOCKED
		`, count, rescanInterval.String())
	} else {
		// rescanInterval == 0: only return never-scanned domains
		rows, err = tx.Query(ctx, `
			SELECT rd.id, rd.domain
			FROM root_domains rd
			WHERE NOT EXISTS (
				SELECT 1 FROM active_scans s WHERE s.root_domain_id = rd.id
			)
			AND rd.last_scanned_at IS NULL
			ORDER BY rd.created_at
			LIMIT $1
			FOR UPDATE OF rd SKIP LOCKED
		`, count)
	}
	if err != nil {
		return nil, err
	}

	var domains []string
	var domainIDs []string
	for rows.Next() {
		var id, domain string
		if err := rows.Scan(&id, &domain); err != nil {
			rows.Close()
			return nil, err
		}
		domainIDs = append(domainIDs, id)
		domains = append(domains, domain)
	}
	rows.Close()

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Insert into active_scans with session_id
	for _, domainID := range domainIDs {
		_, err := tx.Exec(ctx, `
			INSERT INTO active_scans (root_domain_id, client_id, session_id)
			VALUES ($1, $2, $3)
		`, domainID, clientID, sessionID)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return domains, nil
}

// GetDomainByName returns a domain by its name.
func (db *DB) GetDomainByName(ctx context.Context, domain string) (*RootDomain, error) {
	var rd RootDomain
	err := db.Pool.QueryRow(ctx, `
		SELECT id, domain, created_at, last_scanned_at, subdomains_scanned
		FROM root_domains WHERE domain = $1
	`, domain).Scan(&rd.ID, &rd.Domain, &rd.CreatedAt, &rd.LastScannedAt, &rd.SubdomainsScanned)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rd, nil
}

// MarkDomainScanned updates the domain's last_scanned_at and subdomains_scanned count.
func (db *DB) MarkDomainScanned(ctx context.Context, domain string, subdomainsScanned int) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE root_domains
		SET last_scanned_at = NOW(),
		    subdomains_scanned = subdomains_scanned + $2
		WHERE domain = $1
	`, domain, subdomainsScanned)
	return err
}

// ReleaseDomain removes a domain from active_scans.
func (db *DB) ReleaseDomain(ctx context.Context, domain string) error {
	_, err := db.Pool.Exec(ctx, `
		DELETE FROM active_scans
		WHERE root_domain_id = (SELECT id FROM root_domains WHERE domain = $1)
	`, domain)
	return err
}

// DomainStats holds domain count statistics.
type DomainStats struct {
	Total                  int
	Scanned                int
	Pending                int
	TotalSubdomainsScanned int64
}

// GetDomainStats returns domain statistics.
func (db *DB) GetDomainStats(ctx context.Context) (*DomainStats, error) {
	var stats DomainStats

	err := db.Pool.QueryRow(ctx, `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE last_scanned_at IS NOT NULL) as scanned,
			COUNT(*) FILTER (WHERE last_scanned_at IS NULL) as pending,
			COALESCE(SUM(subdomains_scanned), 0) as total_subdomains
		FROM root_domains
	`).Scan(&stats.Total, &stats.Scanned, &stats.Pending, &stats.TotalSubdomainsScanned)

	return &stats, err
}

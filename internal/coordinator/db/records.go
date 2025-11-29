package db

import (
	"context"
	"time"

	"github.com/boet/loc-scanner/pkg/api"
	"github.com/jackc/pgx/v5"
)

// StoredLOCRecord represents a LOC record in the database.
type StoredLOCRecord struct {
	ID           string
	RootDomainID string
	FQDN         string
	RawRecord    string
	Latitude     float64
	Longitude    float64
	AltitudeM    float64
	SizeM        float64
	HorizPrecM   float64
	VertPrecM    float64
	FirstSeenAt  time.Time
	LastSeenAt   time.Time
}

// UpsertLOCRecord inserts or updates a LOC record.
// If the FQDN already exists, updates last_seen_at.
func (db *DB) UpsertLOCRecord(ctx context.Context, rootDomainID string, rec api.LOCRecord) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO loc_records (root_domain_id, fqdn, raw_record, latitude, longitude, altitude_m, size_m, horiz_prec_m, vert_prec_m)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (fqdn) DO UPDATE SET
			raw_record = EXCLUDED.raw_record,
			latitude = EXCLUDED.latitude,
			longitude = EXCLUDED.longitude,
			altitude_m = EXCLUDED.altitude_m,
			size_m = EXCLUDED.size_m,
			horiz_prec_m = EXCLUDED.horiz_prec_m,
			vert_prec_m = EXCLUDED.vert_prec_m,
			last_seen_at = NOW()
	`, rootDomainID, rec.FQDN, rec.RawRecord, rec.Latitude, rec.Longitude, rec.AltitudeM, rec.SizeM, rec.HorizPrecM, rec.VertPrecM)
	return err
}

// ListLOCRecords returns paginated LOC records with optional domain filter.
func (db *DB) ListLOCRecords(ctx context.Context, limit, offset int, domainFilter string) ([]api.PublicLOCRecord, int, error) {
	// Count total
	var total int
	countQuery := `SELECT COUNT(*) FROM loc_records l JOIN root_domains rd ON rd.id = l.root_domain_id`
	countArgs := []any{}

	if domainFilter != "" {
		countQuery += ` WHERE rd.domain = $1`
		countArgs = append(countArgs, domainFilter)
	}

	if err := db.Pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get records
	var rows pgx.Rows
	if domainFilter != "" {
		rows, err = db.Pool.Query(ctx, `
			SELECT l.fqdn, rd.domain, l.raw_record, l.latitude, l.longitude,
			       l.altitude_m, l.size_m, l.horiz_prec_m, l.vert_prec_m,
			       l.first_seen_at, l.last_seen_at
			FROM loc_records l
			JOIN root_domains rd ON rd.id = l.root_domain_id
			WHERE rd.domain = $1
			ORDER BY l.last_seen_at DESC
			LIMIT $2 OFFSET $3
		`, domainFilter, limit, offset)
	} else {
		rows, err = db.Pool.Query(ctx, `
			SELECT l.fqdn, rd.domain, l.raw_record, l.latitude, l.longitude,
			       l.altitude_m, l.size_m, l.horiz_prec_m, l.vert_prec_m,
			       l.first_seen_at, l.last_seen_at
			FROM loc_records l
			JOIN root_domains rd ON rd.id = l.root_domain_id
			ORDER BY l.last_seen_at DESC
			LIMIT $1 OFFSET $2
		`, limit, offset)
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var records []api.PublicLOCRecord
	for rows.Next() {
		var r api.PublicLOCRecord
		if err := rows.Scan(&r.FQDN, &r.RootDomain, &r.RawRecord, &r.Latitude, &r.Longitude,
			&r.AltitudeM, &r.SizeM, &r.HorizPrecM, &r.VertPrecM, &r.FirstSeenAt, &r.LastSeenAt); err != nil {
			return nil, 0, err
		}
		records = append(records, r)
	}

	return records, total, rows.Err()
}

// CountLOCRecords returns total LOC record count.
func (db *DB) CountLOCRecords(ctx context.Context) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM loc_records`).Scan(&count)
	return count, err
}

// CountUniqueRootDomainsWithLOC returns count of root domains that have at least one LOC record.
func (db *DB) CountUniqueRootDomainsWithLOC(ctx context.Context) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `SELECT COUNT(DISTINCT root_domain_id) FROM loc_records`).Scan(&count)
	return count, err
}

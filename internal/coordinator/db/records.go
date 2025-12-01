package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/locplace/scanner/pkg/api"
)

// StoredLOCRecord represents a LOC record in the database.
type StoredLOCRecord struct {
	ID          string
	RootDomain  string
	FQDN        string
	RawRecord   string
	Latitude    float64
	Longitude   float64
	AltitudeM   float64
	SizeM       float64
	HorizPrecM  float64
	VertPrecM   float64
	FirstSeenAt time.Time
	LastSeenAt  time.Time
}

// UpsertLOCRecord inserts or updates a LOC record.
// If the FQDN already exists, updates last_seen_at.
func (db *DB) UpsertLOCRecord(ctx context.Context, rootDomain string, rec api.LOCRecord) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO loc_records (root_domain, fqdn, raw_record, latitude, longitude, altitude_m, size_m, horiz_prec_m, vert_prec_m)
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
	`, rootDomain, rec.FQDN, rec.RawRecord, rec.Latitude, rec.Longitude, rec.AltitudeM, rec.SizeM, rec.HorizPrecM, rec.VertPrecM)
	return err
}

// ListLOCRecords returns paginated LOC records with optional domain filter.
func (db *DB) ListLOCRecords(ctx context.Context, limit, offset int, domainFilter string) ([]api.PublicLOCRecord, int, error) {
	// Count total
	var total int
	countQuery := `SELECT COUNT(*) FROM loc_records`
	countArgs := []any{}

	if domainFilter != "" {
		countQuery += ` WHERE root_domain = $1`
		countArgs = append(countArgs, domainFilter)
	}

	if err := db.Pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get records
	var rows pgx.Rows
	var err error
	if domainFilter != "" {
		rows, err = db.Pool.Query(ctx, `
			SELECT fqdn, root_domain, raw_record, latitude, longitude,
			       altitude_m, size_m, horiz_prec_m, vert_prec_m,
			       first_seen_at, last_seen_at
			FROM loc_records
			WHERE root_domain = $1
			ORDER BY last_seen_at DESC
			LIMIT $2 OFFSET $3
		`, domainFilter, limit, offset)
	} else {
		rows, err = db.Pool.Query(ctx, `
			SELECT fqdn, root_domain, raw_record, latitude, longitude,
			       altitude_m, size_m, horiz_prec_m, vert_prec_m,
			       first_seen_at, last_seen_at
			FROM loc_records
			ORDER BY last_seen_at DESC
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
	err := db.Pool.QueryRow(ctx, `SELECT COUNT(DISTINCT root_domain) FROM loc_records`).Scan(&count)
	return count, err
}

// GetAllLOCRecordsForGeoJSON returns all LOC records for GeoJSON export.
// Returns records without pagination for map rendering.
func (db *DB) GetAllLOCRecordsForGeoJSON(ctx context.Context) ([]api.PublicLOCRecord, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT fqdn, root_domain, raw_record, latitude, longitude,
		       altitude_m, size_m, horiz_prec_m, vert_prec_m,
		       first_seen_at, last_seen_at
		FROM loc_records
		ORDER BY last_seen_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []api.PublicLOCRecord
	for rows.Next() {
		var r api.PublicLOCRecord
		if err := rows.Scan(&r.FQDN, &r.RootDomain, &r.RawRecord, &r.Latitude, &r.Longitude,
			&r.AltitudeM, &r.SizeM, &r.HorizPrecM, &r.VertPrecM, &r.FirstSeenAt, &r.LastSeenAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}

	return records, rows.Err()
}

// GetAggregatedLocationsForGeoJSON returns LOC records aggregated by coordinates.
// Multiple FQDNs at the same location are combined into a single feature.
func (db *DB) GetAggregatedLocationsForGeoJSON(ctx context.Context) ([]api.AggregatedLocation, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT
			array_agg(fqdn ORDER BY fqdn) as fqdns,
			array_agg(DISTINCT root_domain ORDER BY root_domain) as root_domains,
			raw_record,
			latitude,
			longitude,
			altitude_m,
			COUNT(*) as count,
			MIN(first_seen_at) as first_seen_at,
			MAX(last_seen_at) as last_seen_at
		FROM loc_records
		GROUP BY latitude, longitude, altitude_m, raw_record
		ORDER BY MAX(last_seen_at) DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var locations []api.AggregatedLocation
	for rows.Next() {
		var loc api.AggregatedLocation
		if err := rows.Scan(&loc.FQDNs, &loc.RootDomains, &loc.RawRecord, &loc.Latitude, &loc.Longitude,
			&loc.AltitudeM, &loc.Count, &loc.FirstSeenAt, &loc.LastSeenAt); err != nil {
			return nil, err
		}
		locations = append(locations, loc)
	}

	return locations, rows.Err()
}

// Package api contains shared types for the coordinator API.
package api

import "time"

// --- Admin API Types ---

// DomainSetInfo represents a domain set in API responses.
type DomainSetInfo struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Source         string    `json:"source"`
	CreatedAt      time.Time `json:"created_at"`
	TotalDomains   int       `json:"total_domains"`
	ScannedDomains int       `json:"scanned_domains"`
}

// CreateDomainSetRequest is the request body for POST /api/admin/domain-sets.
type CreateDomainSetRequest struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

// CreateDomainSetResponse is the response for POST /api/admin/domain-sets.
type CreateDomainSetResponse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Source string `json:"source"`
}

// ListDomainSetsResponse is the response for GET /api/admin/domain-sets.
type ListDomainSetsResponse struct {
	Sets []DomainSetInfo `json:"sets"`
}

// AddDomainsToSetRequest is the request body for POST /api/admin/domain-sets/{id}/domains.
type AddDomainsToSetRequest struct {
	Domains []string `json:"domains"`
}

// AddDomainsToSetResponse is the response for POST /api/admin/domain-sets/{id}/domains.
type AddDomainsToSetResponse struct {
	Inserted   int `json:"inserted"`
	Duplicates int `json:"duplicates"`
}

// RegisterClientRequest is the request body for POST /api/admin/clients.
type RegisterClientRequest struct {
	Name string `json:"name"`
}

// RegisterClientResponse is the response for POST /api/admin/clients.
type RegisterClientResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Token string `json:"token"`
}

// ClientInfo represents a scanner client in the list response.
type ClientInfo struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	CreatedAt     time.Time  `json:"created_at"`
	LastHeartbeat *time.Time `json:"last_heartbeat,omitempty"`
	ActiveDomains int        `json:"active_domains"`
	IsAlive       bool       `json:"is_alive"`
}

// ListClientsResponse is the response for GET /api/admin/clients.
type ListClientsResponse struct {
	Clients []ClientInfo `json:"clients"`
}

// --- Scanner API Types ---

// GetJobsRequest is the request body for POST /api/scanner/jobs.
type GetJobsRequest struct {
	Count     int    `json:"count"`
	SessionID string `json:"session_id"`
}

// DomainJob represents a domain assignment.
type DomainJob struct {
	Domain string `json:"domain"`
}

// GetJobsResponse is the response for POST /api/scanner/jobs.
type GetJobsResponse struct {
	Domains []DomainJob `json:"domains"`
}

// HeartbeatRequest is the request body for POST /api/scanner/heartbeat.
type HeartbeatRequest struct {
	ActiveDomains []string `json:"active_domains"`
	SessionID     string   `json:"session_id"`
}

// HeartbeatResponse is the response for POST /api/scanner/heartbeat.
type HeartbeatResponse struct {
	OK bool `json:"ok"`
}

// LOCRecord represents a discovered LOC record.
type LOCRecord struct {
	FQDN       string  `json:"fqdn"`
	RawRecord  string  `json:"raw_record"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	AltitudeM  float64 `json:"altitude_m"`
	SizeM      float64 `json:"size_m"`
	HorizPrecM float64 `json:"horiz_prec_m"`
	VertPrecM  float64 `json:"vert_prec_m"`
}

// DomainResult represents scan results for a single domain.
type DomainResult struct {
	Domain            string      `json:"domain"`
	SubdomainsScanned int         `json:"subdomains_scanned"`
	LOCRecords        []LOCRecord `json:"loc_records"`
}

// SubmitResultsRequest is the request body for POST /api/scanner/results.
type SubmitResultsRequest struct {
	Results []DomainResult `json:"results"`
}

// SubmitResultsResponse is the response for POST /api/scanner/results.
type SubmitResultsResponse struct {
	Accepted int `json:"accepted"`
}

// --- Public API Types ---

// PublicLOCRecord represents a LOC record in the public API.
type PublicLOCRecord struct {
	FQDN        string    `json:"fqdn"`
	RootDomain  string    `json:"root_domain"`
	RawRecord   string    `json:"raw_record"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	AltitudeM   float64   `json:"altitude_m"`
	SizeM       float64   `json:"size_m"`
	HorizPrecM  float64   `json:"horiz_prec_m"`
	VertPrecM   float64   `json:"vert_prec_m"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
}

// AggregatedLocation represents multiple LOC records at the same coordinates.
// Used for GeoJSON export to avoid supercluster issues with identical coordinates.
type AggregatedLocation struct {
	FQDNs       []string  `json:"fqdns"`
	RootDomains []string  `json:"root_domains"`
	RawRecord   string    `json:"raw_record"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	AltitudeM   float64   `json:"altitude_m"`
	Count       int       `json:"count"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
}

// ListRecordsResponse is the response for GET /api/public/records.
type ListRecordsResponse struct {
	Records []PublicLOCRecord `json:"records"`
	Total   int               `json:"total"`
	Limit   int               `json:"limit"`
	Offset  int               `json:"offset"`
}

// StatsResponse is the response for GET /api/public/stats.
type StatsResponse struct {
	TotalRootDomains         int   `json:"total_root_domains"`
	ScannedRootDomains       int   `json:"scanned_root_domains"`
	PendingRootDomains       int   `json:"pending_root_domains"`
	InProgressRootDomains    int   `json:"in_progress_root_domains"`
	TotalSubdomainsScanned   int64 `json:"total_subdomains_scanned"`
	ActiveScanners           int   `json:"active_scanners"`
	TotalLOCRecords          int   `json:"total_loc_records"`
	UniqueRootDomainsWithLOC int   `json:"unique_root_domains_with_loc"`
}

// ErrorResponse is a standard error response.
type ErrorResponse struct {
	Error string `json:"error"`
}

// --- GeoJSON Types (RFC 7946) ---

// GeoJSONFeatureCollection is a GeoJSON FeatureCollection.
type GeoJSONFeatureCollection struct {
	Type     string           `json:"type"` // Always "FeatureCollection"
	Features []GeoJSONFeature `json:"features"`
}

// GeoJSONFeature is a GeoJSON Feature with Point geometry.
type GeoJSONFeature struct {
	Type       string         `json:"type"` // Always "Feature"
	Geometry   GeoJSONPoint   `json:"geometry"`
	Properties map[string]any `json:"properties"`
}

// GeoJSONPoint is a GeoJSON Point geometry.
type GeoJSONPoint struct {
	Type        string    `json:"type"`        // Always "Point"
	Coordinates []float64 `json:"coordinates"` // [longitude, latitude] or [longitude, latitude, altitude]
}

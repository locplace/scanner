// Package api contains shared types for the coordinator API.
package api

import "time"

// --- Admin API Types ---

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
	ActiveBatches int        `json:"active_batches"`
	IsAlive       bool       `json:"is_alive"`
}

// ListClientsResponse is the response for GET /api/admin/clients.
type ListClientsResponse struct {
	Clients []ClientInfo `json:"clients"`
}

// DiscoverFilesResponse is the response for POST /api/admin/discover-files.
type DiscoverFilesResponse struct {
	FilesDiscovered int `json:"files_discovered"`
}

// ResetScanResponse is the response for POST /api/admin/reset-scan.
type ResetScanResponse struct {
	FilesReset int `json:"files_reset"`
}

// --- Scanner API Types ---

// GetBatchRequest is the request body for POST /api/scanner/jobs.
type GetBatchRequest struct {
	SessionID string `json:"session_id"`
}

// GetBatchResponse is the response for POST /api/scanner/jobs.
// Returns a batch of FQDNs to scan for LOC records.
type GetBatchResponse struct {
	BatchID int64    `json:"batch_id,omitempty"`
	Domains []string `json:"domains"`
}

// HeartbeatRequest is the request body for POST /api/scanner/heartbeat.
type HeartbeatRequest struct {
	SessionID string `json:"session_id"`
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

// SubmitBatchRequest is the request body for POST /api/scanner/results.
type SubmitBatchRequest struct {
	BatchID        int64       `json:"batch_id"`
	DomainsChecked int         `json:"domains_checked"`
	LOCRecords     []LOCRecord `json:"loc_records"`
}

// SubmitBatchResponse is the response for POST /api/scanner/results.
type SubmitBatchResponse struct {
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

// DomainFileStats holds statistics for domain file processing.
type DomainFileStats struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	Processing int `json:"processing"`
	Complete   int `json:"complete"`
}

// BatchQueueStats holds statistics for the batch queue.
type BatchQueueStats struct {
	Pending  int `json:"pending"`
	InFlight int `json:"in_flight"`
}

// CurrentFileProgress holds progress info for the currently processing file.
type CurrentFileProgress struct {
	Filename         string  `json:"filename,omitempty"`
	ProcessedLines   int64   `json:"processed_lines"`
	BatchesCreated   int     `json:"batches_created"`
	BatchesCompleted int     `json:"batches_completed"`
	ProgressPct      float64 `json:"progress_pct"`
}

// StatsResponse is the response for GET /api/public/stats.
type StatsResponse struct {
	// LOC record stats
	TotalLOCRecords          int `json:"total_loc_records"`
	UniqueRootDomainsWithLOC int `json:"unique_root_domains_with_loc"`

	// Scanner stats
	ActiveScanners int `json:"active_scanners"`

	// File-based scanning stats
	DomainFiles DomainFileStats      `json:"domain_files"`
	BatchQueue  BatchQueueStats      `json:"batch_queue"`
	CurrentFile *CurrentFileProgress `json:"current_file,omitempty"`
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

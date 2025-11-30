package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/locplace/scanner/pkg/api"
)

func TestWriteJSON(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		data       any
		wantBody   string
		wantStatus int
	}{
		{
			name:       "simple struct",
			status:     http.StatusOK,
			data:       api.HeartbeatResponse{OK: true},
			wantBody:   `{"ok":true}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "error response",
			status:     http.StatusBadRequest,
			data:       api.ErrorResponse{Error: "test error"},
			wantBody:   `{"error":"test error"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "complex response",
			status: http.StatusOK,
			data: api.AddDomainsToSetResponse{
				Inserted:   5,
				Duplicates: 2,
			},
			wantBody:   `{"inserted":5,"duplicates":2}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "empty struct",
			status:     http.StatusOK,
			data:       struct{}{},
			wantBody:   `{}`,
			wantStatus: http.StatusOK,
		},
		{
			name:   "response with array",
			status: http.StatusOK,
			data: api.GetJobsResponse{
				Domains: []api.DomainJob{
					{Domain: "example.com"},
					{Domain: "test.org"},
				},
			},
			wantBody:   `{"domains":[{"domain":"example.com"},{"domain":"test.org"}]}`,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			writeJSON(rr, tt.status, tt.data)

			if rr.Code != tt.wantStatus {
				t.Errorf("status code = %d, want %d", rr.Code, tt.wantStatus)
			}

			contentType := rr.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
			}

			// Normalize JSON for comparison (remove whitespace)
			gotBody := strings.TrimSpace(rr.Body.String())
			if gotBody != tt.wantBody {
				t.Errorf("body = %q, want %q", gotBody, tt.wantBody)
			}
		})
	}
}

func TestWriteError(t *testing.T) {
	tests := []struct {
		name       string
		message    string
		status     int
		wantBody   string
		wantStatus int
	}{
		{
			name:       "bad request",
			message:    "invalid input",
			status:     http.StatusBadRequest,
			wantBody:   `{"error":"invalid input"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "not found",
			message:    "resource not found",
			status:     http.StatusNotFound,
			wantBody:   `{"error":"resource not found"}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "internal error",
			message:    "something went wrong",
			status:     http.StatusInternalServerError,
			wantBody:   `{"error":"something went wrong"}`,
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "unauthorized",
			message:    "unauthorized",
			status:     http.StatusUnauthorized,
			wantBody:   `{"error":"unauthorized"}`,
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			writeError(rr, tt.message, tt.status)

			if rr.Code != tt.wantStatus {
				t.Errorf("status code = %d, want %d", rr.Code, tt.wantStatus)
			}

			contentType := rr.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
			}

			gotBody := strings.TrimSpace(rr.Body.String())
			if gotBody != tt.wantBody {
				t.Errorf("body = %q, want %q", gotBody, tt.wantBody)
			}
		})
	}
}

func TestParseIntParam(t *testing.T) {
	tests := []struct {
		name       string
		queryParam string
		paramName  string
		defaultVal int
		want       int
	}{
		{
			name:       "valid positive integer",
			queryParam: "limit=50",
			paramName:  "limit",
			defaultVal: 100,
			want:       50,
		},
		{
			name:       "missing parameter uses default",
			queryParam: "",
			paramName:  "limit",
			defaultVal: 100,
			want:       100,
		},
		{
			name:       "different parameter name uses default",
			queryParam: "offset=10",
			paramName:  "limit",
			defaultVal: 100,
			want:       100,
		},
		{
			name:       "zero value",
			queryParam: "limit=0",
			paramName:  "limit",
			defaultVal: 100,
			want:       0,
		},
		{
			name:       "negative value uses default",
			queryParam: "limit=-5",
			paramName:  "limit",
			defaultVal: 100,
			want:       100,
		},
		{
			name:       "non-numeric value uses default",
			queryParam: "limit=abc",
			paramName:  "limit",
			defaultVal: 100,
			want:       100,
		},
		{
			name:       "float value uses default",
			queryParam: "limit=10.5",
			paramName:  "limit",
			defaultVal: 100,
			want:       100,
		},
		{
			name:       "large number",
			queryParam: "limit=999999",
			paramName:  "limit",
			defaultVal: 100,
			want:       999999,
		},
		{
			name:       "offset parameter",
			queryParam: "offset=25",
			paramName:  "offset",
			defaultVal: 0,
			want:       25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/test"
			if tt.queryParam != "" {
				url = "/test?" + tt.queryParam
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)

			got := parseIntParam(req, tt.paramName, tt.defaultVal)
			if got != tt.want {
				t.Errorf("parseIntParam() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAddDomainsRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name:    "valid request with domains",
			body:    `{"domains":["example.com","test.org"]}`,
			wantErr: false,
		},
		{
			name:    "empty domains array",
			body:    `{"domains":[]}`,
			wantErr: false, // This is valid JSON, validation happens in handler
		},
		{
			name:    "invalid JSON",
			body:    `{"domains":`,
			wantErr: true,
		},
		{
			name:    "missing domains field",
			body:    `{}`,
			wantErr: false, // This is valid JSON, just empty
		},
		{
			name:    "wrong type for domains",
			body:    `{"domains":"not an array"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req api.AddDomainsToSetRequest
			err := json.NewDecoder(strings.NewReader(tt.body)).Decode(&req)

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestRegisterClientRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name:    "valid request",
			body:    `{"name":"scanner-1"}`,
			wantErr: false,
		},
		{
			name:    "empty name",
			body:    `{"name":""}`,
			wantErr: false, // Valid JSON, validation in handler
		},
		{
			name:    "invalid JSON",
			body:    `{"name":}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req api.RegisterClientRequest
			err := json.NewDecoder(strings.NewReader(tt.body)).Decode(&req)

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestGetJobsRequest_Validation(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "valid request",
			body:      `{"count":10}`,
			wantCount: 10,
			wantErr:   false,
		},
		{
			name:      "zero count",
			body:      `{"count":0}`,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "negative count",
			body:      `{"count":-5}`,
			wantCount: -5,
			wantErr:   false, // Handler normalizes this
		},
		{
			name:      "missing count",
			body:      `{}`,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:    "invalid JSON",
			body:    `{count:10}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req api.GetJobsRequest
			err := json.NewDecoder(strings.NewReader(tt.body)).Decode(&req)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if req.Count != tt.wantCount {
				t.Errorf("Count = %d, want %d", req.Count, tt.wantCount)
			}
		})
	}
}

func TestSubmitResultsRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantLen int
		wantErr bool
	}{
		{
			name: "valid request with LOC records",
			body: `{
				"results": [{
					"domain": "example.com",
					"subdomains_scanned": 100,
					"loc_records": [{
						"fqdn": "example.com",
						"raw_record": "52 22 23.000 N 4 53 32.000 E -2.00m 1m 10000m 10m",
						"latitude": 52.373055,
						"longitude": 4.892222,
						"altitude_m": -2.0,
						"size_m": 1.0,
						"horiz_prec_m": 10000.0,
						"vert_prec_m": 10.0
					}]
				}]
			}`,
			wantLen: 1,
			wantErr: false,
		},
		{
			name:    "empty results array",
			body:    `{"results":[]}`,
			wantLen: 0,
			wantErr: false,
		},
		{
			name: "multiple results",
			body: `{
				"results": [
					{"domain": "a.com", "subdomains_scanned": 10, "loc_records": []},
					{"domain": "b.com", "subdomains_scanned": 20, "loc_records": []}
				]
			}`,
			wantLen: 2,
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			body:    `{"results": [}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req api.SubmitResultsRequest
			err := json.NewDecoder(strings.NewReader(tt.body)).Decode(&req)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(req.Results) != tt.wantLen {
				t.Errorf("len(Results) = %d, want %d", len(req.Results), tt.wantLen)
			}
		})
	}
}

func TestLOCRecordDeduplication(t *testing.T) {
	// Test the deduplication logic from SubmitResults
	// Given a set of LOC records, simulate the deduplication
	tests := []struct {
		name          string
		domain        string
		records       []api.LOCRecord
		wantKeptCount int
		wantKeptFQDNs []string
	}{
		{
			name:   "root domain record deduplicates matching subdomains",
			domain: "example.com",
			records: []api.LOCRecord{
				{FQDN: "example.com", RawRecord: "52 0 0 N 4 0 0 E 0m 1m 1m 1m"},
				{FQDN: "www.example.com", RawRecord: "52 0 0 N 4 0 0 E 0m 1m 1m 1m"},     // Same as root - skip
				{FQDN: "mail.example.com", RawRecord: "52 0 0 N 4 0 0 E 0m 1m 1m 1m"},    // Same as root - skip
				{FQDN: "unique.example.com", RawRecord: "40 0 0 N 74 0 0 W 0m 1m 1m 1m"}, // Different - keep
			},
			wantKeptCount: 2,
			wantKeptFQDNs: []string{"example.com", "unique.example.com"},
		},
		{
			name:   "no root domain record keeps all",
			domain: "example.com",
			records: []api.LOCRecord{
				{FQDN: "www.example.com", RawRecord: "52 0 0 N 4 0 0 E 0m 1m 1m 1m"},
				{FQDN: "mail.example.com", RawRecord: "52 0 0 N 4 0 0 E 0m 1m 1m 1m"},
			},
			wantKeptCount: 2,
			wantKeptFQDNs: []string{"www.example.com", "mail.example.com"},
		},
		{
			name:   "only root domain record",
			domain: "example.com",
			records: []api.LOCRecord{
				{FQDN: "example.com", RawRecord: "52 0 0 N 4 0 0 E 0m 1m 1m 1m"},
			},
			wantKeptCount: 1,
			wantKeptFQDNs: []string{"example.com"},
		},
		{
			name:          "empty records",
			domain:        "example.com",
			records:       []api.LOCRecord{},
			wantKeptCount: 0,
			wantKeptFQDNs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Find root LOC record
			var rootLOCRecord string
			for _, loc := range tt.records {
				if loc.FQDN == tt.domain {
					rootLOCRecord = loc.RawRecord
					break
				}
			}

			// Apply deduplication logic
			var kept []api.LOCRecord
			for _, loc := range tt.records {
				if rootLOCRecord != "" && loc.FQDN != tt.domain && loc.RawRecord == rootLOCRecord {
					continue // Skip
				}
				kept = append(kept, loc)
			}

			if len(kept) != tt.wantKeptCount {
				t.Errorf("kept count = %d, want %d", len(kept), tt.wantKeptCount)
			}

			// Verify correct FQDNs are kept
			keptFQDNs := make(map[string]bool)
			for _, loc := range kept {
				keptFQDNs[loc.FQDN] = true
			}

			for _, wantFQDN := range tt.wantKeptFQDNs {
				if !keptFQDNs[wantFQDN] {
					t.Errorf("expected FQDN %q to be kept, but it wasn't", wantFQDN)
				}
			}
		})
	}
}

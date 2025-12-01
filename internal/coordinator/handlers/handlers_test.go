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
			name:   "submit batch response",
			status: http.StatusOK,
			data: api.SubmitBatchResponse{
				Accepted: 5,
			},
			wantBody:   `{"accepted":5}`,
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
			name:   "batch response with domains",
			status: http.StatusOK,
			data: api.GetBatchResponse{
				BatchID: 123,
				Domains: []string{"example.com", "test.org"},
			},
			wantBody:   `{"batch_id":123,"domains":["example.com","test.org"]}`,
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

func TestGetBatchRequest_Validation(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		wantSessionID string
		wantErr       bool
	}{
		{
			name:          "valid request",
			body:          `{"session_id":"abc123"}`,
			wantSessionID: "abc123",
			wantErr:       false,
		},
		{
			name:          "empty session_id",
			body:          `{"session_id":""}`,
			wantSessionID: "",
			wantErr:       false, // Handler normalizes this
		},
		{
			name:          "missing session_id",
			body:          `{}`,
			wantSessionID: "",
			wantErr:       false,
		},
		{
			name:    "invalid JSON",
			body:    `{session_id:10}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req api.GetBatchRequest
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

			if req.SessionID != tt.wantSessionID {
				t.Errorf("SessionID = %q, want %q", req.SessionID, tt.wantSessionID)
			}
		})
	}
}

func TestSubmitBatchRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantLen int
		wantErr bool
	}{
		{
			name: "valid request with LOC records",
			body: `{
				"batch_id": 123,
				"domains_checked": 100,
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
			}`,
			wantLen: 1,
			wantErr: false,
		},
		{
			name:    "empty loc_records array",
			body:    `{"batch_id": 456, "domains_checked": 50, "loc_records":[]}`,
			wantLen: 0,
			wantErr: false,
		},
		{
			name: "multiple LOC records",
			body: `{
				"batch_id": 789,
				"domains_checked": 200,
				"loc_records": [
					{"fqdn": "a.com", "raw_record": "52 0 0 N 4 0 0 E 0m 1m 1m 1m", "latitude": 52.0, "longitude": 4.0},
					{"fqdn": "b.com", "raw_record": "40 0 0 N 74 0 0 W 0m 1m 1m 1m", "latitude": 40.0, "longitude": -74.0}
				]
			}`,
			wantLen: 2,
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			body:    `{"batch_id": 123, "loc_records": [}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req api.SubmitBatchRequest
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

			if len(req.LOCRecords) != tt.wantLen {
				t.Errorf("len(LOCRecords) = %d, want %d", len(req.LOCRecords), tt.wantLen)
			}
		})
	}
}

func TestLOCRecord_Parsing(t *testing.T) {
	// Test that LOC records parse correctly
	tests := []struct {
		name     string
		body     string
		wantFQDN string
		wantLat  float64
		wantLong float64
		wantErr  bool
	}{
		{
			name: "valid LOC record",
			body: `{
				"fqdn": "example.com",
				"raw_record": "52 22 23.000 N 4 53 32.000 E -2.00m 1m 10000m 10m",
				"latitude": 52.373055,
				"longitude": 4.892222,
				"altitude_m": -2.0,
				"size_m": 1.0,
				"horiz_prec_m": 10000.0,
				"vert_prec_m": 10.0
			}`,
			wantFQDN: "example.com",
			wantLat:  52.373055,
			wantLong: 4.892222,
			wantErr:  false,
		},
		{
			name: "negative longitude (West)",
			body: `{
				"fqdn": "nyc.example.com",
				"raw_record": "40 42 46 N 74 0 22 W 0m 1m 1m 1m",
				"latitude": 40.7128,
				"longitude": -74.006,
				"altitude_m": 0,
				"size_m": 1.0,
				"horiz_prec_m": 1.0,
				"vert_prec_m": 1.0
			}`,
			wantFQDN: "nyc.example.com",
			wantLat:  40.7128,
			wantLong: -74.006,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rec api.LOCRecord
			err := json.NewDecoder(strings.NewReader(tt.body)).Decode(&rec)

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

			if rec.FQDN != tt.wantFQDN {
				t.Errorf("FQDN = %q, want %q", rec.FQDN, tt.wantFQDN)
			}
			if rec.Latitude != tt.wantLat {
				t.Errorf("Latitude = %f, want %f", rec.Latitude, tt.wantLat)
			}
			if rec.Longitude != tt.wantLong {
				t.Errorf("Longitude = %f, want %f", rec.Longitude, tt.wantLong)
			}
		})
	}
}

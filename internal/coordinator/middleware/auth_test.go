package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/locplace/scanner/internal/coordinator/db"
)

func TestAdminAuth(t *testing.T) {
	const validKey = "test-admin-key-12345"

	tests := []struct {
		name           string
		headerKey      string
		headerValue    string
		wantStatusCode int
		wantNextCalled bool
	}{
		{
			name:           "valid API key",
			headerKey:      "X-Admin-Key",
			headerValue:    validKey,
			wantStatusCode: http.StatusOK,
			wantNextCalled: true,
		},
		{
			name:           "missing API key header",
			headerKey:      "",
			headerValue:    "",
			wantStatusCode: http.StatusUnauthorized,
			wantNextCalled: false,
		},
		{
			name:           "wrong API key",
			headerKey:      "X-Admin-Key",
			headerValue:    "wrong-key",
			wantStatusCode: http.StatusUnauthorized,
			wantNextCalled: false,
		},
		{
			name:           "empty API key value",
			headerKey:      "X-Admin-Key",
			headerValue:    "",
			wantStatusCode: http.StatusUnauthorized,
			wantNextCalled: false,
		},
		{
			name:           "wrong header name",
			headerKey:      "Authorization",
			headerValue:    validKey,
			wantStatusCode: http.StatusUnauthorized,
			wantNextCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextCalled := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
			})

			middleware := AdminAuth(validKey)
			handler := middleware(next)

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.headerKey != "" {
				req.Header.Set(tt.headerKey, tt.headerValue)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatusCode {
				t.Errorf("status code = %d, want %d", rr.Code, tt.wantStatusCode)
			}

			if nextCalled != tt.wantNextCalled {
				t.Errorf("next handler called = %v, want %v", nextCalled, tt.wantNextCalled)
			}

			// Verify error response format for unauthorized
			if tt.wantStatusCode == http.StatusUnauthorized {
				body := strings.TrimSpace(rr.Body.String())
				if body != `{"error":"unauthorized"}` {
					t.Errorf("error response = %q, want %q", body, `{"error":"unauthorized"}`)
				}
			}
		})
	}
}

func TestAdminAuth_EmptyConfiguredKey(t *testing.T) {
	// Edge case: what happens if the configured key is empty?
	// This should reject all requests since "" != "" after the empty check
	middleware := AdminAuth("")
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	handler := middleware(next)

	// Even with empty header value matching empty configured key,
	// it should reject because we check for empty key first
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Admin-Key", "")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status code = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
	if nextCalled {
		t.Error("next handler should not be called for empty key")
	}
}

func TestGetClient(t *testing.T) {
	tests := []struct {
		name       string
		ctx        context.Context
		wantClient *db.ScannerClient
	}{
		{
			name: "client in context",
			ctx: context.WithValue(context.Background(), ClientContextKey, &db.ScannerClient{
				ID:   "test-id",
				Name: "test-client",
			}),
			wantClient: &db.ScannerClient{
				ID:   "test-id",
				Name: "test-client",
			},
		},
		{
			name:       "no client in context",
			ctx:        context.Background(),
			wantClient: nil,
		},
		{
			name:       "wrong type in context",
			ctx:        context.WithValue(context.Background(), ClientContextKey, "not a client"),
			wantClient: nil,
		},
		{
			name:       "nil value in context",
			ctx:        context.WithValue(context.Background(), ClientContextKey, (*db.ScannerClient)(nil)),
			wantClient: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetClient(tt.ctx)

			if tt.wantClient == nil {
				if got != nil {
					t.Errorf("GetClient() = %v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Errorf("GetClient() = nil, want %v", tt.wantClient)
				return
			}

			if got.ID != tt.wantClient.ID || got.Name != tt.wantClient.Name {
				t.Errorf("GetClient() = %v, want %v", got, tt.wantClient)
			}
		})
	}
}

func TestGetClient_FullClient(t *testing.T) {
	// Test with a fully populated client
	now := time.Now()
	client := &db.ScannerClient{
		ID:            "uuid-123",
		Name:          "scanner-1",
		TokenHash:     "hashed-token",
		CreatedAt:     now,
		LastHeartbeat: &now,
	}

	ctx := context.WithValue(context.Background(), ClientContextKey, client)
	got := GetClient(ctx)

	if got == nil {
		t.Fatal("GetClient() returned nil, expected client")
	}

	if got.ID != client.ID {
		t.Errorf("ID = %q, want %q", got.ID, client.ID)
	}
	if got.Name != client.Name {
		t.Errorf("Name = %q, want %q", got.Name, client.Name)
	}
	if got.TokenHash != client.TokenHash {
		t.Errorf("TokenHash = %q, want %q", got.TokenHash, client.TokenHash)
	}
	if !got.CreatedAt.Equal(client.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, client.CreatedAt)
	}
	if got.LastHeartbeat == nil || !got.LastHeartbeat.Equal(*client.LastHeartbeat) {
		t.Errorf("LastHeartbeat = %v, want %v", got.LastHeartbeat, client.LastHeartbeat)
	}
}

// TestScannerAuth_HeaderParsing tests the header parsing logic of ScannerAuth
// without requiring a database connection. These test the early-exit paths.
func TestScannerAuth_HeaderParsing(t *testing.T) {
	// We can't fully test ScannerAuth without a database, but we can test
	// the header parsing logic that rejects requests before DB lookup.

	tests := []struct {
		name           string
		authHeader     string
		wantStatusCode int
	}{
		{
			name:           "missing Authorization header",
			authHeader:     "",
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name:           "wrong auth scheme - Basic",
			authHeader:     "Basic dXNlcjpwYXNz",
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name:           "wrong auth scheme - no scheme",
			authHeader:     "just-a-token",
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name:           "Bearer with no token",
			authHeader:     "Bearer ",
			wantStatusCode: http.StatusUnauthorized, // Empty token will fail DB lookup
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock DB that would be called if header parsing passes
			// Since we're testing early-exit paths, we use nil DB
			// This will cause a panic if the middleware tries to use it,
			// which helps us verify the early-exit logic works.

			// For this test, we need a real DB interface, so we'll skip
			// the cases that would reach the DB lookup. We're only testing
			// the header validation that happens BEFORE the DB call.

			// The middleware checks: auth == "" || !strings.HasPrefix(auth, "Bearer ")
			// So "Bearer " (with space, empty token) will pass the header check
			// and try to call the DB.

			if tt.authHeader == "Bearer " {
				// This case reaches the DB, skip it in this unit test
				t.Skip("This case requires DB integration test")
			}

			// For cases that fail before DB lookup, we can test with nil
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("next handler should not be called")
			})

			middleware := ScannerAuth(nil) // nil DB is fine for early-exit tests
			handler := middleware(next)

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatusCode {
				t.Errorf("status code = %d, want %d", rr.Code, tt.wantStatusCode)
			}
		})
	}
}

func TestClientContextKey_Type(t *testing.T) {
	// Verify the context key is the expected type and value
	// This is a sanity check that the key hasn't been accidentally changed
	if ClientContextKey != contextKey("client") {
		t.Errorf("ClientContextKey = %v, want %v", ClientContextKey, contextKey("client"))
	}
}

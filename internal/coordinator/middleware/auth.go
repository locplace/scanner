// Package middleware provides HTTP middleware for the coordinator.
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/locplace/scanner/internal/coordinator/db"
)

type contextKey string

const (
	// ClientContextKey is the context key for the authenticated client.
	ClientContextKey contextKey = "client"
)

// AdminAuth returns middleware that validates the admin API key.
func AdminAuth(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-Admin-Key")
			if key == "" || key != apiKey {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ScannerAuth returns middleware that validates scanner bearer tokens.
func ScannerAuth(database *db.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			token := strings.TrimPrefix(auth, "Bearer ")
			client, err := database.GetClientByToken(r.Context(), token)
			if err != nil {
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
				return
			}
			if client == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), ClientContextKey, client)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetClient retrieves the authenticated client from the request context.
// Returns nil if no client is present or if the value is not a *ScannerClient.
func GetClient(ctx context.Context) *db.ScannerClient {
	client, _ := ctx.Value(ClientContextKey).(*db.ScannerClient) //nolint:errcheck // Type assertion returns (nil, false) on failure, which is the desired behavior
	return client
}

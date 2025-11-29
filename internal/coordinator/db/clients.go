package db

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/jackc/pgx/v5"
)

// ScannerClient represents a registered scanner client.
type ScannerClient struct {
	ID            string
	Name          string
	TokenHash     string
	CreatedAt     time.Time
	LastHeartbeat *time.Time
}

// generateToken creates a secure random token.
func generateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// hashToken creates a SHA-256 hash of the token.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// CreateClient creates a new scanner client and returns the plaintext token.
func (db *DB) CreateClient(ctx context.Context, name string) (id, token string, err error) {
	token, err = generateToken()
	if err != nil {
		return "", "", err
	}

	tokenHash := hashToken(token)

	err = db.Pool.QueryRow(ctx, `
		INSERT INTO scanner_clients (name, token_hash)
		VALUES ($1, $2)
		RETURNING id
	`, name, tokenHash).Scan(&id)
	if err != nil {
		return "", "", err
	}

	return id, token, nil
}

// GetClientByToken retrieves a client by their token.
func (db *DB) GetClientByToken(ctx context.Context, token string) (*ScannerClient, error) {
	tokenHash := hashToken(token)

	var client ScannerClient
	err := db.Pool.QueryRow(ctx, `
		SELECT id, name, token_hash, created_at, last_heartbeat
		FROM scanner_clients WHERE token_hash = $1
	`, tokenHash).Scan(&client.ID, &client.Name, &client.TokenHash, &client.CreatedAt, &client.LastHeartbeat)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &client, nil
}

// GetClientByID retrieves a client by ID.
func (db *DB) GetClientByID(ctx context.Context, id string) (*ScannerClient, error) {
	var client ScannerClient
	err := db.Pool.QueryRow(ctx, `
		SELECT id, name, token_hash, created_at, last_heartbeat
		FROM scanner_clients WHERE id = $1
	`, id).Scan(&client.ID, &client.Name, &client.TokenHash, &client.CreatedAt, &client.LastHeartbeat)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &client, nil
}

// ClientWithStats represents a client with active domain count.
type ClientWithStats struct {
	ScannerClient
	ActiveDomains int
}

// ListClients returns all clients with their active domain counts.
func (db *DB) ListClients(ctx context.Context) ([]ClientWithStats, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT
			c.id, c.name, c.token_hash, c.created_at, c.last_heartbeat,
			COUNT(s.root_domain_id) as active_domains
		FROM scanner_clients c
		LEFT JOIN active_scans s ON s.client_id = c.id
		GROUP BY c.id
		ORDER BY c.created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clients []ClientWithStats
	for rows.Next() {
		var c ClientWithStats
		if err := rows.Scan(&c.ID, &c.Name, &c.TokenHash, &c.CreatedAt, &c.LastHeartbeat, &c.ActiveDomains); err != nil {
			return nil, err
		}
		clients = append(clients, c)
	}
	return clients, rows.Err()
}

// DeleteClient deletes a client by ID.
func (db *DB) DeleteClient(ctx context.Context, id string) error {
	tag, err := db.Pool.Exec(ctx, `DELETE FROM scanner_clients WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// UpdateHeartbeat updates the client's last_heartbeat timestamp.
func (db *DB) UpdateHeartbeat(ctx context.Context, clientID string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE scanner_clients SET last_heartbeat = NOW() WHERE id = $1
	`, clientID)
	return err
}

// CountActiveClients returns the number of clients with recent heartbeats.
func (db *DB) CountActiveClients(ctx context.Context, timeout time.Duration) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM scanner_clients
		WHERE last_heartbeat > NOW() - $1::interval
	`, timeout.String()).Scan(&count)
	return count, err
}

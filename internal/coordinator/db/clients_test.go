package db

import (
	"testing"
)

func TestHashToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "typical token",
			token: "abc123def456",
		},
		{
			name:  "empty token",
			token: "",
		},
		{
			name:  "long token",
			token: "this-is-a-very-long-token-that-might-be-used-in-production-systems-12345678901234567890",
		},
		{
			name:  "special characters",
			token: "token-with-special!@#$%^&*()chars",
		},
		{
			name:  "hex token like real ones",
			token: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := hashToken(tt.token)

			// SHA-256 produces 64 hex characters
			if len(hash) != 64 {
				t.Errorf("hash length = %d, want 64", len(hash))
			}

			// Hash should be deterministic
			hash2 := hashToken(tt.token)
			if hash != hash2 {
				t.Errorf("hash is not deterministic: %q != %q", hash, hash2)
			}

			// Hash should be hex string
			for _, c := range hash {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
					t.Errorf("hash contains non-hex character: %c", c)
				}
			}
		})
	}
}

func TestHashToken_DifferentInputs(t *testing.T) {
	// Different inputs should produce different hashes
	tokens := []string{
		"token1",
		"token2",
		"Token1",  // Case difference
		"token1 ", // Trailing space
		" token1", // Leading space
	}

	hashes := make(map[string]string)
	for _, token := range tokens {
		hash := hashToken(token)
		if existing, ok := hashes[hash]; ok {
			t.Errorf("collision: %q and %q produce same hash", token, existing)
		}
		hashes[hash] = token
	}
}

func TestGenerateToken(t *testing.T) {
	// Test that generateToken produces valid tokens
	token, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken() error: %v", err)
	}

	// Token should be 64 hex characters (32 bytes encoded as hex)
	if len(token) != 64 {
		t.Errorf("token length = %d, want 64", len(token))
	}

	// Token should be hex string
	for _, c := range token {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("token contains non-hex character: %c", c)
		}
	}
}

func TestGenerateToken_Uniqueness(t *testing.T) {
	// Generate multiple tokens and verify they're unique
	tokens := make(map[string]bool)
	const numTokens = 100

	for i := 0; i < numTokens; i++ {
		token, err := generateToken()
		if err != nil {
			t.Fatalf("generateToken() error: %v", err)
		}

		if tokens[token] {
			t.Errorf("duplicate token generated: %s", token)
		}
		tokens[token] = true
	}
}

func TestScannerClient_Fields(t *testing.T) {
	// Test that ScannerClient struct can hold all expected data
	client := ScannerClient{
		ID:        "test-id",
		Name:      "test-scanner",
		TokenHash: "abc123",
	}

	if client.ID != "test-id" {
		t.Errorf("ID = %q, want %q", client.ID, "test-id")
	}
	if client.Name != "test-scanner" {
		t.Errorf("Name = %q, want %q", client.Name, "test-scanner")
	}
	if client.TokenHash != "abc123" {
		t.Errorf("TokenHash = %q, want %q", client.TokenHash, "abc123")
	}
	if client.LastHeartbeat != nil {
		t.Errorf("LastHeartbeat should be nil for new client")
	}
}

func TestClientWithStats_Embedding(t *testing.T) {
	// Test that ClientWithStats properly embeds ScannerClient
	client := ClientWithStats{
		ScannerClient: ScannerClient{
			ID:   "test-id",
			Name: "test-scanner",
		},
		ActiveDomains: 5,
	}

	// Can access embedded fields directly
	if client.ID != "test-id" {
		t.Errorf("ID = %q, want %q", client.ID, "test-id")
	}
	if client.Name != "test-scanner" {
		t.Errorf("Name = %q, want %q", client.Name, "test-scanner")
	}
	if client.ActiveDomains != 5 {
		t.Errorf("ActiveDomains = %d, want %d", client.ActiveDomains, 5)
	}
}

package scanner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/locplace/scanner/pkg/api"
)

// CoordinatorClient is an HTTP client for the coordinator API.
type CoordinatorClient struct {
	BaseURL    string
	Token      string
	SessionID  string // Unique ID for this scanner session (generated on startup)
	HTTPClient *http.Client
}

// NewCoordinatorClient creates a new coordinator API client.
// A new session ID is generated to track this scanner instance.
func NewCoordinatorClient(baseURL, token string) *CoordinatorClient {
	return &CoordinatorClient{
		BaseURL:   baseURL,
		Token:     token,
		SessionID: uuid.New().String(),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Batch represents a batch of FQDNs to scan.
type Batch struct {
	ID      int64
	Domains []string
}

// GetBatch requests a batch of FQDNs to scan from the coordinator.
func (c *CoordinatorClient) GetBatch(ctx context.Context) (*Batch, error) {
	req := api.GetBatchRequest{SessionID: c.SessionID}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/api/scanner/jobs", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck // Close error not actionable

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body) //nolint:errcheck // Best effort to get error details
		return nil, fmt.Errorf("get batch failed: %d %s", resp.StatusCode, string(bodyBytes))
	}

	var result api.GetBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// Empty response means no batches available
	if result.BatchID == 0 && len(result.Domains) == 0 {
		return nil, nil
	}

	return &Batch{
		ID:      result.BatchID,
		Domains: result.Domains,
	}, nil
}

// Heartbeat sends a keepalive signal to the coordinator.
func (c *CoordinatorClient) Heartbeat(ctx context.Context) error {
	req := api.HeartbeatRequest{SessionID: c.SessionID}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/api/scanner/heartbeat", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // Close error not actionable

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body) //nolint:errcheck // Best effort to get error details
		return fmt.Errorf("heartbeat failed: %d %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// SubmitBatch sends scan results for a batch to the coordinator.
// Uses a longer timeout than other requests since large result sets may take time to process.
func (c *CoordinatorClient) SubmitBatch(ctx context.Context, batchID int64, domainsChecked int, locRecords []api.LOCRecord) error {
	req := api.SubmitBatchRequest{
		BatchID:        batchID,
		DomainsChecked: domainsChecked,
		LOCRecords:     locRecords,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	// Use a longer timeout for submitting results (60s instead of 30s)
	submitCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(submitCtx, "POST", c.BaseURL+"/api/scanner/results", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // Close error not actionable

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body) //nolint:errcheck // Best effort to get error details
		return fmt.Errorf("submit batch failed: %d %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

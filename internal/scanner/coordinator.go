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

// GetJobs requests domains to scan from the coordinator.
func (c *CoordinatorClient) GetJobs(ctx context.Context, count int) ([]string, error) {
	req := api.GetJobsRequest{Count: count, SessionID: c.SessionID}
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
		return nil, fmt.Errorf("get jobs failed: %d %s", resp.StatusCode, string(bodyBytes))
	}

	var result api.GetJobsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	domains := make([]string, len(result.Domains))
	for i, d := range result.Domains {
		domains[i] = d.Domain
	}
	return domains, nil
}

// Heartbeat sends a keepalive signal to the coordinator.
func (c *CoordinatorClient) Heartbeat(ctx context.Context, activeDomains []string) error {
	req := api.HeartbeatRequest{ActiveDomains: activeDomains, SessionID: c.SessionID}
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

// SubmitResults sends scan results to the coordinator.
func (c *CoordinatorClient) SubmitResults(ctx context.Context, results []api.DomainResult) error {
	req := api.SubmitResultsRequest{Results: results}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/api/scanner/results", bytes.NewReader(body))
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
		return fmt.Errorf("submit results failed: %d %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

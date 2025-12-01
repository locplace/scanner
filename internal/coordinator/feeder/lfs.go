package feeder

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	// LFSBatchURL is the Git LFS batch API endpoint for the domains repository.
	LFSBatchURL = "https://github.com/tb0hdan/domains.git/info/lfs/objects/batch"
)

// LFSPointer represents a Git LFS pointer file.
type LFSPointer struct {
	Version string
	OID     string // SHA256 hash
	Size    int64
}

// LFSBatchRequest is the request body for the LFS batch API.
type LFSBatchRequest struct {
	Operation string      `json:"operation"`
	Transfers []string    `json:"transfers"`
	Objects   []LFSObject `json:"objects"`
}

// LFSObject represents an object in an LFS batch request/response.
type LFSObject struct {
	OID  string `json:"oid"`
	Size int64  `json:"size"`
}

// LFSBatchResponse is the response from the LFS batch API.
type LFSBatchResponse struct {
	Transfer string              `json:"transfer"`
	Objects  []LFSObjectResponse `json:"objects"`
}

// LFSObjectResponse represents an object in the LFS batch response.
type LFSObjectResponse struct {
	OID     string            `json:"oid"`
	Size    int64             `json:"size"`
	Actions map[string]Action `json:"actions"`
	Error   *LFSError         `json:"error,omitempty"`
}

// Action represents a download/upload action from LFS.
type Action struct {
	Href      string            `json:"href"`
	Header    map[string]string `json:"header,omitempty"`
	ExpiresAt string            `json:"expires_at,omitempty"`
}

// LFSError represents an error from the LFS API.
type LFSError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// LFSClient handles Git LFS operations.
type LFSClient struct {
	HTTPClient  *http.Client
	BatchURL    string
	GitHubToken string // Optional: GitHub PAT for authenticated downloads
}

// NewLFSClient creates a new LFS client.
func NewLFSClient() *LFSClient {
	return &LFSClient{
		HTTPClient: http.DefaultClient,
		BatchURL:   LFSBatchURL,
	}
}

// NewLFSClientWithToken creates a new LFS client with GitHub authentication.
// Using a token allows downloads to count against your account's LFS quota
// instead of the repository owner's quota.
func NewLFSClientWithToken(token string) *LFSClient {
	return &LFSClient{
		HTTPClient:  http.DefaultClient,
		BatchURL:    LFSBatchURL,
		GitHubToken: token,
	}
}

// ParsePointer parses a Git LFS pointer file content.
func ParsePointer(content []byte) (*LFSPointer, error) {
	pointer := &LFSPointer{}
	scanner := bufio.NewScanner(bytes.NewReader(content))

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "version ") {
			pointer.Version = strings.TrimPrefix(line, "version ")
		} else if strings.HasPrefix(line, "oid sha256:") {
			pointer.OID = strings.TrimPrefix(line, "oid sha256:")
		} else if strings.HasPrefix(line, "size ") {
			_, err := fmt.Sscanf(line, "size %d", &pointer.Size)
			if err != nil {
				return nil, fmt.Errorf("parse size: %w", err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan pointer: %w", err)
	}

	if pointer.OID == "" {
		return nil, fmt.Errorf("no OID found in pointer file")
	}

	return pointer, nil
}

// GetDownloadURL fetches the download URL for an LFS object.
func (c *LFSClient) GetDownloadURL(ctx context.Context, oid string, size int64) (string, map[string]string, error) {
	reqBody := LFSBatchRequest{
		Operation: "download",
		Transfers: []string{"basic"},
		Objects: []LFSObject{
			{OID: oid, Size: size},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BatchURL, bytes.NewReader(body))
	if err != nil {
		return "", nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/vnd.git-lfs+json")
	req.Header.Set("Accept", "application/vnd.git-lfs+json")

	// Add authentication if token is provided
	// This allows downloads to count against your LFS quota instead of the repo's
	if c.GitHubToken != "" {
		req.Header.Set("Authorization", "token "+c.GitHubToken)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("lfs batch request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // Close error not actionable

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", nil, fmt.Errorf("lfs batch: status %d: %s", resp.StatusCode, string(respBody))
	}

	var batchResp LFSBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&batchResp); err != nil {
		return "", nil, fmt.Errorf("decode response: %w", err)
	}

	if len(batchResp.Objects) == 0 {
		return "", nil, fmt.Errorf("no objects in response")
	}

	obj := batchResp.Objects[0]
	if obj.Error != nil {
		return "", nil, fmt.Errorf("lfs error %d: %s", obj.Error.Code, obj.Error.Message)
	}

	action, ok := obj.Actions["download"]
	if !ok {
		return "", nil, fmt.Errorf("no download action in response")
	}

	return action.Href, action.Header, nil
}

// Download downloads an LFS object and returns a reader for its content.
// It first tries the web-based download (github.com/raw/) which may have different
// quota handling, then falls back to the LFS batch API.
func (c *LFSClient) Download(ctx context.Context, oid string, size int64) (io.ReadCloser, error) {
	url, headers, err := c.GetDownloadURL(ctx, oid, size)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create download request: %w", err)
	}

	// Add any headers from the LFS response
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close() //nolint:errcheck // Close error not actionable
		return nil, fmt.Errorf("download: status %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// DownloadViaWeb downloads an LFS file using GitHub's web interface URL pattern.
// This uses github.com/{owner}/{repo}/raw/{branch}/{path} which redirects to
// the actual LFS content. This may have different quota handling than the LFS batch API.
func (c *LFSClient) DownloadViaWeb(ctx context.Context, owner, repo, branch, path string) (io.ReadCloser, error) {
	// Construct the GitHub web raw URL
	// e.g., https://github.com/tb0hdan/domains/raw/master/data/afghanistan/domain2multi-af00.txt.xz
	webURL := fmt.Sprintf("https://github.com/%s/%s/raw/%s/%s", owner, repo, branch, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, webURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Add authentication
	if c.GitHubToken != "" {
		req.Header.Set("Authorization", "token "+c.GitHubToken)
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "locplace-scanner/1.0")

	// Use a client that follows redirects (default behavior)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close() //nolint:errcheck // Close error not actionable
		return nil, fmt.Errorf("download: status %d: %s", resp.StatusCode, string(body))
	}

	return resp.Body, nil
}

// FetchPointer fetches and parses an LFS pointer file from a raw GitHub URL.
func (c *LFSClient) FetchPointer(ctx context.Context, rawURL string) (*LFSPointer, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch pointer: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // Close error not actionable

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch pointer: status %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read pointer: %w", err)
	}

	return ParsePointer(content)
}

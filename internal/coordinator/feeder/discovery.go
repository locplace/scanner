package feeder

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/locplace/scanner/internal/coordinator/db"
)

const (
	// GitHubTreeURL is the URL to fetch the repository tree.
	// Uses recursive=1 to get all files in one request.
	GitHubTreeURL = "https://api.github.com/repos/tb0hdan/domains/git/trees/master?recursive=1"

	// RawFileBaseURL is the base URL for raw file downloads.
	RawFileBaseURL = "https://raw.githubusercontent.com/tb0hdan/domains/master/"
)

// GitHubTree represents the GitHub API tree response.
type GitHubTree struct {
	SHA       string          `json:"sha"`
	URL       string          `json:"url"`
	Tree      []GitHubTreeObj `json:"tree"`
	Truncated bool            `json:"truncated"`
}

// GitHubTreeObj represents a single object in the tree.
type GitHubTreeObj struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"`
	SHA  string `json:"sha"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

// DiscoveredFile represents a discovered domain file.
type DiscoveredFile struct {
	Filename  string
	URL       string
	SizeBytes int64
}

// DiscoverFiles fetches the repository tree and returns all .xz domain files.
func DiscoverFiles(ctx context.Context) ([]DiscoveredFile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, GitHubTreeURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// GitHub API prefers Accept header
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "locplace-scanner/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch tree: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // Close error not actionable

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api: status %d", resp.StatusCode)
	}

	var tree GitHubTree
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if tree.Truncated {
		log.Println("Warning: GitHub tree response was truncated, some files may be missing")
	}

	// Filter for .xz files in the data directory
	var files []DiscoveredFile
	for _, obj := range tree.Tree {
		if obj.Type != "blob" {
			continue
		}

		// We want files like "data/a.txt.xz", "data/b.txt.xz", etc.
		if !strings.HasPrefix(obj.Path, "data/") {
			continue
		}
		if !strings.HasSuffix(obj.Path, ".txt.xz") {
			continue
		}

		files = append(files, DiscoveredFile{
			Filename:  obj.Path,
			URL:       RawFileBaseURL + obj.Path,
			SizeBytes: obj.Size,
		})
	}

	return files, nil
}

// DiscoverAndInsertFiles discovers files from GitHub and inserts them into the database.
// Returns the number of new files discovered.
func DiscoverAndInsertFiles(ctx context.Context, database *db.DB) (int, error) {
	files, err := DiscoverFiles(ctx)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, f := range files {
		if err := database.UpsertDomainFile(ctx, f.Filename, f.URL, f.SizeBytes); err != nil {
			log.Printf("Error upserting file %s: %v", f.Filename, err)
			continue
		}
		count++
	}

	log.Printf("Discovery complete: %d files in database", count)
	return count, nil
}

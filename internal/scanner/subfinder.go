package scanner

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// SubfinderConfig holds configuration for subfinder.
type SubfinderConfig struct {
	// Threads is the number of concurrent sources to query.
	Threads int
	// Timeout is the timeout in seconds for each source.
	Timeout int
	// MaxEnumerationTime is the maximum time in minutes for enumeration.
	MaxEnumerationTime int
}

// DefaultSubfinderConfig returns the default subfinder configuration.
func DefaultSubfinderConfig() SubfinderConfig {
	return SubfinderConfig{
		Threads:            10,
		Timeout:            30,
		MaxEnumerationTime: 5,
	}
}

// Subfinder wraps the subfinder CLI for subdomain enumeration.
type Subfinder struct {
	config SubfinderConfig
}

// NewSubfinder creates a new subfinder instance.
func NewSubfinder(config SubfinderConfig) *Subfinder {
	return &Subfinder{config: config}
}

// EnumerateSubdomains discovers subdomains for a given domain using the subfinder CLI.
// Returns a list of discovered subdomains (not including the root domain).
// Requires subfinder to be installed and available in PATH.
func (s *Subfinder) EnumerateSubdomains(ctx context.Context, domain string) ([]string, error) {
	// Build command with arguments
	args := []string{
		"-d", domain,
		"-silent",
		"-t", fmt.Sprintf("%d", s.config.Threads),
		"-timeout", fmt.Sprintf("%d", s.config.Timeout),
		"-max-time", fmt.Sprintf("%d", s.config.MaxEnumerationTime),
		"-all", // Use all sources
	}

	cmd := exec.CommandContext(ctx, "subfinder", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Check if it's just a "not found" error
		if exitErr, ok := err.(*exec.ExitError); ok {
			// subfinder exits with non-zero on some errors but may still have output
			if stdout.Len() == 0 {
				return nil, fmt.Errorf("subfinder failed: %v, stderr: %s", exitErr, stderr.String())
			}
		} else if err == exec.ErrNotFound {
			return nil, fmt.Errorf("subfinder not found in PATH - please install it: go install -v github.com/projectdiscovery/subfinder/v2/cmd/subfinder@latest")
		} else {
			return nil, fmt.Errorf("subfinder error: %w", err)
		}
	}

	// Parse results from stdout (newline-separated)
	var subdomains []string
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		subdomain := strings.TrimSpace(scanner.Text())
		if subdomain != "" && subdomain != domain {
			subdomains = append(subdomains, subdomain)
		}
	}

	return subdomains, scanner.Err()
}

// IsAvailable checks if subfinder is installed and available.
func IsSubfinderAvailable() bool {
	_, err := exec.LookPath("subfinder")
	return err == nil
}

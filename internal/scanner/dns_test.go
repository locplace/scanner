package scanner

import (
	"testing"
	"time"
)

func TestDefaultDNSConfig(t *testing.T) {
	config := DefaultDNSConfig()

	// Verify nameservers are set
	if len(config.Nameservers) == 0 {
		t.Error("DefaultDNSConfig() returned empty nameservers")
	}

	// Verify we have well-known DNS servers
	expectedServers := map[string]bool{
		"8.8.8.8": false, // Google
		"1.1.1.1": false, // Cloudflare
		"9.9.9.9": false, // Quad9
	}

	for _, ns := range config.Nameservers {
		if _, ok := expectedServers[ns]; ok {
			expectedServers[ns] = true
		}
	}

	for ns, found := range expectedServers {
		if !found {
			t.Errorf("DefaultDNSConfig() missing expected nameserver %s", ns)
		}
	}

	// Verify timeout is reasonable (not zero, not too long)
	if config.Timeout == 0 {
		t.Error("DefaultDNSConfig() timeout is zero")
	}
	if config.Timeout > 30*time.Second {
		t.Errorf("DefaultDNSConfig() timeout %v seems too long", config.Timeout)
	}

	// Verify workers count is reasonable
	if config.Workers <= 0 {
		t.Errorf("DefaultDNSConfig() workers %d should be positive", config.Workers)
	}
	if config.Workers > 100 {
		t.Errorf("DefaultDNSConfig() workers %d seems too high", config.Workers)
	}
}

func TestNewDNSScanner(t *testing.T) {
	config := DNSConfig{
		Nameservers: []string{"8.8.8.8"},
		Timeout:     5 * time.Second,
		Workers:     5,
	}

	scanner := NewDNSScanner(config)
	if scanner == nil {
		t.Fatal("NewDNSScanner() returned nil")
	}

	if len(scanner.config.Nameservers) != 1 {
		t.Errorf("scanner nameservers count = %d, want 1", len(scanner.config.Nameservers))
	}
	if scanner.config.Nameservers[0] != "8.8.8.8" {
		t.Errorf("scanner nameserver = %q, want %q", scanner.config.Nameservers[0], "8.8.8.8")
	}
	if scanner.config.Timeout != 5*time.Second {
		t.Errorf("scanner timeout = %v, want %v", scanner.config.Timeout, 5*time.Second)
	}
	if scanner.config.Workers != 5 {
		t.Errorf("scanner workers = %d, want %d", scanner.config.Workers, 5)
	}
}

func TestLOCResult_Fields(t *testing.T) {
	// Test that LOCResult struct can hold all expected data
	result := LOCResult{
		FQDN:      "example.com",
		HasLOC:    true,
		RawRecord: "52 22 23.000 N 4 53 32.000 E -2.00m 1m 10000m 10m",
		Error:     nil,
	}

	if result.FQDN != "example.com" {
		t.Errorf("FQDN = %q, want %q", result.FQDN, "example.com")
	}
	if !result.HasLOC {
		t.Error("HasLOC should be true")
	}
	if result.RawRecord == "" {
		t.Error("RawRecord should not be empty")
	}
	if result.Error != nil {
		t.Errorf("Error should be nil, got %v", result.Error)
	}
}

func TestDNSConfig_ZeroValues(t *testing.T) {
	// Test that zero-value DNSConfig is usable (even if not ideal)
	config := DNSConfig{}

	if config.Timeout != 0 {
		t.Errorf("Zero-value Timeout = %v, want 0", config.Timeout)
	}
	if config.Workers != 0 {
		t.Errorf("Zero-value Workers = %d, want 0", config.Workers)
	}
	if len(config.Nameservers) != 0 {
		t.Errorf("Zero-value Nameservers = %v, want empty", config.Nameservers)
	}
}

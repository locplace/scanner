package scanner

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// DNSConfig holds configuration for DNS lookups.
type DNSConfig struct {
	// Nameservers to use for lookups.
	Nameservers []string
	// Timeout for each DNS query.
	Timeout time.Duration
	// Workers is the number of concurrent DNS resolvers.
	Workers int
}

// DefaultDNSConfig returns the default DNS configuration.
func DefaultDNSConfig() DNSConfig {
	return DNSConfig{
		Nameservers: []string{"8.8.8.8", "1.1.1.1", "9.9.9.9"},
		Timeout:     5 * time.Second,
		Workers:     10,
	}
}

// DNSScanner performs DNS LOC record lookups.
type DNSScanner struct {
	config DNSConfig
}

// NewDNSScanner creates a new DNS scanner.
func NewDNSScanner(config DNSConfig) *DNSScanner {
	return &DNSScanner{config: config}
}

// LOCResult represents the result of a LOC lookup.
type LOCResult struct {
	FQDN      string
	HasLOC    bool
	RawRecord string
	Error     error
}

// LookupLOC performs a LOC record lookup for a single domain using raw UDP sockets.
func (s *DNSScanner) LookupLOC(ctx context.Context, fqdn string) LOCResult {
	result := LOCResult{FQDN: fqdn}

	// Ensure FQDN is fully qualified
	if fqdn[len(fqdn)-1] != '.' {
		fqdn = fqdn + "."
	}

	// Build DNS query message
	msg := new(dns.Msg)
	msg.SetQuestion(fqdn, dns.TypeLOC)
	msg.RecursionDesired = true

	// Serialize the query
	queryBytes, err := msg.Pack()
	if err != nil {
		result.Error = fmt.Errorf("failed to pack DNS query: %w", err)
		return result
	}

	// Try each nameserver until we get a response
	var lastErr error
	for _, ns := range s.config.Nameservers {
		resp, err := s.sendUDPQuery(ctx, ns, queryBytes)
		if err != nil {
			lastErr = err
			continue
		}

		// Parse the response
		responseMsg := new(dns.Msg)
		if err := responseMsg.Unpack(resp); err != nil {
			lastErr = fmt.Errorf("failed to unpack DNS response: %w", err)
			continue
		}

		// Check for valid response
		if responseMsg.Rcode != dns.RcodeSuccess {
			// NXDOMAIN or other error, try next server or return no LOC
			if responseMsg.Rcode == dns.RcodeNameError {
				return result // Domain doesn't exist, no LOC
			}
			lastErr = fmt.Errorf("DNS error: %s", dns.RcodeToString[responseMsg.Rcode])
			continue
		}

		// Look for LOC records in the answer section
		for _, rr := range responseMsg.Answer {
			if loc, ok := rr.(*dns.LOC); ok {
				result.HasLOC = true
				result.RawRecord = formatLOCRecord(loc)
				return result
			}
		}

		// No LOC record found, but query was successful
		return result
	}

	if lastErr != nil {
		result.Error = lastErr
	}
	return result
}

// sendUDPQuery sends a DNS query over UDP and returns the response.
func (s *DNSScanner) sendUDPQuery(ctx context.Context, nameserver string, query []byte) ([]byte, error) {
	// Create UDP address
	addr, err := net.ResolveUDPAddr("udp", nameserver+":53")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve nameserver address: %w", err)
	}

	// Create UDP connection
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, fmt.Errorf("failed to create UDP connection: %w", err)
	}
	defer conn.Close()

	// Set deadline based on timeout and context
	deadline := time.Now().Add(s.config.Timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	conn.SetDeadline(deadline)

	// Send the query
	_, err = conn.Write(query)
	if err != nil {
		return nil, fmt.Errorf("failed to send DNS query: %w", err)
	}

	// Read the response (DNS over UDP max size is typically 512 bytes, but EDNS can extend this)
	response := make([]byte, 4096)
	n, err := conn.Read(response)
	if err != nil {
		// Check if context was cancelled
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			return nil, fmt.Errorf("failed to read DNS response: %w", err)
		}
	}

	return response[:n], nil
}

// formatLOCRecord formats a LOC record into a human-readable string.
func formatLOCRecord(loc *dns.LOC) string {
	// Convert latitude
	lat := float64(loc.Latitude-1<<31) / 3600000.0
	latDir := "N"
	if lat < 0 {
		lat = -lat
		latDir = "S"
	}
	latDeg := int(lat)
	latMin := int((lat - float64(latDeg)) * 60)
	latSec := (lat - float64(latDeg) - float64(latMin)/60.0) * 3600

	// Convert longitude
	lon := float64(loc.Longitude-1<<31) / 3600000.0
	lonDir := "E"
	if lon < 0 {
		lon = -lon
		lonDir = "W"
	}
	lonDeg := int(lon)
	lonMin := int((lon - float64(lonDeg)) * 60)
	lonSec := (lon - float64(lonDeg) - float64(lonMin)/60.0) * 3600

	// Convert altitude (centimeters relative to reference ellipsoid, with 100000m offset)
	alt := float64(loc.Altitude)/100.0 - 100000.0

	// Convert size, horizontal precision, and vertical precision from RFC 1876 format
	size := locSizeToMeters(loc.Size)
	horizPrec := locSizeToMeters(loc.HorizPre)
	vertPrec := locSizeToMeters(loc.VertPre)

	return fmt.Sprintf("%d %d %.3f %s %d %d %.3f %s %.2fm %.0fm %.0fm %.0fm",
		latDeg, latMin, latSec, latDir,
		lonDeg, lonMin, lonSec, lonDir,
		alt, size, horizPrec, vertPrec)
}

// locSizeToMeters converts the RFC 1876 size/precision encoding to meters.
func locSizeToMeters(val uint8) float64 {
	mantissa := float64(val >> 4)
	exponent := val & 0x0F
	return mantissa * float64(pow10(int(exponent))) / 100.0
}

// pow10 returns 10^n for small non-negative n.
func pow10(n int) int {
	result := 1
	for i := 0; i < n; i++ {
		result *= 10
	}
	return result
}

// LookupLOCBatch performs LOC lookups for multiple domains concurrently.
func (s *DNSScanner) LookupLOCBatch(ctx context.Context, fqdns []string) []LOCResult {
	results := make([]LOCResult, len(fqdns))
	var wg sync.WaitGroup
	var mu sync.Mutex
	resultIdx := 0

	// Create a semaphore channel to limit concurrency
	sem := make(chan struct{}, s.config.Workers)

	for _, fqdn := range fqdns {
		wg.Add(1)
		go func(domain string) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				mu.Lock()
				results[resultIdx] = LOCResult{FQDN: domain, Error: ctx.Err()}
				resultIdx++
				mu.Unlock()
				return
			}

			result := s.LookupLOC(ctx, domain)

			mu.Lock()
			results[resultIdx] = result
			resultIdx++
			mu.Unlock()
		}(fqdn)
	}

	wg.Wait()
	return results[:resultIdx]
}

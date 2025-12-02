package scanner

import (
	"context"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/zmap/zdns/v2/src/zdns"
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
	config       DNSConfig
	resolverPool chan *zdns.Resolver
	poolSize     int
	initOnce     sync.Once
	initErr      error
	mu           sync.Mutex
}

// NewDNSScanner creates a new DNS scanner.
func NewDNSScanner(config DNSConfig) *DNSScanner {
	// Pool size matches worker count to ensure each concurrent lookup can get a resolver
	poolSize := config.Workers
	if poolSize < 1 {
		poolSize = 10
	}
	return &DNSScanner{
		config:       config,
		resolverPool: make(chan *zdns.Resolver, poolSize),
		poolSize:     poolSize,
	}
}

// initPool initializes the resolver pool (called once lazily)
func (s *DNSScanner) initPool() error {
	s.initOnce.Do(func() {
		for i := 0; i < s.poolSize; i++ {
			resolver, err := s.createResolver()
			if err != nil {
				s.initErr = err
				return
			}
			s.resolverPool <- resolver
		}
	})
	return s.initErr
}

// createResolver creates a new zdns resolver instance
func (s *DNSScanner) createResolver() (*zdns.Resolver, error) {
	// Build nameserver list
	nameservers := make([]zdns.NameServer, len(s.config.Nameservers))
	for i, ns := range s.config.Nameservers {
		nameservers[i] = zdns.NameServer{
			IP:   net.ParseIP(ns),
			Port: 53,
		}
	}

	// Create resolver config
	config := zdns.NewResolverConfig()
	config.ExternalNameServersV4 = nameservers
	config.Timeout = s.config.Timeout
	config.IPVersionMode = zdns.IPv4Only

	return zdns.InitResolver(config)
}

// getResolver borrows a resolver from the pool
func (s *DNSScanner) getResolver() (*zdns.Resolver, error) {
	if err := s.initPool(); err != nil {
		return nil, err
	}
	return <-s.resolverPool, nil
}

// returnResolver returns a resolver to the pool
func (s *DNSScanner) returnResolver(r *zdns.Resolver) {
	select {
	case s.resolverPool <- r:
	default:
		// Pool is full, close the resolver
		r.Close()
	}
}

// Close releases any resources held by the scanner.
func (s *DNSScanner) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Drain and close all resolvers in the pool
	close(s.resolverPool)
	for resolver := range s.resolverPool {
		resolver.Close()
	}
	return nil
}

// LOCResult represents the result of a LOC lookup.
type LOCResult struct {
	FQDN      string
	HasLOC    bool
	RawRecord string
	Error     error
}

// LookupLOC performs a LOC record lookup for a single domain.
func (s *DNSScanner) LookupLOC(ctx context.Context, fqdn string) LOCResult {
	result := LOCResult{FQDN: fqdn}

	// Sanitize input: strip trailing dot to prevent zdns fatal error
	// ("name already has trailing dot")
	if strings.HasSuffix(fqdn, ".") {
		log.Printf("Warning: domain %q has trailing dot, stripping", fqdn)
		fqdn = strings.TrimSuffix(fqdn, ".")
		result.FQDN = fqdn
	}

	// Borrow resolver from pool
	resolver, err := s.getResolver()
	if err != nil {
		result.Error = err
		return result
	}
	defer s.returnResolver(resolver)

	// Create LOC query
	question := &zdns.Question{
		Type:  dns.TypeLOC,
		Class: dns.ClassINET,
		Name:  fqdn,
	}

	// Perform lookup
	queryResult, _, status, err := resolver.ExternalLookup(ctx, question, nil)
	if err != nil {
		result.Error = err
		return result
	}

	// Check status
	if status != zdns.StatusNoError {
		return result // No LOC record, not an error
	}

	// Check for LOC answers
	if queryResult != nil && queryResult.Answers != nil {
		for _, answer := range queryResult.Answers {
			// zdns returns value types, not pointers
			if locAnswer, ok := answer.(zdns.LOCAnswer); ok {
				result.HasLOC = true
				result.RawRecord = locAnswer.Coordinates
				return result
			}
		}
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

package scanner

import (
	"sync"
)

// DomainTracker tracks which domains are currently being scanned.
// It is safe for concurrent use by multiple goroutines.
type DomainTracker struct {
	mu      sync.RWMutex
	domains map[string]struct{}
}

// NewDomainTracker creates a new domain tracker.
func NewDomainTracker() *DomainTracker {
	return &DomainTracker{
		domains: make(map[string]struct{}),
	}
}

// Add registers domains as being actively scanned.
func (t *DomainTracker) Add(domains ...string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, d := range domains {
		t.domains[d] = struct{}{}
	}
}

// Remove unregisters domains from active scanning.
func (t *DomainTracker) Remove(domains ...string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, d := range domains {
		delete(t.domains, d)
	}
}

// List returns a copy of all currently tracked domains.
func (t *DomainTracker) List() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]string, 0, len(t.domains))
	for d := range t.domains {
		result = append(result, d)
	}
	return result
}

// Count returns the number of tracked domains.
func (t *DomainTracker) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.domains)
}

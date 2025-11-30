package scanner

import (
	"sort"
	"sync"
	"testing"
)

func TestNewDomainTracker(t *testing.T) {
	tracker := NewDomainTracker()
	if tracker == nil {
		t.Fatal("NewDomainTracker() returned nil")
	}
	if tracker.Count() != 0 {
		t.Errorf("New tracker should have count 0, got %d", tracker.Count())
	}
	if len(tracker.List()) != 0 {
		t.Errorf("New tracker should have empty list, got %v", tracker.List())
	}
}

func TestDomainTracker_Add(t *testing.T) {
	tests := []struct {
		name      string
		domains   []string
		wantCount int
	}{
		{
			name:      "add single domain",
			domains:   []string{"example.com"},
			wantCount: 1,
		},
		{
			name:      "add multiple domains",
			domains:   []string{"a.com", "b.com", "c.com"},
			wantCount: 3,
		},
		{
			name:      "add duplicate domains",
			domains:   []string{"example.com", "example.com", "example.com"},
			wantCount: 1, // Duplicates should be collapsed
		},
		{
			name:      "add empty list",
			domains:   []string{},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := NewDomainTracker()
			tracker.Add(tt.domains...)

			if got := tracker.Count(); got != tt.wantCount {
				t.Errorf("Count() = %d, want %d", got, tt.wantCount)
			}
		})
	}
}

func TestDomainTracker_Remove(t *testing.T) {
	tests := []struct {
		name           string
		initialDomains []string
		removeDomains  []string
		wantCount      int
		wantList       []string
	}{
		{
			name:           "remove existing domain",
			initialDomains: []string{"a.com", "b.com", "c.com"},
			removeDomains:  []string{"b.com"},
			wantCount:      2,
			wantList:       []string{"a.com", "c.com"},
		},
		{
			name:           "remove non-existing domain",
			initialDomains: []string{"a.com", "b.com"},
			removeDomains:  []string{"nonexistent.com"},
			wantCount:      2,
			wantList:       []string{"a.com", "b.com"},
		},
		{
			name:           "remove all domains",
			initialDomains: []string{"a.com", "b.com"},
			removeDomains:  []string{"a.com", "b.com"},
			wantCount:      0,
			wantList:       []string{},
		},
		{
			name:           "remove from empty tracker",
			initialDomains: []string{},
			removeDomains:  []string{"example.com"},
			wantCount:      0,
			wantList:       []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := NewDomainTracker()
			tracker.Add(tt.initialDomains...)
			tracker.Remove(tt.removeDomains...)

			if got := tracker.Count(); got != tt.wantCount {
				t.Errorf("Count() = %d, want %d", got, tt.wantCount)
			}

			got := tracker.List()
			sort.Strings(got)
			sort.Strings(tt.wantList)

			if len(got) != len(tt.wantList) {
				t.Errorf("List() = %v, want %v", got, tt.wantList)
				return
			}

			for i := range got {
				if got[i] != tt.wantList[i] {
					t.Errorf("List() = %v, want %v", got, tt.wantList)
					break
				}
			}
		})
	}
}

func TestDomainTracker_List(t *testing.T) {
	tracker := NewDomainTracker()
	domains := []string{"example.com", "test.org", "demo.net"}
	tracker.Add(domains...)

	list := tracker.List()

	// Verify all domains are present
	if len(list) != len(domains) {
		t.Errorf("List() returned %d items, want %d", len(list), len(domains))
	}

	// Verify List returns a copy, not the internal map reference
	// by modifying the returned slice and checking the tracker is unaffected
	list[0] = "modified.com"
	newList := tracker.List()

	foundModified := false
	for _, d := range newList {
		if d == "modified.com" {
			foundModified = true
			break
		}
	}
	if foundModified {
		t.Error("List() should return a copy, but modification affected internal state")
	}
}

func TestDomainTracker_Concurrent(t *testing.T) {
	tracker := NewDomainTracker()
	const numGoroutines = 100
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3) // 3 types of operations

	// Concurrent adds
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				tracker.Add("domain.com")
			}
		}()
	}

	// Concurrent removes
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				tracker.Remove("domain.com")
			}
		}()
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				_ = tracker.Count()
				_ = tracker.List()
			}
		}()
	}

	wg.Wait()

	// If we get here without panics or deadlocks, the test passes
	// The final count is indeterminate due to concurrent add/remove
	t.Logf("Final count after concurrent operations: %d", tracker.Count())
}

func TestDomainTracker_AddRemoveSequence(t *testing.T) {
	tracker := NewDomainTracker()

	// Add domains
	tracker.Add("a.com", "b.com", "c.com")
	if tracker.Count() != 3 {
		t.Errorf("After Add, Count() = %d, want 3", tracker.Count())
	}

	// Remove one
	tracker.Remove("b.com")
	if tracker.Count() != 2 {
		t.Errorf("After Remove, Count() = %d, want 2", tracker.Count())
	}

	// Add more including duplicate
	tracker.Add("d.com", "a.com") // a.com is duplicate
	if tracker.Count() != 3 {
		t.Errorf("After second Add, Count() = %d, want 3", tracker.Count())
	}

	// Remove multiple
	tracker.Remove("a.com", "c.com", "d.com")
	if tracker.Count() != 0 {
		t.Errorf("After final Remove, Count() = %d, want 0", tracker.Count())
	}
}

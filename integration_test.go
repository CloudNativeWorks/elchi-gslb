package elchi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
)

// TestIntegration_FullLifecycle tests the complete plugin lifecycle.
func TestIntegration_FullLifecycle(t *testing.T) {
	// Setup mock controller
	controller := newMockController()
	controller.setSnapshot(&DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "v1",
		Records: []DNSRecord{
			{
				Name: "test.gslb.elchi",
				Type: "A",
				TTL:  300,
				IPs:  []string{"192.168.1.10", "192.168.1.11"},
			},
		},
	})

	server := httptest.NewServer(controller)
	defer server.Close()

	// Create plugin instance
	e := &Elchi{
		Zone:         "gslb.elchi.",
		Endpoint:     server.URL,
		Secret:       "test-secret",
		cache:        NewRecordCache("gslb.elchi."),
		client:       NewElchiClient(server.URL, "gslb.elchi.", "test-secret", 10*time.Second),
		SyncInterval: 100 * time.Millisecond,
		TTL:          300,
		Next:         test.NextHandler(dns.RcodeSuccess, nil),
	}

	// Fetch initial snapshot using client
	ctx := context.Background()
	snapshot, err := e.client.FetchSnapshot(ctx)
	if err != nil {
		t.Fatalf("Failed to fetch initial snapshot: %v", err)
	}

	if err := e.cache.ReplaceFromSnapshot(snapshot, e.TTL); err != nil {
		t.Fatalf("Failed to load snapshot into cache: %v", err)
	}

	// Test 1: Query should return cached records
	m := new(dns.Msg)
	m.SetQuestion("test.gslb.elchi.", dns.TypeA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	code, err := e.ServeDNS(context.Background(), rec, m)

	if err != nil {
		t.Fatalf("ServeDNS failed: %v", err)
	}

	if code != dns.RcodeSuccess {
		t.Errorf("Expected RcodeSuccess, got %d", code)
	}

	if len(rec.Msg.Answer) != 2 {
		t.Fatalf("Expected 2 answers, got %d", len(rec.Msg.Answer))
	}

	// Verify first answer
	aRecord, ok := rec.Msg.Answer[0].(*dns.A)
	if !ok {
		t.Fatal("Expected A record")
	}

	if aRecord.A.String() != "192.168.1.10" {
		t.Errorf("Expected IP 192.168.1.10, got %s", aRecord.A.String())
	}

	// Test 2: Update snapshot
	controller.setSnapshot(&DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "v2",
		Records: []DNSRecord{
			{
				Name: "test.gslb.elchi",
				Type: "A",
				TTL:  300,
				IPs:  []string{"192.168.2.20"},
			},
		},
	})

	// Manually fetch changes
	changes, err := e.client.CheckChanges(ctx, "v1")
	if err != nil {
		t.Fatalf("Failed to fetch changes: %v", err)
	}

	if changes.Unchanged {
		t.Error("Expected changes but got unchanged response")
	}

	// Update cache
	if err := e.cache.ReplaceFromSnapshot(&DNSSnapshot{
		Zone:        changes.Zone,
		VersionHash: changes.VersionHash,
		Records:     changes.Records,
	}, e.TTL); err != nil {
		t.Fatalf("Failed to update cache: %v", err)
	}

	// Query again
	rec2 := dnstest.NewRecorder(&test.ResponseWriter{})
	_, err = e.ServeDNS(context.Background(), rec2, m)

	if err != nil {
		t.Fatalf("ServeDNS failed: %v", err)
	}

	if len(rec2.Msg.Answer) != 1 {
		t.Fatalf("Expected 1 answer after update, got %d", len(rec2.Msg.Answer))
	}

	aRecord2, ok := rec2.Msg.Answer[0].(*dns.A)
	if !ok {
		t.Fatal("Expected A record")
	}

	if aRecord2.A.String() != "192.168.2.20" {
		t.Errorf("Expected IP 192.168.2.20 after update, got %s", aRecord2.A.String())
	}

	// Test 3: Query for non-existent record should return NXDOMAIN
	m3 := new(dns.Msg)
	m3.SetQuestion("nonexistent.gslb.elchi.", dns.TypeA)

	rec3 := dnstest.NewRecorder(&test.ResponseWriter{})
	code, err = e.ServeDNS(context.Background(), rec3, m3)

	if err != nil {
		t.Fatalf("ServeDNS failed: %v", err)
	}

	if code != dns.RcodeNameError {
		t.Errorf("Expected RcodeNameError for non-existent record, got %d", code)
	}
}

// TestIntegration_ErrorRecovery tests error handling.
func TestIntegration_ErrorRecovery(t *testing.T) {
	// Setup mock controller that returns errors
	controller := newMockController()
	controller.setSnapshot(&DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "v1",
		Records: []DNSRecord{
			{
				Name: "test.gslb.elchi",
				Type: "A",
				TTL:  300,
				IPs:  []string{"192.168.1.10"},
			},
		},
	})

	server := httptest.NewServer(controller)
	defer server.Close()

	// Create plugin instance
	e := &Elchi{
		Zone:     "gslb.elchi.",
		Endpoint: server.URL,
		Secret:   "test-secret",
		cache:    NewRecordCache("gslb.elchi."),
		client:   NewElchiClient(server.URL, "gslb.elchi.", "test-secret", 10*time.Second),
		TTL:      300,
		Next:     test.NextHandler(dns.RcodeSuccess, nil),
	}

	// Fetch initial snapshot
	ctx := context.Background()
	snapshot, err := e.client.FetchSnapshot(ctx)
	if err != nil {
		t.Fatalf("Failed to fetch initial snapshot: %v", err)
	}

	if err := e.cache.ReplaceFromSnapshot(snapshot, e.TTL); err != nil {
		t.Fatalf("Failed to load snapshot: %v", err)
	}

	// Verify initial data is cached
	m := new(dns.Msg)
	m.SetQuestion("test.gslb.elchi.", dns.TypeA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	code, err := e.ServeDNS(context.Background(), rec, m)

	if err != nil {
		t.Fatalf("ServeDNS failed: %v", err)
	}

	if code != dns.RcodeSuccess {
		t.Errorf("Expected RcodeSuccess, got %d", code)
	}

	// Make controller return errors
	controller.setError(true)

	// Try to fetch - should fail
	_, err = e.client.CheckChanges(ctx, "v1")
	if err == nil {
		t.Error("Expected error from controller but got nil")
	}

	// Verify plugin still serves from stale cache
	rec2 := dnstest.NewRecorder(&test.ResponseWriter{})
	code, err = e.ServeDNS(context.Background(), rec2, m)

	if err != nil {
		t.Fatalf("ServeDNS failed during controller error: %v", err)
	}

	if code != dns.RcodeSuccess {
		t.Errorf("Expected RcodeSuccess with stale cache, got %d", code)
	}

	if len(rec2.Msg.Answer) != 1 {
		t.Errorf("Expected 1 answer from stale cache, got %d", len(rec2.Msg.Answer))
	}

	// Restore controller
	controller.setError(false)
	controller.setSnapshot(&DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "v2",
		Records: []DNSRecord{
			{
				Name: "test.gslb.elchi",
				Type: "A",
				TTL:  300,
				IPs:  []string{"192.168.2.20"},
			},
		},
	})

	// Fetch new snapshot
	snapshot2, err := e.client.FetchSnapshot(ctx)
	if err != nil {
		t.Fatalf("Failed to fetch snapshot after recovery: %v", err)
	}

	if err := e.cache.ReplaceFromSnapshot(snapshot2, e.TTL); err != nil {
		t.Fatalf("Failed to update cache: %v", err)
	}

	// Verify plugin recovered and updated cache
	rec3 := dnstest.NewRecorder(&test.ResponseWriter{})
	_, err = e.ServeDNS(context.Background(), rec3, m)

	if err != nil {
		t.Fatalf("ServeDNS failed after recovery: %v", err)
	}

	if len(rec3.Msg.Answer) != 1 {
		t.Fatalf("Expected 1 answer after recovery, got %d", len(rec3.Msg.Answer))
	}

	aRecord, ok := rec3.Msg.Answer[0].(*dns.A)
	if !ok {
		t.Fatal("Expected A record")
	}

	if aRecord.A.String() != "192.168.2.20" {
		t.Errorf("Expected updated IP 192.168.2.20, got %s", aRecord.A.String())
	}
}

// TestIntegration_ConcurrentQueries tests concurrent DNS queries.
func TestIntegration_ConcurrentQueries(t *testing.T) {
	// Setup mock controller
	controller := newMockController()
	controller.setSnapshot(&DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "v1",
		Records: []DNSRecord{
			{
				Name: "test.gslb.elchi",
				Type: "A",
				TTL:  300,
				IPs:  []string{"192.168.1.10"},
			},
		},
	})

	server := httptest.NewServer(controller)
	defer server.Close()

	// Create plugin instance
	e := &Elchi{
		Zone:     "gslb.elchi.",
		Endpoint: server.URL,
		Secret:   "test-secret",
		cache:    NewRecordCache("gslb.elchi."),
		client:   NewElchiClient(server.URL, "gslb.elchi.", "test-secret", 10*time.Second),
		TTL:      300,
		Next:     test.NextHandler(dns.RcodeSuccess, nil),
	}

	// Fetch initial snapshot
	ctx := context.Background()
	snapshot, err := e.client.FetchSnapshot(ctx)
	if err != nil {
		t.Fatalf("Failed to fetch initial snapshot: %v", err)
	}

	if err := e.cache.ReplaceFromSnapshot(snapshot, e.TTL); err != nil {
		t.Fatalf("Failed to load snapshot: %v", err)
	}

	// Launch concurrent queries
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			m := new(dns.Msg)
			m.SetQuestion("test.gslb.elchi.", dns.TypeA)

			rec := dnstest.NewRecorder(&test.ResponseWriter{})
			code, err := e.ServeDNS(context.Background(), rec, m)

			if err != nil {
				errors <- fmt.Errorf("ServeDNS failed: %w", err)
				return
			}

			if code != dns.RcodeSuccess {
				errors <- fmt.Errorf("expected RcodeSuccess, got %d", code)
				return
			}

			if len(rec.Msg.Answer) != 1 {
				errors <- fmt.Errorf("expected 1 answer, got %d", len(rec.Msg.Answer))
				return
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Error(err)
	}
}

// TestIntegration_MultiZone tests behavior with queries for different zones.
func TestIntegration_MultiZone(t *testing.T) {
	// Setup mock controller
	controller := newMockController()
	controller.setSnapshot(&DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "v1",
		Records: []DNSRecord{
			{
				Name: "test.gslb.elchi",
				Type: "A",
				TTL:  300,
				IPs:  []string{"192.168.1.10"},
			},
		},
	})

	server := httptest.NewServer(controller)
	defer server.Close()

	// Create plugin instance
	e := &Elchi{
		Zone:     "gslb.elchi.",
		Endpoint: server.URL,
		Secret:   "test-secret",
		cache:    NewRecordCache("gslb.elchi."),
		client:   NewElchiClient(server.URL, "gslb.elchi.", "test-secret", 10*time.Second),
		TTL:      300,
		Next:     test.NextHandler(dns.RcodeSuccess, nil),
	}

	// Fetch initial snapshot
	ctx := context.Background()
	snapshot, err := e.client.FetchSnapshot(ctx)
	if err != nil {
		t.Fatalf("Failed to fetch initial snapshot: %v", err)
	}

	if err := e.cache.ReplaceFromSnapshot(snapshot, e.TTL); err != nil {
		t.Fatalf("Failed to load snapshot: %v", err)
	}

	// Test 1: Query for our zone - should be handled by plugin
	m1 := new(dns.Msg)
	m1.SetQuestion("test.gslb.elchi.", dns.TypeA)

	rec1 := dnstest.NewRecorder(&test.ResponseWriter{})
	code, err := e.ServeDNS(context.Background(), rec1, m1)

	if err != nil {
		t.Fatalf("ServeDNS failed: %v", err)
	}

	if code != dns.RcodeSuccess {
		t.Errorf("Expected RcodeSuccess for our zone, got %d", code)
	}

	// Test 2: Query for different zone - should delegate to Next
	m2 := new(dns.Msg)
	m2.SetQuestion("example.com.", dns.TypeA)

	rec2 := dnstest.NewRecorder(&test.ResponseWriter{})
	code, err = e.ServeDNS(context.Background(), rec2, m2)

	if err != nil {
		t.Fatalf("ServeDNS failed for external zone: %v", err)
	}

	// Should be handled by Next handler (returns RcodeSuccess)
	if code != dns.RcodeSuccess {
		t.Errorf("Expected delegation to Next for external zone, got %d", code)
	}
}

// TestIntegration_ChangesNotModified tests 304 Not Modified response.
func TestIntegration_ChangesNotModified(t *testing.T) {
	// Setup mock controller
	controller := newMockController()
	controller.setSnapshot(&DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "v1",
		Records:     []DNSRecord{},
	})

	server := httptest.NewServer(controller)
	defer server.Close()

	// Create client
	client := NewElchiClient(server.URL, "gslb.elchi.", "test-secret", 10*time.Second)

	// Fetch initial snapshot
	ctx := context.Background()
	snapshot, err := client.FetchSnapshot(ctx)
	if err != nil {
		t.Fatalf("Failed to fetch snapshot: %v", err)
	}

	// Fetch changes with same version - should return unchanged
	changes, err := client.CheckChanges(ctx, snapshot.VersionHash)
	if err != nil {
		t.Fatalf("CheckChanges failed: %v", err)
	}

	if !changes.Unchanged {
		t.Error("Expected unchanged=true for same version hash")
	}
}

// mockController is a test HTTP handler that simulates the Elchi controller.
type mockController struct {
	mu       sync.RWMutex
	snapshot *DNSSnapshot
	error    bool
}

func newMockController() *mockController {
	return &mockController{}
}

func (mc *mockController) setSnapshot(s *DNSSnapshot) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.snapshot = s
}

func (mc *mockController) setError(e bool) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.error = e
}

func (mc *mockController) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	// Check authentication
	if r.Header.Get("X-Elchi-Secret") != "test-secret" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Return error if configured
	if mc.error {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	switch r.URL.Path {
	case "/dns/snapshot":
		mc.handleSnapshot(w, r)
	case "/dns/changes":
		mc.handleChanges(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (mc *mockController) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	zone := r.URL.Query().Get("zone")
	if zone == "" {
		http.Error(w, "Missing zone parameter", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(mc.snapshot); err != nil {
		http.Error(w, "Failed to encode snapshot", http.StatusInternalServerError)
	}
}

func (mc *mockController) handleChanges(w http.ResponseWriter, r *http.Request) {
	zone := r.URL.Query().Get("zone")
	since := r.URL.Query().Get("since")

	if zone == "" || since == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	// If version matches, return 304
	if since == mc.snapshot.VersionHash {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Otherwise return full snapshot
	resp := &DNSChangesResponse{
		Unchanged:   false,
		Zone:        mc.snapshot.Zone,
		VersionHash: mc.snapshot.VersionHash,
		Records:     mc.snapshot.Records,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "Failed to encode changes", http.StatusInternalServerError)
	}
}

package elchi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleNotify_Success(t *testing.T) {
	// Create Elchi instance with cache
	e := &Elchi{
		Zone:   "gslb.elchi.",
		Secret: "test-secret",
		TTL:    300,
	}
	e.cache = NewRecordCache("gslb.elchi.")
	e.syncStatus = &SyncStatus{lastSyncStatus: "initial"}

	// Create webhook server
	ws := NewWebhookServer(e, ":8053")

	// Create test request
	notifyReq := NotifyRequest{
		Records: []DNSRecord{
			{
				Name: "test.gslb.elchi",
				Type: "A",
				TTL:  300,
				IPs:  []string{"192.168.1.10"},
			},
		},
	}

	body, _ := json.Marshal(notifyReq)
	req := httptest.NewRequest(http.MethodPost, "/notify", bytes.NewReader(body))
	req.Header.Set("X-Elchi-Secret", "test-secret")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()

	// Call handler
	ws.handleNotify(rr, req)

	// Check response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	var resp NotifyResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", resp.Status)
	}
	if resp.Updated != 1 {
		t.Errorf("Expected 1 updated, got %d", resp.Updated)
	}

	// Verify record was added to cache
	rrs := e.cache.Get("test.gslb.elchi.", 1) // TypeA = 1
	if len(rrs) != 1 {
		t.Errorf("Expected 1 record in cache, got %d", len(rrs))
	}
}

func TestHandleNotify_WithDeletes(t *testing.T) {
	// Create Elchi instance with pre-populated cache
	e := &Elchi{
		Zone:   "gslb.elchi.",
		Secret: "test-secret",
		TTL:    300,
	}
	e.cache = NewRecordCache("gslb.elchi.")
	e.syncStatus = &SyncStatus{lastSyncStatus: "initial"}

	// Pre-populate cache
	snapshot := &DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "test123",
		Records: []DNSRecord{
			{
				Name: "old.gslb.elchi",
				Type: "A",
				TTL:  300,
				IPs:  []string{"192.168.1.10"},
			},
		},
	}
	e.cache.ReplaceFromSnapshot(snapshot, 300)

	// Create webhook server
	ws := NewWebhookServer(e, ":8053")

	// Create notify request with deletes
	notifyReq := NotifyRequest{
		Deletes: []DeleteRecord{
			{
				Name: "old.gslb.elchi",
				Type: "A",
			},
		},
	}

	body, _ := json.Marshal(notifyReq)
	req := httptest.NewRequest(http.MethodPost, "/notify", bytes.NewReader(body))
	req.Header.Set("X-Elchi-Secret", "test-secret")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()

	// Call handler
	ws.handleNotify(rr, req)

	// Check response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	var resp NotifyResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp.Deleted != 1 {
		t.Errorf("Expected 1 deleted, got %d", resp.Deleted)
	}

	// Verify record was removed from cache
	rrs := e.cache.Get("old.gslb.elchi.", 1)
	if len(rrs) != 0 {
		t.Errorf("Expected 0 records in cache, got %d", len(rrs))
	}
}

func TestHandleNotify_Unauthorized(t *testing.T) {
	e := &Elchi{
		Zone:   "gslb.elchi.",
		Secret: "correct-secret",
		TTL:    300,
	}
	e.cache = NewRecordCache("gslb.elchi.")
	e.syncStatus = &SyncStatus{lastSyncStatus: "initial"}

	ws := NewWebhookServer(e, ":8053")

	// Request with wrong secret
	req := httptest.NewRequest(http.MethodPost, "/notify", bytes.NewReader([]byte("{}")))
	req.Header.Set("X-Elchi-Secret", "wrong-secret")

	rr := httptest.NewRecorder()

	// Call handler through middleware
	handler := ws.authMiddleware(ws.handleNotify)
	handler(rr, req)

	// Should return 401
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rr.Code)
	}
}

func TestHandleHealth_Healthy(t *testing.T) {
	e := &Elchi{
		Zone:         "gslb.elchi.",
		Secret:       "test-secret",
		TTL:          300,
		SyncInterval: 5 * time.Minute,
	}
	e.cache = NewRecordCache("gslb.elchi.")
	e.syncStatus = &SyncStatus{
		lastSyncTime:   time.Now(),
		lastSyncStatus: "success",
	}

	// Add some records
	snapshot := &DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "abc123",
		Records: []DNSRecord{
			{Name: "test.gslb.elchi", Type: "A", TTL: 300, IPs: []string{"192.168.1.10"}},
		},
	}
	e.cache.ReplaceFromSnapshot(snapshot, 300)

	ws := NewWebhookServer(e, ":8053")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	ws.handleHealth(rr, req)

	// Should return 200
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	var resp HealthResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp.Status != "healthy" {
		t.Errorf("Expected status 'healthy', got '%s'", resp.Status)
	}
	if resp.RecordsCount != 1 {
		t.Errorf("Expected 1 record, got %d", resp.RecordsCount)
	}
	if resp.VersionHash != "abc123" {
		t.Errorf("Expected hash 'abc123', got '%s'", resp.VersionHash)
	}
}

func TestHandleHealth_Degraded(t *testing.T) {
	e := &Elchi{
		Zone:         "gslb.elchi.",
		Secret:       "test-secret",
		TTL:          300,
		SyncInterval: 5 * time.Minute,
	}
	e.cache = NewRecordCache("gslb.elchi.")
	e.syncStatus = &SyncStatus{
		lastSyncTime:   time.Now().Add(-15 * time.Minute), // Old sync
		lastSyncStatus: "failed",
		lastError:      "connection timeout",
	}

	ws := NewWebhookServer(e, ":8053")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	ws.handleHealth(rr, req)

	// Should return 503
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", rr.Code)
	}

	var resp HealthResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp.Status != "degraded" {
		t.Errorf("Expected status 'degraded', got '%s'", resp.Status)
	}
	if resp.Error != "connection timeout" {
		t.Errorf("Expected error 'connection timeout', got '%s'", resp.Error)
	}
}

func TestHandleRecords_All(t *testing.T) {
	e := &Elchi{
		Zone:   "gslb.elchi.",
		Secret: "test-secret",
		TTL:    300,
	}
	e.cache = NewRecordCache("gslb.elchi.")
	e.syncStatus = &SyncStatus{lastSyncStatus: "initial"}

	// Add multiple records
	snapshot := &DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "test123",
		Records: []DNSRecord{
			{Name: "test1.gslb.elchi", Type: "A", TTL: 300, IPs: []string{"192.168.1.10"}},
			{Name: "test2.gslb.elchi", Type: "A", TTL: 300, IPs: []string{"192.168.1.11"}},
			{Name: "test3.gslb.elchi", Type: "AAAA", TTL: 300, IPs: []string{"2001:db8::1"}},
		},
	}
	e.cache.ReplaceFromSnapshot(snapshot, 300)

	ws := NewWebhookServer(e, ":8053")

	req := httptest.NewRequest(http.MethodGet, "/records", nil)
	req.Header.Set("X-Elchi-Secret", "test-secret")
	rr := httptest.NewRecorder()

	handler := ws.authMiddleware(ws.handleRecords)
	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	var resp RecordsResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp.Count != 3 {
		t.Errorf("Expected 3 records, got %d", resp.Count)
	}
	if resp.VersionHash != "test123" {
		t.Errorf("Expected hash 'test123', got '%s'", resp.VersionHash)
	}
}

func TestHandleRecords_Filtered(t *testing.T) {
	e := &Elchi{
		Zone:   "gslb.elchi.",
		Secret: "test-secret",
		TTL:    300,
	}
	e.cache = NewRecordCache("gslb.elchi.")
	e.syncStatus = &SyncStatus{lastSyncStatus: "initial"}

	// Add multiple records
	snapshot := &DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "test123",
		Records: []DNSRecord{
			{Name: "api.gslb.elchi", Type: "A", TTL: 300, IPs: []string{"192.168.1.10"}},
			{Name: "web.gslb.elchi", Type: "A", TTL: 300, IPs: []string{"192.168.1.11"}},
			{Name: "db.gslb.elchi", Type: "AAAA", TTL: 300, IPs: []string{"2001:db8::1"}},
		},
	}
	e.cache.ReplaceFromSnapshot(snapshot, 300)

	ws := NewWebhookServer(e, ":8053")

	// Filter by name
	req := httptest.NewRequest(http.MethodGet, "/records?name=api", nil)
	req.Header.Set("X-Elchi-Secret", "test-secret")
	rr := httptest.NewRecorder()

	handler := ws.authMiddleware(ws.handleRecords)
	handler(rr, req)

	var resp RecordsResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp.Count != 1 {
		t.Errorf("Expected 1 filtered record, got %d", resp.Count)
	}

	// Filter by type
	req2 := httptest.NewRequest(http.MethodGet, "/records?type=AAAA", nil)
	req2.Header.Set("X-Elchi-Secret", "test-secret")
	rr2 := httptest.NewRecorder()

	handler(rr2, req2)

	var resp2 RecordsResponse
	json.Unmarshal(rr2.Body.Bytes(), &resp2)

	if resp2.Count != 1 {
		t.Errorf("Expected 1 AAAA record, got %d", resp2.Count)
	}
}

func TestSyncStatus_ThreadSafety(t *testing.T) {
	ss := &SyncStatus{lastSyncStatus: "initial"}

	// Concurrent updates
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				if j%2 == 0 {
					ss.Update("success", nil)
				} else {
					ss.Update("failed", nil)
				}
				ss.Get()
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should complete without race condition
	t.Log("Thread safety test completed")
}

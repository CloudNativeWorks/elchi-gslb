package elchi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchSnapshot_Success(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.URL.Path != "/dns/snapshot" {
			t.Errorf("Expected path /dns/snapshot, got %s", r.URL.Path)
		}

		zone := r.URL.Query().Get("zone")
		if zone != "gslb.elchi" {
			t.Errorf("Expected zone gslb.elchi, got %s", zone)
		}

		// Verify auth headers
		if secret := r.Header.Get("X-Elchi-Secret"); secret != "test-secret" {
			t.Errorf("Expected secret test-secret, got %s", secret)
		}

		// Send response
		resp := DNSSnapshot{
			Zone:        "gslb.elchi",
			VersionHash: "abc123",
			Records: []DNSRecord{
				{
					Name: "test1.gslb.elchi",
					Type: "A",
					TTL:  300,
					IPs:  []string{"192.168.1.10", "192.168.1.11"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create client
	client := NewElchiClient(server.URL, "gslb.elchi", "test-secret", 5*time.Second, false)

	// Fetch snapshot
	ctx := context.Background()
	snapshot, err := client.FetchSnapshot(ctx)

	// Verify
	if err != nil {
		t.Fatalf("FetchSnapshot failed: %v", err)
	}
	if snapshot.Zone != "gslb.elchi" {
		t.Errorf("Expected zone gslb.elchi, got %s", snapshot.Zone)
	}
	if snapshot.VersionHash != "abc123" {
		t.Errorf("Expected hash abc123, got %s", snapshot.VersionHash)
	}
	if len(snapshot.Records) != 1 {
		t.Errorf("Expected 1 record, got %d", len(snapshot.Records))
	}
}

func TestFetchSnapshot_HTTPError(t *testing.T) {
	// Mock server returning 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal server error"))
	}))
	defer server.Close()

	client := NewElchiClient(server.URL, "gslb.elchi", "test-secret", 5*time.Second, false)

	ctx := context.Background()
	_, err := client.FetchSnapshot(ctx)

	if err == nil {
		t.Fatal("Expected error for HTTP 500, got nil")
	}
}

func TestFetchSnapshot_InvalidJSON(t *testing.T) {
	// Mock server returning invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := NewElchiClient(server.URL, "gslb.elchi", "test-secret", 5*time.Second, false)

	ctx := context.Background()
	_, err := client.FetchSnapshot(ctx)

	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestFetchSnapshot_Timeout(t *testing.T) {
	// Mock server with delay
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		json.NewEncoder(w).Encode(DNSSnapshot{})
	}))
	defer server.Close()

	client := NewElchiClient(server.URL, "gslb.elchi", "test-secret", 100*time.Millisecond, false)

	ctx := context.Background()
	_, err := client.FetchSnapshot(ctx)

	if err == nil {
		t.Fatal("Expected timeout error, got nil")
	}
}

func TestCheckChanges_Unchanged(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dns/changes" {
			t.Errorf("Expected path /dns/changes, got %s", r.URL.Path)
		}

		zone := r.URL.Query().Get("zone")
		since := r.URL.Query().Get("since")

		if zone != "gslb.elchi" {
			t.Errorf("Expected zone gslb.elchi, got %s", zone)
		}
		if since != "abc123" {
			t.Errorf("Expected since abc123, got %s", since)
		}

		// Return unchanged response
		resp := DNSChangesResponse{
			Unchanged: true,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewElchiClient(server.URL, "gslb.elchi", "test-secret", 5*time.Second, false)

	ctx := context.Background()
	changes, err := client.CheckChanges(ctx, "abc123")

	if err != nil {
		t.Fatalf("CheckChanges failed: %v", err)
	}
	if !changes.Unchanged {
		t.Error("Expected unchanged=true, got false")
	}
}

func TestCheckChanges_Changed(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return changed response with new snapshot
		resp := DNSChangesResponse{
			Unchanged:   false,
			Zone:        "gslb.elchi",
			VersionHash: "def456",
			Records: []DNSRecord{
				{
					Name: "test2.gslb.elchi",
					Type: "A",
					TTL:  300,
					IPs:  []string{"192.168.2.10"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewElchiClient(server.URL, "gslb.elchi", "test-secret", 5*time.Second, false)

	ctx := context.Background()
	changes, err := client.CheckChanges(ctx, "abc123")

	if err != nil {
		t.Fatalf("CheckChanges failed: %v", err)
	}
	if changes.Unchanged {
		t.Error("Expected unchanged=false, got true")
	}
	if changes.VersionHash != "def456" {
		t.Errorf("Expected hash def456, got %s", changes.VersionHash)
	}
	if len(changes.Records) != 1 {
		t.Errorf("Expected 1 record, got %d", len(changes.Records))
	}
}

func TestSignRequest(t *testing.T) {
	// Create a simple request handler to capture headers
	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header
		json.NewEncoder(w).Encode(DNSSnapshot{
			Zone:        "gslb.elchi",
			VersionHash: "test",
			Records:     []DNSRecord{},
		})
	}))
	defer server.Close()

	client := NewElchiClient(server.URL, "gslb.elchi", "my-secret-key", 5*time.Second, false)

	ctx := context.Background()
	_, _ = client.FetchSnapshot(ctx)

	// Verify headers
	if secret := capturedHeaders.Get("X-Elchi-Secret"); secret != "my-secret-key" {
		t.Errorf("Expected X-Elchi-Secret: my-secret-key, got %s", secret)
	}
	if accept := capturedHeaders.Get("Accept"); accept != "application/json" {
		t.Errorf("Expected Accept: application/json, got %s", accept)
	}
	// X-Elchi-Zone header removed - zone is sent via query parameter only
}

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// DNSSnapshot represents the full DNS snapshot response
type DNSSnapshot struct {
	Zone        string      `json:"zone"`
	VersionHash string      `json:"version_hash"`
	Records     []DNSRecord `json:"records"`
}

// DNSChangesResponse represents the response from the changes endpoint
type DNSChangesResponse struct {
	Unchanged   bool        `json:"unchanged"`
	Zone        string      `json:"zone,omitempty"`
	VersionHash string      `json:"version_hash,omitempty"`
	Records     []DNSRecord `json:"records,omitempty"`
}

// DNSRecord represents a single DNS record
type DNSRecord struct {
	Name string   `json:"name"`
	Type string   `json:"type"`
	TTL  uint32   `json:"ttl"`
	IPs  []string `json:"ips"`
}

var mockSnapshot = DNSSnapshot{
	Zone:        "gslb.elchi",
	VersionHash: "mock-v1-" + time.Now().Format("20060102150405"),
	Records: []DNSRecord{
		{
			Name: "listener1.gslb.elchi",
			Type: "A",
			TTL:  300,
			IPs:  []string{"192.168.1.10", "192.168.1.11"},
		},
		{
			Name: "listener2.gslb.elchi",
			Type: "A",
			TTL:  300,
			IPs:  []string{"192.168.2.20", "192.168.2.21"},
		},
		{
			Name: "listener3.gslb.elchi",
			Type: "AAAA",
			TTL:  600,
			IPs:  []string{"2001:db8::1", "2001:db8::2"},
		},
	},
}

func main() {
	http.HandleFunc("/dns/snapshot", handleSnapshot)
	http.HandleFunc("/dns/changes", handleChanges)
	http.HandleFunc("/health", handleHealth)

	addr := ":1052"
	fmt.Printf("Mock Elchi Controller starting on %s\n", addr)
	fmt.Printf("Endpoints:\n")
	fmt.Printf("   GET  %s/dns/snapshot?zone=gslb.elchi\n", addr)
	fmt.Printf("   GET  %s/dns/changes?zone=gslb.elchi&since=xyz\n", addr)
	fmt.Printf("   GET  %s/health\n", addr)
	fmt.Printf("\n")

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Failed to start mock controller: %v", err)
	}
}

func handleSnapshot(w http.ResponseWriter, r *http.Request) {
	// Check authentication
	secret := r.Header.Get("X-Elchi-Secret")
	if secret != "test-secret-key" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check zone parameter
	zone := r.URL.Query().Get("zone")
	if zone == "" {
		http.Error(w, "Missing zone parameter", http.StatusBadRequest)
		return
	}

	// Normalize zone (remove trailing dot if present)
	zone = strings.TrimSuffix(zone, ".")
	normalizedMockZone := strings.TrimSuffix(mockSnapshot.Zone, ".")

	if zone != normalizedMockZone {
		http.Error(w, "Unknown zone", http.StatusNotFound)
		return
	}

	log.Printf("Snapshot requested for zone: %s", zone)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(mockSnapshot); err != nil {
		log.Printf("Failed to encode snapshot: %v", err)
	}
}

func handleChanges(w http.ResponseWriter, r *http.Request) {
	// Check authentication
	secret := r.Header.Get("X-Elchi-Secret")
	if secret != "test-secret-key" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check zone parameter
	zone := r.URL.Query().Get("zone")
	if zone == "" {
		http.Error(w, "Missing zone parameter", http.StatusBadRequest)
		return
	}

	since := r.URL.Query().Get("since")
	if since == "" {
		http.Error(w, "Missing since parameter", http.StatusBadRequest)
		return
	}

	// Normalize zone (remove trailing dot if present)
	zone = strings.TrimSuffix(zone, ".")
	normalizedMockZone := strings.TrimSuffix(mockSnapshot.Zone, ".")

	if zone != normalizedMockZone {
		http.Error(w, "Unknown zone", http.StatusNotFound)
		return
	}

	log.Printf("Changes requested for zone: %s, since: %s", zone, since)

	// If version matches current, return 304 Not Modified
	if since == mockSnapshot.VersionHash {
		w.WriteHeader(http.StatusNotModified)
		log.Printf("No changes (304)")
		return
	}

	// Otherwise, return full snapshot
	resp := DNSChangesResponse{
		Unchanged:   false,
		Zone:        mockSnapshot.Zone,
		VersionHash: mockSnapshot.VersionHash,
		Records:     mockSnapshot.Records,
	}

	log.Printf("Changes found, returning new snapshot")

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Failed to encode changes: %v", err)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"status":        "healthy",
		"zone":          mockSnapshot.Zone,
		"records_count": len(mockSnapshot.Records),
		"version_hash":  mockSnapshot.VersionHash,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Failed to encode health: %v", err)
	}
}

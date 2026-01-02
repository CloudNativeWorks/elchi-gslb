package elchi

import (
	"sync"
	"testing"

	"github.com/miekg/dns"
)

func TestReplaceFromSnapshot(t *testing.T) {
	cache := NewRecordCache("gslb.elchi.")

	snapshot := &DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "abc123",
		Records: []DNSRecord{
			{
				Name: "test1.gslb.elchi",
				Type: "A",
				TTL:  300,
				IPs:  []string{"192.168.1.10", "192.168.1.11"},
			},
			{
				Name: "test2.gslb.elchi",
				Type: "AAAA",
				TTL:  600,
				IPs:  []string{"2001:db8::1"},
			},
		},
	}

	err := cache.ReplaceFromSnapshot(snapshot, 300)
	if err != nil {
		t.Fatalf("ReplaceFromSnapshot failed: %v", err)
	}

	// Verify version hash
	if hash := cache.GetVersionHash(); hash != "abc123" {
		t.Errorf("Expected hash abc123, got %s", hash)
	}

	// Verify domain count
	if count := cache.Count(); count != 2 {
		t.Errorf("Expected 2 domains, got %d", count)
	}
}

func TestGet_ARecords(t *testing.T) {
	cache := NewRecordCache("gslb.elchi.")

	snapshot := &DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "abc123",
		Records: []DNSRecord{
			{
				Name: "test.gslb.elchi",
				Type: "A",
				TTL:  300,
				IPs:  []string{"192.168.1.10", "192.168.1.11"},
			},
		},
	}

	cache.ReplaceFromSnapshot(snapshot, 300)

	// Query for A records
	rrs := cache.Get("test.gslb.elchi", dns.TypeA)

	if len(rrs) != 2 {
		t.Fatalf("Expected 2 A records, got %d", len(rrs))
	}

	// Verify first record
	aRecord, ok := rrs[0].(*dns.A)
	if !ok {
		t.Fatal("Expected dns.A record")
	}
	if aRecord.A.String() != "192.168.1.10" {
		t.Errorf("Expected IP 192.168.1.10, got %s", aRecord.A.String())
	}
	if aRecord.Hdr.Ttl != 300 {
		t.Errorf("Expected TTL 300, got %d", aRecord.Hdr.Ttl)
	}
}

func TestGet_AAAARecords(t *testing.T) {
	cache := NewRecordCache("gslb.elchi.")

	snapshot := &DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "abc123",
		Records: []DNSRecord{
			{
				Name: "test.gslb.elchi",
				Type: "AAAA",
				TTL:  600,
				IPs:  []string{"2001:db8::1", "2001:db8::2"},
			},
		},
	}

	cache.ReplaceFromSnapshot(snapshot, 300)

	// Query for AAAA records
	rrs := cache.Get("test.gslb.elchi", dns.TypeAAAA)

	if len(rrs) != 2 {
		t.Fatalf("Expected 2 AAAA records, got %d", len(rrs))
	}

	// Verify first record
	aaaaRecord, ok := rrs[0].(*dns.AAAA)
	if !ok {
		t.Fatal("Expected dns.AAAA record")
	}
	if aaaaRecord.AAAA.String() != "2001:db8::1" {
		t.Errorf("Expected IP 2001:db8::1, got %s", aaaaRecord.AAAA.String())
	}
	if aaaaRecord.Hdr.Ttl != 600 {
		t.Errorf("Expected TTL 600, got %d", aaaaRecord.Hdr.Ttl)
	}
}

func TestGet_NotFound(t *testing.T) {
	cache := NewRecordCache("gslb.elchi.")

	snapshot := &DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "abc123",
		Records:     []DNSRecord{},
	}

	cache.ReplaceFromSnapshot(snapshot, 300)

	// Query for non-existent domain
	rrs := cache.Get("nonexistent.gslb.elchi", dns.TypeA)

	if rrs != nil {
		t.Errorf("Expected nil for non-existent domain, got %d records", len(rrs))
	}
}

func TestGet_WrongType(t *testing.T) {
	cache := NewRecordCache("gslb.elchi.")

	snapshot := &DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "abc123",
		Records: []DNSRecord{
			{
				Name: "test.gslb.elchi",
				Type: "A",
				TTL:  300,
				IPs:  []string{"192.168.1.10"},
			},
		},
	}

	cache.ReplaceFromSnapshot(snapshot, 300)

	// Query for AAAA when only A exists
	rrs := cache.Get("test.gslb.elchi", dns.TypeAAAA)

	if rrs != nil {
		t.Errorf("Expected nil for wrong qtype, got %d records", len(rrs))
	}
}

func TestGetVersionHash(t *testing.T) {
	cache := NewRecordCache("gslb.elchi.")

	// Initially empty
	if hash := cache.GetVersionHash(); hash != "" {
		t.Errorf("Expected empty hash initially, got %s", hash)
	}

	// After snapshot
	snapshot := &DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "xyz789",
		Records:     []DNSRecord{},
	}

	cache.ReplaceFromSnapshot(snapshot, 300)

	if hash := cache.GetVersionHash(); hash != "xyz789" {
		t.Errorf("Expected hash xyz789, got %s", hash)
	}
}

func TestBuildDNSRecords_A(t *testing.T) {
	record := DNSRecord{
		Name: "test.example.com",
		Type: "A",
		TTL:  300,
		IPs:  []string{"192.168.1.10", "192.168.1.11"},
	}

	rrs, err := buildDNSRecords(record, 600)
	if err != nil {
		t.Fatalf("buildDNSRecords failed: %v", err)
	}

	if len(rrs) != 2 {
		t.Fatalf("Expected 2 RRs, got %d", len(rrs))
	}

	// Verify record uses its own TTL, not default
	if rrs[0].Header().Ttl != 300 {
		t.Errorf("Expected TTL 300, got %d", rrs[0].Header().Ttl)
	}
}

func TestBuildDNSRecords_DefaultTTL(t *testing.T) {
	record := DNSRecord{
		Name: "test.example.com",
		Type: "A",
		TTL:  0, // No TTL specified
		IPs:  []string{"192.168.1.10"},
	}

	rrs, err := buildDNSRecords(record, 600)
	if err != nil {
		t.Fatalf("buildDNSRecords failed: %v", err)
	}

	// Should use default TTL
	if rrs[0].Header().Ttl != 600 {
		t.Errorf("Expected default TTL 600, got %d", rrs[0].Header().Ttl)
	}
}

func TestBuildDNSRecords_InvalidIP(t *testing.T) {
	record := DNSRecord{
		Name: "test.example.com",
		Type: "A",
		TTL:  300,
		IPs:  []string{"invalid-ip", "192.168.1.10"},
	}

	rrs, err := buildDNSRecords(record, 300)

	// Should skip invalid IP but include valid one
	if err != nil {
		t.Fatalf("buildDNSRecords failed: %v", err)
	}
	if len(rrs) != 1 {
		t.Errorf("Expected 1 RR (skipped invalid), got %d", len(rrs))
	}
}

func TestBuildDNSRecords_UnsupportedType(t *testing.T) {
	record := DNSRecord{
		Name: "test.example.com",
		Type: "CNAME",
		TTL:  300,
		IPs:  []string{"192.168.1.10"},
	}

	_, err := buildDNSRecords(record, 300)
	if err == nil {
		t.Fatal("Expected error for unsupported type CNAME, got nil")
	}
}

func TestConcurrentAccess(t *testing.T) {
	cache := NewRecordCache("gslb.elchi.")

	// Initial snapshot
	snapshot := &DNSSnapshot{
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
	}
	cache.ReplaceFromSnapshot(snapshot, 300)

	// Simulate concurrent readers and writers
	var wg sync.WaitGroup

	// Multiple readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = cache.Get("test.gslb.elchi", dns.TypeA)
				_ = cache.GetVersionHash()
				_ = cache.Count()
			}
		}()
	}

	// Multiple writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				newSnapshot := &DNSSnapshot{
					Zone:        "gslb.elchi.",
					VersionHash: "v2",
					Records:     []DNSRecord{},
				}
				cache.ReplaceFromSnapshot(newSnapshot, 300)
			}
		}(i)
	}

	wg.Wait()

	// Verify cache is still functional
	hash := cache.GetVersionHash()
	if hash != "v2" && hash != "v1" {
		t.Errorf("Unexpected hash after concurrent access: %s", hash)
	}
}

func BenchmarkGet(b *testing.B) {
	cache := NewRecordCache("gslb.elchi.")

	snapshot := &DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "bench",
		Records: []DNSRecord{
			{
				Name: "test.gslb.elchi",
				Type: "A",
				TTL:  300,
				IPs:  []string{"192.168.1.10", "192.168.1.11", "192.168.1.12"},
			},
		},
	}
	cache.ReplaceFromSnapshot(snapshot, 300)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.Get("test.gslb.elchi", dns.TypeA)
	}
}

func BenchmarkReplaceFromSnapshot(b *testing.B) {
	cache := NewRecordCache("gslb.elchi.")

	snapshot := &DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "bench",
		Records: []DNSRecord{
			{
				Name: "test1.gslb.elchi",
				Type: "A",
				TTL:  300,
				IPs:  []string{"192.168.1.10"},
			},
			{
				Name: "test2.gslb.elchi",
				Type: "A",
				TTL:  300,
				IPs:  []string{"192.168.2.10"},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.ReplaceFromSnapshot(snapshot, 300)
	}
}

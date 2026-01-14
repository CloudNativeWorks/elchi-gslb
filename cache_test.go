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
	if count := cache.DomainCount(); count != 2 {
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
				_ = cache.DomainCount()
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

func TestBuildDNSRecords_CNAME_Failover(t *testing.T) {
	// When IPs is empty and failover is set, return CNAME
	record := DNSRecord{
		Name:     "abc.asya-gslb.elchi",
		Type:     "A",
		TTL:      20,
		IPs:      []string{}, // Empty IPs triggers failover
		Failover: "abc.avrupa-gslb.elchi",
	}

	rrs, err := buildDNSRecords(record, 300)
	if err != nil {
		t.Fatalf("buildDNSRecords failed: %v", err)
	}

	if len(rrs) != 1 {
		t.Fatalf("Expected 1 CNAME record, got %d", len(rrs))
	}

	cname, ok := rrs[0].(*dns.CNAME)
	if !ok {
		t.Fatal("Expected CNAME record type")
	}

	if cname.Target != "abc.avrupa-gslb.elchi." {
		t.Errorf("Expected CNAME target abc.avrupa-gslb.elchi., got %s", cname.Target)
	}

	if cname.Hdr.Ttl != 20 {
		t.Errorf("Expected TTL 20, got %d", cname.Hdr.Ttl)
	}
}

func TestBuildDNSRecords_EmptyIPs_NoFailover(t *testing.T) {
	// When IPs is empty and no failover, return nil (skip record)
	record := DNSRecord{
		Name:     "abc.asya-gslb.elchi",
		Type:     "A",
		TTL:      20,
		IPs:      []string{}, // Empty IPs
		Failover: "",         // No failover
	}

	rrs, err := buildDNSRecords(record, 300)
	if err != nil {
		t.Fatalf("buildDNSRecords should not return error: %v", err)
	}

	if rrs != nil {
		t.Errorf("Expected nil (skip record), got %d records", len(rrs))
	}
}

func TestBuildDNSRecords_WithIPs_IgnoresFailover(t *testing.T) {
	// When IPs is set, failover is ignored - return A records
	record := DNSRecord{
		Name:     "abc.asya-gslb.elchi",
		Type:     "A",
		TTL:      20,
		IPs:      []string{"10.10.1.20", "10.10.1.21"},
		Failover: "abc.avrupa-gslb.elchi", // Should be ignored
	}

	rrs, err := buildDNSRecords(record, 300)
	if err != nil {
		t.Fatalf("buildDNSRecords failed: %v", err)
	}

	if len(rrs) != 2 {
		t.Fatalf("Expected 2 A records, got %d", len(rrs))
	}

	// Verify they are A records, not CNAME
	for _, rr := range rrs {
		if _, ok := rr.(*dns.A); !ok {
			t.Errorf("Expected A record, got %T", rr)
		}
	}
}

func TestCache_CNAME_Failover_Integration(t *testing.T) {
	cache := NewRecordCache("gslb.elchi.")

	snapshot := &DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "v1",
		Records: []DNSRecord{
			{
				Name:     "service1.gslb.elchi",
				Type:     "A",
				TTL:      30,
				IPs:      []string{}, // Empty IPs triggers failover
				Failover: "service1-backup.gslb.elchi",
			},
			{
				Name: "service1-backup.gslb.elchi",
				Type: "A",
				TTL:  30,
				IPs:  []string{"192.168.2.10", "192.168.2.11"},
			},
		},
	}

	err := cache.ReplaceFromSnapshot(snapshot, 300)
	if err != nil {
		t.Fatalf("ReplaceFromSnapshot failed: %v", err)
	}

	// Query for service1 with type A should return CNAME
	rrs := cache.Get("service1.gslb.elchi.", dns.TypeCNAME)
	if len(rrs) != 1 {
		t.Fatalf("Expected 1 CNAME record for service1, got %d", len(rrs))
	}

	cname, ok := rrs[0].(*dns.CNAME)
	if !ok {
		t.Fatal("Expected CNAME record")
	}

	if cname.Target != "service1-backup.gslb.elchi." {
		t.Errorf("Expected CNAME to service1-backup.gslb.elchi., got %s", cname.Target)
	}

	// Query for backup service should return A records
	aRRs := cache.Get("service1-backup.gslb.elchi.", dns.TypeA)
	if len(aRRs) != 2 {
		t.Fatalf("Expected 2 A records for backup service, got %d", len(aRRs))
	}
}

func TestBuildDNSRecords_CNAME_EmptyIPs(t *testing.T) {
	// Test case: IPs empty but enabled=true should still return CNAME
	record := DNSRecord{
		Name:     "service.region1.elchi",
		Type:     "A",
		TTL:      30,
		IPs:      []string{}, // Empty IPs - no healthy endpoints
		Failover: "service.region2.elchi",
	}

	rrs, err := buildDNSRecords(record, 300)
	if err != nil {
		t.Fatalf("buildDNSRecords failed: %v", err)
	}

	if len(rrs) != 1 {
		t.Fatalf("Expected 1 CNAME record when IPs empty, got %d", len(rrs))
	}

	cname, ok := rrs[0].(*dns.CNAME)
	if !ok {
		t.Fatal("Expected CNAME record when IPs empty")
	}

	if cname.Target != "service.region2.elchi." {
		t.Errorf("Expected CNAME to service.region2.elchi., got %s", cname.Target)
	}

	if cname.Hdr.Ttl != 30 {
		t.Errorf("Expected TTL 30, got %d", cname.Hdr.Ttl)
	}
}

func TestBuildDNSRecords_CNAME_EmptyIPs_NoFailover(t *testing.T) {
	// Test case: IPs empty and no failover should return nil (skip record)
	record := DNSRecord{
		Name:     "service.region1.elchi",
		Type:     "A",
		TTL:      30,
		IPs:      []string{}, // Empty IPs
		Failover: "",         // No failover configured
	}

	rrs, err := buildDNSRecords(record, 300)
	if err != nil {
		t.Fatalf("buildDNSRecords should not return error: %v", err)
	}

	if rrs != nil {
		t.Errorf("Expected nil (skip record) when IPs empty and no failover, got %d records", len(rrs))
	}
}

func TestCache_CNAME_EmptyIPs_Integration(t *testing.T) {
	cache := NewRecordCache("gslb.elchi.")

	snapshot := &DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "v1",
		Records: []DNSRecord{
			{
				Name:     "api-asia.gslb.elchi",
				Type:     "A",
				TTL:      20,
				IPs:      []string{}, // No healthy endpoints in Asia
				Failover: "api-europe.gslb.elchi",
			},
			{
				Name: "api-europe.gslb.elchi",
				Type: "A",
				TTL:  20,
				IPs:  []string{"10.20.1.10", "10.20.1.11"},
			},
		},
	}

	err := cache.ReplaceFromSnapshot(snapshot, 300)
	if err != nil {
		t.Fatalf("ReplaceFromSnapshot failed: %v", err)
	}

	// Query for Asia service with empty IPs should return CNAME
	rrs := cache.Get("api-asia.gslb.elchi.", dns.TypeCNAME)
	if len(rrs) != 1 {
		t.Fatalf("Expected 1 CNAME record for api-asia.gslb.elchi, got %d", len(rrs))
	}

	cname, ok := rrs[0].(*dns.CNAME)
	if !ok {
		t.Fatal("Expected CNAME record when IPs empty")
	}

	if cname.Target != "api-europe.gslb.elchi." {
		t.Errorf("Expected CNAME to api-europe.gslb.elchi., got %s", cname.Target)
	}

	// Query for Europe service should return A records
	aRRs := cache.Get("api-europe.gslb.elchi.", dns.TypeA)
	if len(aRRs) != 2 {
		t.Fatalf("Expected 2 A records for Europe service, got %d", len(aRRs))
	}
}

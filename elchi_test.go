package elchi

import (
	"context"
	"net"
	"testing"

	"github.com/miekg/dns"
)

func TestServeDNS_InZone(t *testing.T) {
	// Create plugin with cache
	e := &Elchi{
		Zone: "gslb.elchi.",
		TTL:  300,
	}
	e.cache = NewRecordCache("gslb.elchi.")

	// Populate cache
	snapshot := &DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "test123",
		Records: []DNSRecord{
			{
				Name:    "test.gslb.elchi",
				Type:    "A",
				TTL:     300,
				IPs:     []string{"192.168.1.10"},
				Enabled: true,
			},
		},
	}
	e.cache.ReplaceFromSnapshot(snapshot, 300)

	// Create DNS query
	m := new(dns.Msg)
	m.SetQuestion("test.gslb.elchi.", dns.TypeA)

	// Create response writer
	rec := &testResponseWriter{}

	// Call ServeDNS
	ctx := context.Background()
	code, err := e.ServeDNS(ctx, rec, m)

	// Verify response
	if err != nil {
		t.Fatalf("ServeDNS failed: %v", err)
	}
	if code != dns.RcodeSuccess {
		t.Errorf("Expected RcodeSuccess, got %d", code)
	}
	if rec.msg == nil {
		t.Fatal("No response message written")
	}
	if len(rec.msg.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(rec.msg.Answer))
	}

	// Verify A record
	aRecord, ok := rec.msg.Answer[0].(*dns.A)
	if !ok {
		t.Fatal("Expected A record")
	}
	if aRecord.A.String() != "192.168.1.10" {
		t.Errorf("Expected IP 192.168.1.10, got %s", aRecord.A.String())
	}
}

func TestServeDNS_OutOfZone(t *testing.T) {
	// Create plugin
	e := &Elchi{
		Zone: "gslb.elchi.",
		TTL:  300,
	}
	e.cache = NewRecordCache("gslb.elchi.")

	// Create query for different zone
	m := new(dns.Msg)
	m.SetQuestion("google.com.", dns.TypeA)

	rec := &testResponseWriter{}

	// Call ServeDNS - should pass to next plugin
	// Since we don't have a Next handler, it will return SERVFAIL
	ctx := context.Background()
	code, _ := e.ServeDNS(ctx, rec, m)

	// Should return SERVFAIL (no next handler)
	if code != dns.RcodeServerFailure {
		t.Errorf("Expected RcodeServerFailure for out-of-zone query, got %d", code)
	}
}

func TestServeDNS_UnsupportedRecordType(t *testing.T) {
	// Create plugin with cache
	e := &Elchi{
		Zone: "gslb.elchi.",
		TTL:  300,
	}
	e.cache = NewRecordCache("gslb.elchi.")

	// Test unsupported record types (CNAME is now supported)
	tests := []struct {
		name  string
		qtype uint16
	}{
		{"MX record", dns.TypeMX},
		{"TXT record", dns.TypeTXT},
		{"NS record", dns.TypeNS},
		{"SOA record", dns.TypeSOA},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := new(dns.Msg)
			m.SetQuestion("test.gslb.elchi.", tt.qtype)

			rec := &testResponseWriter{}
			ctx := context.Background()
			code, _ := e.ServeDNS(ctx, rec, m)

			// Should pass to next plugin (SERVFAIL with no next handler)
			if code != dns.RcodeServerFailure {
				t.Errorf("Expected RcodeServerFailure for unsupported type %s, got %d",
					dns.TypeToString[tt.qtype], code)
			}
		})
	}
}

func TestServeDNS_NoRecords(t *testing.T) {
	// Create plugin with empty cache
	e := &Elchi{
		Zone: "gslb.elchi.",
		TTL:  300,
	}
	e.cache = NewRecordCache("gslb.elchi.")

	// Empty snapshot
	snapshot := &DNSSnapshot{
		Zone:        "gslb.elchi.",
		VersionHash: "empty",
		Records:     []DNSRecord{},
	}
	e.cache.ReplaceFromSnapshot(snapshot, 300)

	// Query for non-existent record
	m := new(dns.Msg)
	m.SetQuestion("nonexistent.gslb.elchi.", dns.TypeA)

	rec := &testResponseWriter{}

	ctx := context.Background()
	code, _ := e.ServeDNS(ctx, rec, m)

	// Should return NXDOMAIN (RcodeNameError) when no records found and fallthrough is disabled
	if code != dns.RcodeNameError {
		t.Errorf("Expected RcodeNameError for no records, got %d", code)
	}
	if rec.msg == nil {
		t.Fatal("No response message written")
	}
	if rec.msg.Rcode != dns.RcodeNameError {
		t.Errorf("Expected NXDOMAIN response, got rcode %d", rec.msg.Rcode)
	}
}

func TestName(t *testing.T) {
	e := &Elchi{}
	if name := e.Name(); name != "elchi" {
		t.Errorf("Expected name 'elchi', got '%s'", name)
	}
}

func TestReady(t *testing.T) {
	tests := []struct {
		name        string
		setupPlugin func() *Elchi
		wantReady   bool
	}{
		{
			name: "not ready - nil cache",
			setupPlugin: func() *Elchi {
				return &Elchi{
					cache: nil,
				}
			},
			wantReady: false,
		},
		{
			name: "not ready - empty version hash",
			setupPlugin: func() *Elchi {
				e := &Elchi{
					Zone: "gslb.elchi.",
				}
				e.cache = NewRecordCache("gslb.elchi.")
				return e
			},
			wantReady: false,
		},
		{
			name: "ready - cache populated with version hash",
			setupPlugin: func() *Elchi {
				e := &Elchi{
					Zone: "gslb.elchi.",
				}
				e.cache = NewRecordCache("gslb.elchi.")
				snapshot := &DNSSnapshot{
					Zone:        "gslb.elchi.",
					VersionHash: "abc123",
					Records: []DNSRecord{
						{
							Name:    "test.gslb.elchi",
							Type:    "A",
							TTL:     300,
							IPs:     []string{"192.168.1.10"},
							Enabled: true,
						},
					},
				}
				e.cache.ReplaceFromSnapshot(snapshot, 300)
				return e
			},
			wantReady: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := tt.setupPlugin()
			if got := e.Ready(); got != tt.wantReady {
				t.Errorf("Ready() = %v, want %v", got, tt.wantReady)
			}
		})
	}
}

// testResponseWriter is a mock dns.ResponseWriter for testing.
type testResponseWriter struct {
	msg *dns.Msg
}

func (t *testResponseWriter) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 53}
}
func (t *testResponseWriter) RemoteAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345}
}
func (t *testResponseWriter) WriteMsg(m *dns.Msg) error {
	t.msg = m
	return nil
}
func (t *testResponseWriter) Write([]byte) (int, error) { return 0, nil }
func (t *testResponseWriter) Close() error              { return nil }
func (t *testResponseWriter) TsigStatus() error         { return nil }
func (t *testResponseWriter) TsigTimersOnly(bool)       {}
func (t *testResponseWriter) Hijack()                   {}

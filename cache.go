package elchi

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// RecordCache is a thread-safe cache for DNS records
type RecordCache struct {
	mu          sync.RWMutex
	zone        string
	versionHash string
	records     map[string]map[uint16][]dns.RR // domain -> qtype -> []RR
	updatedAt   time.Time
}

// NewRecordCache creates a new record cache for the given zone
func NewRecordCache(zone string) *RecordCache {
	return &RecordCache{
		zone:    zone,
		records: make(map[string]map[uint16][]dns.RR),
	}
}

// ReplaceFromSnapshot atomically replaces the entire cache from a DNS snapshot
func (c *RecordCache) ReplaceFromSnapshot(snapshot *DNSSnapshot, defaultTTL uint32) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot is nil")
	}

	// Build new cache from snapshot
	newRecords := make(map[string]map[uint16][]dns.RR)

	for _, record := range snapshot.Records {
		// Normalize domain name
		domain := normalizeDomain(record.Name)

		// Validate record is within our zone
		if !strings.HasSuffix(domain, c.zone) && domain != c.zone {
			log.Warningf("Skipping record %s: not in zone %s", domain, c.zone)
			continue
		}

		// Build dns.RR objects from the record
		rrs, err := buildDNSRecords(record, defaultTTL)
		if err != nil {
			log.Warningf("Failed to build DNS records for %s: %v", record.Name, err)
			continue
		}

		if len(rrs) == 0 {
			continue
		}

		// Determine qtype from first RR
		qtype := rrs[0].Header().Rrtype

		// Initialize nested map if needed
		if newRecords[domain] == nil {
			newRecords[domain] = make(map[uint16][]dns.RR)
		}

		// Append RRs to cache
		newRecords[domain][qtype] = append(newRecords[domain][qtype], rrs...)
	}

	// Atomically replace cache
	c.mu.Lock()
	c.records = newRecords
	c.versionHash = snapshot.VersionHash
	c.updatedAt = time.Now()

	// Update cache size metric
	recordCount := 0
	for _, qtypeMap := range c.records {
		for _, rrs := range qtypeMap {
			recordCount += len(rrs)
		}
	}
	cacheSize.WithLabelValues(c.zone).Set(float64(recordCount))
	c.mu.Unlock()

	return nil
}

// Get retrieves pre-built dns.RR objects for a query
func (c *RecordCache) Get(qname string, qtype uint16) []dns.RR {
	c.mu.RLock()
	defer c.mu.RUnlock()

	domain := normalizeDomain(qname)

	// Check if domain exists in cache
	qtypeMap, exists := c.records[domain]
	if !exists {
		return nil
	}

	// Check if qtype exists for this domain
	rrs, exists := qtypeMap[qtype]
	if !exists {
		return nil
	}

	// Return copy of RRs to prevent external modification
	result := make([]dns.RR, len(rrs))
	copy(result, rrs)
	return result
}

// GetVersionHash returns the current version hash
func (c *RecordCache) GetVersionHash() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.versionHash
}

// Count returns the total number of unique domains in the cache
func (c *RecordCache) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.records)
}

// Update merges new records into the cache (used by webhook /notify endpoint)
func (c *RecordCache) Update(records []DNSRecord, defaultTTL uint32) error {
	if len(records) == 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, record := range records {
		// Normalize domain name
		domain := normalizeDomain(record.Name)

		// Validate record is within our zone
		if !strings.HasSuffix(domain, c.zone) && domain != c.zone {
			log.Warningf("Skipping record %s: not in zone %s", domain, c.zone)
			continue
		}

		// Build dns.RR objects from the record
		rrs, err := buildDNSRecords(record, defaultTTL)
		if err != nil {
			log.Warningf("Failed to build DNS records for %s: %v", record.Name, err)
			continue
		}

		if len(rrs) == 0 {
			continue
		}

		// Determine qtype from first RR
		qtype := rrs[0].Header().Rrtype

		// Initialize nested map if needed
		if c.records[domain] == nil {
			c.records[domain] = make(map[uint16][]dns.RR)
		}

		// Replace existing RRs for this domain+qtype
		c.records[domain][qtype] = rrs
	}

	c.updatedAt = time.Now()

	// Update cache size metric
	recordCount := 0
	for _, qtypeMap := range c.records {
		for _, rrs := range qtypeMap {
			recordCount += len(rrs)
		}
	}
	cacheSize.WithLabelValues(c.zone).Set(float64(recordCount))

	return nil
}

// Delete removes specific records from the cache (used by webhook /notify endpoint)
func (c *RecordCache) Delete(deletes []DeleteRecord) error {
	if len(deletes) == 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, del := range deletes {
		// Normalize domain name
		domain := normalizeDomain(del.Name)

		// Parse record type
		var qtype uint16
		switch strings.ToUpper(del.Type) {
		case "A":
			qtype = dns.TypeA
		case "AAAA":
			qtype = dns.TypeAAAA
		default:
			log.Warningf("Unsupported record type for deletion: %s", del.Type)
			continue
		}

		// Check if domain exists
		qtypeMap, exists := c.records[domain]
		if !exists {
			continue
		}

		// Delete the specific qtype
		delete(qtypeMap, qtype)

		// If no more qtypes for this domain, remove the domain entry
		if len(qtypeMap) == 0 {
			delete(c.records, domain)
		}
	}

	c.updatedAt = time.Now()

	// Update cache size metric
	recordCount := 0
	for _, qtypeMap := range c.records {
		for _, rrs := range qtypeMap {
			recordCount += len(rrs)
		}
	}
	cacheSize.WithLabelValues(c.zone).Set(float64(recordCount))

	return nil
}

// GetAllRecords returns all cached records (for /records endpoint)
func (c *RecordCache) GetAllRecords() []DNSRecord {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var records []DNSRecord

	for domain, qtypeMap := range c.records {
		for qtype, rrs := range qtypeMap {
			if len(rrs) == 0 {
				continue
			}

			// Extract IPs and TTL from RRs
			var ips []string
			var ttl uint32

			for _, rr := range rrs {
				ttl = rr.Header().Ttl
				switch r := rr.(type) {
				case *dns.A:
					ips = append(ips, r.A.String())
				case *dns.AAAA:
					ips = append(ips, r.AAAA.String())
				}
			}

			// Determine type string
			var typeStr string
			switch qtype {
			case dns.TypeA:
				typeStr = "A"
			case dns.TypeAAAA:
				typeStr = "AAAA"
			}

			records = append(records, DNSRecord{
				Name: strings.TrimSuffix(domain, "."),
				Type: typeStr,
				TTL:  ttl,
				IPs:  ips,
			})
		}
	}

	return records
}

// DeleteRecord represents a record to be deleted
type DeleteRecord struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// buildDNSRecords converts a DNSRecord from API to []dns.RR objects
func buildDNSRecords(record DNSRecord, defaultTTL uint32) ([]dns.RR, error) {
	var rrs []dns.RR

	// Normalize domain name to FQDN
	name := normalizeDomain(record.Name)

	// Determine TTL: use record TTL if set, otherwise use default
	ttl := record.TTL
	if ttl == 0 {
		ttl = defaultTTL
	}

	// Check if failover is active (enabled=false)
	if !record.Enabled {
		// If failover is empty, return error (will result in NXDOMAIN)
		if record.Failover == "" {
			return nil, fmt.Errorf("record %s is disabled but failover is empty", record.Name)
		}

		// Build CNAME record pointing to failover
		cname := &dns.CNAME{
			Hdr: dns.RR_Header{
				Name:   name,
				Rrtype: dns.TypeCNAME,
				Class:  dns.ClassINET,
				Ttl:    ttl,
			},
			Target: normalizeDomain(record.Failover),
		}
		return []dns.RR{cname}, nil
	}

	// Build RRs based on record type
	recordType := strings.ToUpper(record.Type)

	switch recordType {
	case "A":
		for _, ipStr := range record.IPs {
			ip := net.ParseIP(ipStr)
			if ip == nil {
				log.Warningf("Invalid IP address: %s", ipStr)
				continue
			}

			// Ensure it's IPv4
			ip4 := ip.To4()
			if ip4 == nil {
				log.Warningf("IP %s is not IPv4, skipping for A record", ipStr)
				continue
			}

			rr := &dns.A{
				Hdr: dns.RR_Header{
					Name:   name,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    ttl,
				},
				A: ip4,
			}
			rrs = append(rrs, rr)
		}

	case "AAAA":
		for _, ipStr := range record.IPs {
			ip := net.ParseIP(ipStr)
			if ip == nil {
				log.Warningf("Invalid IP address: %s", ipStr)
				continue
			}

			// Ensure it's IPv6
			if ip.To4() != nil {
				log.Warningf("IP %s is IPv4, skipping for AAAA record", ipStr)
				continue
			}

			rr := &dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   name,
					Rrtype: dns.TypeAAAA,
					Class:  dns.ClassINET,
					Ttl:    ttl,
				},
				AAAA: ip,
			}
			rrs = append(rrs, rr)
		}

	default:
		return nil, fmt.Errorf("unsupported record type: %s", record.Type)
	}

	if len(rrs) == 0 {
		return nil, fmt.Errorf("no valid IPs found for record %s", record.Name)
	}

	return rrs, nil
}

// normalizeDomain normalizes a domain name to FQDN format
func normalizeDomain(domain string) string {
	// Convert to lowercase and trim spaces
	normalized := strings.ToLower(strings.TrimSpace(domain))

	// Ensure trailing dot (FQDN)
	if !strings.HasSuffix(normalized, ".") {
		normalized += "."
	}

	return normalized
}

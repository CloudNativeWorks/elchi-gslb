# ADR-005: Pre-Built DNS Records in Cache

## Status
Accepted

## Context

The in-memory cache needs to store DNS record data. When a DNS query arrives, the plugin must construct a DNS response message with `dns.RR` (Resource Record) objects. We have two choices for when to build these objects:

1. **On-demand**: Store raw data (IPs), build `dns.RR` during query
2. **Pre-built**: Build `dns.RR` during cache update, store ready objects

Performance is critical - DNS queries must be answered in <1ms.

## Alternatives Considered

### 1. Store Raw Data, Build On Query
```go
type CacheEntry struct {
    Name string
    Type string
    TTL  uint32
    IPs  []string  // Raw data
}

// On each query
func ServeDNS(qname string) {
    entry := cache.Get(qname)
    rrs := make([]dns.RR, len(entry.IPs))
    for i, ip := range entry.IPs {
        rr := &dns.A{
            Hdr: dns.RR_Header{Name: qname, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: entry.TTL},
            A:   net.ParseIP(ip),
        }
        rrs[i] = rr  // Build on every query
    }
    return rrs
}
```

**Pros:**
- ✅ Less memory (raw strings vs. full RR objects)
- ✅ Flexible (can change RR format later)

**Cons:**
- ❌ **CPU overhead on hot path**: Parse IP, create RR header on every query
- ❌ **Latency**: 10-50μs per query for building RRs
- ❌ **GC pressure**: Allocating objects on every query

### 2. Pre-Build and Cache (Selected)
```go
type CacheEntry struct {
    RRs []dns.RR  // Pre-built, ready to use
}

// During cache update (cold path)
func UpdateCache(record DNSRecord) {
    rrs := make([]dns.RR, len(record.IPs))
    for i, ip := range record.IPs {
        rr := &dns.A{
            Hdr: dns.RR_Header{Name: record.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: record.TTL},
            A:   net.ParseIP(ip),
        }
        rrs[i] = rr  // Build once
    }
    cache.Store(key, rrs)
}

// On each query (hot path)
func ServeDNS(qname string) {
    rrs := cache.Get(qname)  // Already built!
    return rrs
}
```

**Pros:**
- ✅ **Fast queries**: Just cache lookup, no building
- ✅ **Low latency**: <1μs for cache get
- ✅ **No GC pressure on hot path**: Objects already allocated
- ✅ **Simple query logic**: Cache returns ready-to-use RRs

**Cons:**
- ⚠️ **More memory**: Full `dns.RR` objects instead of strings
  - Impact: ~100 bytes/record vs. ~50 bytes (2x memory)
  - Acceptable: 10k records = 1MB vs. 500KB

## Decision

**Pre-build `dns.RR` objects and store them in cache.**

### Rationale

DNS query serving is the **hot path** - it happens thousands of times per second. Cache updates (sync) are the **cold path** - they happen every 5 minutes.

**Optimize the hot path, even at the cost of cold path complexity.**

### Implementation

```go
func (c *RecordCache) buildDNSRecords(record *DNSRecord, defaultTTL uint32) ([]dns.RR, error) {
    ttl := record.TTL
    if ttl == 0 {
        ttl = defaultTTL
    }

    rrs := make([]dns.RR, 0, len(record.IPs))

    switch record.Type {
    case "A":
        for _, ipStr := range record.IPs {
            ip := net.ParseIP(ipStr)
            if ip == nil || ip.To4() == nil {
                return nil, fmt.Errorf("invalid IPv4: %s", ipStr)
            }
            rr := &dns.A{
                Hdr: dns.RR_Header{
                    Name:   record.Name,
                    Rrtype: dns.TypeA,
                    Class:  dns.ClassINET,
                    Ttl:    ttl,
                },
                A: ip.To4(),
            }
            rrs = append(rrs, rr)
        }
    case "AAAA":
        // Similar for AAAA
    }

    return rrs, nil  // Pre-built, ready to use
}
```

## Consequences

### Positive
- ✅ **Very fast DNS responses**: Query path is just cache lookup
- ✅ **Predictable latency**: No variable parsing time
- ✅ **Low CPU usage**: No repeated work on hot path
- ✅ **Less GC pressure**: Objects allocated once during sync
- ✅ **Simpler query code**: Just return cached RRs

### Negative
- ⚠️ **Higher memory usage**: 2x memory per record
  - Mitigation: DNS records are small, total memory is still low
  - Example: 10k records = 1MB extra (acceptable)
- ⚠️ **More complex cache updates**: Building RRs during sync
  - Mitigation: Sync is cold path, complexity is acceptable
  - Mitigation: Errors caught during sync, not during queries

### Performance Comparison

| Metric | Raw Data | Pre-Built | Improvement |
|--------|----------|-----------|-------------|
| Query latency | ~50μs | ~1μs | **50x faster** |
| Memory/record | ~50 bytes | ~100 bytes | 2x more |
| GC allocations/query | 2-3 objects | 0 objects | **No GC** |
| Cache update time | ~1ms | ~5ms | 5x slower (acceptable) |

## Memory Impact Analysis

Typical production deployment:
- **1,000 records**: 100KB (negligible)
- **10,000 records**: 1MB (acceptable)
- **100,000 records**: 10MB (still acceptable for modern systems)

For large deployments (>100k records), the 10MB extra memory is worth the query performance gain.

## Related Decisions
- [ADR-001: In-Memory Cache](001-in-memory-cache.md) - Cache architecture
- [ADR-004: Graceful Degradation](004-graceful-degradation.md) - Errors during RR building

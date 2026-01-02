# ADR-001: In-Memory Cache Usage

## Status
Accepted

## Context

DNS queries are extremely high-frequency requests. A typical production DNS server can receive thousands of queries per second. Making an HTTP request to the Elchi controller for each DNS query would cause:

1. **Latency problem**: HTTP round-trip time can be 10-50ms, while DNS queries must be answered in <1ms
2. **Throughput problem**: Controller cannot handle 10k+ HTTP requests per second
3. **Reliability problem**: A temporary issue in the controller would stop all DNS resolution
4. **Network problem**: Network traffic for each query would overwhelm the controller

## Alternatives Considered

### 1. Controller Call for Every Query
- ❌ Very high latency
- ❌ Overwhelms the controller
- ❌ Network bandwidth waste
- ❌ Single point of failure

### 2. External Cache (Redis/Memcached)
- ✅ Centralized cache
- ✅ Shareable across multiple CoreDNS instances
- ❌ Extra dependency (Redis cluster setup)
- ❌ Adds network hop (still 1-5ms latency)
- ❌ DNS fails if Redis fails

### 3. In-Memory Cache (Selected)
- ✅ Very low latency (<1ms, typically <100μs)
- ✅ No extra dependencies
- ✅ Simple implementation
- ✅ Thread-safe with Go map + RWMutex
- ⚠️ Each CoreDNS instance maintains its own cache
- ⚠️ Memory usage (but DNS records are small)

## Decision

**We will use in-memory cache.**

Cache implementation:
- Thread-safe access with `sync.RWMutex`
- Cache key: `name:type` (e.g., `test.gslb.elchi:A`)
- Value: Pre-built `dns.RR` slice (DNS response ready)
- TTL from controller's record TTL is used
- Records never expire (controller's responsibility)

## Consequences

### Positive
- ✅ **Very fast DNS responses**: <1ms latency
- ✅ **High throughput**: 10k+ QPS easily handled
- ✅ **Simple architecture**: No extra components
- ✅ **Low operational complexity**: Only CoreDNS needs deployment
- ✅ **Independent from controller**: DNS works even if controller is down

### Negative
- ⚠️ **Memory usage**: Each instance maintains its own cache
  - Mitigation: DNS records are small (~100 bytes/record), 10k records = ~1MB
- ⚠️ **Cache consistency**: Eventual consistency across multiple CoreDNS instances
  - Mitigation: Instant sync via webhook, periodic sync ensures consistency
- ⚠️ **Initial load time**: Snapshot fetch required at startup
  - Mitigation: Background fetch, continue serving with empty cache

## Related Decisions
- [ADR-002: Hash-Based Change Detection](002-hash-based-sync.md) - How to update the cache
- [ADR-005: Pre-Built DNS Records](005-pre-built-records.md) - What to store in cache

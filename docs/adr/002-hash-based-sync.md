# ADR-002: Hash-Based Change Detection

## Status
Accepted

## Context

The plugin needs to keep its in-memory cache synchronized with the Elchi controller. The plugin polls the controller periodically (default: 5 minutes). We need an efficient way to:

1. **Detect changes**: Know if DNS records have changed since last sync
2. **Minimize bandwidth**: Avoid transferring full snapshot if nothing changed
3. **Simplify logic**: Avoid complex diff algorithms
4. **Ensure consistency**: Guarantee cache matches controller state

## Alternatives Considered

### 1. Always Fetch Full Snapshot
- ✅ Simple implementation
- ✅ Guaranteed consistency
- ❌ Wastes bandwidth (10k records = ~1MB every 5 min)
- ❌ Controller CPU waste generating unchanged data
- ❌ Plugin CPU waste processing unchanged data

### 2. Timestamp-Based Change Detection
- ✅ Standard approach (`If-Modified-Since` header)
- ⚠️ Requires clock synchronization between plugin and controller
- ⚠️ Clock skew can cause missed updates
- ❌ Doesn't handle records deleted then re-added in same second

### 3. Version Number
- ✅ Simple increment on each change
- ❌ Requires persistent storage on controller
- ❌ Complex rollback scenarios
- ❌ Doesn't detect if version counter resets

### 4. Hash-Based (Selected)
- ✅ Content-based, not time-based
- ✅ No clock synchronization needed
- ✅ Detects any change in records
- ✅ Stateless (hash computed from current data)
- ✅ HTTP 304 Not Modified for unchanged data
- ⚠️ Controller must compute hash on each request

## Decision

**Use hash-based change detection with HTTP 304 Not Modified.**

Implementation:
- Controller computes hash of all DNS records (e.g., SHA256 of sorted JSON)
- Plugin sends `since={hash}` query parameter
- Controller compares request hash with current hash
- If unchanged: Return HTTP 304 Not Modified (empty body)
- If changed: Return HTTP 200 with full snapshot + new hash

The hash is opaque to the plugin (could be SHA256, MD5, or even version number).

## Consequences

### Positive
- ✅ **Bandwidth efficient**: No data transfer if unchanged (HTTP 304)
- ✅ **Deterministic**: Same data always produces same hash
- ✅ **Simple plugin logic**: Just compare hash strings
- ✅ **Controller flexibility**: Can change hash algorithm anytime
- ✅ **No persistent state**: Hash computed from current data
- ✅ **Clock-independent**: No timestamp issues

### Negative
- ⚠️ **Controller CPU**: Must compute hash on each check
  - Mitigation: Hash is cached, recomputed only when data changes
- ⚠️ **Full snapshot on change**: Cannot send diffs
  - Mitigation: DNS records are small, full snapshot is acceptable
  - Future: Could add incremental sync later if needed

### Trade-offs
- We chose **simplicity over incremental updates**
- DNS record set sizes are typically small (< 10k records = ~1MB)
- Network bandwidth saved by HTTP 304 is more valuable than incremental sync complexity

## API Contract

### Request
```
GET /dns/changes?zone=gslb.elchi&since=abc123def456
X-Elchi-Secret: <secret>
```

### Response - No Changes
```
HTTP/1.1 304 Not Modified
```

### Response - Has Changes
```json
HTTP/1.1 200 OK
{
  "unchanged": false,
  "zone": "gslb.elchi",
  "version_hash": "xyz789new",
  "records": [...]
}
```

## Related Decisions
- [ADR-001: In-Memory Cache](001-in-memory-cache.md) - What we're synchronizing
- [ADR-003: Webhook Architecture](003-webhook-architecture.md) - Instant updates bypass polling

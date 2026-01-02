# ADR-004: Graceful Degradation Strategy

## Status
Accepted

## Context

The plugin depends on the Elchi controller for DNS record data. However, DNS is a critical service that must remain available even when dependencies fail. We need a strategy for handling:

1. **Controller unavailability**: Network partition, controller crash, deployment
2. **Partial failures**: Timeouts, slow responses, malformed data
3. **Startup scenarios**: Controller not ready when plugin starts
4. **Degraded operation**: How to serve DNS during issues

The key principle: **Stale data is better than no data.**

## Alternatives Considered

### 1. Fail Closed (Stop Serving DNS)
- ✅ Ensures no stale data served
- ✅ Clear failure signal
- ❌ **Unacceptable**: Breaks all DNS resolution
- ❌ Cascading failures across infrastructure

### 2. Return SERVFAIL
- ✅ Standard DNS error code
- ✅ Clients can retry or use fallback
- ❌ Many clients don't handle SERVFAIL well
- ❌ Applications break instead of using cached data

### 3. Delegate to Next Plugin
- ✅ Allows fallback to other DNS sources
- ❌ Next plugin likely doesn't have GSLB data
- ❌ Loses the purpose of the plugin

### 4. Graceful Degradation (Selected)
- ✅ Continue serving from cache
- ✅ Log warnings but don't fail
- ✅ Retry in background
- ✅ Eventual consistency when controller recovers
- ⚠️ May serve stale data

## Decision

**Implement graceful degradation with stale cache serving.**

### Principles

1. **Never panic**: All errors are logged and handled gracefully
2. **Never expire cache**: Records stay until explicitly removed
3. **Continue serving**: Serve stale data during controller outages
4. **Retry periodically**: Background sync continues trying
5. **Expose health status**: Monitoring can detect degraded state

### Behavior Matrix

| Scenario | Behavior | DNS Response | Health Endpoint |
|----------|----------|--------------|-----------------|
| Controller healthy | Normal operation | Fresh data | `200 OK` "healthy" |
| Initial sync fails | Continue with empty cache | NXDOMAIN | `503` "degraded" |
| Periodic sync fails | Serve last known cache | Stale data (works) | `503` "degraded" |
| Webhook fails | Use periodic sync | Eventually fresh | `200 OK` "healthy" |
| Controller down 1 hour | Serve stale cache | Stale data (works) | `503` "degraded" |
| Controller recovers | Update cache | Fresh data | `200 OK` "healthy" |

## Implementation

### Error Handling

```go
// Initial snapshot failure
snapshot, err := client.FetchSnapshot(ctx)
if err != nil {
    log.Warningf("Initial snapshot failed: %v (will retry)", err)
    // Continue with empty cache, don't return error
}

// Periodic sync failure
changes, err := client.CheckChanges(ctx, versionHash)
if err != nil {
    log.Errorf("Sync failed: %v (serving stale cache)", err)
    syncStatus.Update("failed", err)
    // Continue with existing cache
    return
}
```

### Health Monitoring

```go
// Health endpoint returns degraded status
GET /health
{
  "status": "degraded",  // or "healthy"
  "zone": "gslb.elchi",
  "records_count": 42,
  "version_hash": "abc123",
  "last_sync": "2025-01-01T08:00:00Z",
  "last_sync_status": "failed",  // or "success"
  "error": "connection timeout"
}
```

### Kubernetes Readiness

```go
// Ready() returns true even if sync failed
// We're "ready" to serve DNS, even if data is stale
func (e *Elchi) Ready() bool {
    return e.cache.GetVersionHash() != ""
}
```

## Consequences

### Positive
- ✅ **High availability**: DNS continues working during controller outages
- ✅ **Fault tolerance**: Temporary network issues don't break DNS
- ✅ **No cascading failures**: Plugin problems don't bring down infrastructure
- ✅ **Observable**: Health endpoint shows degraded state
- ✅ **Self-healing**: Automatically recovers when controller returns

### Negative
- ⚠️ **Stale data risk**: May serve outdated IPs during outages
  - Mitigation: Monitor health endpoint, alert on degraded state
  - Mitigation: DNS TTLs limit how long clients cache stale data
- ⚠️ **Silent degradation**: Plugin keeps working with old data
  - Mitigation: Health endpoint returns 503 when degraded
  - Mitigation: Prometheus metrics track sync failures

### Acceptable Staleness

- **Short outages** (< 5 min): No impact, next sync succeeds
- **Medium outages** (5-60 min): Serve stale data, acceptable for most use cases
- **Long outages** (> 1 hour): Stale data, but better than no DNS
- **Critical**: Use webhook for instant updates of critical changes

## Monitoring

### Metrics to Track
```
elchi_sync_success_total     # Successful syncs
elchi_sync_failure_total     # Failed syncs
elchi_cache_age_seconds      # Time since last successful sync
elchi_health_status          # 1=healthy, 0=degraded
```

### Alerts to Configure
```yaml
- alert: ElchiSyncFailing
  expr: elchi_sync_failure_total > 3
  annotations:
    summary: "Elchi plugin failing to sync with controller"

- alert: ElchiCacheStale
  expr: elchi_cache_age_seconds > 600  # 10 minutes
  annotations:
    summary: "Elchi cache is stale (no updates in 10 min)"
```

## Related Decisions
- [ADR-001: In-Memory Cache](001-in-memory-cache.md) - Cache never expires
- [ADR-002: Hash-Based Sync](002-hash-based-sync.md) - Sync retry mechanism
- [ADR-003: Webhook Architecture](003-webhook-architecture.md) - Fast updates reduce staleness window

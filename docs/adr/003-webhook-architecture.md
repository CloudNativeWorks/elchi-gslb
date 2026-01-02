# ADR-003: Webhook Architecture for Instant Updates

## Status
Accepted

## Context

Periodic polling (every 5 minutes) creates a delay between when a record changes in the controller and when it becomes active in DNS responses. For critical changes (e.g., failover scenarios), waiting up to 5 minutes is unacceptable.

Requirements:
1. **Low latency**: Changes should propagate in seconds, not minutes
2. **Optional feature**: Should not be required for basic operation
3. **Secure**: Only controller should be able to push updates
4. **Partial updates**: Should support updating/deleting specific records without full snapshot

## Alternatives Considered

### 1. Reduce Poll Interval (e.g., 10 seconds)
- ✅ Simple, no architecture change
- ❌ High network traffic (even if no changes)
- ❌ Controller load increases 30x
- ❌ Still 10s latency

### 2. WebSocket Connection
- ✅ True real-time updates
- ✅ Bi-directional communication
- ❌ Complex implementation (connection management, reconnection, heartbeats)
- ❌ Stateful connection (doesn't fit CoreDNS plugin model)
- ❌ Load balancer compatibility issues

### 3. Message Queue (RabbitMQ/Kafka)
- ✅ Reliable delivery
- ✅ Pub/sub pattern
- ❌ Heavy dependency
- ❌ Operational complexity
- ❌ Overkill for simple DNS updates

### 4. HTTP Webhook (Selected)
- ✅ Simple HTTP POST from controller to plugin
- ✅ Stateless (each request independent)
- ✅ Easy to implement and test
- ✅ Standard authentication (shared secret)
- ✅ Optional (fallback to polling if webhook down)
- ⚠️ Plugin must expose HTTP endpoint

## Decision

**Implement optional HTTP webhook endpoint for instant updates.**

Architecture:
- Plugin exposes HTTP server on configurable port (default: 8053)
- Controller sends POST requests to plugin when records change
- Webhook payload contains only changed/deleted records (partial update)
- Same authentication mechanism as polling (X-Elchi-Secret header)
- Webhook failures are logged but don't stop normal operation
- Periodic polling still runs as fallback

## Implementation Details

### Endpoints

#### 1. POST /notify - Instant Updates
```json
{
  "records": [
    {"name": "updated.gslb.elchi", "type": "A", "ttl": 300, "ips": ["1.2.3.4"]}
  ],
  "deletes": [
    {"name": "removed.gslb.elchi", "type": "A"}
  ]
}
```

#### 2. GET /health - Health Check
```json
{
  "status": "healthy",
  "zone": "gslb.elchi",
  "records_count": 42,
  "version_hash": "abc123",
  "last_sync": "2025-01-01T12:00:00Z"
}
```

#### 3. GET /records - Inspection
```json
{
  "zone": "gslb.elchi",
  "records": [...]
}
```

### Configuration
```
elchi {
    endpoint http://controller:8080
    secret my-secret-key
    webhook :8053  # Enable webhook on port 8053
}
```

## Consequences

### Positive
- ✅ **Fast propagation**: Changes active within seconds
- ✅ **Reduced latency**: No need to wait for next poll interval
- ✅ **Partial updates**: Only changed records transferred
- ✅ **Optional**: Works without webhook (falls back to polling)
- ✅ **Simple**: Just HTTP POST, no complex protocols
- ✅ **Debuggable**: Easy to test with curl/httpie
- ✅ **Load balancer friendly**: Standard HTTP traffic

### Negative
- ⚠️ **Extra port**: Plugin must listen on additional port
  - Mitigation: Configurable, can be disabled
- ⚠️ **Network requirements**: Controller must reach plugin
  - Mitigation: Works in Kubernetes with Service
- ⚠️ **Partial state**: Webhook can fail, leaving inconsistent state
  - Mitigation: Periodic polling ensures eventual consistency

### Security Considerations
- 🔒 Same secret-based authentication as API calls
- 🔒 Reject requests without valid X-Elchi-Secret header
- 🔒 Rate limiting recommended in production
- 🔒 Webhook endpoint should not be public (internal network only)

## Usage Patterns

### Pattern 1: Kubernetes Deployment
```yaml
apiVersion: v1
kind: Service
metadata:
  name: coredns-elchi-webhook
spec:
  selector:
    app: coredns
  ports:
  - name: webhook
    port: 8053
    targetPort: 8053
```

Controller can call: `http://coredns-elchi-webhook:8053/notify`

### Pattern 2: Critical Update Flow
1. User updates listener in Elchi
2. Controller updates database
3. Controller computes new version hash
4. Controller sends webhook POST to all CoreDNS instances
5. Instances update cache immediately
6. DNS queries return new IPs within seconds

### Pattern 3: Webhook Failure Handling
1. Webhook POST fails (network error, timeout)
2. Controller logs warning but doesn't retry
3. Plugin continues with old data
4. Next periodic sync (within 5 min) fixes inconsistency

## Related Decisions
- [ADR-001: In-Memory Cache](001-in-memory-cache.md) - Cache being updated
- [ADR-002: Hash-Based Sync](002-hash-based-sync.md) - Periodic sync as fallback
- [ADR-004: Graceful Degradation](004-graceful-degradation.md) - Handling webhook failures

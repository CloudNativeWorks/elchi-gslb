# Performance Tuning Guide

This guide helps you optimize the Elchi GSLB plugin for maximum performance and efficiency.

## Table of Contents

1. [Performance Characteristics](#performance-characteristics)
2. [Tuning Parameters](#tuning-parameters)
3. [Resource Requirements](#resource-requirements)
4. [Optimization Strategies](#optimization-strategies)
5. [Benchmarking](#benchmarking)
6. [Troubleshooting Performance Issues](#troubleshooting-performance-issues)

## Performance Characteristics

### Typical Latency

| Operation | Latency | Notes |
|-----------|---------|-------|
| DNS query (cache hit) | 50-100μs | In-memory lookup |
| DNS query (cache miss) | 200-500μs | NXDOMAIN response |
| Cache update | 1-5ms | Per 1000 records |
| Initial snapshot load | 10-50ms | Per 10k records |
| Webhook notification | 1-2ms | Partial update |

### Throughput Capacity

| Record Count | QPS (Queries/Sec) | Memory Usage | Notes |
|--------------|-------------------|--------------|-------|
| 1,000 | 50,000+ | ~1 MB | Single core |
| 10,000 | 50,000+ | ~10 MB | Cache size doesn't affect QPS |
| 100,000 | 45,000+ | ~100 MB | Slight GC impact |
| 1,000,000 | 40,000+ | ~1 GB | Significant GC impact |

**Key insight**: Query performance is nearly constant regardless of cache size due to O(1) map lookups.

## Tuning Parameters

### 1. Sync Interval

Controls how often the plugin checks for updates.

```
elchi {
    sync_interval 5m  # Default: 5 minutes
}
```

**Tuning Guidance:**

- **High-traffic production**: `1m` - More up-to-date, higher controller load
- **Normal production**: `5m` - Default, good balance
- **Low-priority zones**: `15m` - Less controller load
- **With webhook enabled**: `10m` - Webhook handles critical updates

**Trade-offs:**
- ✅ Shorter interval = fresher data
- ❌ Shorter interval = more controller load and bandwidth
- ✅ Longer interval = less overhead
- ❌ Longer interval = stale data during controller issues

### 2. HTTP Timeout

Controls request timeout to controller.

```
elchi {
    timeout 10s  # Default: 10 seconds
}
```

**Tuning Guidance:**

- **Fast local network**: `5s` - Fail faster
- **Normal network**: `10s` - Default
- **Slow/distant controller**: `30s` - Avoid premature timeout
- **Unreliable network**: `15s` - Balance between wait and retry

**Trade-offs:**
- ✅ Shorter timeout = faster failure detection
- ❌ Shorter timeout = false positives on slow network
- ✅ Longer timeout = handles network delays
- ❌ Longer timeout = slower error detection

### 3. TTL (Time to Live)

Controls DNS response TTL for clients.

```
elchi {
    ttl 300  # Default: 5 minutes (300 seconds)
}
```

**Tuning Guidance:**

- **Highly dynamic IPs**: `60s` - Fast failover, more queries
- **Normal usage**: `300s` - Default, balanced
- **Stable infrastructure**: `3600s` - Less query load, slower updates
- **CDN/edge caching**: `1800s` - 30 minutes

**Trade-offs:**
- ✅ Lower TTL = faster client updates on IP change
- ❌ Lower TTL = more DNS queries (higher load)
- ✅ Higher TTL = better client-side caching
- ❌ Higher TTL = slower propagation of changes

### 4. Webhook Port

```
elchi {
    webhook :8053  # Optional, enables instant updates
}
```

**Recommendation**: Enable webhook for production to get sub-second update latency.

## Resource Requirements

### CPU

| Scenario | CPU Usage | Notes |
|----------|-----------|-------|
| Idle (just sync) | 0.01 cores | Minimal background activity |
| 1,000 QPS | 0.05 cores | Mostly idle |
| 10,000 QPS | 0.3 cores | Linear scaling |
| 50,000 QPS | 1.5 cores | Near single-core limit |
| 100,000 QPS | 3+ cores | Need multiple instances |

**Scaling strategy**: Deploy multiple CoreDNS instances with load balancer.

### Memory

```
Memory = Base + (Records × 100 bytes)

Examples:
- 1,000 records:   10 MB  (base: 5 MB, cache: 5 MB)
- 10,000 records:  20 MB  (base: 5 MB, cache: 15 MB)
- 100,000 records: 120 MB (base: 20 MB, cache: 100 MB)
```

**Memory recommendations:**

| Record Count | Recommended Memory | Kubernetes Limit |
|--------------|-------------------|------------------|
| <10k | 128 MB | 256 MB |
| 10k-100k | 256 MB | 512 MB |
| 100k-1M | 1 GB | 2 GB |

### Network

| Operation | Bandwidth | Frequency |
|-----------|-----------|-----------|
| Periodic sync (no changes) | ~1 KB | Every 5 min |
| Periodic sync (10k records) | ~500 KB | When changed |
| Webhook notification | ~1-10 KB | On-demand |
| DNS query | ~100 bytes | Per query |

**Bandwidth estimate**: 10,000 QPS = ~8 Mbps outbound (DNS responses)

## Optimization Strategies

### 1. Enable Webhook for Critical Updates

```
elchi {
    endpoint http://controller:8080
    secret my-secret
    webhook :8053          # Enable this!
    sync_interval 10m      # Can increase since webhook handles urgency
}
```

**Benefit**: Get instant updates while reducing periodic sync frequency.

### 2. Adjust Sync Interval Based on Change Frequency

If your records change:
- **Hourly or less**: Use `sync_interval 15m`
- **Multiple times/hour**: Use `sync_interval 5m` (default)
- **Constantly**: Use `sync_interval 1m` + webhook

### 3. Use Appropriate TTL for Your Use Case

```
# Fast failover (load balancing, A/B testing)
ttl 60

# Normal usage (general GSLB)
ttl 300

# Stable infrastructure (rarely changes)
ttl 3600
```

### 4. Deploy Multiple Instances

For high availability and throughput:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: coredns
spec:
  replicas: 3  # Multiple instances
  template:
    spec:
      containers:
      - name: coredns
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 1000m
            memory: 256Mi
```

### 5. Enable Prometheus Monitoring

Monitor key metrics:

```
# Add to Corefile
prometheus :9253

# Key metrics to watch:
elchi_cache_size          # Number of cached records
elchi_query_total         # Total queries handled
elchi_query_duration_seconds  # Query latency
elchi_sync_success_total  # Successful syncs
elchi_sync_failure_total  # Failed syncs
```

### 6. Optimize Controller Response Time

Controller should:
- ✅ Cache version hash (recompute only on change)
- ✅ Use HTTP 304 for unchanged data
- ✅ Compress large snapshots (gzip)
- ✅ Respond within 1 second for `/dns/changes`

## Benchmarking

### Using dnsperf

```bash
# Install dnsperf
brew install dnsperf  # macOS
apt-get install dnsperf  # Ubuntu

# Create query file
cat > queries.txt <<EOF
test1.gslb.elchi A
test2.gslb.elchi A
test3.gslb.elchi A
EOF

# Run benchmark
dnsperf -s localhost -p 1053 -d queries.txt -c 10 -l 30

# Expected results:
# QPS: 20,000-50,000
# Latency: <1ms
```

### Using Go Benchmark

```go
func BenchmarkDNSQuery(b *testing.B) {
    e := setupTestPlugin()  // Your test setup

    m := new(dns.Msg)
    m.SetQuestion("test.gslb.elchi.", dns.TypeA)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        rec := dnstest.NewRecorder(&test.ResponseWriter{})
        e.ServeDNS(context.Background(), rec, m)
    }
}

// Expected: 10,000-50,000 ns/op (0.01-0.05ms per query)
```

## Troubleshooting Performance Issues

### High Query Latency

**Symptom**: DNS queries taking >10ms

**Possible causes:**
1. **High CPU usage**: Check `kubectl top pod`
   - Solution: Scale horizontally (more instances)

2. **Large cache**: Check `elchi_cache_size` metric
   - Solution: Increase memory limits

3. **GC pressure**: Check GC logs
   - Solution: Increase `GOGC` environment variable

4. **Lock contention**: Check with pprof
   - Solution: Already minimized, scale horizontally

### High Sync Latency

**Symptom**: Sync taking >1 second

**Possible causes:**
1. **Controller slow**: Check controller response time
   - Solution: Optimize controller, add caching

2. **Large snapshot**: Check transfer size
   - Solution: Enable HTTP compression

3. **Network latency**: Check ping time to controller
   - Solution: Move closer, increase timeout

### High Memory Usage

**Symptom**: Memory usage higher than expected

**Possible causes:**
1. **Many records**: Check `elchi_cache_size`
   - Solution: Increase limits, this is normal

2. **Memory leak**: Check if memory keeps growing
   - Solution: Report issue, temporary: restart pod

3. **Inefficient RR building**: Check code
   - Solution: Already optimized, not likely

## Performance Checklist

- [ ] Enabled webhook for instant updates
- [ ] Set appropriate `sync_interval` (5-10m for webhook)
- [ ] Set appropriate `ttl` (300s default, 60-3600s based on use case)
- [ ] Deployed multiple instances (3+ for HA)
- [ ] Set CPU limits (100m-1000m per instance)
- [ ] Set memory limits based on record count
- [ ] Enabled Prometheus metrics
- [ ] Configured alerts for sync failures
- [ ] Tested with realistic load (dnsperf)
- [ ] Validated controller response time (<1s)

## Summary

**Key takeaways:**
1. Plugin is very fast (50-100μs per query) out of the box
2. Performance is mostly independent of cache size (O(1) lookups)
3. Enable webhook for best update latency
4. Scale horizontally for high QPS (>50k)
5. Monitor sync health and cache staleness
6. Adjust sync_interval and ttl based on your change frequency

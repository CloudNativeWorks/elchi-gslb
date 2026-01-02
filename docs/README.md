# Elchi GSLB CoreDNS Plugin Documentation

Comprehensive documentation for the Elchi GSLB CoreDNS plugin.

## Quick Links

- [Main README](../README.md) - Plugin overview and quick start
- [CLAUDE.md](../CLAUDE.md) - Project context and design specifications
- [CHANGELOG](../CHANGELOG.md) - Version history and changes

## Documentation Structure

### 📐 Architecture Decision Records (ADRs)

Understand why the plugin is designed the way it is.

- [ADR Index](adr/README.md)
- [ADR-001: In-Memory Cache Usage](adr/001-in-memory-cache.md)
- [ADR-002: Hash-Based Change Detection](adr/002-hash-based-sync.md)
- [ADR-003: Webhook Architecture](adr/003-webhook-architecture.md)
- [ADR-004: Graceful Degradation Strategy](adr/004-graceful-degradation.md)
- [ADR-005: Pre-Built DNS Records](adr/005-pre-built-records.md)

### 📊 Sequence Diagrams

Visual representations of key flows.

- [Diagram Index](diagrams/README.md)
- [Startup Flow](diagrams/01-startup-flow.md) - Plugin initialization
- [Periodic Sync Flow](diagrams/02-periodic-sync.md) - Background synchronization
- [Webhook Update Flow](diagrams/03-webhook-flow.md) - Instant updates
- [DNS Query Flow](diagrams/04-query-flow.md) - Query handling
- [Error Recovery Flow](diagrams/05-error-recovery.md) - Graceful degradation

### 📚 Guides

Practical guides for using and optimizing the plugin.

- [Performance Tuning Guide](guides/performance-tuning.md) - Optimize for your workload

## Getting Started

### New Users

1. Read the [Main README](../README.md) for quick start
2. Understand the architecture via [ADRs](adr/README.md)
3. Review [DNS Query Flow](diagrams/04-query-flow.md)

### Operations

1. [Performance Tuning Guide](guides/performance-tuning.md) - Configure for production
2. [Error Recovery Flow](diagrams/05-error-recovery.md) - Understand failure handling
3. [ADR-004: Graceful Degradation](adr/004-graceful-degradation.md) - Know the behavior

## Key Concepts

### Plugin Architecture

The Elchi GSLB plugin follows these design principles:

1. **Fast DNS queries** (<1ms) via in-memory cache
2. **Graceful degradation** - serve stale data during controller outages
3. **Hash-based sync** - efficient change detection
4. **Optional webhook** - instant updates for critical changes
5. **Thread-safe** - concurrent query handling

### Data Flow

```
[Elchi Controller]
       ↓ (periodic sync + webhook)
[In-Memory Cache]
       ↓ (<1ms lookup)
[DNS Response]
```

### Update Mechanisms

1. **Periodic Sync** (default: 5 min)
   - Hash-based change detection
   - HTTP 304 for unchanged data
   - Full snapshot on changes

2. **Webhook** (optional)
   - Instant updates (<1s)
   - Partial record updates
   - Falls back to periodic sync

## Performance Characteristics

| Metric | Value | Notes |
|--------|-------|-------|
| Query latency | 50-100μs | Cache hit |
| Throughput | 50,000+ QPS | Single instance |
| Memory | ~100 bytes/record | Includes pre-built RRs |
| Sync latency | 1-5ms | Per 10k records |
| Update latency | <1s | With webhook |

See [Performance Tuning Guide](guides/performance-tuning.md) for details.

## API Reference

### Corefile Configuration

```
gslb.elchi {
    elchi {
        endpoint http://controller:8080  # Required
        secret my-secret-key             # Required
        sync_interval 5m                 # Optional, default: 5m
        timeout 10s                      # Optional, default: 10s
        ttl 300                          # Optional, default: 300s
        webhook :8053                    # Optional, enables instant updates
    }
}
```

### Controller API

The plugin consumes these endpoints:

- `GET /dns/snapshot?zone=<zone>` - Initial snapshot
- `GET /dns/changes?zone=<zone>&since=<hash>` - Check for changes

### Webhook API

The plugin exposes these endpoints:

- `POST /notify` - Receive instant updates
- `GET /health` - Health check
- `GET /records` - Inspect cached records

See [CLAUDE.md](../CLAUDE.md) for full API specification.

## Monitoring

### Prometheus Metrics

```
elchi_cache_size              # Number of cached records
elchi_query_total             # Total queries handled
elchi_query_duration_seconds  # Query latency histogram
elchi_sync_success_total      # Successful syncs
elchi_sync_failure_total      # Failed syncs
elchi_cache_age_seconds       # Time since last sync
```

### Recommended Alerts

```yaml
- alert: ElchiSyncFailing
  expr: elchi_sync_failure_total > 3
  annotations:
    summary: "Elchi sync failing repeatedly"

- alert: ElchiCacheStale
  expr: elchi_cache_age_seconds > 900
  annotations:
    summary: "Elchi cache is stale (>15min)"
```

## Troubleshooting

### Common Issues

1. **"Unknown directive 'elchi'"**
   - Cause: Plugin not registered in CoreDNS
   - Solution: Use `make build` or follow [build instructions](../README.md#building)

2. **DNS queries timing out**
   - Cause: Plugin not in Corefile or wrong zone
   - Solution: Check Corefile configuration

3. **Stale data being served**
   - Cause: Controller unavailable, sync failing
   - Solution: Check `/health` endpoint, controller connectivity

4. **High memory usage**
   - Cause: Large number of records
   - Solution: Normal, ~100 bytes per record. See [performance guide](guides/performance-tuning.md)

### Debug Steps

```bash
# 1. Check plugin health
curl http://localhost:8053/health

# 2. Inspect cached records
curl -H "X-Elchi-Secret: your-secret" http://localhost:8053/records

# 3. Test DNS query
dig @localhost -p 1053 test.gslb.elchi

# 4. Check CoreDNS logs
kubectl logs -l app=coredns

# 5. Verify controller connectivity
kubectl exec coredns-pod -- curl http://controller:8080/dns/snapshot?zone=gslb.elchi
```

## Development

### Project Structure

```
elchi-gslb/
├── docs/               # Documentation (you are here)
│   ├── adr/           # Architecture decisions
│   ├── diagrams/      # Sequence diagrams
│   └── guides/        # Usage guides
├── elchi.go           # Main plugin logic
├── cache.go           # In-memory cache
├── client.go          # HTTP client for controller
├── webhook.go         # Webhook server
├── setup.go           # CoreDNS setup
├── *_test.go          # Tests
├── integration_test.go # Integration tests
└── mock-controller/   # Test mock server
```

### Testing

```bash
# Unit tests
make test

# Integration tests
make test-integration

# All tests
make test-all

# With race detector
make test-race
```

### Contributing

See [CONTRIBUTING.md](../CONTRIBUTING.md) (if exists) or open an issue.

## Resources

### External Documentation

- [CoreDNS Plugin Development](https://coredns.io/manual/plugins-dev/)
- [DNS RFC 1035](https://tools.ietf.org/html/rfc1035)
- [Kubernetes DNS](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/)

### Related Projects

- [Elchi Controller](https://github.com/cloudnativeworks/elchi) - GSLB control plane
- [CoreDNS](https://github.com/coredns/coredns) - DNS server
- [miekg/dns](https://github.com/miekg/dns) - DNS library

## Support

- **Issues**: [GitHub Issues](https://github.com/cloudnativeworks/elchi-gslb/issues)
- **Discussions**: [GitHub Discussions](https://github.com/cloudnativeworks/elchi-gslb/discussions)
- **Slack**: #elchi channel (if available)

## License

Apache 2.0 - See [LICENSE](../LICENSE) for details.

# Elchi GSLB - CoreDNS Plugin

CoreDNS plugin for Global Server Load Balancing (GSLB) integration with Elchi Controller.

## Name

*elchi* - provides dynamic DNS resolution for Global Server Load Balancing zones managed by the Elchi Controller

## Description

The *elchi* plugin integrates CoreDNS with the Elchi backend controller to provide dynamic DNS responses for GSLB zones. It maintains an in-memory cache of DNS records that is synchronized periodically from the Elchi backend using hash-based change detection.

**Note**: The zone name is completely flexible - you can use any zone you control (e.g., `gslb.example.org`, `lb.mycompany.com`, `dns.internal`, etc.). The examples use `gslb.elchi` and `gslb.example.org` for illustration purposes only.

The plugin answers DNS queries from a pre-built cache synchronized from an external API, providing:
- Hash-based change detection (only updates when backend data changes)
- Periodic background sync (default: 5 minutes)
- Instant updates via optional webhook endpoint
- Thread-safe cache operations with minimal lock duration
- Graceful degradation on backend failures (continues serving stale data)
- Support for A and AAAA record types

## Syntax

~~~ txt
*elchi* {
    endpoint **URL**
    secret **KEY**
    [ttl **SECONDS**]
    [sync_interval **DURATION**]
    [timeout **DURATION**]
    [webhook [**ADDRESS**]]
    [fallthrough [**ZONES**...]]
}
~~~

- **URL** is the Elchi backend API endpoint (required)
- **KEY** is the shared secret for authentication, must match ELCHI_JWT_SECRET in backend (required, minimum 8 characters)
- **SECONDS** is the default TTL for DNS records without explicit TTL (optional, default: 300)
- **DURATION** is a Go duration string (e.g., `5m`, `30s`) for sync_interval or timeout
  - **sync_interval** specifies how often to check for changes (optional, default: `5m`, minimum: `1m`)
  - **timeout** specifies HTTP request timeout (optional, default: `10s`, minimum: `1s`)
- **ADDRESS** is the webhook server listen address (optional, default: `:8053`)
- **ZONES** are zones to fall through for (optional, defaults to all zones if fallthrough is enabled)

## Examples

Minimal configuration for zone `gslb.example.org`:

~~~ corefile
gslb.example.org {
    elchi {
        endpoint http://elchi-backend:8080
        secret my-shared-secret
    }
}
~~~

Full configuration with all options:

~~~ corefile
gslb.example.org {
    elchi {
        endpoint http://elchi-backend:8080
        secret my-shared-secret
        ttl 300
        sync_interval 5m
        timeout 10s
        webhook :8053
        fallthrough
    }
    ready
    prometheus :9253
    log
    errors
}

. {
    forward . 8.8.8.8
}
~~~

Multiple zones with different backends:

~~~ corefile
gslb.prod.example.org {
    elchi {
        endpoint http://elchi-prod:8080
        secret prod-secret
    }
}

gslb.staging.example.org {
    elchi {
        endpoint http://elchi-staging:8080
        secret staging-secret
    }
}
~~~

CNAME-based regional failover:

~~~ corefile
# Asia region
asya-gslb.elchi {
    elchi {
        endpoint http://elchi-asia:8080
        secret asia-secret
    }
}

# Europe region (backup)
avrupa-gslb.elchi {
    elchi {
        endpoint http://elchi-europe:8080
        secret europe-secret
    }
}
~~~

When the Asia region is disabled (e.g., during maintenance), the controller sends:

```json
{
  "name": "service.asya-gslb.elchi",
  "type": "A",
  "ttl": 20,
  "ips": ["10.10.1.20", "10.10.1.21"],
  "enabled": false,
  "failover": "service.avrupa-gslb.elchi"
}
```

DNS clients querying `service.asya-gslb.elchi` will receive a CNAME to `service.avrupa-gslb.elchi` and automatically resolve to the Europe region IPs.

## Architecture

```
┌─────────────┐         ┌──────────────┐         ┌─────────────────┐
│   DNS       │  Query  │   CoreDNS    │  HTTP   │  Elchi Backend  │
│   Client    │────────▶│   + Elchi    │────────▶│   Controller    │
│             │         │   Plugin     │         │   (/dns/*)      │
└─────────────┘         └──────────────┘         └─────────────────┘
                               │                          │
                               │  Periodic Sync (5m)      │
                               │  Check: hash changed?    │
                               │◀─────────────────────────┘
                               │
                               ▼
                        ┌──────────────┐
                        │  DNS Record  │
                        │  Cache       │
                        │  (in-memory) │
                        └──────────────┘
```

### Sync Mechanism

1. **Initial Load**: On startup, fetches complete DNS snapshot from `/dns/snapshot?zone={zone}`
2. **Periodic Check**: Every 5 minutes (configurable), calls `/dns/changes?zone={zone}&since={hash}`
3. **Change Detection**: If hash differs, fetches new snapshot and replaces cache atomically
4. **Query Handling**: DNS queries are answered from pre-built in-memory cache (no backend calls on hot path)

## Quick Start

### Option 1: Docker (Recommended)

```bash
# Pull the image
docker pull cloudnativeworks/elchi-coredns:latest

# Create Corefile
cat > Corefile <<EOF
gslb.elchi {
    elchi {
        endpoint http://elchi-backend:8080
        secret your-secret-key
        sync_interval 5m
        webhook :8053
    }
    log
    errors
}
EOF

# Run
docker run -d \
  --name elchi-coredns \
  -p 53:53/udp \
  -p 53:53/tcp \
  -p 8053:8053 \
  -v $(pwd)/Corefile:/etc/coredns/Corefile \
  cloudnativeworks/elchi-coredns:latest
```

### Option 2: Build from Source

#### 1. Setup

```bash
make setup
```

This will:
- Clone CoreDNS v1.13.2 into `./coredns/`
- Create `Corefile` from `Corefile.example`
- Download Go dependencies

#### 2. Configure

Edit `Corefile` with your settings:

```
gslb.example.org {
    elchi {
        endpoint http://localhost:8080
        secret your-secret-key-here
        sync_interval 5m
        timeout 10s
        ttl 300
        fallthrough
    }
    log
    errors
}

. {
    forward . 8.8.8.8
    log
    errors
}
```

### 3. Run (No Build Required!)

```bash
make run
```

This runs CoreDNS via `go run` with sudo (required for port 53 on macOS). No build step needed!

**Alternative: Build Binary First (Optional)**

If you prefer to use a compiled binary:

```bash
make build      # Creates ./coredns-elchi binary
make run-build  # Run from binary
```

The build process:
- Registers the elchi plugin in CoreDNS `plugin.cfg`
- Uses `go mod replace` to use local plugin code
- Builds CoreDNS with the elchi plugin included
- Creates `./coredns-elchi` binary

### 4. Test

In another terminal:

```bash
make query
# Or manually:
dig @localhost -p 53 project1.gslb.elchi A
```

## Authentication

The plugin uses **simple secret header authentication**:

```http
X-Elchi-Secret: <shared-secret>
```

The `secret` in Corefile must match `ELCHI_JWT_SECRET` environment variable in the Elchi backend.

This is suitable for pod-to-pod communication within Kubernetes where network traffic is already secured. HTTPS is not required.

## Backend API Specification

The Elchi backend must implement these DNS API endpoints:

### GET /dns/snapshot

Fetches the complete DNS snapshot for a zone.

**Query Parameters:**
- `zone` (required) - DNS zone name (e.g., `gslb.elchi`)

**Request Headers:**
```
X-Elchi-Secret: <ELCHI_JWT_SECRET>
Accept: application/json
```

**Response (200 OK):**
```json
{
  "zone": "gslb.elchi",
  "version_hash": "abc123def456",
  "records": [
    {
      "name": "listener1.gslb.elchi",
      "type": "A",
      "ttl": 300,
      "ips": ["192.168.1.10", "192.168.1.11"]
    },
    {
      "name": "listener2.gslb.elchi",
      "type": "AAAA",
      "ttl": 600,
      "ips": ["2001:db8::1", "2001:db8::2"]
    }
  ]
}
```

**Response Fields:**
- `zone` - DNS zone name (must match request)
- `version_hash` - Opaque hash string for change detection
- `records` - Array of DNS records

**Record Fields:**
- `name` - Fully qualified domain name (FQDN)
- `type` - Record type ("A" or "AAAA")
- `ttl` - Time-to-live in seconds (0 = use default)
- `ips` - Array of IP address strings

**Error Responses:**
- `400 Bad Request` - Invalid zone or missing parameters
- `401 Unauthorized` - Invalid or missing secret
- `404 Not Found` - Zone not found
- `500 Internal Server Error` - Server error

### GET /dns/changes

Checks for DNS changes since a given version hash.

**Query Parameters:**
- `zone` (required) - DNS zone name
- `since` (required) - Last known version_hash

**Request Headers:** Same as `/dns/snapshot`

**Response - No Changes (200 OK):**
```json
{
  "unchanged": true
}
```

**Response - Has Changes (200 OK):**
```json
{
  "unchanged": false,
  "zone": "gslb.elchi",
  "version_hash": "xyz789abc012",
  "records": [
    // Full snapshot of current records
  ]
}
```

**Note:** When `unchanged=false`, the response includes the complete new snapshot, not a diff. The plugin replaces the entire cache atomically.

**Error Responses:** Same as `/dns/snapshot`

### Backend Implementation Notes

1. **Version Hash Generation:**
   - Generate a hash (e.g., SHA256) of the current DNS records
   - Hash should change whenever records are added, removed, or modified
   - Hash can be based on record content, timestamps, or database version

2. **Authentication:**
   - Validate `X-Elchi-Secret` header matches `ELCHI_JWT_SECRET`
   - Zone information comes from the `zone` query parameter
   - Return 401 if authentication fails

3. **Performance:**
   - Plugin queries `/dns/changes` every 5 minutes
   - Optimize for quick hash comparison
   - Only generate full snapshot when hash differs

4. **Record Generation:**
   - Records should be within the specified zone
   - IPs must be valid IPv4 (for A) or IPv6 (for AAAA)
   - TTL of 0 means use plugin default

## Plugin Webhook Endpoints

The plugin can optionally expose webhook endpoints for instant updates, health monitoring, and record inspection. Enable with the `webhook` directive in Corefile.

### Configuration

```
gslb.elchi {
    elchi {
        endpoint http://localhost:8080
        secret your-secret-key-here
        webhook :8053  # Enable webhook server on port 8053
    }
}
```

The `webhook` directive accepts an optional address parameter (default: `:8053`).

### POST /notify

Webhook endpoint for instant DNS record updates from the Elchi controller. Allows pushing changes without waiting for the periodic sync interval.

**Authentication:** Requires `X-Elchi-Secret` header

**Request Body (Updates):**
```json
{
  "records": [
    {
      "name": "new.gslb.elchi",
      "type": "A",
      "ttl": 300,
      "ips": ["192.168.3.10"]
    }
  ]
}
```

**Request Body (Deletes):**
```json
{
  "deletes": [
    {
      "name": "old.gslb.elchi",
      "type": "A"
    }
  ]
}
```

**Request Body (Mixed):**
```json
{
  "records": [
    {"name": "updated.gslb.elchi", "type": "A", "ttl": 300, "ips": ["192.168.4.10"]}
  ],
  "deletes": [
    {"name": "removed.gslb.elchi", "type": "A"}
  ]
}
```

**Response (200 OK):**
```json
{
  "status": "ok",
  "updated": 1,
  "deleted": 1
}
```

**Usage:**
```bash
curl -X POST http://localhost:8053/notify \
  -H "X-Elchi-Secret: your-secret-key" \
  -H "Content-Type: application/json" \
  -d '{"records": [{"name": "test.gslb.elchi", "type": "A", "ttl": 300, "ips": ["192.168.1.10"]}]}'
```

### GET /health

Health check endpoint that returns plugin status, cache statistics, and sync status.

**Authentication:** None required (public endpoint)

**Response (200 OK - Healthy):**
```json
{
  "status": "healthy",
  "zone": "gslb.elchi",
  "records_count": 42,
  "version_hash": "abc123",
  "last_sync": "2025-12-31T08:30:00Z",
  "last_sync_status": "success"
}
```

**Response (503 Service Unavailable - Degraded):**
```json
{
  "status": "degraded",
  "zone": "gslb.elchi",
  "records_count": 42,
  "version_hash": "abc123",
  "last_sync": "2025-12-31T08:00:00Z",
  "last_sync_status": "failed",
  "error": "connection timeout"
}
```

**Health Status Logic:**
- `healthy`: Last sync succeeded OR last failure was recent (< 2x sync_interval)
- `degraded`: Last sync failed AND it's been > 2x sync_interval since last success

**Usage:**
```bash
curl http://localhost:8053/health
```

### GET /records

Returns currently cached DNS records, with optional filtering by name or type.

**Authentication:** Requires `X-Elchi-Secret` header

**Query Parameters:**
- `name` (optional) - Filter by domain name (substring match)
- `type` (optional) - Filter by record type (A or AAAA)

**Response (200 OK):**
```json
{
  "zone": "gslb.elchi",
  "version_hash": "abc123",
  "count": 2,
  "records": [
    {
      "name": "test1.gslb.elchi",
      "type": "A",
      "ttl": 300,
      "ips": ["192.168.1.10"]
    },
    {
      "name": "test2.gslb.elchi",
      "type": "A",
      "ttl": 300,
      "ips": ["192.168.2.10"]
    }
  ]
}
```

**Usage:**
```bash
# Get all records
curl -H "X-Elchi-Secret: your-secret-key" http://localhost:8053/records

# Filter by name
curl -H "X-Elchi-Secret: your-secret-key" http://localhost:8053/records?name=test1

# Filter by type
curl -H "X-Elchi-Secret: your-secret-key" http://localhost:8053/records?type=A
```

### Webhook Integration Workflow

1. **Periodic Sync (Default):**
   - Every 5 minutes, plugin checks `/dns/changes` for updates
   - Only fetches full snapshot if hash changed

2. **Instant Updates (Optional):**
   - Enable webhook server with `webhook` directive
   - Controller can push updates immediately via POST /notify
   - Updates are merged with existing cache (partial updates)
   - No need to wait for next sync interval

3. **Monitoring:**
   - Use GET /health for automated health checks
   - Monitor `last_sync_status` and `records_count`
   - Alert on `degraded` status

## Development

### Project Structure

```
elchi-gslb/
├── elchi.go              # Main plugin logic
├── setup.go              # Configuration parsing
├── client.go             # Elchi backend HTTP client
├── cache.go              # Thread-safe DNS record cache
├── webhook.go            # Webhook server and endpoints
├── *_test.go             # Unit and integration tests
├── coredns/              # CoreDNS clone (created by make setup)
├── Corefile.example      # Example configuration
├── Makefile              # Development commands
├── go.mod                # Go module definition
└── README.md             # This file
```

### Running Tests

```bash
make test
```

This runs all unit tests including:
- Client tests (HTTP mocking)
- Cache tests (concurrency, benchmarks)
- Integration tests (ServeDNS)

### Building

```bash
make build
```

This:
1. Clones CoreDNS (if not already cloned)
2. Registers elchi plugin in CoreDNS `plugin.cfg` (after `kubernetes` plugin)
3. Uses `go mod replace` to use local plugin code
4. Builds CoreDNS with elchi plugin
5. Creates `./coredns-elchi` binary

**Note**: The build process modifies `coredns/plugin.cfg` and `coredns/go.mod` to include the elchi plugin.

### Debug Logging

Enable debug logs in Corefile:

```
gslb.elchi {
    elchi { ... }
    log
    errors
}
```

Watch logs:
```bash
make run
# Logs show:
# [INFO] Initial snapshot loaded: 42 records, hash=abc123
# [DEBUG] Checking for changes since hash=abc123
# [DEBUG] No changes detected
```

## Troubleshooting

### Plugin doesn't start

**Check:** Corefile syntax and zone format
```bash
# Zone must be in server block declaration
gslb.elchi {  # ← Zone here
    elchi {
        # NOT here
    }
}
```

### "secret is required" error

**Fix:** Add secret to Corefile:
```
elchi {
    secret your-secret-key-here  # ← Must be at least 8 chars
}
```

### "Initial snapshot fetch failed"

**Possible causes:**
1. Backend not running → Start Elchi backend
2. Wrong endpoint → Check `endpoint` in Corefile
3. Wrong secret → Verify matches ELCHI_JWT_SECRET
4. Network issue → Check connectivity

**Note:** Plugin continues running and retries in background.

### No DNS responses

1. **Check cache is populated:**
   - Look for "Initial snapshot loaded" in logs
   - If not, check backend API

2. **Check query domain matches zone:**
   ```bash
   # Corefile zone: gslb.elchi
   # Query must be: *.gslb.elchi
   dig @localhost test.gslb.elchi A
   ```

3. **Check record exists in backend:**
   ```bash
   curl -H "X-Elchi-Secret: <secret>" \
     "http://localhost:8080/dns/snapshot?zone=gslb.elchi"
   ```

### "Permission denied" on port 53

**Solution:** Use sudo
```bash
sudo go run cmd/coredns/main.go -conf Corefile
# Or:
make run  # Already includes sudo
```

### Changes not appearing

**Possible causes:**
1. **Sync interval not elapsed** → Wait 5 minutes or restart
2. **Hash unchanged** → Verify backend actually changed data
3. **Backend error** → Check logs for "Change check failed"

## How It Works

### Record Mapping

The plugin maps Elchi DNS records to CoreDNS responses:

**Backend Record:**
```json
{
  "name": "api.gslb.elchi",
  "type": "A",
  "ttl": 300,
  "ips": ["192.168.1.10", "192.168.1.11"]
}
```

**DNS Query:**
```bash
dig @localhost api.gslb.elchi A
```

**DNS Response:**
```
api.gslb.elchi. 300 IN A 192.168.1.10
api.gslb.elchi. 300 IN A 192.168.1.11
```

### Cache Behavior

- **Pre-built Records:** DNS RR objects built during sync, not during query
- **Atomic Updates:** Entire cache replaced atomically on changes
- **Thread-Safe:** Concurrent queries + background sync are safe
- **Version Tracking:** Stores current version_hash for change detection

### Zone Matching

- Plugin only answers queries **within configured zone**
- Queries outside zone → passed to next plugin
- Zone format: `gslb.elchi`
- Supports multiple zones by configuring multiple server blocks

## Ready

This plugin implements the `ready` plugin's readiness interface. It reports ready when:
- The cache has been initialized
- At least one successful sync has occurred (version_hash is not empty)

The plugin will report not ready during:
- Initial startup before first successful sync
- If the cache is nil (initialization failed)

This integrates with Kubernetes readiness probes when used with the CoreDNS `ready` plugin:

~~~ corefile
gslb.example.org {
    elchi {
        endpoint http://elchi-backend:8080
        secret my-secret
    }
    ready
}
~~~

The `ready` plugin will serve readiness checks on `:8181/ready` by default. The *elchi* plugin will be considered ready once the initial DNS snapshot has been successfully loaded.

## Metrics

The *elchi* plugin exports Prometheus metrics following CoreDNS naming standards. All metrics use the `coredns_elchi_` prefix.

### DNS Query Metrics

- **`coredns_elchi_requests_total{zone, type}`** (Counter)
  - Total number of DNS requests handled by the plugin
  - Labels: `zone` (DNS zone), `type` (query type: A, AAAA, etc.)

- **`coredns_elchi_cache_hits_total{zone, type}`** (Counter)
  - Total number of successful cache lookups
  - Labels: `zone`, `type`

- **`coredns_elchi_cache_misses_total{zone, type}`** (Counter)
  - Total number of failed cache lookups
  - Labels: `zone`, `type`

### Synchronization Metrics

- **`coredns_elchi_sync_duration_seconds{zone, type}`** (Histogram)
  - Duration of backend sync operations in seconds
  - Labels: `zone`, `type` (operation type: "snapshot" or "changes")
  - Buckets: 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10

- **`coredns_elchi_sync_errors_total{zone, type}`** (Counter)
  - Total number of backend sync errors
  - Labels: `zone`, `type` ("snapshot" or "changes")

### Cache Metrics

- **`coredns_elchi_cache_size{zone}`** (Gauge)
  - Number of DNS records currently in cache
  - Labels: `zone`

### Webhook Metrics

- **`coredns_elchi_webhook_requests_total{endpoint, status}`** (Counter)
  - Total number of webhook requests received
  - Labels: `endpoint` ("health", "records", "notify"), `status` ("success", "error", "unauthorized")

### Accessing Metrics

Configure Prometheus endpoint in your Corefile:

```
gslb.elchi {
    elchi {
        endpoint http://localhost:8080
        secret your-secret-key
    }
    prometheus localhost:9253
}
```

Metrics are available at `http://localhost:9253/metrics`.

### Example PromQL Queries

```promql
# Cache hit rate by query type
sum(rate(coredns_elchi_cache_hits_total[5m])) by (type) /
sum(rate(coredns_elchi_requests_total[5m])) by (type)

# Sync error rate
rate(coredns_elchi_sync_errors_total[5m])

# 95th percentile sync duration
histogram_quantile(0.95, rate(coredns_elchi_sync_duration_seconds_bucket[5m]))

# Current cache size
coredns_elchi_cache_size

# Webhook unauthorized attempts
rate(coredns_elchi_webhook_requests_total{status="unauthorized"}[5m])
```

## Production Deployment

### Docker

Pre-built multi-architecture Docker images are available on Docker Hub:

```bash
# Pull latest version
docker pull cloudnativeworks/elchi-coredns:latest

# Pull specific version
docker pull cloudnativeworks/elchi-coredns:v0.1.0

# Run with custom Corefile
docker run -d \
  --name elchi-coredns \
  -p 53:53/udp \
  -p 53:53/tcp \
  -p 8053:8053 \
  -v $(pwd)/Corefile:/etc/coredns/Corefile \
  cloudnativeworks/elchi-coredns:latest
```

**Supported architectures:**
- `linux/amd64`
- `linux/arm64`

### Kubernetes Example

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: coredns-elchi
data:
  Corefile: |
    gslb.elchi {
        elchi {
            endpoint http://elchi-backend:8080
            secret ${ELCHI_SECRET}
            sync_interval 5m
            webhook :8053
        }
        prometheus :9253
        errors
        log
    }
    . {
        forward . /etc/resolv.conf
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: coredns-elchi
spec:
  replicas: 3
  selector:
    matchLabels:
      app: coredns-elchi
  template:
    metadata:
      labels:
        app: coredns-elchi
    spec:
      containers:
      - name: coredns
        image: cloudnativeworks/elchi-coredns:latest
        ports:
        - containerPort: 53
          protocol: UDP
        - containerPort: 53
          protocol: TCP
        - containerPort: 9253
          name: metrics
        env:
        - name: ELCHI_SECRET
          valueFrom:
            secretKeyRef:
              name: elchi-credentials
              key: secret
        volumeMounts:
        - name: config
          mountPath: /etc/coredns
      volumes:
      - name: config
        configMap:
          name: coredns-elchi
```

## Performance

- **Query Latency:** <1ms (in-memory cache lookup)
- **Sync Overhead:** Minimal (only when hash changes)
- **Memory:** ~100 bytes per DNS record
- **Throughput:** >10K queries/sec on modern hardware

## Bugs

- The plugin currently only supports A and AAAA record types. Other record types (CNAME, MX, TXT, etc.) are not supported.
- When the backend is unreachable at startup, the plugin continues with an empty cache and serves NXDOMAIN for all queries until the first successful sync.
- The webhook server does not support TLS. It is designed for internal pod-to-pod communication in Kubernetes where network traffic is already secured.

## Documentation

Comprehensive documentation is available in the [docs/](docs/) directory:

### Architecture & Design

- **[Architecture Decision Records (ADRs)](docs/adr/)** - Understand design decisions
  - [In-Memory Cache Usage](docs/adr/001-in-memory-cache.md) - Why cache instead of proxying
  - [Hash-Based Sync](docs/adr/002-hash-based-sync.md) - Efficient change detection
  - [Webhook Architecture](docs/adr/003-webhook-architecture.md) - Instant updates
  - [Graceful Degradation](docs/adr/004-graceful-degradation.md) - Fault tolerance
  - [Pre-Built DNS Records](docs/adr/005-pre-built-records.md) - Performance optimization

- **[Sequence Diagrams](docs/diagrams/)** - Visual flow documentation
  - [Startup Flow](docs/diagrams/01-startup-flow.md) - Plugin initialization
  - [Periodic Sync](docs/diagrams/02-periodic-sync.md) - Background synchronization
  - [Webhook Updates](docs/diagrams/03-webhook-flow.md) - Instant propagation
  - [DNS Query Handling](docs/diagrams/04-query-flow.md) - Query path
  - [Error Recovery](docs/diagrams/05-error-recovery.md) - Failure handling

### Guides

- **[Performance Tuning Guide](docs/guides/performance-tuning.md)** - Optimize for your workload
  - Configuration parameters explained
  - Resource requirements (CPU/memory)
  - Benchmarking instructions
  - Troubleshooting performance issues

### Quick Links

- [Full Documentation Index](docs/README.md)

## License

Apache License 2.0 - See [LICENSE](LICENSE) file for details.

## Contributing

Contributions welcome! Please ensure:
- Tests pass: `make test`
- Code follows Go best practices
- Documentation updated

## Support

- Issues: [GitHub Issues](https://github.com/your-org/elchi-gslb/issues)

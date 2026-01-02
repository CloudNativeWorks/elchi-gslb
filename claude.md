# Elchi CoreDNS Plugin – Project Context

This repository contains a CoreDNS plugin called **`elchi`** (registered as "elchi" in CoreDNS).

The purpose of this plugin is to integrate CoreDNS with the **Elchi** control plane and provide GSLB-like DNS answers for per-project zones, typically:

- `gslb.elchi`

For each listener created in Elchi, a unique hostname is generated (e.g. `listener123.gslb.elchi.`).
Other DNS zones (internal or external) create **CNAME records** pointing to these hostnames.
This plugin answers queries for `*.gslb.elchi` from an in-memory cache that is synchronized from Elchi over **REST + JSON**.

---

## High-level design

- The plugin is named `elchi`.
- It is authoritative only for a configured zone (e.g. `gslb.elchi`).
- It **never calls Elchi on the hot query path** (DNS queries).
- Instead, it maintains an in-memory cache of DNS records, fetched periodically from Elchi.
- The plugin provides management endpoints for health checks, record inspection, and webhook-based updates.

### Sync mechanism

1. **On startup** (Initial bulk load):
   - Perform an **initial full snapshot** fetch from Elchi at:
     ```
     GET {api_endpoint}/dns/snapshot?zone={zone}
     ```
   - The response includes:
     - `zone` (string) - DNS zone name
     - `version_hash` (string, opaque) - Hash for change detection
     - `records` (array of records with `name`, `type`, `ttl`, `ips`)

   - The plugin builds an internal cache of all records and stores the `version_hash`.
   - **All requests include authentication header:** `X-Elchi-Secret: <secret>`

2. **Background periodic sync** (Every 5 minutes by default):
   - A goroutine runs in a loop with a configured `sync_interval` (default: 5 minutes).
   - Each cycle:
     - Calls:
       ```
       GET {api_endpoint}/dns/changes?zone={zone}&since={version_hash}
       Header: X-Elchi-Secret: <secret>
       ```

     - **If controller returns HTTP 304 (Not Modified)**: No changes, plugin continues with current cache.
     - **If controller returns HTTP 200 with data**:
       - Response includes: `{unchanged: false, version_hash: "new-hash", records: [...]}`
       - The plugin replaces the entire cache from this new snapshot and updates `version_hash`.

   - **Error resilience**:
     - If fetch fails (timeout, HTTP error, network error), log the error.
     - **Continue serving the last known good cache** (records never expire).
     - Retry on next sync cycle.

3. **Webhook-based instant updates** (Optional real-time updates):
   - The plugin exposes a webhook endpoint:
     ```
     POST /notify
     Header: X-Elchi-Secret: <secret>
     Body: {
       "records": [
         {"name": "...", "type": "A", "ttl": 300, "ips": ["..."]}
       ]
     }
     ```

   - Controller can push updates for **specific records** without waiting for sync interval.
   - **Partial update**: Only the records in the notification are updated (merged with existing cache).
   - Format is same as bulk snapshot, but fewer records.
   - Used for instant propagation of critical changes.

4. **Delete operations**:
   - Webhook endpoint also supports deletions:
     ```
     POST /notify
     Body: {
       "deletes": [
         {"name": "old.gslb.elchi", "type": "A"}
       ]
     }
     ```

   - Specified records are removed from cache.
   - Can be combined with updates in same request.

5. **DNS query handling**:
   - On `ServeDNS`:
     - If `qname` is under the configured zone:
       - Look up `(qname, qtype)` in the cache.
       - Build a response from the cached **pre-built** `dns.RR` objects.
       - Use the **TTL from the record** (learned from controller).
       - If not found, fall through to the next plugin.
     - If `qname` is outside the zone, immediately delegate to `Next`.

This design ensures:
- **Fast queries** (in-memory cache only, pre-built RR objects).
- **Minimal dependency** on Elchi during runtime (stale data is better than no data).
- **Periodic hash-based synchronization** for configuration changes.
- **Instant updates** via webhook for critical changes.
- **Graceful degradation** on controller failures.

---

## REST API specification

### Authentication

**All API requests include:**
```
X-Elchi-Secret: <shared-secret>
```

The secret must match the `ELCHI_JWT_SECRET` environment variable in the controller.

### Endpoints the plugin consumes

#### 1. GET /dns/snapshot

Fetches the complete DNS snapshot for initial load.

**Query Parameters:**
- `zone` (required) - DNS zone name (e.g., `gslb.elchi`)

**Request Headers:**
```
X-Elchi-Secret: <secret>
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
    }
  ]
}
```

#### 2. GET /dns/changes

Checks for DNS changes since a given version hash.

**Query Parameters:**
- `zone` (required)
- `since` (required) - Last known version_hash

**Request Headers:** Same as snapshot

**Response - No changes (304 Not Modified):**
```
HTTP/1.1 304 Not Modified
```

**Response - Has changes (200 OK):**
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

**Note:** When controller detects changes, it returns the complete new snapshot, not a diff.

### Endpoints the plugin exposes

#### 1. POST /notify

Webhook endpoint for instant updates from controller.

**Request Headers:**
```
X-Elchi-Secret: <secret>
Content-Type: application/json
```

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

#### 2. GET /health

Health check endpoint.

**Response (200 OK):**
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

**Response (503 Service Unavailable):**
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

#### 3. GET /records

Returns current cached records.

**Query Parameters:**
- `name` (optional) - Filter by domain name
- `type` (optional) - Filter by record type (A, AAAA)

**Request Headers:**
```
X-Elchi-Secret: <secret>
```

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

---

## Detailed requirements

### 1. Authentication on all requests
- Plugin **must** include `X-Elchi-Secret` header on every request to controller.
- Controller validates the secret matches `ELCHI_JWT_SECRET`.
- Plugin webhook endpoints also require this header for authorization.

### 2. Graceful degradation on errors
- If snapshot/changes fetch fails (HTTP error, timeout, network error):
  - Log the error clearly.
  - **Continue serving last known good cache**.
  - Records **never expire** (no TTL-based expiration in cache).
  - Retry on next sync cycle.

### 3. Use record TTL from controller
- Each record has a `ttl` field from controller JSON.
- Plugin must use this TTL when building `dns.RR` objects.
- If TTL is 0 or missing, use plugin default (configurable, default: 300).

### 4. Initial bulk load on startup
- Plugin **must** fetch full snapshot on startup via `/dns/snapshot`.
- Build complete cache before serving DNS queries.
- If initial fetch fails, log warning but continue (empty cache).
- Background sync will retry.

### 5. Periodic sync with hash-based change detection
- Every 5 minutes (configurable), check for changes via `/dns/changes?since={hash}`.
- Send current `version_hash` to controller.
- If controller returns **304 Not Modified**: No action, continue with current cache.
- If controller returns **200 OK** with new data: Replace entire cache atomically.

### 6. Webhook endpoint for instant updates
- Expose `POST /notify` endpoint.
- Accept partial record updates (not full snapshot).
- **Merge** incoming records with existing cache (update specific records).
- Format same as snapshot `records` array, but fewer records.
- Requires authentication via `X-Elchi-Secret` header.

### 7. Health endpoint
- Expose `GET /health` endpoint.
- Returns:
  - Status (healthy/degraded)
  - Current record count
  - Version hash
  - Last sync timestamp
  - Last sync status (success/failed)
- Returns 503 if plugin is degraded (consecutive sync failures, but still serving stale data).

### 8. Records inspection endpoint
- Expose `GET /records` endpoint.
- Returns current cached records in JSON format.
- Supports filtering by `name` and `type` query parameters.
- Requires authentication.

### 9. Delete operations via webhook
- `POST /notify` accepts `deletes` array.
- Each delete specifies `{name, type}`.
- Removes matching records from cache.
- Can be combined with updates in same request.

### 10. Comprehensive testing
- Unit tests for all components:
  - Cache operations (get, update, delete, concurrent access)
  - Client HTTP methods (snapshot, changes, auth)
  - Webhook handlers (notify, health, records)
  - DNS serving logic
- Integration tests:
  - Full sync lifecycle
  - Webhook updates/deletes
  - Error scenarios
  - Concurrent access
- Benchmarks for hot paths (cache get, DNS serving).

### 11. No code duplication
- Extract common logic into helper functions.
- Reuse authentication logic.
- Reuse record building logic.
- Reuse JSON parsing/validation.

### 12. Modern Go syntax
- Use `any` instead of `interface{}`.
- Use generics where appropriate.
- Use modern error handling patterns.
- Use modern HTTP client patterns.
- Follow Go 1.23 best practices.

### 13. Comprehensive error coverage
- Handle all error scenarios:
  - Network timeouts
  - HTTP errors (4xx, 5xx)
  - Invalid JSON
  - Invalid IPs
  - Concurrent access
  - Cache overflow (if applicable)
- Never panic in production code.
- All errors logged clearly.
- All errors handled gracefully.

---

## Coding guidelines for this repository

When you (Claude) generate or modify code in this project, please follow these rules:

### 1. Single responsibility per function and type
- Each function should do one thing well.
- Avoid mixing concerns (e.g. a function should not both fetch HTTP data and also build DNS responses and also mutate global state).
- If a function grows beyond ~30–40 lines, consider splitting it.

### 2. No duplicate logic
- If the same transformation or check appears more than once, extract it into a helper function.
- Use helper functions for:
  - Building cache keys
  - Converting Elchi JSON records into `dns.RR` objects
  - Logging common error patterns
  - Authentication validation
  - JSON response building

### 3. Idiomatic Go (Go 1.23+)
- Use clear, descriptive names (`ElchiClient`, `RecordCache`, `FetchSnapshot`, etc.).
- Use `context.Context` for all outbound HTTP calls.
- Prefer early returns for error handling instead of nested `if` statements.
- Avoid global variables for mutable state; encapsulate state in structs.
- **Use `any` instead of `interface{}`**.
- Use `errors.Join()` for multiple errors.
- Use `slices` package for slice operations.
- Use `maps` package for map operations.

### 4. Thread safety
- The cache will be accessed concurrently from:
  - The DNS query path (ServeDNS)
  - The background sync loop
  - The webhook handlers
- Use `sync.RWMutex` to guard cache state.
- Do **not** hold locks while performing network I/O.
- Minimize lock duration (build new data structures outside lock, then swap under lock).

### 5. Error handling & robustness
- Never panic in normal operation.
- If Elchi is unavailable or returns invalid data:
  - Log a clear error using CoreDNS logging.
  - Continue serving the last known good cache.
- Treat network errors and JSON parse errors as non-fatal:
  - The plugin should remain functional with stale data until a successful sync occurs.
- Validate all input data:
  - Zone names
  - IP addresses
  - Record types
  - TTL values

### 6. Configuration validation
- On plugin setup:
  - Validate that required fields (`api_endpoint`, `zone`, `secret`) are present.
  - Validate that durations (`sync_interval`, `timeout`) are positive and reasonable.
  - Validate secret minimum length (8 chars).
- Fail fast if configuration is invalid, with clear error messages.

### 7. Testing
- Provide unit tests for:
  - Cache behavior (`Get`, `Update`, `Delete`, `ReplaceFromSnapshot`)
  - Conversion of Elchi JSON records into `dns.RR` objects
  - Sync logic (initial snapshot, periodic sync, 304 handling)
  - Webhook handlers (notify, health, records)
  - Concurrent access patterns
- Provide integration tests for:
  - Full lifecycle (startup, sync, serve, update, delete)
  - Error scenarios (timeout, HTTP errors, invalid data)
- Keep tests focused, deterministic, and fast.
- Use table-driven tests where appropriate.
- Mock external dependencies (HTTP server).

### 8. Clarity over cleverness
- Prefer clear, straightforward code to overly clever solutions.
- Add short, focused comments where intent is not obvious.
- Public exported types and functions **must** have GoDoc comments.
- Document complex algorithms or non-obvious optimizations.

### 9. HTTP handling best practices
- Use `http.NewRequestWithContext` for all requests.
- Set proper timeouts on HTTP clients.
- Close response bodies properly (defer close).
- Handle all HTTP status codes explicitly.
- Set appropriate headers (Content-Type, Accept).
- Use connection pooling (http.Transport configuration).

### 10. Logging best practices
- Use CoreDNS logging (`clog.NewWithPlugin`).
- Log levels:
  - **Info**: Important state changes (snapshot loaded, cache updated)
  - **Warning**: Recoverable errors (sync failed, invalid record skipped)
  - **Error**: Serious errors (configuration invalid, cache corruption)
  - **Debug**: Detailed flow information (query handling, sync checks)
- Include context in logs (zone, hash, record count, error details).
- Never log secrets or sensitive data.

By following these guidelines, the resulting CoreDNS plugin should be:
- Easy to read and maintain
- Safe to run in production
- Resilient to failures
- Easy to extend later (additional record types, new endpoints, metrics)
- Well-tested and reliable

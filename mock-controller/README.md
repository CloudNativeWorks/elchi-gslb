# Mock Elchi Controller

Simple HTTP server that simulates the Elchi controller API for development and testing.

## Purpose

This mock controller provides the DNS snapshot and changes endpoints that the CoreDNS plugin expects, allowing local development without needing a full Elchi backend.

## Endpoints

### GET /dns/snapshot?zone=gslb.elchi

Returns the complete DNS snapshot.

**Headers:**
- `X-Elchi-Secret: test-secret-key` (required)

**Response (200 OK):**
```json
{
  "zone": "gslb.elchi",
  "version_hash": "mock-v1-20251231...",
  "records": [
    {
      "name": "listener1.gslb.elchi",
      "type": "A",
      "ttl": 300,
      "ips": ["192.168.1.10", "192.168.1.11"]
    },
    {
      "name": "listener2.gslb.elchi",
      "type": "A",
      "ttl": 300,
      "ips": ["192.168.2.20", "192.168.2.21"]
    },
    {
      "name": "listener3.gslb.elchi",
      "type": "AAAA",
      "ttl": 600,
      "ips": ["2001:db8::1", "2001:db8::2"]
    }
  ]
}
```

### GET /dns/changes?zone=gslb.elchi&since=xyz

Checks for changes since a given version hash.

**Headers:**
- `X-Elchi-Secret: test-secret-key` (required)

**Response - No changes (304 Not Modified):**
When `since` parameter matches current version_hash.

**Response - Changes detected (200 OK):**
Returns full snapshot when version_hash differs.

### GET /health

Health check endpoint (no authentication required).

**Response (200 OK):**
```json
{
  "status": "healthy",
  "zone": "gslb.elchi",
  "records_count": 3,
  "version_hash": "mock-v1-20251231..."
}
```

## Running

### Standalone
```bash
go run ./mock-controller
```

Server starts on `:1052`

### With Makefile
```bash
make run  # Starts both mock controller and CoreDNS
```

## Testing

```bash
# Health check
curl http://localhost:1052/health

# Snapshot (requires auth)
curl -H "X-Elchi-Secret: test-secret-key" \
  "http://localhost:1052/dns/snapshot?zone=gslb.elchi"

# Changes (requires auth)
curl -H "X-Elchi-Secret: test-secret-key" \
  "http://localhost:1052/dns/changes?zone=gslb.elchi&since=old-hash"
```

## Mock Data

The controller serves 3 test records:
- `listener1.gslb.elchi` - A records: 192.168.1.10, 192.168.1.11
- `listener2.gslb.elchi` - A records: 192.168.2.20, 192.168.2.21
- `listener3.gslb.elchi` - AAAA records: 2001:db8::1, 2001:db8::2

To modify the test data, edit the `mockSnapshot` variable in `main.go`.

## Authentication

Secret key: `test-secret-key`

This must match the `secret` configuration in `Corefile`.

# Startup Flow

This diagram shows the plugin initialization and initial snapshot fetch.

```mermaid
sequenceDiagram
    participant CoreDNS
    participant Plugin as Elchi Plugin
    participant Cache as In-Memory Cache
    participant Client as HTTP Client
    participant Controller as Elchi Controller

    Note over CoreDNS,Controller: Plugin Initialization

    CoreDNS->>Plugin: setup(config)
    Plugin->>Plugin: Validate config<br/>(zone, endpoint, secret)
    Plugin->>Cache: NewRecordCache(zone)
    Cache-->>Plugin: cache instance
    Plugin->>Client: NewElchiClient(endpoint, secret)
    Client-->>Plugin: client instance
    Plugin-->>CoreDNS: Plugin ready (Ready()=false)

    Note over CoreDNS,Controller: Initial Snapshot Fetch

    CoreDNS->>Plugin: InitClient()
    Plugin->>Client: FetchSnapshot(ctx)
    Client->>Controller: GET /dns/snapshot?zone=gslb.elchi<br/>X-Elchi-Secret: <secret>

    alt Controller Available
        Controller->>Controller: Compute version hash<br/>of all records
        Controller-->>Client: 200 OK<br/>{zone, version_hash, records[]}
        Client-->>Plugin: DNSSnapshot
        Plugin->>Cache: ReplaceFromSnapshot(snapshot)
        Cache->>Cache: Build dns.RR objects<br/>from each record
        Cache->>Cache: Store in map<br/>key=name:type
        Cache-->>Plugin: Success
        Plugin->>Plugin: Ready() = true

    else Controller Unavailable
        Controller-->>Client: 500 Error / Timeout
        Client-->>Plugin: Error
        Plugin->>Plugin: Log warning<br/>"Initial snapshot failed"
        Plugin->>Plugin: Ready() = false<br/>(empty cache)
        Note over Plugin: Continue with empty cache<br/>Background sync will retry
    end

    Plugin->>Plugin: Start background sync goroutine
    Plugin->>Plugin: Start webhook server (if enabled)
    Plugin-->>CoreDNS: Ready

    Note over CoreDNS,Controller: Plugin is now serving DNS queries
```

## Key Points

1. **Non-blocking startup**: Plugin doesn't fail if initial snapshot fails
2. **Graceful degradation**: Starts with empty cache, retries in background
3. **Pre-built records**: RR objects built during cache load, not during queries
4. **Ready status**: `Ready()` returns true only after successful snapshot fetch
5. **Background sync**: Starts immediately to keep cache updated

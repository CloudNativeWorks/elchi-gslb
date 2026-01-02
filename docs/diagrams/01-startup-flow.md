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
    activate Plugin

    Plugin->>Plugin: Validate config<br/>(zone, endpoint, secret)

    Plugin->>Cache: NewRecordCache(zone)
    activate Cache
    Cache-->>Plugin: cache instance
    deactivate Cache

    Plugin->>Client: NewElchiClient(endpoint, secret)
    activate Client
    Client-->>Plugin: client instance
    deactivate Client

    Plugin-->>CoreDNS: Plugin ready (Ready()=false)
    deactivate Plugin

    Note over CoreDNS,Controller: Initial Snapshot Fetch

    CoreDNS->>Plugin: InitClient()
    activate Plugin

    Plugin->>Client: FetchSnapshot(ctx)
    activate Client

    Client->>Controller: GET /dns/snapshot?zone=gslb.elchi<br/>X-Elchi-Secret: <secret>
    activate Controller

    alt Controller Available
        Controller->>Controller: Compute version hash<br/>of all records
        Controller-->>Client: 200 OK<br/>{zone, version_hash, records[]}

        Client-->>Plugin: DNSSnapshot
        deactivate Client

        Plugin->>Cache: ReplaceFromSnapshot(snapshot)
        activate Cache

        Cache->>Cache: Build dns.RR objects<br/>from each record
        Cache->>Cache: Store in map<br/>key=name:type

        Cache-->>Plugin: Success
        deactivate Cache

        Plugin->>Plugin: Ready() = true

    else Controller Unavailable
        Controller-->>Client: 500 Error / Timeout
        deactivate Controller

        Client-->>Plugin: Error
        deactivate Client

        Plugin->>Plugin: Log warning<br/>"Initial snapshot failed"
        Plugin->>Plugin: Ready() = false<br/>(empty cache)

        Note over Plugin: Continue with empty cache<br/>Background sync will retry
    end

    Plugin->>Plugin: Start background sync goroutine
    Plugin->>Plugin: Start webhook server (if enabled)

    Plugin-->>CoreDNS: Ready
    deactivate Plugin

    Note over CoreDNS,Controller: Plugin is now serving DNS queries
```

## Key Points

1. **Non-blocking startup**: Plugin doesn't fail if initial snapshot fails
2. **Graceful degradation**: Starts with empty cache, retries in background
3. **Pre-built records**: RR objects built during cache load, not during queries
4. **Ready status**: `Ready()` returns true only after successful snapshot fetch
5. **Background sync**: Starts immediately to keep cache updated

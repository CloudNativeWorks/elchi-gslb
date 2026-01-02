# Periodic Sync Flow

This diagram shows the background synchronization with hash-based change detection.

```mermaid
sequenceDiagram
    participant Timer as Sync Timer<br/>(every 5 min)
    participant Plugin as Elchi Plugin
    participant Cache as In-Memory Cache
    participant Client as HTTP Client
    participant Controller as Elchi Controller

    Note over Timer,Controller: Background Sync Loop

    loop Every sync_interval (default: 5 minutes)
        Timer->>Plugin: Trigger sync

        Plugin->>Cache: GetVersionHash()
        Cache-->>Plugin: current_hash (e.g., "abc123")

        Plugin->>Client: CheckChanges(ctx, current_hash)

        Client->>Controller: GET /dns/changes?zone=gslb.elchi&since=abc123<br/>X-Elchi-Secret: <secret>

        Controller->>Controller: Compute current hash

        alt No Changes (Hash Matches)
            Controller-->>Client: 304 Not Modified

            Client-->>Plugin: DNSChangesResponse{Unchanged: true}

            Plugin->>Plugin: Log "No changes detected"
            Plugin->>Plugin: Update sync status: "success"

            Note over Plugin: No cache update needed<br/>Continue serving existing data

        else Has Changes (Hash Different)
            Controller->>Controller: Generate full snapshot
            Controller-->>Client: 200 OK<br/>{unchanged: false, version_hash: "xyz789", records[]}

            Client-->>Plugin: DNSChangesResponse with new data

            Plugin->>Cache: ReplaceFromSnapshot(new_data)

            Cache->>Cache: Clear old cache
            Cache->>Cache: Build dns.RR objects<br/>for each record
            Cache->>Cache: Store new version_hash

            Cache-->>Plugin: Success

            Plugin->>Plugin: Log "Updated %d records"
            Plugin->>Plugin: Update sync status: "success"
            Plugin->>Plugin: Update metrics:<br/>elchi_sync_success_total++

            Note over Plugin: New data now being served

        else Controller Error
            Controller-->>Client: 500 Error / Timeout / Network Error

            Client-->>Plugin: Error

            Plugin->>Plugin: Log error:<br/>"Sync failed: <error>"
            Plugin->>Plugin: Update sync status: "failed"
            Plugin->>Plugin: Update metrics:<br/>elchi_sync_failure_total++

            Note over Plugin: Continue serving stale cache<br/>Will retry on next interval

        end

    end

    Note over Timer,Controller: DNS queries continue unaffected
```

## Key Points

1. **Hash-based optimization**: Only transfers data when hash changes
2. **HTTP 304 efficiency**: No body sent when unchanged, saves bandwidth
3. **Atomic updates**: Cache replaced atomically, no partial state
4. **Error tolerance**: Sync failures don't stop DNS serving
5. **Metrics tracking**: Success/failure counters for monitoring
6. **Stale cache strategy**: Continue serving old data during errors

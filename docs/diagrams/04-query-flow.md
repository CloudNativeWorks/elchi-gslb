# DNS Query Flow

This diagram shows how incoming DNS queries are handled by the plugin.

```mermaid
sequenceDiagram
    participant Client as DNS Client
    participant CoreDNS
    participant Plugin as Elchi Plugin
    participant Cache as In-Memory Cache
    participant Next as Next Plugin

    Note over Client,Next: DNS Query Handling

    Client->>CoreDNS: DNS Query<br/>test.gslb.elchi A?

    CoreDNS->>Plugin: ServeDNS(ctx, qname, qtype)

    Plugin->>Plugin: Check if qname in our zone

    alt Query is for our zone (gslb.elchi)
        Plugin->>Plugin: Extract qname and qtype

        Plugin->>Cache: Get(qname, qtype)

        Cache->>Cache: Build cache key<br/>"test.gslb.elchi:A"
        Cache->>Cache: Lookup in map

        alt Record Found in Cache
            Cache-->>Plugin: []dns.RR (pre-built records)

            Plugin->>Plugin: Build DNS response<br/>- Set authoritative flag<br/>- Add RRs to answer section

            Plugin->>CoreDNS: Write response
            CoreDNS-->>Client: 200 OK (NOERROR)<br/>test.gslb.elchi 300 IN A 192.168.1.10<br/>test.gslb.elchi 300 IN A 192.168.1.11

            Note over Plugin: Total time: <1ms

        else Record Not Found
            Cache-->>Plugin: nil (not found)

            Plugin->>Plugin: Build NXDOMAIN response<br/>- Set authoritative flag<br/>- Empty answer section

            Plugin->>CoreDNS: Write response
            CoreDNS-->>Client: 200 OK (NXDOMAIN)<br/>Name does not exist

            Note over Plugin: Authoritative NXDOMAIN

        end

    else Query is NOT for our zone (e.g., example.com)
        Plugin->>Next: Delegate to next plugin

        Next->>Next: Handle query<br/>(forward, cache, etc.)
        Next-->>Plugin: Response


        Plugin->>CoreDNS: Pass through response
        CoreDNS-->>Client: Response from next plugin

        Note over Plugin: Not our zone, delegated

    end


    Note over Client,Next: Cache lookup is very fast (<100μs)<br/>Pre-built RRs eliminate parsing overhead
```

## Performance Breakdown

```
Total query latency: ~500μs - 1ms

Breakdown:
- Zone check:           ~10μs   (string comparison)
- Cache lookup:         ~50μs   (map lookup)
- Response building:    ~100μs  (copy pre-built RRs)
- DNS encoding:         ~300μs  (miekg/dns library)
- Network write:        ~50μs   (local buffer)
```

## Query Types Handled

```mermaid
graph TD
    A[DNS Query] --> B{In our zone?}
    B -->|Yes| C{Record type?}
    B -->|No| D[Delegate to Next]

    C -->|A| E[IPv4 lookup]
    C -->|AAAA| F[IPv6 lookup]
    C -->|Other| G[NOERROR empty]

    E --> H{Found?}
    F --> H
    G --> I[Return empty answer]

    H -->|Yes| J[Return IP records]
    H -->|No| K[Return NXDOMAIN]

    D --> L[Forward/Cache/Other]
```

## Key Points

1. **Fast path**: Cache lookup is the only I/O operation
2. **Pre-built records**: No parsing or building during query
3. **Authoritative responses**: Plugin is authoritative for its zone
4. **Zone delegation**: Queries for other zones delegated to next plugin
5. **Zero network calls**: All data from in-memory cache
6. **Thread-safe**: RWMutex allows concurrent reads

# Webhook Update Flow

This diagram shows instant updates via webhook for low-latency propagation.

```mermaid
sequenceDiagram
    participant User
    participant Controller as Elchi Controller
    participant Webhook as Plugin Webhook<br/>Server (:8053)
    participant Cache as In-Memory Cache
    participant DNS as DNS Query Handler

    Note over User,DNS: Critical Change Scenario

    User->>Controller: Update listener IP<br/>(e.g., failover)

    Controller->>Controller: Update database
    Controller->>Controller: Compute new version_hash

    Note over Controller: Controller decides to push update

    Controller->>Webhook: POST /notify<br/>X-Elchi-Secret: <secret>

    Note over Webhook: Payload:<br/>{<br/>  "records": [{name, type, ttl, ips}],<br/>  "deletes": [{name, type}]<br/>}

    Webhook->>Webhook: Validate secret

    alt Valid Secret
        Webhook->>Cache: UpdateFromNotification(payload)

        alt Has Records to Update
            Cache->>Cache: For each record:<br/>Build dns.RR objects
            Cache->>Cache: Store/update in map

            Note over Cache: Partial update<br/>Only changed records
        end

        alt Has Deletes
            Cache->>Cache: For each delete:<br/>Remove from map
        end

        Cache-->>Webhook: Success (updated: N, deleted: M)

        Webhook-->>Controller: 200 OK<br/>{status: "ok", updated: N, deleted: M}

        Controller->>Controller: Log success
        Controller-->>User: Change applied

        Note over DNS: New DNS queries now return updated IPs<br/>Propagation time: < 1 second

    else Invalid Secret
        Webhook-->>Controller: 401 Unauthorized

        Controller->>Controller: Log authentication failure
        Controller-->>User: Error

    else Webhook Server Down
        Controller->>Webhook: POST /notify (timeout)
        Webhook-->>Controller: Connection refused / Timeout

        Controller->>Controller: Log warning:<br/>"Webhook failed, will sync on next poll"
        Controller-->>User: Change saved<br/>(will propagate via periodic sync)

        Note over Cache: Periodic sync (within 5 min)<br/>will pick up changes

    end

    Note over User,DNS: Multiple CoreDNS Instances

    par Webhook to Instance 1
        Controller->>Webhook: POST /notify to instance-1
        Webhook->>Cache: Update
    and Webhook to Instance 2
        Controller->>Webhook: POST /notify to instance-2
        Webhook->>Cache: Update
    and Webhook to Instance 3
        Controller->>Webhook: POST /notify to instance-3
        Webhook->>Cache: Update
    end

    Note over Controller,DNS: All instances updated within seconds
```

## Key Points

1. **Low latency**: Changes propagate in <1 second
2. **Partial updates**: Only send changed/deleted records
3. **Parallel updates**: Controller notifies all instances concurrently
4. **Fault tolerant**: Webhook failures don't block changes
5. **Security**: Secret-based authentication
6. **Fallback**: Periodic sync ensures eventual consistency if webhook fails
7. **Idempotent**: Multiple notifications of same change are safe

## Webhook vs Periodic Sync

| Aspect | Webhook | Periodic Sync |
|--------|---------|---------------|
| Latency | <1 second | Up to 5 minutes |
| Network usage | On-demand | Every 5 minutes |
| Reliability | Optional | Required |
| Use case | Critical updates | Normal updates |
| Failure impact | Falls back to sync | DNS stops updating |

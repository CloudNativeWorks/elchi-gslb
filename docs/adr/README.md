# Architecture Decision Records (ADR)

This directory contains important architectural decisions made for the Elchi GSLB CoreDNS Plugin.

## ADR Format

Each ADR follows this format:

- **Title**: Short and descriptive
- **Status**: Proposed / Accepted / Rejected / Deprecated
- **Context**: Why this decision was needed
- **Decision**: What we decided to do
- **Consequences**: Effects of the decision (positive and negative)

## ADR List

1. [ADR-001: In-Memory Cache Usage](001-in-memory-cache.md)
2. [ADR-002: Hash-Based Change Detection](002-hash-based-sync.md)
3. [ADR-003: Webhook Architecture](003-webhook-architecture.md)
4. [ADR-004: Graceful Degradation Strategy](004-graceful-degradation.md)
5. [ADR-005: Pre-Built DNS Records](005-pre-built-records.md)

## Adding New ADRs

To add a new ADR:

1. Use the next sequential number (e.g., 006)
2. File name: `NNN-short-description.md`
3. Follow the format above
4. Add to this README

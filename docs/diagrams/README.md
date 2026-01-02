# Sequence Diagrams

This directory contains sequence diagrams for key flows in the Elchi GSLB plugin.

All diagrams are in Mermaid format and can be viewed on GitHub or in any Mermaid-compatible viewer.

## Diagrams

1. [Initial Startup Flow](01-startup-flow.md) - Plugin initialization and first snapshot fetch
2. [Periodic Sync Flow](02-periodic-sync.md) - Background synchronization with hash-based change detection
3. [Webhook Update Flow](03-webhook-flow.md) - Instant updates via webhook
4. [DNS Query Flow](04-query-flow.md) - Handling incoming DNS queries
5. [Error Recovery Flow](05-error-recovery.md) - Graceful degradation and recovery

## Viewing Diagrams

### GitHub
Diagrams render automatically on GitHub.

### VS Code
Install the "Markdown Preview Mermaid Support" extension.

### CLI
```bash
npm install -g @mermaid-js/mermaid-cli
mmdc -i diagram.md -o diagram.png
```

### Online
Copy diagram code to https://mermaid.live/

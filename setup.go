package elchi

import (
	"fmt"
	"strconv"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

// Configuration defaults
const (
	defaultTTL          = 300              // Default TTL in seconds (5 minutes)
	defaultSyncInterval = 5 * time.Minute  // Default sync interval
	defaultTimeout      = 10 * time.Second // Default HTTP timeout
	defaultWebhookAddr  = ":8053"          // Default webhook server address
	minSyncInterval     = 1 * time.Minute  // Minimum allowed sync interval
	minTimeout          = 1 * time.Second  // Minimum allowed timeout
	minSecretLength     = 8                // Minimum secret length for security
)

// init registers this plugin within the Caddy plugin framework.
func init() {
	plugin.Register("elchi", setup)
}

// setup is the function that gets called when the config parser sees the "elchi" token.
func setup(c *caddy.Controller) error {
	e, err := parseElchi(c)
	if err != nil {
		return plugin.Error("elchi", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		e.Next = next
		return e
	})

	// Register shutdown hook for graceful cleanup
	c.OnShutdown(func() error {
		return e.Shutdown()
	})

	return nil
}

// parseElchi parses the Corefile configuration for the elchi plugin.
//
//nolint:gocyclo // Config parsing is inherently complex.
func parseElchi(c *caddy.Controller) (*Elchi, error) {
	e := &Elchi{
		TTL:           defaultTTL,
		SyncInterval:  defaultSyncInterval,
		Timeout:       defaultTimeout,
		WebhookEnable: false,
		WebhookAddr:   defaultWebhookAddr,
	}

	// Extract zone from server block keys
	// Format: gslb.elchi { ... }
	if len(c.ServerBlockKeys) > 0 {
		// Normalize the zone (ensure trailing dot)
		zone := plugin.Host(c.ServerBlockKeys[0]).NormalizeExact()[0]
		e.Zone = zone
	}

	for c.Next() {
		// Configuration block: elchi { endpoint ... secret ... }
		for c.NextBlock() {
			switch c.Val() {
			case "endpoint":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				e.Endpoint = c.Val()

			case "secret":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				e.Secret = c.Val()

			case "ttl":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				ttl, err := strconv.ParseUint(c.Val(), 10, 32)
				if err != nil {
					return nil, c.Errf("invalid ttl value: %v", err)
				}
				if ttl == 0 {
					return nil, c.Errf("ttl must be greater than 0")
				}
				e.TTL = uint32(ttl)

			case "sync_interval":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				interval, err := time.ParseDuration(c.Val())
				if err != nil {
					return nil, c.Errf("invalid sync_interval: %v", err)
				}
				if interval < minSyncInterval {
					return nil, c.Errf("sync_interval must be at least %v", minSyncInterval)
				}
				e.SyncInterval = interval

			case "timeout":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				timeout, err := time.ParseDuration(c.Val())
				if err != nil {
					return nil, c.Errf("invalid timeout: %v", err)
				}
				if timeout < minTimeout {
					return nil, c.Errf("timeout must be at least %v", minTimeout)
				}
				e.Timeout = timeout

			case "webhook":
				// webhook directive: enable webhook server
				// Optional argument: address (e.g., "webhook :8053" or just "webhook")
				e.WebhookEnable = true
				if c.NextArg() {
					e.WebhookAddr = c.Val()
				}

			case "fallthrough":
				// fallthrough directive: continue to next plugin if no records found
				// Can optionally specify zones to fall through for
				e.Fall.SetZonesFromArgs(c.RemainingArgs())

			default:
				return nil, c.Errf("unknown directive '%s'", c.Val())
			}
		}
	}

	// Validate required fields
	if e.Zone == "" {
		return nil, fmt.Errorf("zone is required (specify zone in server block, e.g., gslb.elchi)")
	}
	if e.Endpoint == "" {
		return nil, fmt.Errorf("endpoint is required")
	}
	if e.Secret == "" {
		return nil, fmt.Errorf("secret is required")
	}

	// Validate secret length (minimum 8 characters for basic security)
	if len(e.Secret) < minSecretLength {
		return nil, fmt.Errorf("secret must be at least %d characters long", minSecretLength)
	}

	// Validate sync_interval vs timeout
	if e.SyncInterval <= e.Timeout {
		return nil, fmt.Errorf("sync_interval must be greater than timeout")
	}

	// Initialize the client
	if err := e.InitClient(); err != nil {
		return nil, fmt.Errorf("failed to initialize elchi client: %w", err)
	}

	return e, nil
}

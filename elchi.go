package elchi

import (
	"context"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/fall"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

var log = clog.NewWithPlugin("elchi")

// Elchi is the main plugin structure.
type Elchi struct {
	Next plugin.Handler
	Fall fall.F

	// Configuration
	Endpoint      string
	Zone          string
	Secret        string
	TTL           uint32
	SyncInterval  time.Duration
	Timeout       time.Duration
	WebhookAddr   string // Address for webhook server (e.g., ":8053")
	WebhookEnable bool   // Enable webhook server
	TLSSkipVerify bool   // Skip TLS certificate verification (insecure, for self-signed certs)

	// Client and cache
	client        *ElchiClient
	cache         *RecordCache
	syncStatus    *SyncStatus
	webhookServer *WebhookServer

	// Lifecycle management
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
}

// ServeDNS implements the plugin.Handler interface.
func (e *Elchi) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	// Check if query is within our zone
	zone := plugin.Zones([]string{e.Zone}).Matches(state.Name())
	if zone == "" {
		// Not our zone, pass to next plugin
		return plugin.NextOrFailure(e.Name(), e.Next, ctx, w, r)
	}

	qtype := state.QType()
	qname := state.Name()
	qtypeStr := dns.TypeToString[qtype]

	// Track request
	requestCount.WithLabelValues(e.Zone, qtypeStr).Inc()

	log.Debugf("Query for %s (type: %s)", qname, qtypeStr)

	// Get pre-built dns.RR objects from cache
	var rrs []dns.RR

	// Check for records based on query type
	switch qtype {
	case dns.TypeA, dns.TypeAAAA:
		// For A/AAAA queries, check if there's a CNAME first (failover scenario)
		cnameRRs := e.cache.Get(qname, dns.TypeCNAME)
		if len(cnameRRs) > 0 {
			// CNAME exists, return it instead of A/AAAA
			// This is the failover case (enabled=false)
			rrs = cnameRRs
			log.Debugf("Found CNAME for %s, returning CNAME instead of %s", qname, qtypeStr)
		} else {
			// No CNAME, look for A/AAAA records
			rrs = e.cache.Get(qname, qtype)
		}
	case dns.TypeCNAME:
		// Explicit CNAME query
		rrs = e.cache.Get(qname, dns.TypeCNAME)
	default:
		// Unsupported type, pass to next plugin
		return plugin.NextOrFailure(e.Name(), e.Next, ctx, w, r)
	}

	if len(rrs) == 0 {
		// No records found, track cache miss
		cacheMisses.WithLabelValues(e.Zone, qtypeStr).Inc()
		log.Debugf("No records found for %s", qname)

		// Check if fallthrough is enabled for this zone
		if e.Fall.Through(qname) {
			return plugin.NextOrFailure(e.Name(), e.Next, ctx, w, r)
		}

		// No fallthrough - return NXDOMAIN
		m := new(dns.Msg)
		m.SetRcode(r, dns.RcodeNameError)
		m.Authoritative = true
		if err := w.WriteMsg(m); err != nil {
			log.Errorf("Failed to write NXDOMAIN response: %v", err)
		}
		return dns.RcodeNameError, nil
	}

	// Track cache hit
	cacheHits.WithLabelValues(e.Zone, qtypeStr).Inc()

	// Build response with pre-built RRs
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	m.Answer = rrs

	log.Debugf("Returning %d records for %s", len(m.Answer), qname)
	if err := w.WriteMsg(m); err != nil {
		log.Errorf("Failed to write DNS response: %v", err)
	}
	return dns.RcodeSuccess, nil
}

// Name implements the plugin.Handler interface.
func (e *Elchi) Name() string {
	return "elchi"
}

// Ready implements the ready.Readiness interface.
// It reports ready when the cache has been populated with at least one successful sync.
func (e *Elchi) Ready() bool {
	if e.cache == nil {
		return false
	}
	return e.cache.GetVersionHash() != ""
}

// InitClient initializes the Elchi backend client and starts the background sync.
func (e *Elchi) InitClient() error {
	e.client = NewElchiClient(e.Endpoint, e.Zone, e.Secret, e.Timeout, e.TLSSkipVerify)
	e.cache = NewRecordCache(e.Zone)
	e.syncStatus = &SyncStatus{lastSyncStatus: "initial"}

	// Create shutdown context for graceful termination
	e.shutdownCtx, e.shutdownCancel = context.WithCancel(context.Background())

	// Start webhook server if enabled
	if e.WebhookEnable {
		e.webhookServer = NewWebhookServer(e, e.WebhookAddr)
		if err := e.webhookServer.Start(); err != nil {
			log.Warningf("Failed to start webhook server: %v", err)
		}
	}

	// Attempt initial snapshot fetch
	ctx, cancel := context.WithTimeout(context.Background(), e.Timeout)
	defer cancel()

	snapshot, err := e.client.FetchSnapshot(ctx)
	if err != nil {
		// Log warning but don't fail - backend might not be ready yet
		log.Warningf("Initial snapshot fetch failed: %v (will retry in background)", err)
		e.syncStatus.Update("failed", err)
	} else {
		if err := e.cache.ReplaceFromSnapshot(snapshot, e.TTL); err != nil {
			log.Errorf("Failed to load initial snapshot: %v", err)
			e.syncStatus.Update("failed", err)
		} else {
			log.Infof("Initial snapshot loaded: %d records, hash=%s",
				len(snapshot.Records), snapshot.VersionHash)
			e.syncStatus.Update("success", nil)
		}
	}

	// Start background sync goroutine with shutdown context
	go e.backgroundSync()

	return nil
}

// It respects the shutdown context for graceful termination.
func (e *Elchi) backgroundSync() {
	ticker := time.NewTicker(e.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.shutdownCtx.Done():
			// Graceful shutdown triggered
			log.Info("Background sync shutting down gracefully")
			return

		case <-ticker.C:
			e.performSync()
		}
	}
}

// performSync executes a single sync cycle with proper context management.
func (e *Elchi) performSync() {
	ctx, cancel := context.WithTimeout(context.Background(), e.Timeout)
	defer cancel()

	currentHash := e.cache.GetVersionHash()

	if currentHash == "" {
		// No snapshot yet, try to fetch initial
		log.Debug("No snapshot yet, attempting initial fetch")

		// Track sync duration
		start := time.Now()
		snapshot, err := e.client.FetchSnapshot(ctx)
		syncDuration.WithLabelValues(e.Zone, "snapshot").Observe(time.Since(start).Seconds())

		if err != nil {
			log.Warningf("Snapshot fetch failed: %v", err)
			syncErrors.WithLabelValues(e.Zone, "snapshot").Inc()
			e.syncStatus.Update("failed", err)
			return
		}

		if err := e.cache.ReplaceFromSnapshot(snapshot, e.TTL); err != nil {
			log.Errorf("Failed to load snapshot: %v", err)
			syncErrors.WithLabelValues(e.Zone, "snapshot").Inc()
			e.syncStatus.Update("failed", err)
		} else {
			log.Infof("Snapshot loaded: %d records, hash=%s",
				len(snapshot.Records), snapshot.VersionHash)
			e.syncStatus.Update("success", nil)
		}
		return
	}

	// Have a snapshot, check for changes
	log.Debugf("Checking for changes since hash=%s", currentHash)

	// Track sync duration
	start := time.Now()
	changes, err := e.client.CheckChanges(ctx, currentHash)
	syncDuration.WithLabelValues(e.Zone, "changes").Observe(time.Since(start).Seconds())

	if err != nil {
		log.Warningf("Change check failed: %v", err)
		syncErrors.WithLabelValues(e.Zone, "changes").Inc()
		e.syncStatus.Update("failed", err)
		return
	}

	if changes.Unchanged {
		log.Debug("No changes detected")
		e.syncStatus.Update("success", nil)
		return
	}

	log.Infof("Changes detected, new hash=%s", changes.VersionHash)

	// Build snapshot from changes response
	snapshot := &DNSSnapshot{
		Zone:        changes.Zone,
		VersionHash: changes.VersionHash,
		Records:     changes.Records,
	}

	if err := e.cache.ReplaceFromSnapshot(snapshot, e.TTL); err != nil {
		log.Errorf("Failed to load updated snapshot: %v", err)
		syncErrors.WithLabelValues(e.Zone, "changes").Inc()
		e.syncStatus.Update("failed", err)
	} else {
		log.Infof("Cache updated: %d records, hash=%s",
			len(snapshot.Records), snapshot.VersionHash)
		e.syncStatus.Update("success", nil)
	}
}

// Shutdown performs graceful shutdown of the plugin.
// This is called by CoreDNS during plugin reload or server shutdown.
func (e *Elchi) Shutdown() error {
	log.Info("Shutting down Elchi plugin")

	// Cancel background sync
	if e.shutdownCancel != nil {
		e.shutdownCancel()
	}

	// Stop webhook server if running
	if e.webhookServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := e.webhookServer.Stop(ctx); err != nil {
			log.Warningf("Error stopping webhook server: %v", err)
		}
	}

	log.Info("Elchi plugin shutdown complete")
	return nil
}

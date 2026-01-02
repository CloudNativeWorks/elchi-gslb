package elchi

import (
	"github.com/coredns/coredns/plugin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus metrics for the elchi plugin
// Following CoreDNS standards: namespace = plugin.Namespace, subsystem = "elchi"
var (
	// requestCount counts total DNS requests handled by the plugin
	requestCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace, // "coredns"
		Subsystem: "elchi",
		Name:      "requests_total",
		Help:      "Total number of DNS requests handled by elchi plugin.",
	}, []string{"zone", "type"})

	// cacheHits counts successful cache lookups
	cacheHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "elchi",
		Name:      "cache_hits_total",
		Help:      "Total number of cache hits.",
	}, []string{"zone", "type"})

	// cacheMisses counts failed cache lookups
	cacheMisses = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "elchi",
		Name:      "cache_misses_total",
		Help:      "Total number of cache misses.",
	}, []string{"zone", "type"})

	// syncDuration tracks how long sync operations take
	syncDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: plugin.Namespace,
		Subsystem: "elchi",
		Name:      "sync_duration_seconds",
		Help:      "Duration of backend sync operations in seconds.",
		Buckets:   prometheus.DefBuckets, // 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10
	}, []string{"zone", "type"}) // type: "snapshot" or "changes"

	// syncErrors counts sync failures
	syncErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "elchi",
		Name:      "sync_errors_total",
		Help:      "Total number of backend sync errors.",
	}, []string{"zone", "type"}) // type: "snapshot" or "changes"

	// cacheSize tracks the number of records in cache
	cacheSize = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: plugin.Namespace,
		Subsystem: "elchi",
		Name:      "cache_size",
		Help:      "Number of DNS records currently in cache.",
	}, []string{"zone"})

	// webhookRequests counts webhook endpoint requests
	webhookRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "elchi",
		Name:      "webhook_requests_total",
		Help:      "Total number of webhook requests received.",
	}, []string{"endpoint", "status"}) // endpoint: "health", "records", "update"; status: "success", "error", "unauthorized"
)

package plugin

import (
	"github.com/gatewayd-io/gatewayd-plugin-sdk/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	CacheHitsCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: metrics.Namespace,
		Name:      "cache_hits_total",
		Help:      "The total number of cache hits",
	})
	CacheMissesCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: metrics.Namespace,
		Name:      "cache_misses_total",
		Help:      "The total number of cache misses",
	})
	CacheSetsCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: metrics.Namespace,
		Name:      "cache_sets_total",
		Help:      "The total number of cache sets",
	})
	CacheGetsCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: metrics.Namespace,
		Name:      "cache_gets_total",
		Help:      "The total number of cache gets",
	})
	CacheDeletesCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: metrics.Namespace,
		Name:      "cache_deletes_total",
		Help:      "The total number of cache deletes",
	})
	CacheScanCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: metrics.Namespace,
		Name:      "cache_scans_total",
		Help:      "The total number of cache scans",
	})
	CacheScanKeysCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: metrics.Namespace,
		Name:      "cache_scan_keys_total",
		Help:      "The total number of cache scan keys",
	})
)

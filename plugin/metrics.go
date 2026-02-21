package plugin

import (
	"github.com/gatewayd-io/gatewayd-plugin-sdk/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	GetPluginConfigCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: metrics.Namespace,
		Name:      "get_plugin_config_total",
		Help:      "The total number of calls to the getPluginConfig method",
	})
	OnClosedCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: metrics.Namespace,
		Name:      "on_closed_total",
		Help:      "The total number of calls to the onClosed method",
	})
	OnTrafficFromClientCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: metrics.Namespace,
		Name:      "on_traffic_from_client_total",
		Help:      "The total number of calls to the onTrafficFromClient method",
	})
	OnTrafficFromServerCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: metrics.Namespace,
		Name:      "on_traffic_from_server_total",
		Help:      "The total number of calls to the onTrafficFromServer method",
	})

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
	CacheErrorsCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: metrics.Namespace,
		Name:      "cache_errors_total",
		Help:      "The total number of Redis operation errors",
	})
)

package plugin

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	Namespace = "gatewayd_plugin_cache"
)

var (
	CacheHitsCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name:      "cache_hits_total",
		Help:      "The total number of cache hits",
		Namespace: Namespace,
	})
	CacheMissesCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name:      "cache_misses_total",
		Help:      "The total number of cache misses",
		Namespace: Namespace,
	})
	CacheSetsCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name:      "cache_sets_total",
		Help:      "The total number of cache sets",
		Namespace: Namespace,
	})
	CacheGetsCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name:      "cache_gets_total",
		Help:      "The total number of cache gets",
		Namespace: Namespace,
	})
)

package plugin

import (
	"net"
	"net/http"
	"os"

	"github.com/hashicorp/go-hclog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	Namespace = "gatewayd"
)

var (
	CacheHitsCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      "cache_hits_total",
		Help:      "The total number of cache hits",
	})
	CacheMissesCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      "cache_misses_total",
		Help:      "The total number of cache misses",
	})
	CacheSetsCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      "cache_sets_total",
		Help:      "The total number of cache sets",
	})
	CacheGetsCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      "cache_gets_total",
		Help:      "The total number of cache gets",
	})
)

type MetricsConfig struct {
	Enabled          bool
	UnixDomainSocket string
	Endpoint         string
}

func ExposeMetrics(metricsConfig MetricsConfig, logger hclog.Logger) {
	logger.Info(
		"Starting metrics server via HTTP over Unix domain socket",
		"unixDomainSocket", metricsConfig.UnixDomainSocket,
		"endpoint", metricsConfig.Endpoint)

	if file, err := os.Stat(metricsConfig.UnixDomainSocket); err == nil && !file.IsDir() && file.Mode().Type() == os.ModeSocket {
		if err := os.Remove(metricsConfig.UnixDomainSocket); err != nil {
			logger.Error("Failed to remove unix domain socket")
		}
	}

	listener, err := net.Listen("unix", metricsConfig.UnixDomainSocket)
	if err != nil {
		logger.Error("Failed to start metrics server")
	}

	if err := http.Serve(listener, promhttp.Handler()); err != nil {
		logger.Error("Failed to start metrics server")
	}
}

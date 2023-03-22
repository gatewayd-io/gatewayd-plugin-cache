package main

import (
	"context"
	"flag"
	"os"

	"github.com/gatewayd-io/gatewayd-plugin-cache/plugin"
	sdkConfig "github.com/gatewayd-io/gatewayd-plugin-sdk/config"
	"github.com/gatewayd-io/gatewayd-plugin-sdk/logging"
	"github.com/gatewayd-io/gatewayd-plugin-sdk/metrics"
	p "github.com/gatewayd-io/gatewayd-plugin-sdk/plugin"
	"github.com/go-redis/redis/v8"
	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/spf13/cast"
)

func main() {
	// Parse command line flags, passed by GatewayD via the plugin config
	logLevel := flag.String("log-level", "info", "Log level")
	flag.Parse()

	logger := hclog.New(&hclog.LoggerOptions{
		Level:      logging.GetLogLevel(*logLevel),
		Output:     os.Stderr,
		JSONFormat: true,
		Color:      hclog.ColorOff,
	})

	pluginInstance := plugin.NewCachePlugin(plugin.Plugin{
		Logger: logger,
	})

	var metricsConfig *metrics.MetricsConfig
	if cfg := cast.ToStringMap(plugin.PluginConfig["config"]); cfg != nil {
		metricsConfig = metrics.NewMetricsConfig(cfg)
		if metricsConfig != nil && metricsConfig.Enabled {
			go metrics.ExposeMetrics(metricsConfig, logger)
		}

		pluginInstance.Impl.RedisURL = cast.ToString(cfg["redisURL"])
		pluginInstance.Impl.Expiry = cast.ToDuration(cfg["expiry"])
		pluginInstance.Impl.DefaultDBName = cast.ToString(cfg["defaultDBName"])
		pluginInstance.Impl.ScanCount = cast.ToInt64(cfg["scanCount"])

		redisConfig, err := redis.ParseURL(pluginInstance.Impl.RedisURL)
		if err != nil {
			logger.Error("Failed to parse Redis URL", "error", err)
			os.Exit(1)
		}

		pluginInstance.Impl.RedisClient = redis.NewClient(redisConfig)

		// Ping the Redis server to check if it is available.
		_, err = pluginInstance.Impl.RedisClient.Ping(context.Background()).Result()
		if err != nil {
			logger.Error("Failed to ping Redis server, plugin exited", "error", err)
			os.Exit(1)
		}

		pluginInstance.Impl.PeriodicInvalidatorEnabled = cast.ToBool(
			cfg["periodicInvalidatorEnabled"])
		pluginInstance.Impl.PeriodicInvalidatorStartDelay = cast.ToDuration(
			cfg["periodicInvalidatorStartDelay"])
		pluginInstance.Impl.PeriodicInvalidatorInterval = cast.ToDuration(
			cfg["periodicInvalidatorInterval"])
		pluginInstance.Impl.APIAddress = cast.ToString(cfg["apiAddress"])

		// Start the periodic invalidator.
		if pluginInstance.Impl.PeriodicInvalidatorEnabled {
			pluginInstance.Impl.PeriodicInvalidator()
		}
	}

	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: goplugin.HandshakeConfig{
			ProtocolVersion:  1,
			MagicCookieKey:   sdkConfig.GetEnv("MAGIC_COOKIE_KEY", ""),
			MagicCookieValue: sdkConfig.GetEnv("MAGIC_COOKIE_VALUE", ""),
		},
		Plugins: goplugin.PluginSet{
			plugin.PluginID.Name: pluginInstance,
		},
		GRPCServer: p.DefaultGRPCServer,
		Logger:     logger,
	})
}

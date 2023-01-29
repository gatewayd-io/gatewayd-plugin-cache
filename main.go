package main

import (
	"flag"
	"os"
	"strconv"

	redis_store "github.com/eko/gocache/store/redis/v4"
	"github.com/gatewayd-io/gatewayd-plugin-cache/plugin"
	sdkConfig "github.com/gatewayd-io/gatewayd-plugin-sdk/config"
	"github.com/gatewayd-io/gatewayd-plugin-sdk/logging"
	"github.com/gatewayd-io/gatewayd-plugin-sdk/metrics"
	"github.com/go-redis/redis/v8"
	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/mitchellh/mapstructure"
)

func main() {
	// Parse command line flags, passed by GatewayD via the plugin config
	logLevel := flag.String("log-level", "debug", "Log level")
	flag.Parse()

	logger := hclog.New(&hclog.LoggerOptions{
		Level:      logging.GetLogLevel(*logLevel),
		Output:     os.Stderr,
		JSONFormat: true,
		Color:      hclog.ColorOff,
	})

	pluginInstance := plugin.NewCachePlugin(plugin.Plugin{
		Logger: logger,
		RedisStore: redis_store.NewRedis(
			redis.NewClient(&redis.Options{
				Addr: os.Getenv("REDIS_ADDR"),
			}),
		),
	})

	var config map[string]interface{}
	mapstructure.Decode(plugin.PluginConfig["config"], &config)
	if metricsEnabled, err := strconv.ParseBool(config["metricsEnabled"].(string)); err == nil {
		metricsConfig := metrics.MetricsConfig{
			Enabled:          metricsEnabled,
			UnixDomainSocket: config["metricsUnixDomainSocket"].(string),
			Endpoint:         config["metricsEndpoint"].(string),
		}
		go metrics.ExposeMetrics(metricsConfig, logger)
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
		GRPCServer: goplugin.DefaultGRPCServer,
		Logger:     logger,
	})
}

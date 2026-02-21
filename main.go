package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/gatewayd-io/gatewayd-plugin-cache/plugin"
	sdkConfig "github.com/gatewayd-io/gatewayd-plugin-sdk/config"
	"github.com/gatewayd-io/gatewayd-plugin-sdk/logging"
	"github.com/gatewayd-io/gatewayd-plugin-sdk/metrics"
	p "github.com/gatewayd-io/gatewayd-plugin-sdk/plugin"
	v1 "github.com/gatewayd-io/gatewayd-plugin-sdk/plugin/v1"
	apiV1 "github.com/gatewayd-io/gatewayd/api/v1"
	"github.com/getsentry/sentry-go"
	"github.com/go-redis/redis/v8"
	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/spf13/cast"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	sentryDSN := sdkConfig.GetEnv("SENTRY_DSN", "")
	// Initialize Sentry SDK
	err := sentry.Init(sentry.ClientOptions{
		Dsn:              sentryDSN,
		TracesSampleRate: 1.0,
	})
	if err != nil {
		log.Fatalf("Failed to initialize Sentry SDK: %s", err.Error())
	}

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

	//nolint:nestif
	if cfg := cast.ToStringMap(plugin.PluginConfig["config"]); cfg != nil {
		pluginInstance.Impl.RedisURL = cast.ToString(cfg["redisURL"])
		pluginInstance.Impl.Expiry = cast.ToDuration(cfg["expiry"])
		pluginInstance.Impl.DefaultDBName = cast.ToString(cfg["defaultDBName"])
		pluginInstance.Impl.ScanCount = cast.ToInt64(cfg["scanCount"])
		pluginInstance.Impl.ExitOnStartupError = cast.ToBool(cfg["exitOnStartupError"])

		metricsConfig := metrics.NewMetricsConfig(cfg)
		if metricsConfig != nil && metricsConfig.Enabled {
			go metrics.ExposeMetrics(metricsConfig, logger)
		}

		// Initialize the API client to get proxies from GatewayD.
		apiGRPCAddress := cast.ToString(cfg["apiGRPCAddress"])
		apiClientConn, err := grpc.NewClient(
			apiGRPCAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil || apiClientConn == nil {
			logger.Error("Failed to initialize API client", "error", err)
			if pluginInstance.Impl.ExitOnStartupError {
				logger.Info("Exiting due to startup error")
				if apiClientConn != nil {
					apiClientConn.Close()
				}
				os.Exit(1)
			}
		}
		defer apiClientConn.Close()
		pluginInstance.Impl.APIClient = apiV1.NewGatewayDAdminAPIServiceClient(apiClientConn)

		cacheBufferSize := cast.ToUint(cfg["cacheBufferSize"])
		if cacheBufferSize <= 0 {
			cacheBufferSize = 100 // default value
		}

		pluginInstance.Impl.UpdateCacheChannel = make(chan *v1.Struct, cacheBufferSize)
		go pluginInstance.Impl.UpdateCache(context.Background())

		redisConfig, err := redis.ParseURL(pluginInstance.Impl.RedisURL)
		if err != nil {
			logger.Error("Failed to parse Redis URL", "error", err)
			if pluginInstance.Impl.ExitOnStartupError {
				logger.Info("Exiting due to startup error")
				if apiClientConn != nil {
					apiClientConn.Close()
				}
				os.Exit(1) //nolint:gocritic
			}
		}

		pluginInstance.Impl.RedisClient = redis.NewClient(redisConfig)

		// Ping the Redis server to check if it is available.
		_, err = pluginInstance.Impl.RedisClient.Ping(context.Background()).Result()
		if err != nil {
			logger.Error("Failed to ping Redis server", "error", err)
			if pluginInstance.Impl.ExitOnStartupError {
				logger.Info("Exiting due to startup error")
				if apiClientConn != nil {
					apiClientConn.Close()
				}
				os.Exit(1)
			}
		}

		pluginInstance.Impl.PeriodicInvalidatorEnabled = cast.ToBool(
			cfg["periodicInvalidatorEnabled"])
		pluginInstance.Impl.PeriodicInvalidatorStartDelay = cast.ToDuration(
			cfg["periodicInvalidatorStartDelay"])
		pluginInstance.Impl.PeriodicInvalidatorInterval = cast.ToDuration(
			cfg["periodicInvalidatorInterval"])

		// Start the periodic invalidator.
		if pluginInstance.Impl.PeriodicInvalidatorEnabled {
			pluginInstance.Impl.PeriodicInvalidator()
		}
	}

	if pluginInstance.Impl.UpdateCacheChannel != nil {
		defer close(pluginInstance.Impl.UpdateCacheChannel)
	}

	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: goplugin.HandshakeConfig{
			ProtocolVersion:  1,
			MagicCookieKey:   sdkConfig.GetEnv("MAGIC_COOKIE_KEY", ""),
			MagicCookieValue: sdkConfig.GetEnv("MAGIC_COOKIE_VALUE", ""),
		},
		Plugins: goplugin.PluginSet{
			plugin.PluginID.GetName(): pluginInstance,
		},
		GRPCServer: p.DefaultGRPCServer,
		Logger:     logger,
	})
}

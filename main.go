package main

import (
	"context"
	"flag"
	"io"
	"log"
	"os"
	"sync"

	"github.com/gatewayd-io/gatewayd-plugin-cache/plugin"
	sdkConfig "github.com/gatewayd-io/gatewayd-plugin-sdk/config"
	"github.com/gatewayd-io/gatewayd-plugin-sdk/logging"
	"github.com/gatewayd-io/gatewayd-plugin-sdk/metrics"
	p "github.com/gatewayd-io/gatewayd-plugin-sdk/plugin"
	v1 "github.com/gatewayd-io/gatewayd-plugin-sdk/plugin/v1"
	apiV1 "github.com/gatewayd-io/gatewayd/api/v1"
	"github.com/getsentry/sentry-go"
	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cast"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// handleStartupError logs an error and exits if ExitOnStartupError is set,
// closing any provided resources before exit.
func handleStartupError(
	logger hclog.Logger, exitOnError bool, msg string, err error, closers ...io.Closer,
) {
	logger.Error(msg, "error", err)
	if exitOnError {
		logger.Info("Exiting due to startup error")
		for _, c := range closers {
			if c != nil {
				c.Close()
			}
		}
		os.Exit(1)
	}
}

func main() {
	sentryDSN := sdkConfig.GetEnv("SENTRY_DSN", "")
	err := sentry.Init(sentry.ClientOptions{
		Dsn:              sentryDSN,
		TracesSampleRate: 1.0,
	})
	if err != nil {
		log.Fatalf("Failed to initialize Sentry SDK: %s", err.Error())
	}

	logLevel := flag.String("log-level", "info", "Log level")
	flag.Parse()

	logger := hclog.New(&hclog.LoggerOptions{
		Level:      logging.GetLogLevel(*logLevel),
		Output:     os.Stderr,
		JSONFormat: true,
		Color:      hclog.ColorOff,
	})

	pluginInstance := plugin.NewCachePlugin(plugin.Plugin{
		Logger:    logger,
		WaitGroup: &sync.WaitGroup{},
	})

	//nolint:nestif
	if cfg := cast.ToStringMap(plugin.PluginConfig["config"]); cfg != nil {
		pluginInstance.Impl.ExitOnStartupError = cast.ToBool(cfg["exitOnStartupError"])

		pluginInstance.Impl.RedisURL = cast.ToString(cfg["redisURL"])
		if pluginInstance.Impl.RedisURL == "" {
			logger.Warn("redisURL is empty, defaulting to redis://localhost:6379")
			pluginInstance.Impl.RedisURL = "redis://localhost:6379"
		}

		pluginInstance.Impl.Expiry = cast.ToDuration(cfg["expiry"])
		if pluginInstance.Impl.Expiry <= 0 {
			logger.Warn("expiry is invalid or unset, defaulting to 1h")
			pluginInstance.Impl.Expiry = cast.ToDuration("1h")
		}

		pluginInstance.Impl.DefaultDBName = cast.ToString(cfg["defaultDBName"])

		pluginInstance.Impl.ScanCount = cast.ToInt64(cfg["scanCount"])
		if pluginInstance.Impl.ScanCount <= 0 {
			logger.Warn("scanCount is invalid or unset, defaulting to 1000")
			pluginInstance.Impl.ScanCount = 1000
		}

		metricsConfig := metrics.NewMetricsConfig(cfg)
		if metricsConfig != nil && metricsConfig.Enabled {
			go metrics.ExposeMetrics(metricsConfig, logger)
		}

		apiGRPCAddress := cast.ToString(cfg["apiGRPCAddress"])
		apiClientConn, err := grpc.NewClient(
			apiGRPCAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil || apiClientConn == nil {
			handleStartupError(
				logger, pluginInstance.Impl.ExitOnStartupError,
				"Failed to initialize API client", err, apiClientConn)
		}
		defer apiClientConn.Close()
		pluginInstance.Impl.APIClient = apiV1.NewGatewayDAdminAPIServiceClient(apiClientConn)

		cacheBufferSize := cast.ToUint(cfg["cacheBufferSize"])
		if cacheBufferSize <= 0 {
			cacheBufferSize = 100
		}

		pluginInstance.Impl.UpdateCacheChannel = make(chan *v1.Struct, cacheBufferSize)
		pluginInstance.Impl.WaitGroup.Add(1)
		go pluginInstance.Impl.UpdateCache(context.Background())

		redisConfig, err := redis.ParseURL(pluginInstance.Impl.RedisURL)
		if err != nil {
			handleStartupError(
				logger, pluginInstance.Impl.ExitOnStartupError,
				"Failed to parse Redis URL", err, apiClientConn)
		}

		pluginInstance.Impl.RedisClient = redis.NewClient(redisConfig)

		_, err = pluginInstance.Impl.RedisClient.Ping(context.Background()).Result()
		if err != nil {
			handleStartupError(
				logger, pluginInstance.Impl.ExitOnStartupError,
				"Failed to ping Redis server", err, apiClientConn)
		}

		pluginInstance.Impl.PeriodicInvalidatorEnabled = cast.ToBool(
			cfg["periodicInvalidatorEnabled"])
		pluginInstance.Impl.PeriodicInvalidatorStartDelay = cast.ToDuration(
			cfg["periodicInvalidatorStartDelay"])
		pluginInstance.Impl.PeriodicInvalidatorInterval = cast.ToDuration(
			cfg["periodicInvalidatorInterval"])

		if pluginInstance.Impl.PeriodicInvalidatorEnabled {
			pluginInstance.Impl.PeriodicInvalidator()
		}
	}

	defer func() {
		if pluginInstance.Impl.UpdateCacheChannel != nil {
			close(pluginInstance.Impl.UpdateCacheChannel)
			pluginInstance.Impl.WaitGroup.Wait()
		}
	}()

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

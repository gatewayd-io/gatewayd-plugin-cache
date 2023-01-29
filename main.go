package main

import (
	"flag"
	"os"

	redis_store "github.com/eko/gocache/store/redis/v4"
	"github.com/gatewayd-io/gatewayd-plugin-cache/plugin"
	"github.com/go-redis/redis/v8"
	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
)

func main() {
	// Parse command line flags, passed by GatewayD via the plugin config
	logLevel := flag.String("log-level", "debug", "Log level")
	flag.Parse()

	logger := hclog.New(&hclog.LoggerOptions{
		Level:      GetLogLevel(*logLevel),
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
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: goplugin.HandshakeConfig{
			ProtocolVersion:  1,
			MagicCookieKey:   os.Getenv("MAGIC_COOKIE_KEY"),
			MagicCookieValue: os.Getenv("MAGIC_COOKIE_VALUE"),
		},
		Plugins: goplugin.PluginSet{
			plugin.PluginID.Name: pluginInstance,
		},
		GRPCServer: goplugin.DefaultGRPCServer,
		Logger:     logger,
	})
}

// GetLogLevel returns the hclog level based on the string passed in.
func GetLogLevel(logLevel string) hclog.Level {
	switch logLevel {
	case "trace":
		return hclog.Trace
	case "debug":
		return hclog.Debug
	case "info":
		return hclog.Info
	case "warn":
		return hclog.Warn
	case "error":
		return hclog.Error
	case "off":
		return hclog.Off
	default:
		return hclog.NoLevel
	}
}

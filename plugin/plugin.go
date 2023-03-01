package plugin

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/store"
	"github.com/eko/gocache/store/redis/v4"
	"github.com/gatewayd-io/gatewayd-plugin-sdk/databases/postgres"
	sdkPlugin "github.com/gatewayd-io/gatewayd-plugin-sdk/plugin"
	v1 "github.com/gatewayd-io/gatewayd-plugin-sdk/plugin/v1"
	goRedis "github.com/go-redis/redis/v8"
	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/spf13/cast"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
)

type Plugin struct {
	goplugin.GRPCPlugin
	v1.GatewayDPluginServiceServer

	Logger        hclog.Logger
	RedisClient   *goRedis.Client
	RedisStore    *redis.RedisStore
	RedisURL      string
	Expiry        time.Duration
	DefaultDBName string
}

type CachePlugin struct {
	goplugin.NetRPCUnsupportedPlugin
	Impl Plugin
}

// NewCachePlugin returns a new instance of the CachePlugin.
func NewCachePlugin(impl Plugin) *CachePlugin {
	return &CachePlugin{
		NetRPCUnsupportedPlugin: goplugin.NetRPCUnsupportedPlugin{},
		Impl:                    impl,
	}
}

// GRPCServer registers the plugin with the gRPC server.
func (p *CachePlugin) GRPCServer(b *goplugin.GRPCBroker, s *grpc.Server) error {
	v1.RegisterGatewayDPluginServiceServer(s, &p.Impl)
	return nil
}

// GRPCClient returns the plugin client.
func (p *CachePlugin) GRPCClient(ctx context.Context, b *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return v1.NewGatewayDPluginServiceClient(c), nil
}

// GetPluginConfig returns the plugin config.
func (p *Plugin) GetPluginConfig(
	ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	return structpb.NewStruct(PluginConfig)
}

// OnConfigLoaded is called when the global config is loaded by GatewayD.
func (p *Plugin) OnConfigLoaded(
	ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	return req, nil
}

// OnTrafficFromClient is called when a request is received by GatewayD from the client.
func (p *Plugin) OnTrafficFromClient(
	ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	cacheManager := cache.New[string](p.RedisStore)

	req, err := postgres.HandleClientMessage(req, p.Logger)
	if err != nil {
		p.Logger.Info("Failed to handle client message", "error", err)
	}

	// This is used as a fallback if the database is not found in the startup message.
	database := p.DefaultDBName
	if database == "" {
		client := cast.ToStringMapString(sdkPlugin.GetAttr(req, "client", nil))
		// Try to get the database from the startup message, which is only sent once by the client.
		// Store the database in the cache so that we can use it for subsequent requests.
		startupMessageEncoded := cast.ToString(sdkPlugin.GetAttr(req, "startupMessage", ""))
		if startupMessageEncoded != "" {
			startupMessageBytes, err := base64.StdEncoding.DecodeString(startupMessageEncoded)
			if err != nil {
				p.Logger.Debug("Failed to decode startup message", "error", err)
			} else {
				startupMessage := cast.ToStringMap(string(startupMessageBytes))
				p.Logger.Trace("Startup message", "startupMessage", startupMessage, "client", client)
				if startupMessage != nil && client != nil {
					startupMsgParams := cast.ToStringMapString(startupMessage["Parameters"])
					if startupMsgParams != nil &&
						startupMsgParams["database"] != "" &&
						client["remote"] != "" {
						if err := cacheManager.Set(
							ctx, client["remote"], startupMsgParams["database"]); err != nil {
							CacheMissesCounter.Inc()
							p.Logger.Debug("Failed to set cache", "error", err)
						}
						CacheSetsCounter.Inc()
						p.Logger.Debug("Set the database in the cache for the current session",
							"database", database, "client", client["remote"])
					}
				}
			}
		}

		// Get the database from the cache if it's not found in the startup message or
		// if the current request is not a startup message.
		if database == "" {
			database, err = cacheManager.Get(ctx, client["remote"])
			if err != nil {
				CacheMissesCounter.Inc()
				p.Logger.Debug("Failed to get cache", "error", err)
			}
			CacheGetsCounter.Inc()
			p.Logger.Debug("Get the database in the cache for the current session",
				"database", database, "client", client["remote"])
		}
	}

	// If the database is still not found, return the response as is without caching.
	// This might also happen if the cache is cleared while the client is still connected.
	// In this case, the client should reconnect and the error will go away.
	if database == "" {
		p.Logger.Error(
			"Database not found in the cache, startup message or plugin config. Skipping cache")
		return req, nil
	}

	query := cast.ToString(sdkPlugin.GetAttr(req, "query", ""))
	request := cast.ToString(sdkPlugin.GetAttr(req, "request", ""))
	server := cast.ToStringMapString(sdkPlugin.GetAttr(req, "server", ""))
	cacheKey := strings.Join([]string{server["remote"], database, request}, ":")

	if query != "" {
		p.Logger.Trace("Query", "query", query)
		response, err := cacheManager.Get(ctx, cacheKey)
		if err != nil {
			CacheMissesCounter.Inc()
			p.Logger.Debug("Failed to get cached response", "error", err)
		}
		CacheGetsCounter.Inc()

		if response != "" {
			CacheHitsCounter.Inc()
			// The response is cached.
			return structpb.NewStruct(map[string]interface{}{
				"terminate": true,
				"response":  response,
			})
		}
	}

	return req, nil
}

// OnTrafficFromServer is called when a response is received by GatewayD from the server.
func (p *Plugin) OnTrafficFromServer(
	ctx context.Context, resp *structpb.Struct) (*structpb.Struct, error) {
	cacheManager := cache.New[string](p.RedisStore)

	resp, err := postgres.HandleServerMessage(resp, p.Logger)
	if err != nil {
		p.Logger.Info("Failed to handle server message", "error", err)
	}

	rowDescription := cast.ToString(sdkPlugin.GetAttr(resp, "rowDescription", ""))
	dataRow := cast.ToStringSlice(sdkPlugin.GetAttr(resp, "dataRow", []interface{}{}))
	errorResponse := cast.ToString(sdkPlugin.GetAttr(resp, "errorResponse", ""))
	request := cast.ToString(sdkPlugin.GetAttr(resp, "request", ""))
	response := cast.ToString(sdkPlugin.GetAttr(resp, "response", ""))
	server := cast.ToStringMapString(sdkPlugin.GetAttr(resp, "server", ""))

	// This is used as a fallback if the database is not found in the startup message.
	database := p.DefaultDBName
	if database == "" {
		client := cast.ToStringMapString(sdkPlugin.GetAttr(resp, "client", ""))
		if client != nil && client["remote"] != "" {
			database, err = cacheManager.Get(ctx, client["remote"])
			if err != nil {
				CacheMissesCounter.Inc()
				p.Logger.Debug("Failed to get cached response", "error", err)
			}
			CacheGetsCounter.Inc()
		}
	}

	// If the database is still not found, return the response as is without caching.
	// This might also happen if the cache is cleared while the client is still connected.
	// In this case, the client should reconnect and the error will go away.
	if database == "" {
		p.Logger.Error(
			"Database not found in the cache, startup message or plugin config. Skipping cache")
		return resp, nil
	}

	cacheKey := strings.Join([]string{server["remote"], database, request}, ":")

	var options []store.Option
	if p.Expiry.Seconds() > 0 {
		p.Logger.Debug("Key expiry is set", "expiry", p.Expiry)
		options = append(options, store.WithExpiration(p.Expiry))
	}

	if errorResponse == "" && rowDescription != "" && dataRow != nil && len(dataRow) > 0 {
		if err := cacheManager.Set(ctx, cacheKey, response, options...); err != nil {
			CacheMissesCounter.Inc()
			p.Logger.Debug("Failed to set cache", "error", err)
		}
		CacheSetsCounter.Inc()
	}

	return resp, nil
}

func (p *Plugin) OnClosed(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	cacheManager := cache.New[string](p.RedisStore)
	client := cast.ToStringMapString(sdkPlugin.GetAttr(req, "client", nil))
	if client != nil {
		if err := cacheManager.Delete(ctx, client["remote"]); err != nil {
			p.Logger.Debug("Failed to delete cache", "error", err)
			CacheMissesCounter.Inc()
		}
		p.Logger.Debug("Client closed", "client", client["remote"])
		CacheDeletesCounter.Inc()
	}
	return req, nil
}

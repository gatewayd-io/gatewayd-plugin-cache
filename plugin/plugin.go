package plugin

import (
	"context"
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

	Logger hclog.Logger

	// Cache configuration.
	RedisClient   *goRedis.Client
	RedisStore    *redis.RedisStore
	RedisURL      string
	Expiry        time.Duration
	DefaultDBName string
	ScanCount     int64

	// Periodic invalidator configuration.
	PeriodicInvalidatorEnabled    bool
	PeriodicInvalidatorStartDelay time.Duration
	PeriodicInvalidatorInterval   time.Duration
	APIAddress                    string
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
	ctx context.Context, req *structpb.Struct,
) (*structpb.Struct, error) {
	return structpb.NewStruct(PluginConfig)
}

// OnTrafficFromClient is called when a request is received by GatewayD from the client.
func (p *Plugin) OnTrafficFromClient(
	ctx context.Context, req *structpb.Struct,
) (*structpb.Struct, error) {
	cacheManager := cache.New[string](p.RedisStore)

	req, err := postgres.HandleClientMessage(req, p.Logger)
	if err != nil {
		p.Logger.Info("Failed to handle client message", "error", err)
	}

	// This is used as a fallback if the database is not found in the startup message.
	database := p.DefaultDBName
	if database == "" {
		client := cast.ToStringMapString(sdkPlugin.GetAttr(req, "client", nil))
		database = p.getDBFromStartupMessage(req, cacheManager, database, client)

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
	preconditions := sdkPlugin.GetAttr(req, "sslRequest", "") != "" ||
		sdkPlugin.GetAttr(req, "saslInitialResponse", "") != "" ||
		sdkPlugin.GetAttr(req, "cancelRequest", "") != ""
	if database == "" && !preconditions {
		p.Logger.Error(
			"Database name not found or set in cache, startup message or plugin config. Skipping cache")
		p.Logger.Error("Consider setting the database name in the plugin config or disabling the plugin if you don't need it")
		return req, nil
	}

	query := cast.ToString(sdkPlugin.GetAttr(req, "query", ""))
	request := cast.ToString(sdkPlugin.GetAttr(req, "request", ""))
	server := cast.ToStringMapString(sdkPlugin.GetAttr(req, "server", ""))
	cacheKey := strings.Join([]string{server["remote"], database, request}, ":")

	if query != "" {
		p.Logger.Trace("Query", "query", query)

		// Clear the cache if the query is an insert, update or delete query.
		p.invalidateDML(query)

		// Check if the query is cached.
		response, err := cacheManager.Get(ctx, cacheKey)
		if err != nil {
			CacheMissesCounter.Inc()
			p.Logger.Debug("Failed to get cached response", "error", err)
		}
		CacheGetsCounter.Inc()

		// If the query is cached, return the cached response.
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
	ctx context.Context, resp *structpb.Struct,
) (*structpb.Struct, error) {
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
		p.Logger.Debug("Database name not found or set in cache, startup message or plugin config. Skipping cache")
		p.Logger.Debug("Consider setting the database name in the plugin config or disabling the plugin if you don't need it")
		return resp, nil
	}

	cacheKey := strings.Join([]string{server["remote"], database, request}, ":")

	options := []store.Option{}
	if p.Expiry.Seconds() > 0 {
		p.Logger.Debug("Key expiry is set", "expiry", p.Expiry)
		options = append(options, store.WithExpiration(p.Expiry))
	}

	if errorResponse == "" && rowDescription != "" && dataRow != nil && len(dataRow) > 0 {
		// The request was successful and the response contains data. Cache the response.
		if err := cacheManager.Set(ctx, cacheKey, response, options...); err != nil {
			CacheMissesCounter.Inc()
			p.Logger.Debug("Failed to set cache", "error", err)
		}
		CacheSetsCounter.Inc()

		// Cache the query as well.
		query, err := GetQueryFromRequest(request)
		if err != nil {
			p.Logger.Debug("Failed to get query from request", "error", err)
		} else {
			tables, err := GetTablesFromQuery(query)
			if err != nil {
				p.Logger.Debug("Failed to get tables from query", "error", err)
			} else {
				// Cache the table(s) used in each cached request. This is used to invalidate
				// the cache when a rows is inserted, updated or deleted into that table.
				for _, table := range tables {
					requestQueryCacheKey := strings.Join([]string{table, cacheKey}, ":")
					if err := cacheManager.Set(
						ctx, requestQueryCacheKey, "", options...); err != nil {
						CacheMissesCounter.Inc()
						p.Logger.Debug("Failed to set cache", "error", err)
					}
					CacheSetsCounter.Inc()
				}
			}
		}
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

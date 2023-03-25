package plugin

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

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
func (p *CachePlugin) GRPCServer(_ *goplugin.GRPCBroker, s *grpc.Server) error {
	v1.RegisterGatewayDPluginServiceServer(s, &p.Impl)
	return nil
}

// GRPCClient returns the plugin client.
func (p *CachePlugin) GRPCClient(_ context.Context, _ *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return v1.NewGatewayDPluginServiceClient(c), nil
}

// GetPluginConfig returns the plugin config.
func (p *Plugin) GetPluginConfig(
	_ context.Context, _ *structpb.Struct,
) (*structpb.Struct, error) {
	GetPluginConfigCounter.Inc()
	return structpb.NewStruct(PluginConfig)
}

// OnTrafficFromClient is called when a request is received by GatewayD from the client.
func (p *Plugin) OnTrafficFromClient(
	ctx context.Context, req *structpb.Struct,
) (*structpb.Struct, error) {
	OnTrafficFromClientCounter.Inc()
	req, err := postgres.HandleClientMessage(req, p.Logger)
	if err != nil {
		p.Logger.Info("Failed to handle client message", "error", err)
	}

	// This is used as a fallback if the database is not found in the startup message.
	database := p.DefaultDBName
	if database == "" {
		client := cast.ToStringMapString(sdkPlugin.GetAttr(req, "client", nil))
		database = p.getDBFromStartupMessage(ctx, req, database, client)

		// Get the database from the cache if it's not found in the startup message or
		// if the current request is not a startup message.
		if database == "" {
			database, err = p.RedisClient.Get(ctx, client["remote"]).Result()
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
		p.invalidateDML(ctx, query)

		// Check if the query is cached.
		response, err := p.RedisClient.Get(ctx, cacheKey).Result()
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
	OnTrafficFromServerCounter.Inc()
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
			database, err = p.RedisClient.Get(ctx, client["remote"]).Result()
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
	if errorResponse == "" && rowDescription != "" && dataRow != nil && len(dataRow) > 0 {
		// The request was successful and the response contains data. Cache the response.
		if err := p.RedisClient.Set(ctx, cacheKey, response, p.Expiry).Err(); err != nil {
			CacheMissesCounter.Inc()
			p.Logger.Debug("Failed to set cache", "error", err)
		}
		CacheSetsCounter.Inc()

		// Cache the query as well.
		query, err := postgres.GetQueryFromRequest(request)
		if err != nil {
			p.Logger.Debug("Failed to get query from request", "error", err)
			return resp, nil
		}

		tables, err := postgres.GetTablesFromQuery(query)
		if err != nil {
			p.Logger.Debug("Failed to get tables from query", "error", err)
			return resp, nil
		}

		// Cache the table(s) used in each cached request. This is used to invalidate
		// the cache when a rows is inserted, updated or deleted into that table.
		for _, table := range tables {
			requestQueryCacheKey := strings.Join([]string{table, cacheKey}, ":")
			if err := p.RedisClient.Set(
				ctx, requestQueryCacheKey, "", p.Expiry).Err(); err != nil {
				CacheMissesCounter.Inc()
				p.Logger.Debug("Failed to set cache", "error", err)
			}
			CacheSetsCounter.Inc()
		}
	}

	return resp, nil
}

func (p *Plugin) OnClosed(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	OnClosedCounter.Inc()
	client := cast.ToStringMapString(sdkPlugin.GetAttr(req, "client", nil))
	if client != nil {
		if err := p.RedisClient.Del(ctx, client["remote"]).Err(); err != nil {
			p.Logger.Debug("Failed to delete cache", "error", err)
			CacheMissesCounter.Inc()
		}
		p.Logger.Debug("Client closed", "client", client["remote"])
		CacheDeletesCounter.Inc()
	}
	return req, nil
}

// invalidateDML invalidates the cache for the tables that are affected by the DML.
// This is done by getting the cached queries for each table and deleting them.
func (p *Plugin) invalidateDML(ctx context.Context, query string) {
	// Check if the query is a UPDATE, INSERT or DELETE.
	queryDecoded, err := base64.StdEncoding.DecodeString(query)
	if err != nil {
		p.Logger.Debug("Failed to decode query", "error", err)
		return
	}

	queryMessage := cast.ToStringMapString(string(queryDecoded))
	p.Logger.Trace("Query message", "query", queryMessage)

	queryString := strings.ToUpper(queryMessage["String"])
	// Ignore SELECT and WITH/SELECT queries.
	// TODO: This is a naive approach, but query parsing has a cost.
	if strings.HasPrefix(queryString, "SELECT") ||
		(strings.HasPrefix(queryString, "WITH") &&
			strings.Contains(queryString, "SELECT")) {
		return
	}

	tables, err := postgres.GetTablesFromQuery(queryMessage["String"])
	if err != nil {
		p.Logger.Debug("Failed to get tables from query", "error", err)
		return
	}

	p.Logger.Trace("Tables", "tables", tables)
	for _, table := range tables {
		// Invalidate the cache for the table.
		// TODO: This is not efficient. We should be able to invalidate the cache
		// for a specific key instead of invalidating the entire table.
		pipeline := p.RedisClient.Pipeline()
		var cursor uint64
		for {
			scanResult := p.RedisClient.Scan(ctx, cursor, table+":*", p.ScanCount)
			if scanResult.Err() != nil {
				CacheMissesCounter.Inc()
				p.Logger.Debug("Failed to scan keys", "error", scanResult.Err())
				break
			}
			CacheScanCounter.Inc()

			// Per each key, delete the cache entry and the table cache key itself.
			var keys []string
			keys, cursor = scanResult.Val()
			CacheScanKeysCounter.Add(float64(len(keys)))
			for _, tableKey := range keys {
				// Invalidate the cache for the table.
				cachedRespnseKey := strings.TrimPrefix(tableKey, table+":")
				pipeline.Del(ctx, cachedRespnseKey)
				// Invalidate the table cache key itself.
				pipeline.Del(ctx, tableKey)
			}

			if cursor == 0 {
				break
			}
		}

		result, err := pipeline.Exec(ctx)
		if err != nil {
			p.Logger.Debug("Failed to execute pipeline", "error", err)
		}

		for _, res := range result {
			if res.Err() != nil {
				CacheMissesCounter.Inc()
			} else {
				CacheDeletesCounter.Inc()
			}
		}
	}
}

// getDBFromStartupMessage gets the database name from the startup message.
func (p *Plugin) getDBFromStartupMessage(
	ctx context.Context,
	req *structpb.Struct,
	database string,
	client map[string]string,
) string {
	// Try to get the database from the startup message, which is only sent once by the client.
	// Store the database in the cache so that we can use it for subsequent requests.
	startupMessageEncoded := cast.ToString(sdkPlugin.GetAttr(req, "startupMessage", ""))
	if startupMessageEncoded == "" {
		return database
	}

	startupMessageBytes, err := base64.StdEncoding.DecodeString(startupMessageEncoded)
	if err != nil {
		p.Logger.Debug("Failed to decode startup message", "error", err)
		return database
	}

	startupMessage := cast.ToStringMap(string(startupMessageBytes))
	p.Logger.Trace("Startup message", "startupMessage", startupMessage, "client", client)
	if startupMessage != nil && client != nil {
		startupMsgParams := cast.ToStringMapString(startupMessage["Parameters"])
		if startupMsgParams != nil &&
			startupMsgParams["database"] != "" &&
			client["remote"] != "" {
			if err := p.RedisClient.Set(
				ctx, client["remote"],
				startupMsgParams["database"],
				time.Duration(0),
			).Err(); err != nil {
				CacheMissesCounter.Inc()
				p.Logger.Debug("Failed to set cache", "error", err)
			}
			CacheSetsCounter.Inc()
			p.Logger.Debug("Set the database in the cache for the current session",
				"database", database, "client", client["remote"])
			return startupMsgParams["database"]
		}
	}

	return database
}

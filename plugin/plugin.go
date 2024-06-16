package plugin

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	sdkAct "github.com/gatewayd-io/gatewayd-plugin-sdk/act"
	"github.com/gatewayd-io/gatewayd-plugin-sdk/databases/postgres"
	sdkPlugin "github.com/gatewayd-io/gatewayd-plugin-sdk/plugin"
	v1 "github.com/gatewayd-io/gatewayd-plugin-sdk/plugin/v1"
	goRedis "github.com/go-redis/redis/v8"
	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/spf13/cast"
	"google.golang.org/grpc"
)

type Plugin struct {
	goplugin.GRPCPlugin
	v1.GatewayDPluginServiceServer

	Logger hclog.Logger

	// Cache configuration.
	RedisClient        *goRedis.Client
	RedisURL           string
	Expiry             time.Duration
	DefaultDBName      string
	ScanCount          int64
	ExitOnStartupError bool

	UpdateCacheChannel chan *v1.Struct

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

// Define a set for PostgreSQL date/time functions
// https://www.postgresql.org/docs/8.2/functions-datetime.html
var pgDateTimeFunctions = map[string]struct{}{
	"AGE":                   {},
	"CLOCK_TIMESTAMP":       {},
	"CURRENT_DATE":          {},
	"CURRENT_TIME":          {},
	"CURRENT_TIMESTAMP":     {},
	"LOCALTIME":             {},
	"LOCALTIMESTAMP":        {},
	"NOW":                   {},
	"STATEMENT_TIMESTAMP":   {},
	"TIMEOFDAY":             {},
	"TRANSACTION_TIMESTAMP": {},
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
	_ context.Context, _ *v1.Struct,
) (*v1.Struct, error) {
	GetPluginConfigCounter.Inc()
	return v1.NewStruct(PluginConfig)
}

// OnTrafficFromClient is called when a request is received by GatewayD from the client.
func (p *Plugin) OnTrafficFromClient(
	ctx context.Context, req *v1.Struct,
) (*v1.Struct, error) {
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

	if query == "" {
		return req, nil
	}

	p.Logger.Trace("Query", "query", query)

	// Clear the cache if the query is an insert, update or delete query.
	p.invalidateDML(ctx, query)

	// Check if the query is cached.
	response, err := p.RedisClient.Get(ctx, cacheKey).Bytes()
	if err != nil {
		CacheMissesCounter.Inc()
		p.Logger.Debug("Failed to get cached response", "error", err)
	}
	CacheGetsCounter.Inc()

	if response == nil {
		// If the query is not cached, return the request as is.
		CacheMissesCounter.Inc()
		return req, nil
	}

	// If the query is cached, return the cached response.
	signals, err := v1.NewList([]any{
		sdkAct.Terminate().ToMap(),
		sdkAct.Log("debug", "Returning cached response", map[string]any{
			"cacheKey": cacheKey,
			"plugin":   PluginID.GetName(),
		}).ToMap(),
	})
	if err != nil {
		CacheMissesCounter.Inc()
		// This should never happen, but log the error just in case.
		p.Logger.Error("Failed to create signals", "error", err)
	} else {
		CacheHitsCounter.Inc()
		// Return the cached response.
		req.Fields[sdkAct.Signals] = v1.NewListValue(signals)
		req.Fields["response"] = v1.NewBytesValue(response)
	}
	return req, nil
}

// IsCacheNeeded determines if caching is needed.
func IsCacheNeeded(upperQuery string) bool {
	// Iterate over each function name in the set of PostgreSQL date/time functions.
	for function := range pgDateTimeFunctions {
		if strings.Contains(upperQuery, function) {
			// If the query contains a date/time function, caching is not needed.
			return false
		}
	}
	return true
}

func (p *Plugin) UpdateCache(ctx context.Context) {
	for {
		serverResponse, ok := <-p.UpdateCacheChannel
		if !ok {
			p.Logger.Info("Channel closed, returning from function")
			return
		}

		OnTrafficFromServerCounter.Inc()
		resp, err := postgres.HandleServerMessage(serverResponse, p.Logger)
		if err != nil {
			p.Logger.Info("Failed to handle server message", "error", err)
		}

		rowDescription := cast.ToString(sdkPlugin.GetAttr(resp, "rowDescription", ""))
		dataRow := cast.ToStringSlice(sdkPlugin.GetAttr(resp, "dataRow", []interface{}{}))
		errorResponse := cast.ToString(sdkPlugin.GetAttr(resp, "errorResponse", ""))
		request, isOk := sdkPlugin.GetAttr(resp, "request", nil).([]byte)
		if !isOk {
			request = []byte{}
		}

		response, isOk := sdkPlugin.GetAttr(resp, "response", nil).([]byte)
		if !isOk {
			response = []byte{}
		}
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
			p.Logger.Debug("Database name not found or set in cache, startup message or plugin config. " +
				"Skipping cache")
			p.Logger.Debug("Consider setting the database name in the " +
				"plugin config or disabling the plugin if you don't need it")
			return
		}

		cacheKey := strings.Join([]string{server["remote"], database, string(request)}, ":")
		if errorResponse == "" && rowDescription != "" && dataRow != nil && len(dataRow) > 0 && IsCacheNeeded(cacheKey) {
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
				return
			}

			tables, err := postgres.GetTablesFromQuery(query)
			if err != nil {
				p.Logger.Debug("Failed to get tables from query", "error", err)
				return
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
	}
}

// OnTrafficFromServer is called when a response is received by GatewayD from the server.
func (p *Plugin) OnTrafficFromServer(
	_ context.Context, resp *v1.Struct,
) (*v1.Struct, error) {
	p.Logger.Debug("Traffic is coming from the server side")
	p.UpdateCacheChannel <- resp
	return resp, nil
}

func (p *Plugin) OnClosed(ctx context.Context, req *v1.Struct) (*v1.Struct, error) {
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
	req *v1.Struct,
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

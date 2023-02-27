package plugin

import (
	"context"

	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/store"
	"github.com/eko/gocache/store/redis/v4"
	"github.com/gatewayd-io/gatewayd-plugin-sdk/databases/postgres"
	sdkPlugin "github.com/gatewayd-io/gatewayd-plugin-sdk/plugin"
	v1 "github.com/gatewayd-io/gatewayd-plugin-sdk/plugin/v1"
	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/spf13/cast"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
)

type Plugin struct {
	goplugin.GRPCPlugin
	v1.GatewayDPluginServiceServer
	Logger     hclog.Logger
	RedisStore *redis.RedisStore
	Expiry     time.Duration
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

	query := cast.ToString(sdkPlugin.GetAttr(req, "query", ""))
	request := cast.ToString(sdkPlugin.GetAttr(req, "request", ""))

	if query != "" {
		p.Logger.Trace("Query", "query", query)
		response, err := cacheManager.Get(ctx, request)
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


	var options []store.Option
	if p.Expiry.Seconds() > 0 {
		p.Logger.Debug("Key expiry is set", "expiry", p.Expiry)
		options = append(options, store.WithExpiration(p.Expiry))
	}

	if errorResponse == "" && rowDescription != "" && dataRow != nil && len(dataRow) > 0 {
		if err := cacheManager.Set(ctx, request, response, options...); err != nil {
			CacheMissesCounter.Inc()
			p.Logger.Debug("Failed to set cache", "error", err)
		}
		CacheSetsCounter.Inc()
	}

	return resp, nil
}

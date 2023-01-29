package plugin

import (
	"context"
	"encoding/base64"

	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/store/redis/v4"
	pluginV1 "github.com/gatewayd-io/gatewayd-plugin-cache/plugin/v1"
	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
)

var (
	PluginID = pluginV1.PluginID{
		Name:      "gatewayd-plugin-cache",
		Version:   "0.0.1",
		RemoteUrl: "github.com/gatewayd-io/gatewayd-plugin-cache",
	}
	PluginMap = map[string]goplugin.Plugin{
		PluginID.Name: &CachePlugin{},
	}
	Config = map[string]interface{}{
		"id": map[string]interface{}{
			"name":      PluginID.Name,
			"version":   PluginID.Version,
			"remoteUrl": PluginID.RemoteUrl,
		},
		"description": "GatewayD plugin for caching query results",
		"authors": []interface{}{
			"Mostafa Moradian <mostafa@gatewayd.io>",
		},
		"license":    "Apache-2.0",
		"projectUrl": "https://github.com/gatewayd-io/gatewayd-plugin-cache",
		"config": map[string]interface{}{
			// If you expect to run multiple instances of this plugin, you should
			// set a unique socket path for each instance.
			"socketPath":  "/tmp/gatewayd-plugin-cache.sock",
			"metricsPath": "/metrics",
		},
		"hooks": []interface{}{
			"onConfigLoaded",
			"onTrafficFromClient",
			"onTrafficFromServer",
		},
		"tags":       []interface{}{"test", "plugin"},
		"categories": []interface{}{"test"},
	}
)

type Plugin struct {
	goplugin.GRPCPlugin
	pluginV1.GatewayDPluginServiceServer
	Logger     hclog.Logger
	RedisStore *redis.RedisStore
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
	pluginV1.RegisterGatewayDPluginServiceServer(s, &p.Impl)
	return nil
}

// GRPCClient returns the plugin client.
func (p *CachePlugin) GRPCClient(ctx context.Context, b *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return pluginV1.NewGatewayDPluginServiceClient(c), nil
}

// GetPluginConfig returns the plugin config.
func (p *Plugin) GetPluginConfig(
	ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	return structpb.NewStruct(Config)
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

	request := req.Fields["request"].GetStringValue()
	if r, err := base64.StdEncoding.DecodeString(request); err == nil {
		// A PostgreSQL request is received from the client.
		if r[0] == 'Q' { // Query
			response, err := cacheManager.Get(ctx, request)
			if err != nil {
				CacheMissesCounter.Inc()
				p.Logger.Error("Failed to get cache", err)
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
	}

	return req, nil
}

// OnTrafficFromServer is called when a response is received by GatewayD from the server.
func (p *Plugin) OnTrafficFromServer(
	ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	cacheManager := cache.New[string](p.RedisStore)

	request := req.Fields["request"].GetStringValue()
	response := req.Fields["response"].GetStringValue()
	if r, err := base64.StdEncoding.DecodeString(response); err == nil {
		// A PostgreSQL response is received from the client.
		if r[0] == 'T' { // RowDescription
			if err := cacheManager.Set(ctx, request, response); err != nil {
				CacheMissesCounter.Inc()
				p.Logger.Error("Failed to set cache", err)
			}
			CacheSetsCounter.Inc()
		}
	}

	return req, nil
}

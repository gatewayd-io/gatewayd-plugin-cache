package plugin

import (
	pluginV1 "github.com/gatewayd-io/gatewayd-plugin-cache/plugin/v1"
	goplugin "github.com/hashicorp/go-plugin"
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
	PluginConfig = map[string]interface{}{
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
		// Compile-time configuration
		"config": map[string]interface{}{
			"metricsEnabled":          "true",
			"metricsUnixDomainSocket": "/tmp/gatewayd-plugin-cache.sock",
			"metricsEndpoint":         "/metrics",
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

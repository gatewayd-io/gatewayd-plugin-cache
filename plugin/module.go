package plugin

import (
	sdkConfig "github.com/gatewayd-io/gatewayd-plugin-sdk/config"
	v1 "github.com/gatewayd-io/gatewayd-plugin-sdk/plugin/v1"
	goplugin "github.com/hashicorp/go-plugin"
)

var (
	PluginID = v1.PluginID{
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
			"metricsEnabled": sdkConfig.GetEnv("METRICS_ENABLED", "true"),
			"metricsUnixDomainSocket": sdkConfig.GetEnv(
				"METRICS_UNIX_DOMAIN_SOCKET", "/tmp/gatewayd-plugin-cache.sock"),
			"metricsEndpoint": sdkConfig.GetEnv("METRICS_ENDPOINT", "/metrics"),
			"redisURL":        sdkConfig.GetEnv("REDIS_URL", "redis://localhost:6379/0"),
			"expiry":          sdkConfig.GetEnv("EXPIRY", "1h"),
			"defaultDBName":   sdkConfig.GetEnv("DEFAULT_DB_NAME", ""),
			"periodicInvalidatorEnabled": sdkConfig.GetEnv(
				"PERIODIC_INVALIDATOR_ENABLED", "true"),
			"periodicInvalidatorStartDelay": sdkConfig.GetEnv(
				"PERIODIC_INVALIDATOR_START_DELAY", "1m"),
			"periodicInvalidatorInterval": sdkConfig.GetEnv(
				"PERIODIC_INVALIDATOR_INTERVAL", "1m"),
		},
		"hooks": []interface{}{
			"onConfigLoaded",
			"onTrafficFromClient",
			"onTrafficFromServer",
			"onClosed",
		},
		"tags":       []interface{}{"plugin", "cache", "redis", "postgres"},
		"categories": []interface{}{"builtin", "cache", "redis", "postgres"},
	}
)

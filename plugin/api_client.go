package plugin

import (
	"encoding/json"
	"net/http"
)

type Proxy struct {
	Available []string `json:"available"`
	Busy      []string `json:"busy"`
	Total     int      `json:"total"`
}

// getProxies returns a list of proxies from GatewayD.
func (p *Plugin) getProxies() map[string]Proxy {
	if p.APIAddress == "" {
		p.Logger.Error("Failed to get a list of proxies from GatewayD", "error", "APIAddress is not set")
		return nil
	}

	//nolint: noctx
	resp, err := http.Get("http://" + p.APIAddress + "/v1/GatewayDPluginService/GetProxies")
	if err != nil {
		p.Logger.Error("Failed to get a list of proxies from GatewayD", "error", err)
		return nil
	}
	defer resp.Body.Close()

	proxies := map[string]Proxy{}
	if err = json.NewDecoder(resp.Body).Decode(&proxies); err != nil {
		p.Logger.Error("Failed to decode response from GatewayD", "error", err)
		return nil
	}

	return proxies
}

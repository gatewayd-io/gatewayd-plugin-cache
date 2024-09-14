package plugin

import (
	"context"
	"encoding/json"

	"google.golang.org/protobuf/types/known/emptypb"
)

type Proxy struct {
	Available []string `json:"available"`
	Busy      []string `json:"busy"`
	Total     int      `json:"total"`
}

// getProxies returns a list of proxies from GatewayD.
func (p *Plugin) getProxies() map[string]map[string]Proxy {
	if p.APIClient == nil {
		p.Logger.Error(
			"Failed to get a list of proxies from GatewayD",
			"error", "API client is not initialized",
		)
		return nil
	}

	proxies, err := p.APIClient.GetProxies(context.Background(), &emptypb.Empty{})
	if err != nil {
		p.Logger.Error("Failed to get a list of proxies from GatewayD", "error", err)
		return nil
	}

	data, err := proxies.MarshalJSON()
	if err != nil {
		p.Logger.Error("Failed to marshal response from GatewayD", "error", err)
		return nil
	}

	var pxy map[string]map[string]Proxy
	if err = json.Unmarshal(data, &pxy); err != nil {
		p.Logger.Error("Failed to unmarshal response from GatewayD", "error", err)
		return nil
	}

	return pxy
}

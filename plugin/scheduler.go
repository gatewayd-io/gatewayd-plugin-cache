package plugin

import (
	"context"
	"time"

	"github.com/go-co-op/gocron"
)

// PeriodicInvalidator is a function that runs periodically and deletes all the
// cached client keys that are not valid anymore. This has two purposes:
// 1. If a client is not connected to the GatewayD anymore, it will be deleted.
// 2. Invalidate stale keys for responses. (This is not implemented yet.)
// https://github.com/gatewayd-io/gatewayd-plugin-cache/issues/4
func (p *Plugin) PeriodicInvalidator() {
	scheduler := gocron.NewScheduler(time.UTC)
	startDelay := time.Now().Add(p.PeriodicInvalidatorStartDelay)

	if _, err := scheduler.Every(p.PeriodicInvalidatorInterval).SingletonMode().StartAt(startDelay).Do(func() {
		proxies := p.getProxies()
		p.Logger.Trace("Got proxies from GatewayD", "proxies", proxies)

		// Get all the client keys and delete the ones that are not valid.
		// TODO: use scan instead of keys.
		for _, address := range p.RedisClient.Keys(context.Background(), "*:*").Val() {
			valid := false

			// Validate the address if the address is an IP address.
			if ok, err := validateAddressPort(address); ok && err == nil {
				valid = true
			} else {
				p.Logger.Trace(
					"Skipping connection because it is invalid", "address", address, "error", err)
			}

			if !valid {
				// Validate the address if the address is a hostname.
				if ok, err := validateHostPort(address); ok && err == nil {
					valid = true
				} else {
					p.Logger.Trace(
						"Skipping connection because it is invalid", "address", address, "error", err)
					continue
				}
			}

			// If the address is not valid, skip it.
			if !valid {
				continue
			}

			// If the connection is busy (a client is connected), it is not safe to delete the key.
			if isBusy(proxies, address) {
				p.Logger.Trace("Skipping connection because it is busy", "address", address)
				continue
			}

			p.RedisClient.Del(context.Background(), address)
			p.Logger.Trace("Deleted stale address", "address", address)
			CacheDeletesCounter.Inc()
		}
	}); err != nil {
		p.Logger.Error("Failed to start periodic invalidator",
			"error", err,
			"interval", p.PeriodicInvalidatorInterval.String(),
			"delay", p.PeriodicInvalidatorStartDelay.String())
		return
	}

	scheduler.StartAsync()
	p.Logger.Debug("Started periodic invalidator",
		"interval", p.PeriodicInvalidatorInterval.String(),
		"delay", p.PeriodicInvalidatorStartDelay.String())
}

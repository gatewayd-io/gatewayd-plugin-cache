package plugin

import (
	"context"
	"time"

	"github.com/go-co-op/gocron"
)

func (p *Plugin) PeriodicInvalidator(interval, delay time.Duration) {
	scheduler := gocron.NewScheduler(time.UTC)
	startDelay := time.Now().Add(delay)

	if _, err := scheduler.Every(interval).SingletonMode().StartAt(startDelay).Do(func() {
		// TODO: Get list of valid keys from GatewayD and bypass those.
		for _, key := range p.RedisClient.Keys(context.Background(), "*:*").Val() {
			if validateAddressPort(key) || validateHostPort(key) {
				p.RedisClient.Del(context.Background(), key)
				p.Logger.Debug("Deleted key", "key", key)
				CacheDeletesCounter.Inc()
			}
		}
	}); err != nil {
		p.Logger.Error("Failed to start periodic invalidator",
			"error", err, "interval", interval.String(), "delay", delay.String())
		return
	}

	scheduler.StartAsync()
	p.Logger.Debug("Started periodic invalidator",
		"interval", interval.String(), "delay", delay.String())
}

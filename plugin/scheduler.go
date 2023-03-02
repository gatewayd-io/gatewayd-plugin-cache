package plugin

import (
	"context"
	"regexp"
	"time"

	"github.com/go-co-op/gocron"
)

func (p *Plugin) PeriodicInvalidator(interval, delay time.Duration) {
	ipPortPattern := regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}:\d{1,5}$`)
	hostPortPattern := regexp.MustCompile(`^\w+:\d{1,5}$`)
	scheduler := gocron.NewScheduler(time.UTC)

	startDelay := time.Now().Add(delay)
	scheduler.Every(interval).SingletonMode().At(startDelay).Do(func() {
		// TODO: Get list of valid keys from GatewayD and bypass those.

		count := 0
		keys := p.RedisClient.Keys(context.Background(), "*:*")
		for _, key := range keys.Val() {
			if ipPortPattern.MatchString(key) || hostPortPattern.MatchString(key) {
				p.RedisClient.Del(context.Background(), key)
				p.Logger.Debug("Deleted key", "key", key)
				CacheDeletesCounter.Inc()
				count++
			}
		}

		if count > 0 {
			p.Logger.Trace("Finished invalidating stale keys")
		} else {
			p.Logger.Trace("No stale keys to invalidate")
		}
	})

	scheduler.StartAsync()
}

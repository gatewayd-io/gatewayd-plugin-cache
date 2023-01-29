package plugin

import (
	"os"

	"github.com/hashicorp/go-hclog"
)

// GetLogLevel returns the hclog level based on the string passed in.
func GetLogLevel(logLevel string) hclog.Level {
	switch logLevel {
	case "trace":
		return hclog.Trace
	case "debug":
		return hclog.Debug
	case "info":
		return hclog.Info
	case "warn":
		return hclog.Warn
	case "error":
		return hclog.Error
	case "off":
		return hclog.Off
	default:
		return hclog.NoLevel
	}
}

func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

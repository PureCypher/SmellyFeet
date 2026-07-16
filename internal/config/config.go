// Package config loads frontend settings from the environment.
package config

import (
	"os"
	"strings"
)

// Config holds frontend runtime settings.
type Config struct {
	APIBaseURL     string // base URL of the Information-Broker API
	Port           string // port the frontend listens on
	MeetupsEnabled bool   // gate the Meetups tab + /meetups* + /api/meetups routes
	MeetupsTZ      string // display timezone for meetup times
}

// Load reads configuration from the environment, applying defaults.
func Load() Config {
	return Config{
		APIBaseURL:     getenv("API_BASE_URL", "http://localhost:8080"),
		Port:           getenv("PORT", "3000"),
		MeetupsEnabled: getenvBool("MEETUPS_ENABLED", true),
		MeetupsTZ:      getenv("MEETUPS_DEFAULT_TZ", "Europe/London"),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// getenvBool reads a boolean env var; empty is the default. "1"/"true"/"yes"
// (any case) are true, everything else false.
func getenvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

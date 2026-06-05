// Package config loads frontend settings from the environment.
package config

import "os"

// Config holds frontend runtime settings.
type Config struct {
	APIBaseURL string // base URL of the Information-Broker API
	Port       string // port the frontend listens on
}

// Load reads configuration from the environment, applying defaults.
func Load() Config {
	return Config{
		APIBaseURL: getenv("API_BASE_URL", "http://localhost:8080"),
		Port:       getenv("PORT", "3000"),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("API_BASE_URL", "")
	t.Setenv("PORT", "")
	c := Load()
	if c.APIBaseURL != "http://localhost:8080" {
		t.Errorf("APIBaseURL default = %q", c.APIBaseURL)
	}
	if c.Port != "3000" {
		t.Errorf("Port default = %q", c.Port)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("API_BASE_URL", "http://192.168.1.135:8080")
	t.Setenv("PORT", "9999")
	c := Load()
	if c.APIBaseURL != "http://192.168.1.135:8080" {
		t.Errorf("APIBaseURL = %q", c.APIBaseURL)
	}
	if c.Port != "9999" {
		t.Errorf("Port = %q", c.Port)
	}
}

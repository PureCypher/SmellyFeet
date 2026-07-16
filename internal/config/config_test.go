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

func TestLoadMeetupDefaults(t *testing.T) {
	t.Setenv("MEETUPS_ENABLED", "")
	t.Setenv("MEETUPS_DEFAULT_TZ", "")
	t.Setenv("MEETUPS_NOTIFY_WEBHOOK", "")
	c := Load()
	if !c.MeetupsEnabled {
		t.Error("MeetupsEnabled should default to true")
	}
	if c.MeetupsTZ != "Europe/London" {
		t.Errorf("MeetupsTZ = %q, want Europe/London", c.MeetupsTZ)
	}
	if c.MeetupsWebhook != "" {
		t.Errorf("MeetupsWebhook = %q, want empty", c.MeetupsWebhook)
	}
}

func TestGetenvBool(t *testing.T) {
	cases := []struct {
		val  string
		def  bool
		want bool
	}{
		{"", true, true}, {"", false, false},
		{"1", false, true}, {"true", false, true}, {"TRUE", false, true},
		{"yes", false, true}, {"0", true, false}, {"nope", true, false},
	}
	for _, tc := range cases {
		t.Setenv("X_BOOL", tc.val)
		if got := getenvBool("X_BOOL", tc.def); got != tc.want {
			t.Errorf("getenvBool(%q, %v) = %v, want %v", tc.val, tc.def, got, tc.want)
		}
	}
}

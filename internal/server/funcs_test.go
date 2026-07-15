package server

import (
	"testing"
	"time"
)

func TestCommas(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{0, "0"}, {5, "5"}, {130, "130"}, {983, "983"},
		{1000, "1,000"}, {50913, "50,913"}, {10488, "10,488"},
		{-1234, "-1,234"},
	}
	for _, tt := range tests {
		if got := commas(tt.in); got != tt.want {
			t.Errorf("commas(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSourceName(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"strips www and path", "https://www.brighttalk.com/channel/7451/feed/rss", "brighttalk.com"},
		{"plain host", "https://feeds.feedburner.com/TheHackersNews", "feeds.feedburner.com"},
		{"not a url", "not a url", "not a url"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sourceName(tt.in); got != tt.want {
				t.Fatalf("sourceName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRelTimeAt(t *testing.T) {
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{"just now", now.Add(-30 * time.Second), "just now"},
		{"minutes", now.Add(-12 * time.Minute), "12m ago"},
		{"hours", now.Add(-3 * time.Hour), "3h ago"},
		{"days", now.Add(-5 * 24 * time.Hour), "5d ago"},
		{"old falls back to date", now.Add(-40 * 24 * time.Hour), "2026-05-24"},
		{"future falls back to date", now.Add(54 * 24 * time.Hour), "2026-08-26"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := relTimeAt(tt.t, now); got != tt.want {
				t.Fatalf("relTimeAt = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRelTimeNilAndZero(t *testing.T) {
	if got := relTime(nil); got != "—" {
		t.Fatalf("relTime(nil) = %q", got)
	}
	if got := relTime(time.Time{}); got != "—" {
		t.Fatalf("relTime(zero) = %q", got)
	}
}

func TestCveID(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"extracts id", "CVE-2026-14615 - Keycloak-services: fgap v2 bypass", "CVE-2026-14615"},
		{"mid-title", "Freeipa: off-by-one (CVE-2026-14612) during oauth2", "CVE-2026-14612"},
		{"none", "NetNut proxy network disrupted", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cveID(tt.in); got != tt.want {
				t.Fatalf("cveID(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

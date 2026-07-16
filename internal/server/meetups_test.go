package server

import (
	"strings"
	"testing"
	"time"
)

func TestLoadMeetupSeed(t *testing.T) {
	seed, err := loadMeetupSeed()
	if err != nil {
		t.Fatalf("loadMeetupSeed: %v", err)
	}
	if len(seed.Meetups) == 0 {
		t.Fatal("seed has no meetups")
	}
	if len(seed.Chapters) == 0 {
		t.Fatal("seed has no chapters")
	}
	seen := map[string]bool{}
	for _, m := range seed.Meetups {
		if m.Slug == "" {
			t.Errorf("meetup %q has empty slug", m.Title)
		}
		if seen[m.Slug] {
			t.Errorf("duplicate slug %q", m.Slug)
		}
		seen[m.Slug] = true
		if m.StartsAt.IsZero() {
			t.Errorf("meetup %q has zero StartsAt", m.Slug)
		}
		for _, u := range []string{m.OnlineURL, m.RSVPURL, m.ChapterURL} {
			if !httpURLOK(u) {
				t.Errorf("meetup %q has non-http(s) url %q", m.Slug, u)
			}
		}
	}
	for _, c := range seed.Chapters {
		if !httpURLOK(c.Website) {
			t.Errorf("chapter %q has non-http(s) website %q", c.Name, c.Website)
		}
	}
}

func TestHTTPURLOK(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", true},
		{"https://example.com", true},
		{"http://example.com/x?y=1", true},
		{"javascript:alert(1)", false},
		{"data:text/html,x", false},
		{"ftp://example.com", false},
		{"/relative", false},
		{"not a url", false},
	}
	for _, tc := range cases {
		if got := httpURLOK(tc.in); got != tc.want {
			t.Errorf("httpURLOK(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func mk(slug, city, chapter, loc string, start time.Time, tags ...string) Meetup {
	return Meetup{Slug: slug, City: city, ChapterName: chapter, LocationType: loc, StartsAt: start, Tags: tags}
}

func TestFilterMeetups(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	ms := []Meetup{
		mk("a", "Liverpool", "BSides Liverpool", "in_person", now, "ctf"),
		mk("b", "Leeds", "BSides Leeds", "online", now, "workshop"),
		mk("c", "Liverpool", "BSides Liverpool", "hybrid", now, "talks"),
	}
	got := func(f meetupFilter) []string {
		out := []string{}
		for _, m := range filterMeetups(ms, f) {
			out = append(out, m.Slug)
		}
		return out
	}
	if s := got(meetupFilter{City: "liverpool"}); len(s) != 2 {
		t.Errorf("city filter = %v, want a,c", s)
	}
	if s := got(meetupFilter{Online: true}); len(s) != 2 { // online + hybrid
		t.Errorf("online filter = %v, want b,c", s)
	}
	if s := got(meetupFilter{Tag: "CTF"}); len(s) != 1 || s[0] != "a" {
		t.Errorf("tag filter = %v, want [a]", s)
	}
	if s := got(meetupFilter{Chapter: "bsides leeds"}); len(s) != 1 || s[0] != "b" {
		t.Errorf("chapter filter = %v, want [b]", s)
	}
	if s := got(meetupFilter{}); len(s) != 3 {
		t.Errorf("empty filter = %v, want all", s)
	}
}

func TestSplitMeetups(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	past1 := Meetup{Slug: "p1", StartsAt: now.Add(-48 * time.Hour)}
	past2 := Meetup{Slug: "p2", StartsAt: now.Add(-24 * time.Hour)}
	up1 := Meetup{Slug: "u1", StartsAt: now.Add(24 * time.Hour)}
	up2 := Meetup{Slug: "u2", StartsAt: now.Add(48 * time.Hour)}
	// EndsAt in the future keeps an already-started event "upcoming/current".
	current := Meetup{Slug: "cur", StartsAt: now.Add(-1 * time.Hour), EndsAt: now.Add(1 * time.Hour)}
	up, past := splitMeetups([]Meetup{past1, up2, past2, up1, current}, now)

	gotUp := []string{}
	for _, m := range up {
		gotUp = append(gotUp, m.Slug)
	}
	// upcoming ASC by StartsAt: cur(-1h) < u1(+24h) < u2(+48h)
	if len(gotUp) != 3 || gotUp[0] != "cur" || gotUp[1] != "u1" || gotUp[2] != "u2" {
		t.Errorf("upcoming = %v, want [cur u1 u2]", gotUp)
	}
	gotPast := []string{}
	for _, m := range past {
		gotPast = append(gotPast, m.Slug)
	}
	// past DESC by StartsAt: p2(-24h) then p1(-48h)
	if len(gotPast) != 2 || gotPast[0] != "p2" || gotPast[1] != "p1" {
		t.Errorf("past = %v, want [p2 p1]", gotPast)
	}
}

func TestICSForMeetup(t *testing.T) {
	now := time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)
	m := Meetup{
		Slug: "x", Title: "Talks; food, fun", Summary: "line1\nline2",
		StartsAt: time.Date(2026, 9, 24, 17, 30, 0, 0, time.UTC),
		EndsAt:   time.Date(2026, 9, 24, 20, 0, 0, 0, time.UTC),
		City:     "Liverpool", RSVPURL: "https://example.com/rsvp",
	}
	ics := icsForMeetup(m, now)
	for _, want := range []string{
		"BEGIN:VCALENDAR", "BEGIN:VEVENT", "END:VEVENT", "END:VCALENDAR",
		"UID:x@smellyfeet",
		"DTSTAMP:20260716T090000Z",
		"DTSTART:20260924T173000Z",
		"DTEND:20260924T200000Z",
		`SUMMARY:Talks\; food\, fun`, // escaped ; and ,
		"URL:https://example.com/rsvp",
	} {
		if !strings.Contains(ics, want) {
			t.Errorf("ics missing %q\n---\n%s", want, ics)
		}
	}
	if !strings.Contains(ics, "\r\n") {
		t.Error("ICS lines must be CRLF-terminated")
	}
}

func TestICSDefaultsEndTo2hAfterStart(t *testing.T) {
	now := time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)
	m := Meetup{Slug: "y", Title: "T", StartsAt: time.Date(2026, 9, 1, 18, 0, 0, 0, time.UTC)}
	ics := icsForMeetup(m, now)
	if !strings.Contains(ics, "DTEND:20260901T200000Z") {
		t.Errorf("missing 2h-default DTEND:\n%s", ics)
	}
}

func TestRateLimiter(t *testing.T) {
	rl := newRateLimiter(3, time.Minute)
	base := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if !rl.allowAt("1.2.3.4", base) {
			t.Fatalf("request %d should be allowed", i)
		}
	}
	if rl.allowAt("1.2.3.4", base) {
		t.Error("4th request in window should be blocked")
	}
	if !rl.allowAt("5.6.7.8", base) {
		t.Error("a different key should be allowed")
	}
	// After the window elapses, the key is allowed again.
	if !rl.allowAt("1.2.3.4", base.Add(2*time.Minute)) {
		t.Error("request after window should be allowed")
	}
}

func TestNewLoadsSeedAndDefaults(t *testing.T) {
	srv, err := New(stubService{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if len(srv.meetups.Meetups) == 0 {
		t.Error("New should load the meetup seed")
	}
	if !srv.meetupsEnabled {
		t.Error("meetups should default to enabled")
	}
	if srv.displayTZ == nil {
		t.Error("displayTZ should be set")
	}
	if !meetupsNavEnabled {
		t.Error("nav gate global should be true after enabled New")
	}
}

func TestWithMeetupsDisabled(t *testing.T) {
	srv, err := New(stubService{}, WithMeetupsEnabled(false))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if srv.meetupsEnabled {
		t.Error("WithMeetupsEnabled(false) should disable meetups")
	}
	if meetupsNavEnabled {
		t.Error("nav gate global should be false after disabled New")
	}
	// Restore the global so later tests (which assume enabled) are unaffected.
	meetupsNavEnabled = true
}

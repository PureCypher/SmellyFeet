package server

import "testing"

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

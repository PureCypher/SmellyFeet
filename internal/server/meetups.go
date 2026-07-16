package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	_ "time/tzdata" // embed the tz database so IANA zones resolve in scratch containers
)

//go:embed meetups_seed.json
var meetupsSeedFS embed.FS

// Meetup is one community meetup entry from the embedded seed. Everything in
// the seed is published; git history is the audit log (design §6).
type Meetup struct {
	Slug         string    `json:"slug"`
	Title        string    `json:"title"`
	Summary      string    `json:"summary"`
	Description  string    `json:"description"` // plain multi-line text, NOT markdown
	StartsAt     time.Time `json:"starts_at"`
	EndsAt       time.Time `json:"ends_at"`
	LocationType string    `json:"location_type"` // in_person | online | hybrid
	VenueName    string    `json:"venue_name"`
	VenueAddress string    `json:"venue_address"`
	City         string    `json:"city"`
	Region       string    `json:"region"`
	Country      string    `json:"country"`
	OnlineURL    string    `json:"online_url"`
	RSVPURL      string    `json:"rsvp_url"`
	ChapterName  string    `json:"chapter_name"`
	ChapterURL   string    `json:"chapter_url"`
	Tags         []string  `json:"tags"`
}

// Chapter is a community chapter reference — discovery only, not an official
// mirror of bsides.org/chapters/, never asserting "approved" status.
type Chapter struct {
	Name    string `json:"name"`
	City    string `json:"city"`
	Country string `json:"country"`
	Website string `json:"website"`
	Email   string `json:"email,omitempty"`
}

type meetupSeed struct {
	Meetups  []Meetup  `json:"meetups"`
	Chapters []Chapter `json:"chapters"`
}

// loadMeetupSeed reads and validates the embedded seed. It fails fast on a
// duplicate/empty slug or any non-http(s) URL so a bad seed never ships.
func loadMeetupSeed() (meetupSeed, error) {
	b, err := meetupsSeedFS.ReadFile("meetups_seed.json")
	if err != nil {
		return meetupSeed{}, fmt.Errorf("read seed: %w", err)
	}
	var seed meetupSeed
	if err := json.Unmarshal(b, &seed); err != nil {
		return meetupSeed{}, fmt.Errorf("parse seed: %w", err)
	}
	seen := map[string]bool{}
	for _, m := range seed.Meetups {
		if m.Slug == "" {
			return meetupSeed{}, fmt.Errorf("meetup %q: empty slug", m.Title)
		}
		if seen[m.Slug] {
			return meetupSeed{}, fmt.Errorf("duplicate slug %q", m.Slug)
		}
		seen[m.Slug] = true
		for _, u := range []string{m.OnlineURL, m.RSVPURL, m.ChapterURL} {
			if !httpURLOK(u) {
				return meetupSeed{}, fmt.Errorf("meetup %q: non-http(s) url %q", m.Slug, u)
			}
		}
	}
	for _, c := range seed.Chapters {
		if !httpURLOK(c.Website) {
			return meetupSeed{}, fmt.Errorf("chapter %q: non-http(s) website %q", c.Name, c.Website)
		}
	}
	return seed, nil
}

// httpURLOK reports whether s is safe to render as an href: empty (optional
// field) or an absolute http/https URL. Blocks javascript:/data:/relative.
func httpURLOK(s string) bool {
	if s == "" {
		return true
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

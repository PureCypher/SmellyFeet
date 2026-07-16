package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
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
	DateOnly     bool      `json:"date_only,omitempty"` // display date without a clock time
	LocationType string    `json:"location_type"`       // in_person | online | hybrid
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

type meetupFilter struct {
	City    string
	Tag     string
	Chapter string
}

func (f meetupFilter) empty() bool {
	return f.City == "" && f.Tag == "" && f.Chapter == ""
}

// filterMeetups returns the meetups matching every set filter field
// (case-insensitive).
func filterMeetups(ms []Meetup, f meetupFilter) []Meetup {
	if f.empty() {
		return ms
	}
	out := make([]Meetup, 0, len(ms))
	for _, m := range ms {
		if f.City != "" && !strings.EqualFold(m.City, f.City) {
			continue
		}
		if f.Chapter != "" && !strings.EqualFold(m.ChapterName, f.Chapter) {
			continue
		}
		if f.Tag != "" && !hasTag(m.Tags, f.Tag) {
			continue
		}
		out = append(out, m)
	}
	return out
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, want) {
			return true
		}
	}
	return false
}

// isPast reports whether the meetup has finished as of now (using EndsAt when
// set, else StartsAt).
func isPast(m Meetup, now time.Time) bool {
	end := m.EndsAt
	if end.IsZero() {
		end = m.StartsAt
	}
	return end.Before(now)
}

// splitMeetups partitions meetups into upcoming (StartsAt ASC) and past
// (StartsAt DESC). Input order is not mutated.
func splitMeetups(ms []Meetup, now time.Time) (upcoming, past []Meetup) {
	for _, m := range ms {
		if isPast(m, now) {
			past = append(past, m)
		} else {
			upcoming = append(upcoming, m)
		}
	}
	sort.SliceStable(upcoming, func(i, j int) bool { return upcoming[i].StartsAt.Before(upcoming[j].StartsAt) })
	sort.SliceStable(past, func(i, j int) bool { return past[i].StartsAt.After(past[j].StartsAt) })
	return upcoming, past
}

const icsStamp = "20060102T150405Z"

// icsForMeetup renders a meetup as a minimal RFC5545 VEVENT. Times are emitted
// in UTC. If EndsAt is unset, the event defaults to two hours. now is passed in
// (not read from the clock) so output is deterministic and testable.
func icsForMeetup(m Meetup, now time.Time) string {
	end := m.EndsAt
	if end.IsZero() {
		end = m.StartsAt.Add(2 * time.Hour)
	}
	loc := "Online"
	if m.LocationType != "online" {
		if m.VenueName != "" {
			loc = m.VenueName
		} else if m.City != "" {
			loc = m.City
		}
	}
	link := m.RSVPURL
	if link == "" {
		link = m.OnlineURL
	}
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//SmellyFeet//Meetups//EN",
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"BEGIN:VEVENT",
		"UID:" + m.Slug + "@smellyfeet",
		"DTSTAMP:" + now.UTC().Format(icsStamp),
		"DTSTART:" + m.StartsAt.UTC().Format(icsStamp),
		"DTEND:" + end.UTC().Format(icsStamp),
		"SUMMARY:" + icsEscape(m.Title),
		"DESCRIPTION:" + icsEscape(m.Summary),
		"LOCATION:" + icsEscape(loc),
	}
	if link != "" {
		lines = append(lines, "URL:"+link)
	}
	lines = append(lines, "END:VEVENT", "END:VCALENDAR")
	return strings.Join(lines, "\r\n") + "\r\n"
}

// icsEscape escapes text per RFC5545 §3.3.11 (backslash, semicolon, comma,
// newline).
func icsEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `;`, `\;`, `,`, `\,`, "\n", `\n`, "\r", "")
	return r.Replace(s)
}

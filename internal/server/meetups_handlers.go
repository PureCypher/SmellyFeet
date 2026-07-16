package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// meetupItem is a meetup with display fields precomputed in the server's tz.
type meetupItem struct {
	Meetup
	StartsDisplay string
	LocationLabel string
	IsOnline      bool
}

func (s *Server) toItem(m Meetup) meetupItem {
	it := meetupItem{Meetup: m, IsOnline: m.LocationType == "online"}
	if m.DateOnly {
		it.StartsDisplay = m.StartsAt.In(s.displayTZ).Format("Mon 2 Jan 2006")
	} else {
		it.StartsDisplay = m.StartsAt.In(s.displayTZ).Format("Mon 2 Jan 2006, 15:04")
	}
	switch {
	case m.LocationType == "online":
		it.LocationLabel = "Online"
	case m.City != "" && m.Country != "":
		it.LocationLabel = m.City + ", " + m.Country
	case m.City != "":
		it.LocationLabel = m.City
	default:
		it.LocationLabel = "TBC"
	}
	return it
}

func (s *Server) toItems(ms []Meetup) []meetupItem {
	out := make([]meetupItem, 0, len(ms))
	for _, m := range ms {
		out = append(out, s.toItem(m))
	}
	return out
}

func (s *Server) findMeetup(slug string) (Meetup, bool) {
	for _, m := range s.meetups.Meetups {
		if m.Slug == slug {
			return m, true
		}
	}
	return Meetup{}, false
}

func parseMeetupFilter(r *http.Request) meetupFilter {
	q := r.URL.Query()
	return meetupFilter{
		City:    strings.TrimSpace(q.Get("city")),
		Tag:     strings.TrimSpace(q.Get("tag")),
		Chapter: strings.TrimSpace(q.Get("chapter")),
		Online:  q.Get("online") == "1",
	}
}

func (s *Server) tzLabel() string { return time.Now().In(s.displayTZ).Format("MST") }

type meetupListView struct {
	Title, Desc    string
	OG, OGArticle  bool
	Nav            string
	Upcoming, Past []meetupItem
	Filter         meetupFilter
	Cities, Tags   []string
	TZLabel        string
}

func (s *Server) handleMeetupsList(w http.ResponseWriter, r *http.Request) {
	f := parseMeetupFilter(r)
	filtered := filterMeetups(s.meetups.Meetups, f)
	upcoming, past := splitMeetups(filtered, time.Now())
	setCache(w, cacheList)
	s.render(w, http.StatusOK, "meetups_list", meetupListView{
		Title:    "Meetups",
		Desc:     "Community infosec meetups in the BSides spirit — talks, workshops, and networking.",
		OG:       true,
		Nav:      "meetups",
		Upcoming: s.toItems(upcoming),
		Past:     s.toItems(past),
		Filter:   f,
		Cities:   distinctSorted(s.meetups.Meetups, func(m Meetup) string { return m.City }),
		Tags:     meetupTags(s.meetups.Meetups),
		TZLabel:  s.tzLabel(),
	})
}

func distinctSorted(ms []Meetup, key func(Meetup) string) []string {
	set := map[string]bool{}
	for _, m := range ms {
		if v := key(m); v != "" {
			set[v] = true
		}
	}
	return sortedKeys(set)
}

func meetupTags(ms []Meetup) []string {
	set := map[string]bool{}
	for _, m := range ms {
		for _, t := range m.Tags {
			if t != "" {
				set[t] = true
			}
		}
	}
	return sortedKeys(set)
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

type meetupDetailView struct {
	Title, Desc   string
	OG, OGArticle bool
	Nav           string
	M             meetupItem
	EndsDisplay   string
	HasEnd        bool
	MapURL        string
	Related       []meetupItem
	TZLabel       string
}

func (s *Server) handleMeetupDetail(w http.ResponseWriter, r *http.Request) {
	m, ok := s.findMeetup(r.PathValue("slug"))
	if !ok {
		setCache(w, cacheNone)
		s.render(w, http.StatusNotFound, "notfound", map[string]any{"Title": "Not Found", "Nav": "meetups"})
		return
	}
	view := meetupDetailView{
		Title:   m.Title,
		Desc:    trimDesc(m.Summary, 200),
		OG:      true,
		Nav:     "meetups",
		M:       s.toItem(m),
		TZLabel: s.tzLabel(),
		Related: s.relatedMeetups(m),
	}
	if !m.EndsAt.IsZero() {
		view.HasEnd = true
		endFmt := "15:04"
		if m.DateOnly {
			endFmt = "Mon 2 Jan 2006"
		}
		view.EndsDisplay = m.EndsAt.In(s.displayTZ).Format(endFmt)
	}
	if m.LocationType != "online" {
		addr := m.VenueAddress
		if addr == "" {
			addr = strings.TrimSpace(m.VenueName + " " + m.City)
		}
		if addr != "" {
			view.MapURL = "https://www.openstreetmap.org/search?query=" + url.QueryEscape(addr)
		}
	}
	setCache(w, cacheList)
	s.render(w, http.StatusOK, "meetup_detail", view)
}

// relatedMeetups returns up to 3 other meetups sharing the chapter or city.
func (s *Server) relatedMeetups(m Meetup) []meetupItem {
	var out []meetupItem
	for _, other := range s.meetups.Meetups {
		if other.Slug == m.Slug {
			continue
		}
		if (m.ChapterName != "" && strings.EqualFold(other.ChapterName, m.ChapterName)) ||
			(m.City != "" && strings.EqualFold(other.City, m.City)) {
			out = append(out, s.toItem(other))
		}
		if len(out) == 3 {
			break
		}
	}
	return out
}

type chaptersView struct {
	Title, Desc   string
	OG, OGArticle bool
	Nav           string
	Chapters      []Chapter
	Countries     []string // distinct sorted country list for the filter dropdown
	Country       string   // currently selected country filter ("" = all)
}

func (s *Server) handleChapters(w http.ResponseWriter, r *http.Request) {
	country := strings.TrimSpace(r.URL.Query().Get("country"))
	chapters := s.meetups.Chapters
	if country != "" {
		filtered := make([]Chapter, 0, len(chapters))
		for _, c := range chapters {
			if strings.EqualFold(c.Country, country) {
				filtered = append(filtered, c)
			}
		}
		chapters = filtered
	}
	setCache(w, cacheList)
	s.render(w, http.StatusOK, "chapters", chaptersView{
		Title:     "Chapters",
		Desc:      "Discovery list of BSides-community chapters — not an official mirror.",
		OG:        true,
		Nav:       "meetups",
		Chapters:  chapters,
		Countries: chapterCountries(s.meetups.Chapters),
		Country:   country,
	})
}

// chapterCountries returns the distinct, sorted country names in the seed for
// the chapters filter dropdown.
func chapterCountries(chapters []Chapter) []string {
	set := map[string]bool{}
	for _, c := range chapters {
		if c.Country != "" {
			set[c.Country] = true
		}
	}
	return sortedKeys(set)
}

func (s *Server) handleMeetupICS(w http.ResponseWriter, r *http.Request) {
	m, ok := s.findMeetup(r.PathValue("slug"))
	if !ok {
		setCache(w, cacheNone)
		http.NotFound(w, r)
		return
	}
	setCache(w, cacheList)
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+m.Slug+".ics\"")
	_, _ = io.WriteString(w, icsForMeetup(m, time.Now()))
}

// handleAPIMeetups serves the seed as JSON for future bots. Everything is
// published; there is no organizer PII on the Meetup type. Optional filters:
// city, tag, online, from, to (RFC3339).
func (s *Server) handleAPIMeetups(w http.ResponseWriter, r *http.Request) {
	items := filterMeetups(s.meetups.Meetups, parseMeetupFilter(r))
	if from, err := time.Parse(time.RFC3339, r.URL.Query().Get("from")); err == nil {
		items = filterByDate(items, from, time.Time{})
	}
	if to, err := time.Parse(time.RFC3339, r.URL.Query().Get("to")); err == nil {
		items = filterByDate(items, time.Time{}, to)
	}
	setCache(w, cacheList)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(map[string]any{"meetups": items}); err != nil {
		log.Printf("api meetups encode: %v", err)
	}
}

// filterByDate keeps meetups whose StartsAt is within [from, to]; a zero bound
// is treated as open.
func filterByDate(ms []Meetup, from, to time.Time) []Meetup {
	out := make([]Meetup, 0, len(ms))
	for _, m := range ms {
		if !from.IsZero() && m.StartsAt.Before(from) {
			continue
		}
		if !to.IsZero() && m.StartsAt.After(to) {
			continue
		}
		out = append(out, m)
	}
	return out
}

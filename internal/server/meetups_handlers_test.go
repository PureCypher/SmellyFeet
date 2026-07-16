package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func newTestServerOpts(t *testing.T, svc ArticleService, opts ...Option) http.Handler {
	t.Helper()
	srv, err := New(svc, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv.Routes()
}

// aMeetup returns the first seed meetup. The seed content changes as the tracker
// refreshes, so tests pull representative values from it rather than hardcoding.
func aMeetup(t *testing.T) Meetup {
	t.Helper()
	seed, err := loadMeetupSeed()
	if err != nil {
		t.Fatalf("loadMeetupSeed: %v", err)
	}
	if len(seed.Meetups) == 0 {
		t.Fatal("seed has no meetups")
	}
	return seed.Meetups[0]
}

// meetupsForCityFilter returns a seed meetup with a non-empty City and, if one
// exists, another meetup in a different city (to check the filter excludes it).
func meetupsForCityFilter(t *testing.T) (with Meetup, other Meetup) {
	t.Helper()
	seed, err := loadMeetupSeed()
	if err != nil {
		t.Fatalf("loadMeetupSeed: %v", err)
	}
	for _, m := range seed.Meetups {
		if m.City == "" {
			continue
		}
		if with.Slug == "" {
			with = m
		} else if !strings.EqualFold(m.City, with.City) {
			return with, m
		}
	}
	if with.Slug == "" {
		t.Skip("no seed meetup has a city")
	}
	return with, Meetup{}
}

func TestMeetupsListRenders(t *testing.T) {
	rec := getPath(t, newTestServer(t, stubService{}), "/meetups")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !containsAll(body, "Community meetups", "bsides.org/chapters/", "/meetups/chapters") {
		t.Errorf("list page missing structural content")
	}
	if m := aMeetup(t); !strings.Contains(body, m.Title) {
		t.Errorf("list page missing seed meetup title %q", m.Title)
	}
}

func TestToItemDateOnlyFormatsWithoutTime(t *testing.T) {
	srv, err := New(stubService{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m := Meetup{Slug: "x", Title: "T", DateOnly: true,
		StartsAt: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)}
	it := srv.toItem(m)
	if !strings.Contains(it.StartsDisplay, "8 May 2026") {
		t.Errorf("date-only StartsDisplay = %q, want the date", it.StartsDisplay)
	}
	if strings.Contains(it.StartsDisplay, ":") {
		t.Errorf("date-only StartsDisplay must not include a clock time: %q", it.StartsDisplay)
	}
}

func TestMeetupDetailAndNotFound(t *testing.T) {
	h := newTestServer(t, stubService{})
	ok := getPath(t, h, "/meetups/"+aMeetup(t).Slug)
	if ok.Code != http.StatusOK {
		t.Fatalf("detail status = %d", ok.Code)
	}
	if !strings.Contains(ok.Body.String(), "Add to calendar") {
		t.Error("detail missing ICS link")
	}
	nf := getPath(t, h, "/meetups/does-not-exist")
	if nf.Code != http.StatusNotFound {
		t.Errorf("unknown slug status = %d, want 404", nf.Code)
	}
}

func TestMeetupICS(t *testing.T) {
	rec := getPath(t, newTestServer(t, stubService{}), "/meetups/"+aMeetup(t).Slug+"/ics")
	if rec.Code != http.StatusOK {
		t.Fatalf("ics status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/calendar") {
		t.Errorf("ics content-type = %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "BEGIN:VEVENT") {
		t.Error("ics body missing VEVENT")
	}
}

func TestChaptersPage(t *testing.T) {
	rec := getPath(t, newTestServer(t, stubService{}), "/meetups/chapters")
	if rec.Code != http.StatusOK {
		t.Fatalf("chapters status = %d", rec.Code)
	}
	if !containsAll(rec.Body.String(), "BSides chapters", "not an official mirror", "View meetups") {
		t.Error("chapters page missing content")
	}
}

func TestChaptersCountryFilter(t *testing.T) {
	h := newTestServer(t, stubService{})
	all := getPath(t, h, "/meetups/chapters").Body.String()
	if !strings.Contains(all, "All countries") {
		t.Error("chapters page missing the country filter dropdown")
	}
	// The real seed has both UK and USA chapters; each card renders "City, Country".
	if !strings.Contains(all, ", UK") || !strings.Contains(all, ", USA") {
		t.Fatal("unfiltered chapters should include both UK and USA chapters")
	}
	uk := getPath(t, h, "/meetups/chapters?country=UK").Body.String()
	if !strings.Contains(uk, ", UK") {
		t.Error("UK filter should still show UK chapters")
	}
	if strings.Contains(uk, ", USA") {
		t.Error("UK filter must exclude USA chapters (a US card leaked through)")
	}
}

func TestMeetupsCityFilter(t *testing.T) {
	with, other := meetupsForCityFilter(t)
	body := getPath(t, newTestServer(t, stubService{}), "/meetups?city="+url.QueryEscape(with.City)).Body.String()
	if !strings.Contains(body, with.Title) {
		t.Errorf("city=%q should include %q", with.City, with.Title)
	}
	if other.Slug != "" && strings.Contains(body, other.Title) {
		t.Errorf("city=%q should exclude %q (which is in %q)", with.City, other.Title, other.City)
	}
}

func TestMeetupsDisabledRoutes404(t *testing.T) {
	h := newTestServerOpts(t, stubService{}, WithMeetupsEnabled(false))
	if rec := getPath(t, h, "/meetups"); rec.Code != http.StatusNotFound {
		t.Errorf("/meetups with meetups disabled = %d, want 404", rec.Code)
	}
	if body := getPath(t, h, "/about").Body.String(); strings.Contains(body, `href="/meetups"`) {
		t.Error("nav should not show Meetups link when disabled")
	}
	meetupsNavEnabled = true // restore for later tests
}

func TestMeetupsNavLinkPresentWhenEnabled(t *testing.T) {
	body := getPath(t, newTestServer(t, stubService{}), "/about").Body.String()
	if !strings.Contains(body, `href="/meetups"`) {
		t.Error("nav should show Meetups link when enabled")
	}
}

func TestAPIMeetupsReturnsJSON(t *testing.T) {
	rec := getPath(t, newTestServer(t, stubService{}), "/api/meetups")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type = %q", ct)
	}
	var out struct {
		Meetups []map[string]any `json:"meetups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(out.Meetups) == 0 {
		t.Fatal("expected meetups in api response")
	}
	for _, m := range out.Meetups {
		for _, k := range []string{"organizer", "organizer_contact", "contact"} {
			if _, ok := m[k]; ok {
				t.Errorf("api leaked field %q", k)
			}
		}
	}
}

func TestAPIMeetupsCityFilter(t *testing.T) {
	with, _ := meetupsForCityFilter(t)
	rec := getPath(t, newTestServer(t, stubService{}), "/api/meetups?city="+url.QueryEscape(with.City))
	var out struct {
		Meetups []Meetup `json:"meetups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(out.Meetups) == 0 {
		t.Fatalf("expected at least one meetup for city %q", with.City)
	}
	for _, m := range out.Meetups {
		if !strings.EqualFold(m.City, with.City) {
			t.Errorf("city filter leaked %q (wanted %q)", m.City, with.City)
		}
	}
}

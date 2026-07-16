package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func newTestServerOpts(t *testing.T, svc ArticleService, opts ...Option) http.Handler {
	t.Helper()
	srv, err := New(svc, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv.Routes()
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
	if !strings.Contains(body, "BSides Kent 2026") {
		t.Errorf("list page missing a seed meetup title")
	}
	if !strings.Contains(body, "8 May 2026") {
		t.Errorf("date-only event should show the date without a time")
	}
}

func TestMeetupDetailAndNotFound(t *testing.T) {
	h := newTestServer(t, stubService{})
	ok := getPath(t, h, "/meetups/bsides-kent-2026")
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
	rec := getPath(t, newTestServer(t, stubService{}), "/meetups/bsides-kent-2026/ics")
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
	rec := getPath(t, newTestServer(t, stubService{}), "/meetups?city=Kent")
	body := rec.Body.String()
	if !strings.Contains(body, "BSides Kent 2026") {
		t.Error("Kent filter should include BSides Kent")
	}
	if strings.Contains(body, "BSides Copenhagen 2026") {
		t.Error("Kent filter should exclude the Copenhagen event")
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
	rec := getPath(t, newTestServer(t, stubService{}), "/api/meetups?city=Kent")
	var out struct {
		Meetups []Meetup `json:"meetups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(out.Meetups) == 0 {
		t.Fatal("expected at least one Kent meetup in api response")
	}
	for _, m := range out.Meetups {
		if !strings.EqualFold(m.City, "Kent") {
			t.Errorf("city filter leaked %q", m.City)
		}
	}
}

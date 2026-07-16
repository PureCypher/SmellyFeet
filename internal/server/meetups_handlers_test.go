package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	if !containsAll(body, "Community meetups", "bsides.org/chapters/", "/meetups/propose") {
		t.Errorf("list page missing structural content")
	}
	if !strings.Contains(body, "Example: London Infosec Autumn Social") {
		t.Errorf("list page missing a seed meetup title")
	}
}

func TestMeetupDetailAndNotFound(t *testing.T) {
	h := newTestServer(t, stubService{})
	ok := getPath(t, h, "/meetups/example-online-workshop")
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
	rec := getPath(t, newTestServer(t, stubService{}), "/meetups/example-online-workshop/ics")
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

func TestMeetupsCityFilter(t *testing.T) {
	rec := getPath(t, newTestServer(t, stubService{}), "/meetups?city=London")
	body := rec.Body.String()
	if !strings.Contains(body, "Example: London Infosec Autumn Social") {
		t.Error("London filter should include the London example")
	}
	if strings.Contains(body, "Example: Online Threat-Modelling Workshop") {
		t.Error("London filter should exclude the online example")
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

func postForm(t *testing.T, h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestProposeFormRenders(t *testing.T) {
	rec := getPath(t, newTestServer(t, stubService{}), "/meetups/propose")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !containsAll(rec.Body.String(), "Propose a meetup", `name="title"`, `name="website"`) {
		t.Error("propose form missing fields/honeypot")
	}
}

func TestProposeValidSubmitFiresWebhook(t *testing.T) {
	var got []proposal
	h := newTestServerOpts(t, stubService{}, WithNotifier(func(_ context.Context, p proposal) error {
		got = append(got, p)
		return nil
	}))
	rec := postForm(t, h, "/meetups/propose", url.Values{
		"title": {"Real Meetup"}, "contact": {"me@example.com"}, "city": {"Liverpool"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(got) != 1 || got[0].Title != "Real Meetup" {
		t.Fatalf("notifier got %+v, want one proposal", got)
	}
	if !strings.Contains(rec.Body.String(), "pending review") {
		t.Error("expected success flash")
	}
}

func TestProposeHoneypotSilentlySkipsWebhook(t *testing.T) {
	var got []proposal
	h := newTestServerOpts(t, stubService{}, WithNotifier(func(_ context.Context, p proposal) error {
		got = append(got, p)
		return nil
	}))
	rec := postForm(t, h, "/meetups/propose", url.Values{
		"title": {"Spam"}, "contact": {"x@y.z"}, "city": {"Leeds"}, "website": {"http://spam"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(got) != 0 {
		t.Errorf("honeypot should skip notify, got %+v", got)
	}
}

func TestProposeInvalidReRendersWithError(t *testing.T) {
	var got []proposal
	h := newTestServerOpts(t, stubService{}, WithNotifier(func(_ context.Context, p proposal) error {
		got = append(got, p)
		return nil
	}))
	rec := postForm(t, h, "/meetups/propose", url.Values{"contact": {"x@y.z"}, "city": {"Leeds"}}) // no title
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if len(got) != 0 {
		t.Error("invalid proposal must not notify")
	}
	if !strings.Contains(rec.Body.String(), "give the meetup a title") {
		t.Error("expected title error message")
	}
}

func TestProposeNotifyErrorShows502(t *testing.T) {
	h := newTestServerOpts(t, stubService{}, WithNotifier(func(_ context.Context, _ proposal) error {
		return errBoom
	}))
	rec := postForm(t, h, "/meetups/propose", url.Values{
		"title": {"T"}, "contact": {"x@y.z"}, "city": {"Leeds"},
	})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "try again") {
		t.Error("expected a soft-failure message")
	}
}

func TestProposeRateLimited(t *testing.T) {
	h := newTestServerOpts(t, stubService{}, WithNotifier(func(_ context.Context, _ proposal) error { return nil }))
	form := url.Values{"title": {"T"}, "contact": {"x@y.z"}, "city": {"Leeds"}}
	var last int
	for i := 0; i < 7; i++ {
		last = postForm(t, h, "/meetups/propose", form).Code
	}
	if last != http.StatusTooManyRequests {
		t.Errorf("7th submit = %d, want 429", last)
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
	rec := getPath(t, newTestServer(t, stubService{}), "/api/meetups?city=London")
	var out struct {
		Meetups []Meetup `json:"meetups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(out.Meetups) == 0 {
		t.Fatal("expected at least one London meetup in api response")
	}
	for _, m := range out.Meetups {
		if !strings.EqualFold(m.City, "London") {
			t.Errorf("city filter leaked %q", m.City)
		}
	}
}

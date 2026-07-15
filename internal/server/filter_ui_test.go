package server

import (
	"strings"
	"testing"
	"time"

	"smellyfeet/internal/apiclient"
)

func TestSortSelectAndPaginationCarrySort(t *testing.T) {
	arts := make([]apiclient.Article, 20)
	for i := range arts {
		arts[i] = apiclient.Article{ID: int64(i + 1), Title: "A"}
	}
	svc := stubService{list: apiclient.ListResult{Articles: arts}}
	body := getPath(t, newTestServer(t, svc), "/?sort=oldest").Body.String()
	if !strings.Contains(body, `<option value="oldest" selected>`) {
		t.Error("sort select missing selected oldest option")
	}
	if !strings.Contains(body, "sort=oldest") || !strings.Contains(body, "page=2") {
		t.Error("pagination must carry sort=oldest")
	}
}

func TestFilterChips(t *testing.T) {
	svc := stubService{}
	body := getPath(t, newTestServer(t, svc), "/?q=keycloak&feed=https%3A%2F%2Fx.example%2Frss&sort=oldest").Body.String()
	for _, want := range []string{
		"source: x.example", "search: “keycloak”", "oldest first", "clear all",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("chips missing %q", want)
		}
	}
	if !strings.Contains(body, `href="/?q=keycloak&amp;sort=oldest"`) {
		t.Error("source chip removal href should keep q and sort, drop feed")
	}
}

func TestNoChipsWhenUnfiltered(t *testing.T) {
	body := getPath(t, newTestServer(t, stubService{}), "/").Body.String()
	if strings.Contains(body, "clear all") {
		t.Error("chips bar should not render without active filters")
	}
}

func TestSearchChipRequiresMinLength(t *testing.T) {
	oneChar := getPath(t, newTestServer(t, stubService{}), "/?q=a").Body.String()
	if strings.Contains(oneChar, "clear all") || strings.Contains(oneChar, "search:") {
		t.Error("1-char query matches the backend's ignore-threshold and must not show a false active-filter chip")
	}

	twoChar := getPath(t, newTestServer(t, stubService{}), "/?q=ab").Body.String()
	if !strings.Contains(twoChar, "search: “ab”") {
		t.Error("2-char query meets the backend's threshold and should show the search chip")
	}
}

func TestSourcePillIsFilterLink(t *testing.T) {
	svc := stubService{list: apiclient.ListResult{Articles: []apiclient.Article{{
		ID: 1, Title: "T", FeedURL: "https://www.brighttalk.com/channel/7451/feed/rss",
	}}}}
	body := getPath(t, newTestServer(t, svc), "/").Body.String()
	if !strings.Contains(body, `href="/?feed=https%3A%2F%2Fwww.brighttalk.com%2Fchannel%2F7451%2Ffeed%2Frss"`) {
		t.Error("card pill should link to feed filter with escaped URL")
	}
}

func TestCardShowsVisibleAbsoluteTime(t *testing.T) {
	pub := time.Date(2026, 7, 14, 13, 41, 0, 0, time.UTC)
	svc := stubService{list: apiclient.ListResult{Articles: []apiclient.Article{{
		ID: 1, Title: "T", PublishedAt: pub,
	}}}}
	body := getPath(t, newTestServer(t, svc), "/").Body.String()
	if !strings.Contains(body, "2026-07-14 13:41 UTC") {
		t.Error("absolute timestamp must be visible text, not title-attribute only")
	}
}

func TestSearchInputHasMinLength(t *testing.T) {
	body := getPath(t, newTestServer(t, stubService{}), "/").Body.String()
	if !strings.Contains(body, `minlength="2"`) {
		t.Error("search input should carry a native minlength=2 constraint")
	}
}

func TestFeedContextStripShowsRawURL(t *testing.T) {
	body := getPath(t, newTestServer(t, stubService{}), "/?feed=https%3A%2F%2Fcvefeed.io%2Frssfeed%2Flatest.xml").Body.String()
	if !strings.Contains(body, "Feed URL:") || !strings.Contains(body, "https://cvefeed.io/rssfeed/latest.xml") {
		t.Error("raw feed URL should be visible text when a feed filter is active")
	}
}

func TestDegradedCalloutWhenFeedsUnavailable(t *testing.T) {
	svc := stubService{feedsErr: errBoom}
	body := getPath(t, newTestServer(t, svc), "/").Body.String()
	if !strings.Contains(body, "DEGRADED") {
		t.Error("expected a DEGRADED callout when the feeds fetch fails")
	}
}

func TestEmptyStateDistinguishesFilteredFromUnfiltered(t *testing.T) {
	filtered := getPath(t, newTestServer(t, stubService{}), "/?q=zzznomatch").Body.String()
	if !strings.Contains(filtered, "No articles match these filters") {
		t.Error("filtered empty state should mention the active filters")
	}
	unfiltered := getPath(t, newTestServer(t, stubService{}), "/").Body.String()
	if !strings.Contains(unfiltered, "No articles found.") {
		t.Error("unfiltered empty state should stay the plain message")
	}
}

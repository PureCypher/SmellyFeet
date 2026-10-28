package server

import (
	"html"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"smellyfeet/internal/apiclient"
)

const (
	feedA = "https://a.example/rss"
	feedB = "https://b.example/rss"
	feedC = "https://c.example/rss"
)

func feedQuery(feeds ...string) string {
	parts := make([]string, len(feeds))
	for i, f := range feeds {
		parts[i] = "feed=" + url.QueryEscape(f)
	}
	return "/?" + strings.Join(parts, "&")
}

func TestListMultiFeedPassesAllFeedsToBackend(t *testing.T) {
	var got apiclient.ListParams
	getPath(t, newTestServer(t, stubService{lastList: &got}), feedQuery(feedA, feedB))
	if !reflect.DeepEqual(got.Feeds, []string{feedA, feedB}) {
		t.Fatalf("backend got Feeds = %v, want both selected feeds", got.Feeds)
	}
}

func TestListLegacySingleFeedLinkStillFilters(t *testing.T) {
	var got apiclient.ListParams
	body := getPath(t, newTestServer(t, stubService{lastList: &got}), feedQuery(feedA)).Body.String()
	if !reflect.DeepEqual(got.Feeds, []string{feedA}) {
		t.Fatalf("backend got Feeds = %v, want [%s]", got.Feeds, feedA)
	}
	if !strings.Contains(body, "source: a.example") {
		t.Error("legacy single-feed link should show its source chip")
	}
}

func TestListEmptyFeedParamIsNoFilter(t *testing.T) {
	var got apiclient.ListParams
	body := getPath(t, newTestServer(t, stubService{lastList: &got}), "/?feed=").Body.String()
	if len(got.Feeds) != 0 {
		t.Errorf("empty feed= must not filter, backend got %v", got.Feeds)
	}
	if strings.Contains(body, "clear all") {
		t.Error("empty feed= must not render an active-filter bar")
	}
}

func TestSourceChipsOnePerSelectedFeed(t *testing.T) {
	body := getPath(t, newTestServer(t, stubService{}), feedQuery(feedA, feedB)).Body.String()
	for _, want := range []string{
		"source: a.example", "source: b.example",
		`href="` + listURL("", []string{feedB}, "", 0) + `"`, // chip a removes only a
		`href="` + listURL("", []string{feedA}, "", 0) + `"`, // chip b removes only b
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
}

func TestPaginationCarriesAllFeeds(t *testing.T) {
	arts := make([]apiclient.Article, 20) // a full page, so a Next link renders
	for i := range arts {
		arts[i] = apiclient.Article{ID: int64(i + 1), Title: "A"}
	}
	svc := stubService{list: apiclient.ListResult{Articles: arts}}
	body := getPath(t, newTestServer(t, svc), feedQuery(feedA, feedB)).Body.String()
	want := html.EscapeString(listURL("", []string{feedA, feedB}, "", 2))
	if !strings.Contains(body, `href="`+want+`"`) {
		t.Errorf("Next link should carry both feeds and page=2, want href %q", want)
	}
}

func TestSourcePickerRendersGroupedCheckboxes(t *testing.T) {
	svc := stubService{feeds: []apiclient.Feed{
		{FeedURL: feedA, ArticleCount: 5},
		{FeedURL: feedB, ArticleCount: 3},
		{FeedURL: feedC, ArticleCount: 9},
	}}
	body := getPath(t, newTestServer(t, svc), feedQuery(feedA, feedB)).Body.String()
	for _, want := range []string{
		`<details id="source-picker"`,
		"2 selected",
		`value="` + feedA + `" checked`,
		`value="` + feedB + `" checked`,
		`value="` + feedC + `"`,
		`href="#src-c"`,
		`id="src-c"`,
		`id="src-selected"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("picker missing %q", want)
		}
	}
	if strings.Contains(body, `value="`+feedC+`" checked`) {
		t.Error("unselected source must not be checked")
	}
	if strings.Contains(body, `href="#src-selected"`) {
		t.Error("the pinned Selected group is not a letter jump target")
	}
	if strings.Contains(body, "<script") {
		t.Error("the list page must stay JavaScript-free (CSP allows no scripts)")
	}

	none := getPath(t, newTestServer(t, svc), "/").Body.String()
	if !strings.Contains(none, "All sources") || strings.Contains(none, `id="src-selected"`) {
		t.Error("with nothing selected the summary says All sources and no Selected group renders")
	}
}

func TestSelectedSourceMissingFromFeedListStaysSubmittable(t *testing.T) {
	gone := "https://gone.example/rss"
	svc := stubService{feeds: []apiclient.Feed{{FeedURL: feedA, ArticleCount: 1}}}
	body := getPath(t, newTestServer(t, svc), feedQuery(gone)).Body.String()
	if !strings.Contains(body, `value="`+gone+`" checked`) {
		t.Error("a selected feed absent from /feeds must stay a checked checkbox so Apply keeps it")
	}
	if !strings.Contains(body, ">—</span>") {
		t.Error("an unlisted source shows an em dash instead of a count")
	}
}

func TestFeedsUnavailableKeepsSelectionAsHiddenInputs(t *testing.T) {
	svc := stubService{feedsErr: errBoom}
	body := getPath(t, newTestServer(t, svc), feedQuery(feedA, feedB)).Body.String()
	for _, f := range []string{feedA, feedB} {
		if !strings.Contains(body, `<input type="hidden" name="feed" value="`+f+`">`) {
			t.Errorf("selection %s must survive Apply as a hidden input", f)
		}
	}
	if strings.Contains(body, `type="checkbox"`) {
		t.Error("no checkbox picker when the feeds list is unavailable")
	}
	if !strings.Contains(body, "DEGRADED") {
		t.Error("expected the DEGRADED callout")
	}
}

func TestFeedContextStripOnlyForSingleFeed(t *testing.T) {
	one := getPath(t, newTestServer(t, stubService{}), feedQuery(feedA)).Body.String()
	if !strings.Contains(one, "Feed URL:") {
		t.Error("a single selected source shows its raw feed URL")
	}
	two := getPath(t, newTestServer(t, stubService{}), feedQuery(feedA, feedB)).Body.String()
	if strings.Contains(two, "Feed URL:") {
		t.Error("the raw-URL strip makes no sense for several sources")
	}
}

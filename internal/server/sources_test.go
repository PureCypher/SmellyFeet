package server

import (
	"fmt"
	"net/url"
	"reflect"
	"testing"

	"smellyfeet/internal/apiclient"
)

func distinctSelection(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("https://f%d.example/rss", i)
	}
	return out
}

func TestParseFeedSelection(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil", nil, nil},
		{"legacy empty submit means no filter", []string{"", ""}, nil},
		{"dedupe keeps first-seen order", []string{"a", "a", "b"}, []string{"a", "b"}},
		{"trims whitespace", []string{" a ", "a"}, []string{"a"}},
		{"keeps only the first 25", distinctSelection(30), distinctSelection(25)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseFeedSelection(c.in)
			if len(got) != len(c.want) || (len(got) > 0 && !reflect.DeepEqual(got, c.want)) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestListURL(t *testing.T) {
	cases := []struct {
		name  string
		q     string
		feeds []string
		sort  string
		page  int
		want  string
	}{
		{"nothing", "", nil, "", 0, "/"},
		{"search and oldest", "keycloak", nil, "oldest", 0, "/?q=keycloak&sort=oldest"},
		{"two feeds on page 3", "", []string{"a", "b"}, "", 3, "/?feed=a&feed=b&page=3"},
		{"page 1 is omitted", "", []string{"a"}, "", 1, "/?feed=a"},
		{"unknown sort is omitted", "", nil, "bogus", 0, "/"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := listURL(c.q, c.feeds, c.sort, c.page); got != c.want {
				t.Fatalf("listURL = %q, want %q", got, c.want)
			}
		})
	}
}

func TestListURLRoundTripsSpecialCharacters(t *testing.T) {
	feeds := []string{"https://x.example/rss?a=1&b=2#f", "https://例え.example/f+g"}
	u, err := url.Parse(listURL("", feeds, "", 0))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := u.Query()["feed"]; !reflect.DeepEqual(got, feeds) {
		t.Fatalf("round trip lost data: got %v, want %v", got, feeds)
	}
}

func TestBuildSourceChips(t *testing.T) {
	a, b := "https://a.example/rss", "https://b.example/rss"
	chips := buildSourceChips([]string{a, b}, "keycloak", "oldest")
	if len(chips) != 2 {
		t.Fatalf("got %d chips, want 2", len(chips))
	}
	if chips[0].Name != "a.example" || chips[1].Name != "b.example" {
		t.Errorf("chip names = %q, %q", chips[0].Name, chips[1].Name)
	}
	if want := listURL("keycloak", []string{b}, "oldest", 0); chips[0].RemoveHref != want {
		t.Errorf("chip a removal href = %q, want %q", chips[0].RemoveHref, want)
	}
	if want := listURL("keycloak", []string{a}, "oldest", 0); chips[1].RemoveHref != want {
		t.Errorf("chip b removal href = %q, want %q", chips[1].RemoveHref, want)
	}
}

func groupLabels(gs []sourceGroup) []string {
	out := make([]string, len(gs))
	for i, g := range gs {
		out[i] = g.Label
	}
	return out
}

func TestBuildSourceGroups(t *testing.T) {
	wired := "https://www.wired.com/feed/rss"
	feeds := []apiclient.Feed{
		{FeedURL: wired, ArticleCount: 5},
		{FeedURL: "https://bleepingcomputer.com/feed/", ArticleCount: 7},
		{FeedURL: "https://bbc.co.uk/rss", ArticleCount: 3},
		{FeedURL: "https://feeds.feedburner.com/BzzzNews", ArticleCount: 2},
		{FeedURL: "https://1password.com/rss", ArticleCount: 1},
		{FeedURL: "https://zdnet.com/rss", ArticleCount: 4},
	}
	groups := buildSourceGroups(feeds, []string{wired})

	if got, want := groupLabels(groups), []string{"Selected", "B", "Z", "#"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("labels = %v, want %v (W must appear only under Selected)", got, want)
	}
	wantAnchors := []string{"src-selected", "src-b", "src-z", "src-other"}
	for i, g := range groups {
		if g.Anchor != wantAnchors[i] {
			t.Errorf("group %q anchor = %q, want %q", g.Label, g.Anchor, wantAnchors[i])
		}
	}
	sel := groups[0].Options
	if len(sel) != 1 || sel[0].URL != wired || sel[0].Name != "wired.com" || sel[0].Count != 5 || !sel[0].Selected || sel[0].Unlisted {
		t.Errorf("selected group = %+v", sel)
	}
	// Case-insensitive order inside a letter group: BzzzNews sorts after bleepingcomputer.com.
	var names []string
	for _, o := range groups[1].Options {
		names = append(names, o.Name)
	}
	if want := []string{"bbc.co.uk", "bleepingcomputer.com", "BzzzNews"}; !reflect.DeepEqual(names, want) {
		t.Errorf("B group order = %v, want %v", names, want)
	}
}

func TestBuildSourceGroupsKeepsUnlistedSelection(t *testing.T) {
	gone := "https://gone.example/rss"
	feeds := []apiclient.Feed{{FeedURL: "https://a.example/rss", ArticleCount: 1}}
	groups := buildSourceGroups(feeds, []string{gone})

	if len(groups) == 0 || groups[0].Label != "Selected" || len(groups[0].Options) != 1 {
		t.Fatalf("groups = %+v, want a Selected group holding the unlisted feed", groups)
	}
	o := groups[0].Options[0]
	if o.URL != gone || o.Name != "gone.example" || !o.Unlisted || o.Count != 0 || !o.Selected {
		t.Errorf("unlisted option = %+v", o)
	}
	if chips := buildSourceChips([]string{gone}, "", ""); len(chips) != 1 || chips[0].Name != "gone.example" {
		t.Errorf("chip for unlisted feed = %+v", chips)
	}
}

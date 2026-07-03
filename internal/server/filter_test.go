package server

import (
	"strings"
	"testing"
	"time"

	"smellyfeet/internal/apiclient"
)

func TestSplitUpcoming(t *testing.T) {
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	past := apiclient.Article{ID: 1, PublishedAt: now.Add(-time.Hour)}
	exact := apiclient.Article{ID: 2, PublishedAt: now}
	future := apiclient.Article{ID: 3, PublishedAt: now.Add(time.Hour)}
	zero := apiclient.Article{ID: 4}

	up, cur := splitUpcoming([]apiclient.Article{future, past, exact, zero}, now)
	if len(up) != 1 || up[0].ID != 3 {
		t.Fatalf("upcoming = %v, want only ID 3", up)
	}
	if len(cur) != 3 || cur[0].ID != 1 || cur[1].ID != 2 || cur[2].ID != 4 {
		t.Fatalf("current = %v, want IDs 1,2,4 in order", cur)
	}
}

func TestHandleListSortAndSplit(t *testing.T) {
	future := apiclient.Article{ID: 9, Title: "Future webinar", PublishedAt: time.Now().Add(48 * time.Hour)}
	past := apiclient.Article{ID: 8, Title: "Past news", PublishedAt: time.Now().Add(-time.Hour)}

	t.Run("sort passed through and normalized", func(t *testing.T) {
		var got apiclient.ListParams
		h := newTestServer(t, stubService{lastList: &got})
		getPath(t, h, "/?sort=oldest")
		if got.Sort != "oldest" {
			t.Fatalf("Sort sent = %q, want oldest", got.Sort)
		}
		getPath(t, h, "/?sort=garbage")
		if got.Sort != "" {
			t.Fatalf("Sort sent = %q, want empty for unknown value", got.Sort)
		}
	})

	t.Run("page 1 newest splits upcoming", func(t *testing.T) {
		svc := stubService{list: apiclient.ListResult{Articles: []apiclient.Article{future, past}}}
		body := getPath(t, newTestServer(t, svc), "/").Body.String()
		if !containsAll(body, "upcoming (1)", "Future webinar", "Past news") {
			t.Fatalf("page 1 should split upcoming; body missing expected markers")
		}
	})

	t.Run("page 2 does not split", func(t *testing.T) {
		svc := stubService{list: apiclient.ListResult{Articles: []apiclient.Article{future, past}}}
		body := getPath(t, newTestServer(t, svc), "/?page=2").Body.String()
		if strings.Contains(body, "upcoming (") {
			t.Fatal("page 2 must not render the upcoming section")
		}
	})

	t.Run("oldest sort does not split", func(t *testing.T) {
		svc := stubService{list: apiclient.ListResult{Articles: []apiclient.Article{future, past}}}
		body := getPath(t, newTestServer(t, svc), "/?sort=oldest").Body.String()
		if strings.Contains(body, "upcoming (") {
			t.Fatal("oldest sort must not render the upcoming section")
		}
	})
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

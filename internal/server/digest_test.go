package server

import (
	"errors"
	"strings"
	"testing"

	"smellyfeet/internal/apiclient"
)

func TestHandleDigestRangeWhitelist(t *testing.T) {
	h := newTestServer(t, stubService{})
	for _, tt := range []struct{ path, wantSelected string }{
		{"/digest", `<option value="daily" selected>`},
		{"/digest?range=weekly", `<option value="weekly" selected>`},
		{"/digest?range=monthly", `<option value="monthly" selected>`},
		{"/digest?range=quarterly", `<option value="quarterly" selected>`},
		{"/digest?range=halfyearly", `<option value="halfyearly" selected>`},
		{"/digest?range=yearly", `<option value="yearly" selected>`},
		{"/digest?range=garbage", `<option value="daily" selected>`},
	} {
		body := getPath(t, h, tt.path).Body.String()
		if !strings.Contains(body, tt.wantSelected) {
			t.Errorf("%s: missing %q in body: %s", tt.path, tt.wantSelected, body)
		}
	}
}

func TestHandleDigestUpstreamErrorRendersInlineCallout(t *testing.T) {
	svc := stubService{digestErr: errors.New("boom")}
	rec := getPath(t, newTestServer(t, svc), "/digest")
	if rec.Code != 502 {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Digest unavailable") {
		t.Error("expected the inline critical callout, not the hard error page")
	}
	if !strings.Contains(body, `name="range"`) {
		t.Error("the range form should stay usable even when the digest fetch fails")
	}
}

func TestDigestImportantAndOtherSections(t *testing.T) {
	svc := stubService{digest: apiclient.DigestResult{
		Range:     "daily",
		Important: []apiclient.Article{{ID: 1, Title: "Big story", CrossFeedCount: 3}},
		Other:     []apiclient.Article{{ID: 2, Title: "Minor item"}},
	}}
	body := getPath(t, newTestServer(t, svc), "/digest").Body.String()
	if !containsAll(body, "Big story", "3 sources", "everything else (1)", "Minor item") {
		t.Fatalf("digest body missing expected markers: %s", body)
	}
}

// clusterRef builds a *int64 for table-test literals.
func clusterRef(id int64) *int64 { return &id }

func TestGroupByClusterMergesSharedClusterIDs(t *testing.T) {
	got := groupByCluster([]apiclient.Article{
		{ID: 1, StoryClusterID: clusterRef(1)},
		{ID: 2, StoryClusterID: clusterRef(1)},
		{ID: 3, StoryClusterID: clusterRef(3)},
	})
	if len(got) != 2 {
		t.Fatalf("len(groups) = %d, want 2", len(got))
	}
	if got[0].Lead.ID != 1 {
		t.Errorf("group 0 lead = %d, want the first-seen article (1)", got[0].Lead.ID)
	}
	if len(got[0].Members) != 1 || got[0].Members[0].ID != 2 {
		t.Errorf("group 0 members = %+v, want just article 2", got[0].Members)
	}
	if got[1].Lead.ID != 3 || len(got[1].Members) != 0 {
		t.Errorf("group 1 = %+v, want article 3 with no members", got[1])
	}
}

func TestGroupByClusterKeepsUnclusteredArticlesSeparate(t *testing.T) {
	// A nil cluster ID means "the clustering job hasn't reached this article
	// yet", not "same story" -- two nils must never merge.
	got := groupByCluster([]apiclient.Article{{ID: 1}, {ID: 2}})
	if len(got) != 2 {
		t.Fatalf("len(groups) = %d, want 2 singletons", len(got))
	}
	if len(got[0].Members) != 0 || len(got[1].Members) != 0 {
		t.Errorf("unclustered articles must not absorb members: %+v", got)
	}
}

func TestGroupByClusterPreservesFirstAppearanceOrder(t *testing.T) {
	// The API orders by cross_feed_count DESC, publish_date DESC. Grouping
	// must not reshuffle that ranking, even when clusters interleave.
	got := groupByCluster([]apiclient.Article{
		{ID: 1, StoryClusterID: clusterRef(7)},
		{ID: 2, StoryClusterID: clusterRef(9)},
		{ID: 3, StoryClusterID: clusterRef(7)},
		{ID: 4},
	})
	if len(got) != 3 {
		t.Fatalf("len(groups) = %d, want 3", len(got))
	}
	if got[0].Lead.ID != 1 || got[1].Lead.ID != 2 || got[2].Lead.ID != 4 {
		t.Fatalf("leads = %d,%d,%d, want 1,2,4", got[0].Lead.ID, got[1].Lead.ID, got[2].Lead.ID)
	}
	if len(got[0].Members) != 1 || got[0].Members[0].ID != 3 {
		t.Errorf("article 3 should join cluster 7's group: %+v", got[0])
	}
}

func TestGroupByClusterEmptyInput(t *testing.T) {
	if got := groupByCluster(nil); len(got) != 0 {
		t.Fatalf("groupByCluster(nil) = %+v, want empty", got)
	}
}

func TestDigestEmptyState(t *testing.T) {
	body := getPath(t, newTestServer(t, stubService{}), "/digest").Body.String()
	if !strings.Contains(body, "No articles found") {
		t.Fatalf("expected empty state message, got: %s", body)
	}
}

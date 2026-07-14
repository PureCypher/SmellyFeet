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
		{"/digest?range=garbage", `<option value="daily" selected>`},
	} {
		body := getPath(t, h, tt.path).Body.String()
		if !strings.Contains(body, tt.wantSelected) {
			t.Errorf("%s: missing %q in body: %s", tt.path, tt.wantSelected, body)
		}
	}
}

func TestHandleDigestUpstreamErrorRendersErrorPage(t *testing.T) {
	svc := stubService{digestErr: errors.New("boom")}
	rec := getPath(t, newTestServer(t, svc), "/digest")
	if rec.Code != 502 {
		t.Fatalf("status = %d, want 502", rec.Code)
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

func TestDigestEmptyState(t *testing.T) {
	body := getPath(t, newTestServer(t, stubService{}), "/digest").Body.String()
	if !strings.Contains(body, "No articles found") {
		t.Fatalf("expected empty state message, got: %s", body)
	}
}

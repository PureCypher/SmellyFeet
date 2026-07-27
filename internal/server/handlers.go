package server

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"smellyfeet/internal/apiclient"
)

// asTime normalizes time.Time and *time.Time for the formatDate func.
func asTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case *time.Time:
		if t == nil {
			return time.Time{}, false
		}
		return *t, true
	}
	return time.Time{}, false
}

func parsePage(s string) int {
	if s == "" {
		return 1
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// splitUpcoming partitions future-dated articles (upcoming webinars/events)
// from the current stream, preserving order within each group.
func splitUpcoming(articles []apiclient.Article, now time.Time) (upcoming, current []apiclient.Article) {
	for _, a := range articles {
		if a.PublishedAt.After(now) {
			upcoming = append(upcoming, a)
		} else {
			current = append(current, a)
		}
	}
	return upcoming, current
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	setCache(w, cacheNone)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

type listView struct {
	Title            string
	Desc             string
	OG               bool
	OGArticle        bool
	Nav              string
	Articles         []apiclient.Article
	Upcoming         []apiclient.Article
	Feeds            []apiclient.Feed
	FeedsUnavailable bool
	Q                string
	QApplied         bool
	Feed             string
	Sort             string
	Page             int
	HasPrev          bool
	HasNext          bool
}

// minSearchQueryLen mirrors the backend's own rule (Information-Broker api.go):
// q shorter than this is ignored server-side, so the frontend must not treat
// it as an active filter either (it would otherwise show a chip claiming a
// filter that was never applied).
const minSearchQueryLen = 2

// sourceBar is one row of the stats top-sources chart. Bar is a quantized
// width step (5..100 in steps of 5) rendered as a static bar-N CSS class,
// because CSP style-src 'self' forbids inline style widths. FeedURL and
// Latest are only populated for per-source rows (topSources), not the
// day/week/month collectionVolume rows.
type sourceBar struct {
	Name    string
	Count   int
	Bar     int
	FeedURL string
	Latest  *time.Time
}

const topSourcesMax = 15

func topSources(feeds []apiclient.Feed) []sourceBar {
	sort.Slice(feeds, func(i, j int) bool { return feeds[i].ArticleCount > feeds[j].ArticleCount })
	if len(feeds) > topSourcesMax {
		feeds = feeds[:topSourcesMax]
	}
	if len(feeds) == 0 || feeds[0].ArticleCount == 0 {
		return nil
	}
	max := float64(feeds[0].ArticleCount)
	out := make([]sourceBar, 0, len(feeds))
	for _, f := range feeds {
		bar := int(math.Round(float64(f.ArticleCount)/max*20)) * 5
		if bar < 5 {
			bar = 5
		}
		out = append(out, sourceBar{Name: sourceName(f.FeedURL), Count: f.ArticleCount, Bar: bar, FeedURL: f.FeedURL, Latest: f.LatestArticle})
	}
	return out
}

// collectionVolume renders the day/week/month article-collection counts as
// the same quantized-bar rows as topSources. This month's count is always
// >= this week's >= today's (each is COUNT(*) over a growing fetch_time
// window), so it's always the max — no need to scan for it like topSources does.
func collectionVolume(stats apiclient.Stats) []sourceBar {
	if stats.ArticlesThisMonth == 0 {
		return nil
	}
	max := float64(stats.ArticlesThisMonth)
	rows := []sourceBar{
		{Name: "Today", Count: stats.ArticlesToday},
		{Name: "This week", Count: stats.ArticlesThisWeek},
		{Name: "This month", Count: stats.ArticlesThisMonth},
	}
	for i := range rows {
		bar := int(math.Round(float64(rows[i].Count)/max*20)) * 5
		if bar < 5 {
			bar = 5
		}
		rows[i].Bar = bar
	}
	return rows
}

// healthStatus is the Signal Lamp ingestion-health indicator: a state
// ("nodata"/"ok"/"degraded"/"failing") plus the sentence shown beside its
// dot and text label. Never color-alone — the label and sentence always
// carry the same information the dot color does (WCAG 1.4.1).
type healthStatus struct {
	State   string
	Message string
}

// ingestionHealth derives the Signal Lamp health state from real /stats
// fields only — never fabricated. The single unambiguous rule (see
// docs/frontend/REDESIGN_PLAN.md's "/stats" route): a fresh install with no
// article history ever is neutral "no data yet", not a red failure; any
// history with zero successful fetches in the last 24h is FAILING
// regardless of whether fetches were attempted at all (both cases mean no
// fresh data arrived); otherwise DEGRADED if any fetch failed, else OK.
func ingestionHealth(st apiclient.Stats) healthStatus {
	if st.TotalArticles == 0 {
		return healthStatus{State: "nodata", Message: "Nothing has been collected in the last 30 days."}
	}
	if st.SuccessfulFetches24h == 0 {
		return healthStatus{State: "failing", Message: "No successful fetch in the last 24 hours."}
	}
	if st.FailedFetches24h > 0 {
		total := st.SuccessfulFetches24h + st.FailedFetches24h
		return healthStatus{State: "degraded", Message: fmt.Sprintf("%s of %s fetches failed in the last 24h", commas(st.FailedFetches24h), commas(total))}
	}
	return healthStatus{State: "ok", Message: fmt.Sprintf("all %s fetches succeeded in the last 24h", commas(st.SuccessfulFetches24h))}
}

// trimDesc collapses whitespace and truncates to n runes for meta descriptions.
func trimDesc(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := parsePage(r.URL.Query().Get("page"))
	q := r.URL.Query().Get("q")
	feed := r.URL.Query().Get("feed")
	sort := r.URL.Query().Get("sort")
	if sort != "oldest" {
		sort = "" // whitelist: only "oldest" changes the order
	}

	res, err := s.svc.ListArticles(ctx, apiclient.ListParams{
		Limit:  s.pageSize,
		Offset: (page - 1) * s.pageSize,
		Feed:   feed,
		Q:      q,
		Sort:   sort,
	})
	if err != nil {
		s.renderError(w, err)
		return
	}

	feeds, err := s.svc.ListFeeds(ctx)
	feedsUnavailable := false
	if err != nil {
		feeds = nil             // non-fatal: filter dropdown simply shows "All feeds"
		feedsUnavailable = true // surfaced as an explicit DEGRADED callout, not silently
	}

	var upcoming []apiclient.Article
	current := res.Articles
	if sort == "" && page == 1 {
		upcoming, current = splitUpcoming(res.Articles, time.Now())
	}

	setCache(w, cacheList)
	s.render(w, http.StatusOK, "list", listView{
		Title:            "Articles",
		Desc:             "AI-summarized cybersecurity intelligence — the latest articles from monitored threat feeds.",
		OG:               true,
		Nav:              "feed",
		Articles:         current,
		Upcoming:         upcoming,
		Feeds:            feeds,
		FeedsUnavailable: feedsUnavailable,
		Q:                q,
		QApplied:         len(strings.TrimSpace(q)) >= minSearchQueryLen,
		Feed:             feed,
		Sort:             sort,
		Page:             page,
		HasPrev:          page > 1,
		HasNext:          len(res.Articles) == s.pageSize,
	})
}

var validDigestRanges = map[string]bool{
	"daily": true, "weekly": true, "monthly": true,
	"quarterly": true, "halfyearly": true, "yearly": true,
}

type digestView struct {
	Title     string
	Desc      string
	OG        bool
	OGArticle bool // required by the shared header partial whenever OG is true; false = og:type "website"
	Nav       string
	Range     string
	Since     time.Time
	Window    string // prose description of the rolling range, e.g. "last 24 hours"
	digestSections
	FetchFailed bool
}

func (s *Server) handleDigest(w http.ResponseWriter, r *http.Request) {
	rangeParam := r.URL.Query().Get("range")
	if !validDigestRanges[rangeParam] {
		rangeParam = "daily"
	}

	res, err := s.svc.GetDigest(r.Context(), rangeParam)
	if err != nil {
		// Inline critical callout instead of the hard error page: the shell,
		// nav, and range form stay usable so a visitor can switch range or
		// navigate away rather than losing the whole page to one failed call.
		setCache(w, cacheNone)
		s.render(w, http.StatusBadGateway, "digest", digestView{
			Title:       "Digest",
			OG:          true,
			Nav:         "digest",
			Range:       rangeParam,
			Window:      digestWindowLabel(rangeParam),
			FetchFailed: true,
		})
		return
	}

	setCache(w, cacheList)
	s.render(w, http.StatusOK, "digest", digestView{
		Title:  "Digest",
		Desc:   "Cross-feed importance digest — security stories ranked by how many independent feeds covered them, over the last day, week, month, quarter, half-year, or year.",
		OG:     true,
		Nav:    "digest",
		Range:  rangeParam,
		Since:  res.Since,
		Window: digestWindowLabel(rangeParam),
		// Each bucket is grouped independently, which is safe: every article
		// in a cluster shares the same cross_feed_count, so splitImportant can
		// never split one cluster across the two buckets.
		digestSections: splitDigestSections(groupByCluster(res.Important), groupByCluster(res.Other)),
	})
}

func (s *Server) handleArticle(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		setCache(w, cacheNone)
		s.render(w, http.StatusNotFound, "notfound", map[string]any{"Title": "Not Found", "Nav": ""})
		return
	}

	a, err := s.svc.GetArticle(r.Context(), id)
	if errors.Is(err, apiclient.ErrNotFound) {
		setCache(w, cacheNone)
		s.render(w, http.StatusNotFound, "notfound", map[string]any{"Title": "Not Found", "Nav": ""})
		return
	}
	if err != nil {
		s.renderError(w, err)
		return
	}

	setCache(w, cacheArticle)

	desc := ""
	if a.Summary != nil {
		desc = trimDesc(*a.Summary, 200)
	}
	s.render(w, http.StatusOK, "article", map[string]any{
		"Title":     a.Title,
		"Article":   a,
		"Desc":      desc,
		"OG":        true,
		"OGArticle": true,
		"Nav":       "",
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.svc.GetStats(r.Context())
	if err != nil {
		s.renderError(w, err)
		return
	}

	var sources []sourceBar
	sourcesUnavailable := false
	if feeds, err := s.svc.ListFeeds(r.Context()); err == nil {
		sources = topSources(feeds)
	} else {
		sourcesUnavailable = true // non-fatal: core stats still render, top-sources shows an explicit unavailable state
	}

	setCache(w, cacheStats)
	s.render(w, http.StatusOK, "stats", map[string]any{
		"Title":              "Statistics",
		"Nav":                "stats",
		"Stats":              st,
		"Health":             ingestionHealth(st),
		"Sources":            sources,
		"SourcesUnavailable": sourcesUnavailable,
		"Collected":          collectionVolume(st),
	})
}

func (s *Server) handleAbout(w http.ResponseWriter, r *http.Request) {
	setCache(w, cacheAbout)
	s.render(w, http.StatusOK, "about", map[string]any{"Title": "About", "Nav": "about"})
}

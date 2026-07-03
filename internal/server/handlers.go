package server

import (
	"errors"
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

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	setCache(w, cacheNone)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

type listView struct {
	Title     string
	Desc      string
	OG        bool
	OGArticle bool
	Articles  []apiclient.Article
	Feeds     []apiclient.Feed
	Q         string
	Feed      string
	Page      int
	HasPrev   bool
	HasNext   bool
}

// sourceBar is one row of the stats top-sources chart. Bar is a quantized
// width step (5..100 in steps of 5) rendered as a static bar-N CSS class,
// because CSP style-src 'self' forbids inline style widths.
type sourceBar struct {
	Name  string
	Count int
	Bar   int
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
		out = append(out, sourceBar{Name: sourceName(f.FeedURL), Count: f.ArticleCount, Bar: bar})
	}
	return out
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

	res, err := s.svc.ListArticles(ctx, apiclient.ListParams{
		Limit:  s.pageSize,
		Offset: (page - 1) * s.pageSize,
		Feed:   feed,
		Q:      q,
	})
	if err != nil {
		s.renderError(w, err)
		return
	}

	setCache(w, cacheList)

	feeds, err := s.svc.ListFeeds(ctx)
	if err != nil {
		feeds = nil // non-fatal: filter dropdown simply shows "All feeds"
	}

	s.render(w, http.StatusOK, "list", listView{
		Title:    "Articles",
		Desc:     "AI-summarized cybersecurity intelligence — the latest articles from monitored threat feeds.",
		OG:       true,
		Articles: res.Articles,
		Feeds:    feeds,
		Q:        q,
		Feed:     feed,
		Page:     page,
		HasPrev:  page > 1,
		HasNext:  len(res.Articles) == s.pageSize,
	})
}

func (s *Server) handleArticle(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		setCache(w, cacheNone)
		s.render(w, http.StatusNotFound, "notfound", map[string]any{"Title": "Not Found"})
		return
	}

	a, err := s.svc.GetArticle(r.Context(), id)
	if errors.Is(err, apiclient.ErrNotFound) {
		setCache(w, cacheNone)
		s.render(w, http.StatusNotFound, "notfound", map[string]any{"Title": "Not Found"})
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
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.svc.GetStats(r.Context())
	if err != nil {
		s.renderError(w, err)
		return
	}

	var sources []sourceBar
	if feeds, err := s.svc.ListFeeds(r.Context()); err == nil {
		sources = topSources(feeds)
	} // non-fatal: section simply omitted

	setCache(w, cacheStats)
	s.render(w, http.StatusOK, "stats", map[string]any{
		"Title":   "Statistics",
		"Stats":   st,
		"Sources": sources,
	})
}

func (s *Server) handleAbout(w http.ResponseWriter, r *http.Request) {
	setCache(w, cacheAbout)
	s.render(w, http.StatusOK, "about", map[string]any{"Title": "About"})
}

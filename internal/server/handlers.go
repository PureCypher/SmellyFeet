package server

import (
	"net/http"
	"strconv"
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
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

type listView struct {
	Title    string
	Articles []apiclient.Article
	Feeds    []apiclient.Feed
	Q        string
	Feed     string
	Page     int
	HasPrev  bool
	HasNext  bool
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

	feeds, err := s.svc.ListFeeds(ctx)
	if err != nil {
		feeds = nil // non-fatal: filter dropdown simply shows "All feeds"
	}

	s.render(w, http.StatusOK, "list", listView{
		Title:    "Articles",
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
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// Package server renders the Information-Broker frontend.
package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"html/template"
	"log"
	"net/http"
	"regexp"
	"strings"

	"smellyfeet/internal/apiclient"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

// assetHash is a short content hash of app.css, used to bust Cloudflare's
// immutable /static/ cache when the stylesheet changes.
var assetHash = func() string {
	b, err := staticFS.ReadFile("static/app.css")
	if err != nil {
		return "dev"
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:4])
}()

// Cache-Control values; Cloudflare's edge honors s-maxage once the HTML
// cache rule from deploy/README.md is enabled.
const (
	cacheList    = "public, max-age=60, s-maxage=120, stale-while-revalidate=300"
	cacheArticle = "public, max-age=300, s-maxage=3600"
	cacheStats   = "public, max-age=30, s-maxage=60"
	cacheAbout   = "public, max-age=3600, s-maxage=86400"
	cacheFeed    = "public, max-age=300, s-maxage=300"
	cacheStatic  = "public, max-age=31536000, immutable"
	cacheNone    = "no-store"
)

func setCache(w http.ResponseWriter, v string) { w.Header().Set("Cache-Control", v) }

// ArticleService is the subset of the API the handlers need.
type ArticleService interface {
	ListArticles(ctx context.Context, p apiclient.ListParams) (apiclient.ListResult, error)
	GetArticle(ctx context.Context, id int64) (apiclient.Article, error)
	ListFeeds(ctx context.Context) ([]apiclient.Feed, error)
	GetStats(ctx context.Context) (apiclient.Stats, error)
}

// Server holds dependencies for the HTTP handlers.
type Server struct {
	svc      ArticleService
	tmpl     *template.Template
	pageSize int
}

var funcMap = template.FuncMap{
	"formatDate":   formatDate,
	"cleanContent": cleanContent,
	"inc":          func(n int) int { return n + 1 },
	"dec":          func(n int) int { return n - 1 },
	"assetHash":    func() string { return assetHash },
}

// formatDate renders a time value for display, returning "—" for a nil or zero time.
func formatDate(t any) string {
	if t == nil {
		return "—"
	}
	tt, ok := asTime(t)
	if !ok || tt.IsZero() {
		return "—"
	}
	return tt.Format("2006-01-02 15:04")
}

var innerWhitespace = regexp.MustCompile(`[ \t\x{00a0}]+`)

// cleanContent normalizes scraped full-text for display. HTML-stripped article
// bodies keep the original page's whitespace skeleton (long runs of blank lines
// and indentation), which renders as huge empty gaps. This trims each line,
// collapses internal runs of spaces/tabs, and drops blank lines so the text
// reads compactly. The result is rendered with CSS white-space: pre-line.
func cleanContent(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(innerWhitespace.ReplaceAllString(ln, " "))
		if ln != "" {
			out = append(out, ln)
		}
	}
	return strings.Join(out, "\n")
}

// New constructs a Server with parsed templates.
func New(svc ArticleService) (*Server, error) {
	tmpl, err := template.New("").Funcs(funcMap).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{svc: svc, tmpl: tmpl, pageSize: 20}, nil
}

// Routes returns the configured HTTP handler.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /{$}", s.handleList)
	mux.HandleFunc("GET /article/{id}", s.handleArticle)
	mux.HandleFunc("GET /stats", s.handleStats)
	mux.HandleFunc("GET /about", s.handleAbout)

	staticSrv := http.FileServerFS(staticFS)
	mux.Handle("GET /static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") { // no directory listings
			http.NotFound(w, r)
			return
		}
		setCache(w, cacheStatic)
		staticSrv.ServeHTTP(w, r)
	}))
	return withRequestLog(withSecurityHeaders(mux))
}

func (s *Server) render(w http.ResponseWriter, status int, name string, data any) {
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("template error (%s): %v", name, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

func (s *Server) renderError(w http.ResponseWriter, err error) {
	setCache(w, cacheNone)
	log.Printf("handler error: %v", err)
	s.render(w, http.StatusBadGateway, "error", map[string]any{
		"Title":   "Error",
		"Message": "The article service is currently unavailable. Please try again later.",
	})
}

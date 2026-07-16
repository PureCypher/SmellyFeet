// Package server renders the Information-Broker frontend.
package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

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
	GetDigest(ctx context.Context, rangeParam string) (apiclient.DigestResult, error)
}

// Server holds dependencies for the HTTP handlers.
type Server struct {
	svc      ArticleService
	tmpl     *template.Template
	pageSize int

	meetups        meetupSeed
	displayTZ      *time.Location
	meetupsEnabled bool
	notifyWebhook  string
	notify         func(context.Context, proposal) error
	rl             *rateLimiter
}

var funcMap = template.FuncMap{
	"formatDate":     formatDate,
	"cleanContent":   cleanContent,
	"sourceName":     sourceName,
	"relTime":        relTime,
	"cveID":          cveID,
	"commas":         commas,
	"inc":            func(n int) int { return n + 1 },
	"dec":            func(n int) int { return n - 1 },
	"assetHash":      func() string { return assetHash },
	"meetupsEnabled": func() bool { return meetupsNavEnabled },
	"dict":           dict,
}

// meetupsNavEnabled gates the Meetups nav link in the shared header partial.
// Set once by New; the frontend runs a single Server per process.
// ponytail: package global mirrors the existing assetHash pattern.
var meetupsNavEnabled = true

// dict builds a map from alternating key/value pairs for passing multiple
// named args into a sub-template.
func dict(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, fmt.Errorf("dict: odd number of args")
	}
	m := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		k, ok := pairs[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key %d not a string", i)
		}
		m[k] = pairs[i+1]
	}
	return m, nil
}

// commas formats an integer with thousands separators (50913 -> "50,913").
// Hand-rolled rather than pulling in golang.org/x/text/message: the frontend's
// go.mod is deliberately zero-dependency, and grouping digits by three is a
// few lines either way.
func commas(n int) string {
	s := fmt.Sprintf("%d", n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
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

// sourceName returns a compact display name for a feed URL: the hostname
// without a leading "www.". Returns the input unchanged if it doesn't parse.
func sourceName(feedURL string) string {
	u, err := url.Parse(feedURL)
	if err != nil || u.Hostname() == "" {
		return feedURL
	}
	return strings.TrimPrefix(u.Hostname(), "www.")
}

// relTimeAt renders t relative to now; dates outside the recent past
// (including future-dated webinar feeds) fall back to the plain date.
func relTimeAt(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < 0 || d >= 7*24*time.Hour:
		return t.Format("2006-01-02")
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func relTime(v any) string {
	t, ok := asTime(v)
	if !ok || t.IsZero() {
		return "—"
	}
	return relTimeAt(t, time.Now())
}

var cveRe = regexp.MustCompile(`CVE-\d{4}-\d{4,}`)

// cveID returns the first CVE identifier in s, or "".
func cveID(s string) string { return cveRe.FindString(s) }

// Option configures a Server at construction (functional options keep the
// existing New(svc) callers valid).
type Option func(*Server)

// WithMeetupsEnabled toggles the Meetups tab and its routes.
func WithMeetupsEnabled(b bool) Option { return func(s *Server) { s.meetupsEnabled = b } }

// WithMeetupTZ sets the meetup display timezone by IANA name; an unknown name
// keeps the current default.
func WithMeetupTZ(name string) Option {
	return func(s *Server) {
		if loc, err := time.LoadLocation(name); err == nil {
			s.displayTZ = loc
		}
	}
}

// WithNotifyWebhook sets the propose-form relay target.
func WithNotifyWebhook(url string) Option { return func(s *Server) { s.notifyWebhook = url } }

// WithNotifier overrides the proposal notifier (used in tests).
func WithNotifier(fn func(context.Context, proposal) error) Option {
	return func(s *Server) { s.notify = fn }
}

// New constructs a Server with parsed templates and the embedded meetup seed.
func New(svc ArticleService, opts ...Option) (*Server, error) {
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		return nil, fmt.Errorf("load default tz: %w", err)
	}
	s := &Server{svc: svc, pageSize: 20, meetupsEnabled: true, displayTZ: london}
	for _, opt := range opts {
		opt(s)
	}
	seed, err := loadMeetupSeed()
	if err != nil {
		return nil, fmt.Errorf("load meetup seed: %w", err)
	}
	s.meetups = seed
	s.rl = newRateLimiter(5, 10*time.Minute)
	if s.notify == nil {
		s.notify = s.defaultNotify
	}
	tmpl, err := template.New("").Funcs(funcMap).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	s.tmpl = tmpl
	meetupsNavEnabled = s.meetupsEnabled
	return s, nil
}

// Routes returns the configured HTTP handler.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /{$}", s.handleList)
	mux.HandleFunc("GET /digest", s.handleDigest)
	mux.HandleFunc("GET /article/{id}", s.handleArticle)
	mux.HandleFunc("GET /stats", s.handleStats)
	mux.HandleFunc("GET /about", s.handleAbout)
	mux.HandleFunc("GET /feed.xml", s.handleFeed)
	mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) {
		setCache(w, cacheAbout)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "User-agent: *\nAllow: /\n")
	})

	staticSrv := http.FileServerFS(staticFS)
	mux.Handle("GET /static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if strings.HasSuffix(r.URL.Path, "/") || !staticFileExists(path) { // no directory listings, no cached 404s
			setCache(w, cacheNone)
			http.NotFound(w, r)
			return
		}
		setCache(w, cacheStatic)
		staticSrv.ServeHTTP(w, r)
	}))
	return withRequestLog(withSecurityHeaders(mux))
}

// staticFileExists reports whether path is a regular file in the embedded FS.
func staticFileExists(path string) bool {
	info, err := fs.Stat(staticFS, path)
	return err == nil && !info.IsDir()
}

func (s *Server) render(w http.ResponseWriter, status int, name string, data any) {
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("template error (%s): %v", name, err)
		setCache(w, cacheNone)
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
		"Nav":     "",
	})
}

// Package apiclient is a thin HTTP client for the Information-Broker API.
package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrNotFound is returned when the API responds 404.
var ErrNotFound = errors.New("not found")

// Article mirrors the JSON returned by the Information-Broker API.
type Article struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Summary     *string   `json:"summary"`
	Content     string    `json:"content"`
	PublishedAt time.Time `json:"published_at"`
	FeedURL     string    `json:"feed_url"`
	ContentHash string    `json:"content_hash"`
}

// ListResult is the envelope returned by GET /articles.
type ListResult struct {
	Articles []Article `json:"articles"`
	Count    int       `json:"count"`
	Limit    int       `json:"limit"`
	Offset   int       `json:"offset"`
}

// Feed is one entry from GET /feeds.
type Feed struct {
	FeedURL      string `json:"feed_url"`
	ArticleCount int    `json:"article_count"`
}

// Stats is the payload from GET /stats.
type Stats struct {
	TotalArticles int        `json:"total_articles"`
	TotalFeeds    int        `json:"total_feeds"`
	LastFetch     *time.Time `json:"last_fetch"`
}

// ListParams are the query options for ListArticles.
type ListParams struct {
	Limit  int
	Offset int
	Feed   string
	Q      string
}

// Client calls the Information-Broker API.
type Client struct {
	baseURL string
	http    *http.Client
}

// New creates a Client for the given API base URL.
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("api %s: status %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

// ListArticles fetches a page of articles with optional feed/search filters.
func (c *Client) ListArticles(ctx context.Context, p ListParams) (ListResult, error) {
	v := url.Values{}
	if p.Limit > 0 {
		v.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.Offset > 0 {
		v.Set("offset", strconv.Itoa(p.Offset))
	}
	if p.Feed != "" {
		v.Set("feed", p.Feed)
	}
	if p.Q != "" {
		v.Set("q", p.Q)
	}
	var res ListResult
	err := c.getJSON(ctx, "/articles?"+v.Encode(), &res)
	return res, err
}

// GetArticle fetches a single article by id. Returns ErrNotFound on 404.
func (c *Client) GetArticle(ctx context.Context, id int64) (Article, error) {
	var a Article
	err := c.getJSON(ctx, "/articles/get?id="+strconv.FormatInt(id, 10), &a)
	return a, err
}

// ListFeeds returns the known feeds and their article counts.
func (c *Client) ListFeeds(ctx context.Context) ([]Feed, error) {
	var res struct {
		Feeds []Feed `json:"feeds"`
	}
	err := c.getJSON(ctx, "/feeds", &res)
	return res.Feeds, err
}

// GetStats returns overall system statistics.
func (c *Client) GetStats(ctx context.Context) (Stats, error) {
	var s Stats
	err := c.getJSON(ctx, "/stats", &s)
	return s, err
}

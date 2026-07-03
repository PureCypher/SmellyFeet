package server

import (
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"time"

	"smellyfeet/internal/apiclient"
)

const feedEntries = 50

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr,omitempty"`
	Type string `xml:"type,attr,omitempty"`
}

type atomEntry struct {
	Title   string   `xml:"title"`
	ID      string   `xml:"id"`
	Link    atomLink `xml:"link"`
	Updated string   `xml:"updated"`
	Summary string   `xml:"summary,omitempty"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type atomDoc struct {
	XMLName xml.Name    `xml:"feed"`
	Xmlns   string      `xml:"xmlns,attr"`
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Author  atomAuthor  `xml:"author"`
	Links   []atomLink  `xml:"link"`
	Entries []atomEntry `xml:"entry"`
}

// baseURL reconstructs the public origin. cloudflared sets X-Forwarded-Proto.
func baseURL(r *http.Request) string {
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	res, err := s.svc.ListArticles(r.Context(), apiclient.ListParams{Limit: feedEntries})
	if err != nil {
		setCache(w, cacheNone)
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}

	base := baseURL(r)
	now := time.Now().UTC().Format(time.RFC3339)
	doc := atomDoc{
		Xmlns:   "http://www.w3.org/2005/Atom",
		Title:   "Information Broker",
		ID:      base + "/",
		Updated: now,
		Author:  atomAuthor{Name: "Information Broker"},
		Links: []atomLink{
			{Href: base + "/feed.xml", Rel: "self", Type: "application/atom+xml"},
			{Href: base + "/"},
		},
	}
	for _, a := range res.Articles {
		link := fmt.Sprintf("%s/article/%d", base, a.ID)
		e := atomEntry{Title: a.Title, ID: link, Link: atomLink{Href: link}, Updated: now}
		if !a.PublishedAt.IsZero() {
			e.Updated = a.PublishedAt.UTC().Format(time.RFC3339)
		}
		if a.Summary != nil {
			e.Summary = *a.Summary
		}
		doc.Entries = append(doc.Entries, e)
	}

	setCache(w, cacheFeed)
	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	_, _ = w.Write([]byte(xml.Header))
	if err := xml.NewEncoder(w).Encode(doc); err != nil {
		log.Printf("feed encode: %v", err)
	}
}

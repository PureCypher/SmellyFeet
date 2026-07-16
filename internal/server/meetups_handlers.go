package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// proposal is a submitted meetup suggestion. It is NEVER rendered on-site; it
// is only relayed to the notify webhook, so no HTML sanitization is required.
type proposal struct {
	Title        string
	Summary      string
	StartsAt     string // raw form value, kept as-is for the human reviewer
	City         string
	LocationType string
	OnlineURL    string
	RSVPURL      string
	ChapterName  string
	Organizer    string
	Contact      string
	Notes        string
}

// defaultNotify relays a proposal to the configured webhook as a simple JSON
// {"content": ...} body (Discord/Slack compatible). Empty webhook = log only,
// so local dev works with nothing configured. Contact is not logged.
func (s *Server) defaultNotify(ctx context.Context, p proposal) error {
	if s.notifyWebhook == "" {
		log.Printf("meetup proposal (no webhook): title=%q city=%q chapter=%q", p.Title, p.City, p.ChapterName)
		return nil
	}
	content := fmt.Sprintf("New meetup proposal: %q — city=%s chapter=%s starts=%s online=%s rsvp=%s (organizer=%s)",
		p.Title, p.City, p.ChapterName, p.StartsAt, p.OnlineURL, p.RSVPURL, p.Organizer)
	body, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		return fmt.Errorf("marshal proposal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.notifyWebhook, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("post proposal: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}

const (
	maxTitleLen   = 200
	maxContactLen = 200
	maxSummaryLen = 500
	maxNotesLen   = 4000
)

// proposalFromForm reads a proposal from POST form values. "website" is the
// honeypot and is intentionally not copied here (the handler checks it).
func proposalFromForm(r *http.Request) proposal {
	g := func(k string) string { return strings.TrimSpace(r.PostFormValue(k)) }
	return proposal{
		Title:        g("title"),
		Summary:      g("summary"),
		StartsAt:     g("starts_at"),
		City:         g("city"),
		LocationType: g("location_type"),
		OnlineURL:    g("online_url"),
		RSVPURL:      g("rsvp_url"),
		ChapterName:  g("chapter_name"),
		Organizer:    g("organizer"),
		Contact:      g("contact"),
		Notes:        g("notes"),
	}
}

// validateProposal returns a field->message map of problems; empty = valid.
func validateProposal(p proposal) map[string]string {
	errs := map[string]string{}
	if p.Title == "" {
		errs["title"] = "Please give the meetup a title."
	} else if len(p.Title) > maxTitleLen {
		errs["title"] = "Title is too long."
	}
	if p.Contact == "" {
		errs["contact"] = "Please leave a contact email or handle so we can follow up."
	} else if len(p.Contact) > maxContactLen {
		errs["contact"] = "Contact is too long."
	}
	if p.City == "" && p.OnlineURL == "" {
		errs["location"] = "Add a city or an online link."
	}
	if !httpURLOK(p.OnlineURL) {
		errs["online_url"] = "Online link must start with http:// or https://."
	}
	if !httpURLOK(p.RSVPURL) {
		errs["rsvp_url"] = "RSVP link must start with http:// or https://."
	}
	if len(p.Summary) > maxSummaryLen {
		errs["summary"] = "Summary is too long."
	}
	if len(p.Notes) > maxNotesLen {
		errs["notes"] = "Notes are too long."
	}
	return errs
}

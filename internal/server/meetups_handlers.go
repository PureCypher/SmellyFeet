package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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

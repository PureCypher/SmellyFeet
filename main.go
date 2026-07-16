package main

import (
	"log"
	"net/http"
	"time"

	"smellyfeet/internal/apiclient"
	"smellyfeet/internal/config"
	"smellyfeet/internal/server"
)

func main() {
	cfg := config.Load()
	client := apiclient.New(cfg.APIBaseURL)

	srv, err := server.New(client,
		server.WithMeetupsEnabled(cfg.MeetupsEnabled),
		server.WithMeetupTZ(cfg.MeetupsTZ),
		server.WithNotifyWebhook(cfg.MeetupsWebhook),
	)
	if err != nil {
		log.Fatalf("init server: %v", err)
	}

	addr := ":" + cfg.Port
	log.Printf("frontend listening on %s (API: %s)", addr, cfg.APIBaseURL)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

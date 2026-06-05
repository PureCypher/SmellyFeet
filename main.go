package main

import (
	"log"
	"net/http"

	"smellyfeet/internal/apiclient"
	"smellyfeet/internal/config"
	"smellyfeet/internal/server"
)

func main() {
	cfg := config.Load()
	client := apiclient.New(cfg.APIBaseURL)

	srv, err := server.New(client)
	if err != nil {
		log.Fatalf("init server: %v", err)
	}

	addr := ":" + cfg.Port
	log.Printf("frontend listening on %s (API: %s)", addr, cfg.APIBaseURL)
	if err := http.ListenAndServe(addr, srv.Routes()); err != nil {
		log.Fatal(err)
	}
}

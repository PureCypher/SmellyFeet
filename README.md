# SmellyFeet — Information Broker Frontend

A server-rendered Go web frontend for [Information-Broker](https://github.com/PureCypher/Information-Broker).
Browse AI-generated summaries of cybersecurity articles, read the full stored text, and jump to the
original source. Supports search, feed filtering, pagination, and a stats page.

## About

SmellyFeet is the reading room for **Information Broker** — a continuously-running intelligence
pipeline that scrapes cybersecurity RSS sources, summarizes every article with a local LLM, and
stores them for fast browsing.

This app turns that firehose into something you can actually read: a clean stream of AI-written
summaries, a full-text view for each article with a link straight back to the original source, plus
keyword search, per-source filtering, and live system statistics.

```
RSS sources → LLM summarize → PostgreSQL → Information Broker API → SmellyFeet
```

It's a small, dark-themed Go application with **no database of its own** — it talks only to the
Information Broker HTTP API, so it deploys and restarts independently of the scraper. An in-app
version of this page is served at `/about`.

## Run

```bash
cp .env.example .env   # edit API_BASE_URL / PORT as needed
go run .
```

Then open http://localhost:3000.

## Configuration

| Variable       | Default                  | Description                          |
|----------------|--------------------------|--------------------------------------|
| `API_BASE_URL` | `http://localhost:8080`  | Base URL of the Information-Broker API |
| `PORT`         | `3000`                   | Port the frontend listens on         |

## Architecture

Talks to the Information-Broker HTTP API only (no direct DB). `internal/apiclient` wraps the API,
`internal/server` renders `html/template` pages styled with Tailwind (CDN). See
`docs/superpowers/specs/2026-06-05-information-broker-frontend-design.md`.

## Test

```bash
go test ./...
```

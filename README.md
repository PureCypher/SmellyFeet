# SmellyFeet — Information Broker Frontend

A server-rendered Go web frontend for [Information-Broker](https://github.com/PureCypher/Information-Broker).
Browse AI-generated summaries of cybersecurity articles, read the full stored text, and jump to the
original source. Supports search, feed filtering, pagination, and a stats page.

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

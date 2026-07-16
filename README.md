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

Styling is a pre-built, committed CSS file. If you change templates or the theme,
regenerate it with `./scripts/build-css.sh` (downloads the Tailwind standalone CLI
on first run).

## Configuration

| Variable       | Default                  | Description                          |
|----------------|--------------------------|--------------------------------------|
| `API_BASE_URL` | `http://localhost:8080`  | Base URL of the Information-Broker API |
| `PORT`         | `3000`                   | Port the frontend listens on         |

## Public deployment

The site is designed to be exposed through a Cloudflare Tunnel — see
[deploy/README.md](deploy/README.md). The app sends per-route `Cache-Control`
headers (articles cache at the edge for an hour), strict security headers, and
serves everything — CSS, fonts, favicon — from the binary with zero JavaScript
and zero third-party requests. An Atom feed is available at `/feed.xml`.

## Meetups

A community-run directory of infosec meetups, in the BSides spirit. Listings live in an embedded
seed file rather than a database, so the app stays a stateless, GET-first proxy.

### Routes

| Route                        | Description                                                                    |
|-------------------------------|--------------------------------------------------------------------------------|
| `GET /meetups`                | List upcoming/past meetups. Filters: `?city=`, `?tag=`, `?online=1`, `?chapter=` |
| `GET /meetups/{slug}`         | Meetup detail page                                                              |
| `GET /meetups/{slug}/ics`     | Download an `.ics` calendar file for the meetup                                |
| `GET /meetups/chapters`       | Chapter directory (discovery only — see attribution below)                     |
| `GET /api/meetups`            | Read-only JSON of the current listings. Filters: `?city=`, `?tag=`, `?from=`, `?to=` (RFC3339) |

All of the above (and the nav tab) are gated by `MEETUPS_ENABLED`.

### Configuration

| Variable                 | Default          | Description                                                                 |
|---------------------------|------------------|------------------------------------------------------------------------------|
| `MEETUPS_ENABLED`         | `true`           | Gates the Meetups tab and every `/meetups*` + `/api/meetups` route          |
| `MEETUPS_DEFAULT_TZ`      | `Europe/London`  | IANA timezone used to display meetup times                                 |

### Adding a meetup

There's no database — meetups are published by editing the embedded seed,
`internal/server/meetups_seed.json`:

1. Add an entry with: `slug`, `title`, `summary`, `description`, `starts_at`/`ends_at` (RFC3339
   with a UTC offset, e.g. `2026-09-24T18:30:00+01:00`), `location_type`
   (`in_person`/`online`/`hybrid`), `venue_name`/`venue_address`, `city`/`region`/`country`,
   `online_url`, `rsvp_url`, `chapter_name`, `chapter_url`, and `tags` (a string array).
2. Every URL field (`online_url`, `rsvp_url`, `chapter_url`, and a chapter's `website`) must be
   `http://` or `https://` — anything else fails validation at startup.
3. If you also changed templates, regenerate CSS with `bash scripts/build-css.sh`.
4. Commit and redeploy. Publishing is just "edit the seed, redeploy" — git history is the audit log.

### Refreshing tracked events

The tracker refreshes **fully autonomously**: a weekly cron on the host runs
`scripts/cron-refresh.sh`, which resets to `origin/main`, re-scrapes every source with
`scripts/import-events.py --apply`, and rebuilds the container. No human in the loop and
no hand-pinned events — the tracker is exactly what the sources yield each week. A safety
floor skips the deploy if a scrape collapses (< 30 events). Install the cron with:

```
crontab -l 2>/dev/null | grep -q cron-refresh || \
  (crontab -l 2>/dev/null; echo "0 4 * * 1 /home/pure/smellyfeet-frontend/scripts/cron-refresh.sh") | crontab -
```

To refresh manually / preview without deploying:

Meetup events are tracked from each chapter's own website. To refresh them:

```
python3 scripts/import-events.py > /tmp/events.json
```

This pulls from BSides chapter sites, the [cryptax/confsec](https://github.com/cryptax/confsec)
conference list, the DEF CON forum calendar (incl. regional editions like DEF CON Middle East),
and a few named conference sites, and prints candidate events. It is best-effort: heterogeneous
sites mean a few dates may be CFP/ticket dates, and JS-only or cert-broken sites yield
nothing (reported on stderr). **Review the dates**, then merge the good ones into the
`meetups` array of `internal/server/meetups_seed.json` and redeploy. Manually-verified
events (e.g. chapters whose date is only rendered client-side) can be pinned by hand.

### Attribution

This is a community meetup tracker in the BSides spirit — it is **not** an official BSides mirror
or endorsed listing. The official, approved chapter list lives at
[bsides.org/chapters](https://bsides.org/chapters/), and every Meetups page links back to it.

## Architecture

Talks to the Information-Broker HTTP API only (no direct DB). `internal/apiclient` wraps the API,
`internal/server` renders `html/template` pages styled with Tailwind (CDN). See
`docs/superpowers/specs/2026-06-05-information-broker-frontend-design.md`.

## Test

```bash
go test ./...
```

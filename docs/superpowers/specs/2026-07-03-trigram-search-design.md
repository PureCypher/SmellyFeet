# Trigram Search Index — Design

**Date:** 2026-07-03
**Status:** Approved
**Goal:** Cut /articles `q` search latency (~2.3s live) by indexing the three ILIKE'd columns
with pg_trgm GIN indexes. No query or API changes.

## Context

`buildArticlesQuery` searches `title ILIKE $ OR summary ILIKE $ OR full_content ILIKE $`
across ~46k rows — a sequential scan. Postgres' planner uses GIN `gin_trgm_ops` indexes for
`ILIKE '%…%'` automatically. Schema lives as an idempotent statement slice in
Information-Broker's `main.go` (mirrored in `schema.sql`); DB user is the postgres superuser,
so `CREATE EXTENSION` is permitted. Approach approved: code migration + zero-downtime
`CONCURRENTLY` apply on the live DB (rejected: startup-built indexes — plain CREATE INDEX
locks writes and stalls startup; psql-only — fresh installs would lose the indexes).

## Changes (Information-Broker repo)

`main.go` migration slice — insert after the articles-table indexes:

```go
`CREATE EXTENSION IF NOT EXISTS pg_trgm`,
`CREATE INDEX IF NOT EXISTS idx_articles_title_trgm ON articles USING GIN (title gin_trgm_ops)`,
`CREATE INDEX IF NOT EXISTS idx_articles_summary_trgm ON articles USING GIN (summary gin_trgm_ops)`,
`CREATE INDEX IF NOT EXISTS idx_articles_full_content_trgm ON articles USING GIN (full_content gin_trgm_ops)`,
```

`schema.sql` — the same four statements appended to its index section.

## Live rollout (zero downtime, before deploying the code)

Via `docker exec information-broker-postgres psql` on the host, autocommit (CONCURRENTLY
cannot run in a transaction), same index names so the startup migration later no-ops:

1. `CREATE EXTENSION IF NOT EXISTS pg_trgm;`
2. `CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_articles_title_trgm ...;` (then summary,
   then full_content — sequentially; the full-content build is the slow one)

Then deploy the code: push, `git pull`, rebuild `rss-monitor` — startup statements find the
indexes already present.

## Verification

- `EXPLAIN ANALYZE` of the exact production search query before (Seq Scan, seconds) and
  after (Bitmap Index Scan on the trgm indexes, milliseconds).
- Live timing of `https://feed.purecypher.com/?q=keycloak` — target < 300ms (from ~2.3s).
- `\di+ idx_articles_*_trgm` to record index sizes.

## Known costs / out of scope

- GIN full-content index likely 100–300 MB and slower inserts — acceptable for a
  low-throughput scraper on this host.
- No relevance ranking / websearch syntax (that would be tsvector full-text search — a
  different, larger design). ILIKE semantics unchanged.

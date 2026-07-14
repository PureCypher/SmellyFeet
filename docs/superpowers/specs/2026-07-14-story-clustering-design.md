# Precomputed Story Clustering — Design

**Date:** 2026-07-14
**Status:** Approved (design only — implementation deferred to a separate follow-up pass)
**Goal:** Replace the digest feature's live exact-title-match heuristic with a precomputed,
embedding-based story-clustering job, so the daily/weekly digest windows actually populate an
"important" (cross-feed) bucket instead of sitting empty.

## Context

The `/digest` feature (spec: `2026-07-14-digest-heuristic-design.md`) went through two live
iterations already, both computed per-request in SQL:

1. A `pg_trgm` title-similarity self-join (`a1.title % a2.title`) — timed out in production
   past ~2k rows. Root cause: the trigram GIN index only accelerates `title % 'literal'`
   (column-to-constant), not column-to-column comparisons, so the self-join fell back to a
   full nested loop.
2. A `GROUP BY` on a normalized title (lowercased, trailing `| Site Name` suffix stripped,
   punctuation/smart-quotes stripped) — fast (~250ms even at 30-day/10k-row scale via
   `EXPLAIN ANALYZE`), but too strict live: daily and weekly digests never populate
   "important" at all. Verified: outlets genuinely reword headlines for the same event rather
   than merely differing in punctuation or branding, and only the 30-day window has enough
   time for byte-identical wire-copy titles to accumulate from 3+ distinct feeds.

Both iterations are fundamentally exact-match (post-normalization) approaches, and neither can
catch two outlets that word the same story differently. Real semantic matching requires either
comparing meaning (embeddings) or judgment (an LLM) — and doing that live, per request, at
digest-page latency is not viable at this data volume. This design moves the matching to a
precomputed background job instead.

**Confirmed prerequisites (verified live this session):**
- Ollama is reachable from the Information-Broker app container at `http://172.17.0.1:11434`
  (the docker bridge gateway — Ollama runs natively on the host, not in a compose-managed
  container). Version 0.23.2.
- Exactly one model is currently pulled: `granite4.1:3b` (3.4B, generative/chat-tuned) — no
  dedicated embedding model. `ollama pull nomic-embed-text` (~270MB) is a one-time host setup
  step this design depends on; it is **not** yet done and must happen before the job can run.
- `pgvector` is **not** installed (`SELECT * FROM pg_available_extensions WHERE name = 'vector'`
  returns zero rows) and the compose stack uses the stock `postgres:15-alpine` image, not a
  pgvector-enabled one. This design avoids that image swap entirely by storing embeddings as a
  native Postgres array and computing cosine similarity in Go, not SQL.

## 1. Why embeddings, not LLM pairwise judgment

An embedding call is O(1) per article: encode once, then compare cheaply (a dot product) many
times. LLM pairwise judgment ("are these two headlines about the same story?") is O(candidates)
per article — one full generation call per comparison — which is far more Ollama load and,
being free-text judgment rather than a numeric score, less deterministic and harder to threshold
consistently. Embeddings are the standard technique for this exact task (near-duplicate/story
clustering) and fit the existing self-hosted-Ollama constraint without adding LLM call volume
that scales quadratically.

## 2. Schema changes (Information-Broker)

Two new nullable columns on `articles`, added via the existing idempotent migration-slice
pattern in `main.go` (mirrored in `schema.sql`), same convention as every prior column addition
in this repo:

```sql
ALTER TABLE articles ADD COLUMN IF NOT EXISTS title_embedding real[];
ALTER TABLE articles ADD COLUMN IF NOT EXISTS story_cluster_id BIGINT;
CREATE INDEX IF NOT EXISTS idx_articles_story_cluster_id ON articles(story_cluster_id);
```

- `title_embedding`: the article title's embedding vector, as a native Postgres `real[]` — no
  `pgvector` needed since similarity is computed in Go, not SQL.
- `story_cluster_id`: self-referencing — points at the **first** article's `id` assigned to
  that story cluster. An unclustered (singleton) article points at its own `id`. This avoids a
  separate `story_clusters` table entirely: the "cluster ID" is just the seed article's own ID.
- Both columns are populated by the new background job (Section 3), never by the RSS
  ingestion path (`monitor.go`) — keeps ingestion fast and unchanged.

## 3. Background job (Information-Broker)

A new `ClusteringScheduler`, structured like the existing `SummarizationScheduler`
(`summarization_scheduler.go`) — same file-per-concern convention, same
ticker-driven-background-worker shape, started alongside it in `main.go`. Runs on a ticker
(proposed 15 minutes — adjustable via config, same `getEnvDuration` pattern as `OLLAMA_TIMEOUT`
etc.). Each cycle:

1. **Embed:** `SELECT id, title FROM articles WHERE publish_date >= now() - interval '35 days'
   AND title_embedding IS NULL LIMIT 200`. For each, call Ollama's `/api/embeddings` with the
   title only (cheaper and sufficient for headline-level matching; full content is out of
   scope — see Section 6) and store the resulting vector. 200 per cycle is a starting point
   (bounds worst-case cycle time if a backlog builds up; a 15-minute ticker easily drains
   normal ingestion volume — 130 feeds isn't publishing 200 new articles every 15 minutes)
   and should be verified against real embedding-call latency during implementation.
2. **Cluster:** `SELECT id, title_embedding FROM articles WHERE publish_date >= now() - interval
   '35 days' AND story_cluster_id IS NULL AND title_embedding IS NOT NULL`. For each such
   article, compare its embedding (cosine similarity, computed in Go — a plain dot-product
   over two `[]float32`, no new dependency) against the embeddings of existing **cluster
   seeds** in the same 35-day window (one representative embedding per distinct
   `story_cluster_id` already assigned). Above a similarity threshold (proposed 0.85 — *not*
   yet tuned against real data; the first implementation task must verify this against actual
   article pairs, the same way the trigram/GROUP BY thresholds were spot-checked live this
   session) joins that cluster (`story_cluster_id = <seed's id>`); otherwise the article seeds
   a new cluster (`story_cluster_id = <its own id>`).

This is **greedy, incremental, single-linkage-ish clustering**: new articles are compared
against existing seeds, not against every article ever processed. Cost is O(new articles ×
active clusters in the window), not O(n²) — the same complexity class problem that broke the
original trigram self-join is deliberately avoided here by construction.

**35-day window, not 30:** matches the digest's largest window (30 days) with a few days of
slack so a cluster "seed" from day 1 of a 30-day digest window is never missed because the job
itself only looks back exactly 30 days.

## 4. Digest query rewrite (Information-Broker)

`buildDigestQuery` drops the entire normalization/`GROUP BY`-on-title mechanism from the prior
iteration. It becomes a plain `GROUP BY story_cluster_id`:

```sql
SELECT a.id, a.title, a.url, a.summary, a.full_content, a.publish_date,
	a.fetch_duration_ms, a.feed_url, a.content_hash,
	(cluster_counts.distinct_feeds - 1) AS cross_feed_count
FROM articles a
JOIN (
	SELECT story_cluster_id, COUNT(DISTINCT feed_url) AS distinct_feeds
	FROM articles
	WHERE publish_date >= $1 AND story_cluster_id IS NOT NULL
	GROUP BY story_cluster_id
) cluster_counts ON cluster_counts.story_cluster_id = a.story_cluster_id
WHERE a.publish_date >= $1
ORDER BY cross_feed_count DESC, a.publish_date DESC
```

Simpler than either prior version — no regex, no self-join, just an indexed `GROUP BY` on
`story_cluster_id` (backed by `idx_articles_story_cluster_id`). Articles with `story_cluster_id
IS NULL` (not yet processed by the clustering job — e.g., published in the last 15 minutes)
are excluded from the "important" calculation for that request but still appear via the
existing `WHERE a.publish_date >= $1` in the main result set, landing in "everything else"
until the next clustering cycle picks them up. `minCrossFeedCountForImportant` (still 2) and
`splitImportant` are unchanged.

## 5. Testing & verification

- **Unit (pure functions, no DB):** cosine similarity function (`cosineSimilarity([]float32,
  []float32) float64`) — table tests for identical vectors (1.0), orthogonal vectors (0.0),
  opposite vectors (-1.0), mismatched lengths (defined behavior — likely an error or 0, needs
  deciding in the plan). Cluster-assignment decision logic (given a new embedding and a set of
  seed embeddings, which cluster does it join, or does it seed a new one) as its own testable
  function, independent of the DB/Ollama calls around it — same "extract the pure logic"
  pattern already used for `extractMainContent`, `splitImportant`, etc. in this codebase.
- **Integration-style verification (manual, live, matching this session's established
  practice):** since this repo has no automated DB-test harness (confirmed during the original
  digest plan), verify via direct `psql`/Ollama calls against the live database and Ollama
  instance before deploying — the same approach used to validate the trigram-to-GROUP-BY fix
  and the normalization fix earlier this session. Specifically: embed a handful of known
  same-story/different-story title pairs from the live corpus, confirm the similarity scores
  separate them at a reasonable threshold, *before* picking the final 0.85 default.
- **Digest query test:** table test on the rewritten `buildDigestQuery`, mirroring the existing
  `TestBuildDigestQuery` style (assert SQL shape, not live execution).

## 6. Known ceiling / out of scope

- **Title-only embeddings.** Full-content embeddings would likely improve accuracy (titles can
  be short/ambiguous) but cost more per call and are unnecessary for v1 — headline-level
  matching is the same granularity the prior two iterations targeted.
- **Single-linkage approximation.** Comparing only against cluster seeds (not full pairwise
  comparison within a cluster) means a chain of loosely-related titles could fail to transitively
  merge. Acceptable for a heuristic feature already documented as imperfect at every prior
  iteration.
- **No historical backfill.** Articles older than the 35-day window are never embedded or
  clustered — irrelevant, since no digest window looks back further than 30 days.
- **No UI for inspecting/correcting cluster assignments.** A human-in-the-loop correction
  mechanism is a plausible future need but not in scope here.
- **Similarity threshold (0.85) is a starting point, not tuned.** The first implementation task
  must verify it against real title pairs from the live corpus before this ships, the same way
  every numeric threshold in this feature's history has been spot-checked live rather than
  guessed.
- **Ollama availability risk.** If the embedding model isn't pulled, or Ollama becomes
  unreachable, the job should degrade gracefully (skip the cycle, log, retry next tick) rather
  than block ingestion or crash the app — matching the existing `SummarizationScheduler`'s
  retry/backoff posture.

## Out of scope for this pass

Per explicit instruction, this document is a design only. Implementation (writing-plans,
task breakdown, actual code) is deferred to a separate follow-up pass — this session stops
after the spec is written, self-reviewed, and committed.

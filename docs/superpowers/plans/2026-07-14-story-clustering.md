# Precomputed Story Clustering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **Superseded during implementation (2026-07-14, commit 7666922):** every task below (and the
> code blocks in them) embeds article **titles** (`title_embedding`, `it.title`, etc.). A live
> threshold spot-check during Task 7 found title-only embeddings unreliable on this corpus —
> different CVE advisories scored more similar (0.85+) than genuine cross-outlet duplicates of
> the same story (0.67-0.74), because CVE titles share heavy boilerplate. The shipped
> implementation embeds the **summary** instead — same architecture, one field swap (`title` →
> `summary` in the query and the embed call, `title_embedding` → `summary_embedding` for the
> column everywhere), plus excluding un-summarized and failed-summary (`'summary unavailable'`)
> articles from the embed batch. Re-verified with real summary text: true duplicates scored
> 0.87-0.93, a different-story pair scored 0.71, both sides of the existing 0.85 threshold. If
> re-running any task from this plan from scratch, swap every `title`/`title_embedding`
> reference for `summary`/`summary_embedding` and add the un-summarized/failed-summary
> exclusion described above; everything else (schema shape, scheduler structure, idle-gating,
> `story_cluster_id` semantics, the digest query rewrite) is unchanged.

**Goal:** Replace the digest feature's live exact-title-match heuristic with a precomputed
background job that embeds article titles via Ollama and incrementally clusters them, so
daily/weekly digest windows actually populate an "important" (cross-feed) bucket.

**Architecture:** A new `ClusteringScheduler` (Information-Broker, ticker-driven, mirroring the
existing `SummarizationScheduler`/`RSSMonitor` background-worker shape) periodically embeds
un-embedded recent titles via Ollama's `/api/embeddings`, then assigns each un-clustered article
to the most similar existing cluster (by cosine similarity against cluster seed embeddings) or
seeds a new cluster. Two new nullable columns on `articles` store the embedding and the cluster
assignment. `buildDigestQuery` is rewritten to a plain `GROUP BY story_cluster_id`. The job
skips its cycle entirely whenever the summarization scheduler is actively processing, so it
never competes with per-article summarization for Ollama.

**Tech Stack:** Go, Postgres (native `real[]` array column, no `pgvector`), Ollama
`/api/embeddings`, `github.com/lib/pq`'s `pq.Array` (already a transitive dependency via the
blank `lib/pq` import — no new dependency).

**Spec:** `docs/superpowers/specs/2026-07-14-story-clustering-design.md`

## Global Constraints

- No new Go dependencies. `pq.Array` comes from `github.com/lib/pq`, already in `go.mod` via
  `main.go`'s `_ "github.com/lib/pq"` import.
- No `pgvector` — similarity is computed in Go over a native Postgres `real[]` column.
- This pass is local commits only in Information-Broker (per session pattern) — no push, no
  live deploy, until explicitly requested afterward.
- Ollama confirmed reachable from the app container at `http://172.17.0.1:11434` (v0.23.2).
  Only `granite4.1:3b` is currently pulled — `nomic-embed-text` must be pulled on the host
  before the scheduler can embed anything (Task 1).
- The clustering job must check `SummarizationScheduler.GetStats()` (`queue_depth`,
  `current_request`) before running its embed/cluster steps each cycle, and skip the cycle
  entirely (log and retry next tick) if the summarizer is active — never a wall-clock schedule.
- `story_cluster_id` is self-referencing: the seed article's own `id`. A singleton article's
  `story_cluster_id` equals its own `id`. No separate clusters table.
- Similarity threshold starts at 0.85 but Task 7 requires live verification against real
  corpus title pairs before considering the feature done, per the spec's explicit requirement
  (every threshold in this feature's history has been spot-checked live, not guessed).
- Follow existing repo conventions: file-per-concern (mirrors `summarizer.go` +
  `summarization_scheduler.go`'s split), `getEnvDuration`/`getEnvInt` config-loading style,
  table-driven tests, no test for config defaults (no existing precedent for that in
  `config_test.go`).

---

## Task 1: Pull the embedding model on the host

**Files:** none (operational step, no code)

**Interfaces:** none — this is a prerequisite for every later task that calls
`/api/embeddings` with model `nomic-embed-text`.

- [ ] **Step 1: Pull the model**

Run: `ssh smellyfeet-host 'curl -s http://localhost:11434/api/pull -d "{\"name\": \"nomic-embed-text\"}"'`

Expected: a stream of JSON status lines ending in `{"status":"success"}`.

- [ ] **Step 2: Verify it's available and returns a real embedding**

Run: `ssh smellyfeet-host 'curl -s http://localhost:11434/api/embeddings -d "{\"model\": \"nomic-embed-text\", \"prompt\": \"test\"}"'`

Expected: JSON with an `"embedding"` key holding an array of floats (768 dimensions for
`nomic-embed-text`). Record the dimension count — later tasks don't hardcode it (Go slices are
dynamically sized), but it's useful to confirm here.

No commit for this task (no files changed).

---

## Task 2: Schema — `title_embedding` and `story_cluster_id` columns

**Files:**
- Modify: `/home/pure/Documents/github/Information-Broker/main.go:214-257` (the `createTables`
  migration slice)
- Modify: `/home/pure/Documents/github/Information-Broker/schema.sql` (mirror, per existing
  convention — the trigram index comment there says "mirrored in schema.sql")

**Interfaces:**
- Produces: columns `articles.title_embedding real[]` (nullable), `articles.story_cluster_id
  BIGINT` (nullable), index `idx_articles_story_cluster_id`

- [ ] **Step 1: Add the migration statements**

In `main.go`, in the `queries := []string{...}` slice (currently ending at line 244 with
`idx_articles_full_content_trgm` before the `fetch_logs` table at line 245), insert after the
trigram index line:

```go
		`CREATE INDEX IF NOT EXISTS idx_articles_full_content_trgm ON articles USING GIN (full_content gin_trgm_ops)`,
		// Story-clustering columns: title_embedding backs the precomputed clustering job's
		// similarity comparisons (no pgvector -- plain Postgres array, compared in Go);
		// story_cluster_id is self-referencing (a cluster's seed article's own id).
		`ALTER TABLE articles ADD COLUMN IF NOT EXISTS title_embedding real[]`,
		`ALTER TABLE articles ADD COLUMN IF NOT EXISTS story_cluster_id BIGINT`,
		`CREATE INDEX IF NOT EXISTS idx_articles_story_cluster_id ON articles(story_cluster_id)`,
		`CREATE TABLE IF NOT EXISTS fetch_logs (
```

(That last line is the existing `fetch_logs` table start — you're inserting three new lines
immediately before it, not replacing it.)

Mirror the same three new lines into `schema.sql`, in its indexes section, following the same
placement convention used for the trigram indexes there.

- [ ] **Step 2: Build and run the existing test suite**

Run: `cd /home/pure/Documents/github/Information-Broker && go build ./... && go test ./...`
Expected: build succeeds, all existing tests still pass (no test exercises `createTables`
directly, so this just confirms nothing else broke).

- [ ] **Step 3: Commit**

```bash
git add main.go schema.sql
git commit -m "feat(db): add title_embedding and story_cluster_id columns for clustering"
```

---

## Task 3: Config — `ClusteringConfig`

**Files:**
- Modify: `/home/pure/Documents/github/Information-Broker/config/config.go`

**Interfaces:**
- Produces: `config.ClusteringConfig{Interval time.Duration; WindowDays int; BatchSize int;
  SimilarityThreshold float64; EmbedModel string}`, field `Config.Clustering
  ClusteringConfig`, helper `getEnvFloat(key string, defaultValue float64) float64`

- [ ] **Step 1: Add the `ClusteringConfig` struct**

After the `SummarizationConfig` struct (`config.go:103-111`), add:

```go
// ClusteringConfig holds configuration for the precomputed story-clustering scheduler.
type ClusteringConfig struct {
	Interval            time.Duration
	WindowDays          int
	BatchSize           int
	SimilarityThreshold float64
	EmbedModel          string
}
```

- [ ] **Step 2: Add the field to `Config` and populate it in `Load()`**

In the `Config` struct (`config.go:12-24`), add `Clustering ClusteringConfig` after
`Summarization SummarizationConfig`.

In `Load()`, after the `Summarization: SummarizationConfig{...}` block (ending at
`config.go:178` with the closing `},`), add:

```go
		Clustering: ClusteringConfig{
			Interval:            getEnvDuration("CLUSTERING_INTERVAL", 15*time.Minute),
			WindowDays:          getEnvInt("CLUSTERING_WINDOW_DAYS", 35),
			BatchSize:           getEnvInt("CLUSTERING_BATCH_SIZE", 200),
			SimilarityThreshold: getEnvFloat("CLUSTERING_SIMILARITY_THRESHOLD", 0.85),
			EmbedModel:          getEnv("CLUSTERING_EMBED_MODEL", "nomic-embed-text"),
		},
```

- [ ] **Step 3: Add the `getEnvFloat` helper**

After `getEnvInt` (`config.go:190-197`), add:

```go
func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}
```

- [ ] **Step 4: Build and run the existing test suite**

Run: `go build ./... && go test ./...`
Expected: build succeeds, all tests pass. (No new test here — this repo has no existing
precedent for testing config default values; `config_test.go` only covers `IsFeedExcluded`.)

- [ ] **Step 5: Commit**

```bash
git add config/config.go
git commit -m "feat(config): add ClusteringConfig for the story-clustering scheduler"
```

---

## Task 4: `embeddings.go` — similarity, cluster assignment, Ollama embedding call

**Files:**
- Create: `/home/pure/Documents/github/Information-Broker/embeddings.go`
- Test: `/home/pure/Documents/github/Information-Broker/embeddings_test.go`

**Interfaces:**
- Consumes: nothing new (stdlib `net/http`, `encoding/json`, `context`, `math`, `fmt`)
- Produces: `cosineSimilarity(a, b []float32) float64`, `assignCluster(newEmbedding []float32,
  seeds map[int64][]float32, threshold float64) (seedID int64, ok bool)`,
  `fetchEmbedding(ctx context.Context, httpClient *http.Client, ollamaURL, model, text string)
  ([]float32, error)`

- [ ] **Step 1: Write the failing tests**

```go
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a, b []float32
		want float64
	}{
		{"identical", []float32{1, 0, 0}, []float32{1, 0, 0}, 1.0},
		{"orthogonal", []float32{1, 0}, []float32{0, 1}, 0.0},
		{"opposite", []float32{1, 0}, []float32{-1, 0}, -1.0},
		{"mismatched lengths", []float32{1, 2, 3}, []float32{1, 2}, 0.0},
		{"zero vector", []float32{0, 0}, []float32{1, 1}, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cosineSimilarity(tt.a, tt.b)
			if diff := got - tt.want; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("cosineSimilarity(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestAssignCluster(t *testing.T) {
	seeds := map[int64][]float32{
		100: {1, 0, 0},
		200: {0, 1, 0},
	}

	t.Run("joins the most similar seed above threshold", func(t *testing.T) {
		id, ok := assignCluster([]float32{0.99, 0.01, 0}, seeds, 0.85)
		if !ok || id != 100 {
			t.Fatalf("got (%d, %v), want (100, true)", id, ok)
		}
	})

	t.Run("no seed above threshold seeds a new cluster", func(t *testing.T) {
		_, ok := assignCluster([]float32{0, 0, 1}, seeds, 0.85)
		if ok {
			t.Fatalf("expected ok=false, got a match")
		}
	})

	t.Run("empty seed set never matches", func(t *testing.T) {
		_, ok := assignCluster([]float32{1, 0, 0}, map[int64][]float32{}, 0.85)
		if ok {
			t.Fatalf("expected ok=false with no seeds")
		}
	})
}

func TestFetchEmbedding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "nomic-embed-text" || body["prompt"] != "some title" {
			t.Errorf("unexpected request body: %+v", body)
		}
		w.Write([]byte(`{"embedding":[0.1,0.2,0.3]}`))
	}))
	defer srv.Close()

	emb, err := fetchEmbedding(context.Background(), srv.Client(), srv.URL, "nomic-embed-text", "some title")
	if err != nil {
		t.Fatalf("fetchEmbedding error: %v", err)
	}
	if len(emb) != 3 || emb[0] != 0.1 || emb[1] != 0.2 || emb[2] != 0.3 {
		t.Fatalf("unexpected embedding: %v", emb)
	}
}

func TestFetchEmbeddingNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := fetchEmbedding(context.Background(), srv.Client(), srv.URL, "nomic-embed-text", "x")
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'TestCosineSimilarity|TestAssignCluster|TestFetchEmbedding' -v`
Expected: FAIL to compile — none of `cosineSimilarity`, `assignCluster`, `fetchEmbedding` exist yet.

- [ ] **Step 3: Write the implementation**

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
)

// cosineSimilarity returns the cosine similarity between two vectors, in
// [-1, 1]. Returns 0 for mismatched lengths or either vector being zero,
// rather than erroring -- callers treat 0 as "not similar," which is the
// correct behavior for both cases in the clustering decision this backs.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// assignCluster finds the seed (keyed by its cluster's seed article id) most
// similar to newEmbedding. Returns ok=false if no seed's similarity reaches
// threshold (or there are no seeds at all) -- the caller should then seed a
// new cluster using the new article's own id.
func assignCluster(newEmbedding []float32, seeds map[int64][]float32, threshold float64) (seedID int64, ok bool) {
	bestSimilarity := threshold
	found := false
	for id, seedEmbedding := range seeds {
		sim := cosineSimilarity(newEmbedding, seedEmbedding)
		if sim >= bestSimilarity {
			bestSimilarity = sim
			seedID = id
			found = true
		}
	}
	return seedID, found
}

// fetchEmbedding calls Ollama's /api/embeddings for a single text input.
func fetchEmbedding(ctx context.Context, httpClient *http.Client, ollamaURL, model, text string) ([]float32, error) {
	reqBody, err := json.Marshal(map[string]string{"model": model, "prompt": text})
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ollamaURL+"/api/embeddings", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed request returned status %d", resp.StatusCode)
	}

	var parsed struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode embed response: %w", err)
	}
	return parsed.Embedding, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run 'TestCosineSimilarity|TestAssignCluster|TestFetchEmbedding' -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add embeddings.go embeddings_test.go
git commit -m "feat: add cosine similarity, cluster assignment, and Ollama embedding call"
```

---

## Task 5: `ClusteringScheduler` — idle-gated background job

**Files:**
- Create: `/home/pure/Documents/github/Information-Broker/clustering_scheduler.go`
- Test: `/home/pure/Documents/github/Information-Broker/clustering_scheduler_test.go`
- Modify: `/home/pure/Documents/github/Information-Broker/main.go:53-92` (construct and start
  alongside the other schedulers)

**Interfaces:**
- Consumes: `cosineSimilarity`, `assignCluster`, `fetchEmbedding` (Task 4),
  `config.ClusteringConfig` (Task 3), `SummarizationScheduler.GetStats() map[string]interface{}`
  (existing, exported)
- Produces: `NewClusteringScheduler(db *sql.DB, cfg *config.Config, summarizer
  *SummarizationScheduler) *ClusteringScheduler`, `(c *ClusteringScheduler) Start(ctx
  context.Context)`, `(c *ClusteringScheduler) isIdle() bool` (unexported, but its own
  testable unit)

This task is the core of the feature. Two pieces are meaningfully separable for testing: the
**idle-gating decision** (pure logic over a stats map) and the **query/DB-touching cycle
steps** (no test harness for live DB queries exists in this repo — same situation Task 4 of
the original digest plan already documented and followed).

- [ ] **Step 1: Write the failing test for idle-gating**

```go
package main

import "testing"

func TestIsIdleFromStats(t *testing.T) {
	tests := []struct {
		name       string
		queueDepth int
		current    bool
		want       bool
	}{
		{"empty queue, nothing in flight", 0, false, true},
		{"queue has pending work", 3, false, false},
		{"queue empty but a request is in flight", 0, true, false},
		{"both busy", 2, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := map[string]interface{}{
				"queue_depth":     tt.queueDepth,
				"current_request": tt.current,
			}
			if got := isIdleFromStats(stats); got != tt.want {
				t.Errorf("isIdleFromStats(%+v) = %v, want %v", stats, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestIsIdleFromStats -v`
Expected: FAIL — `undefined: isIdleFromStats`.

- [ ] **Step 3: Write the full scheduler implementation**

```go
package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"

	"information-broker/config"

	"github.com/lib/pq"
)

// ClusteringScheduler periodically embeds recent article titles and assigns
// them to story clusters, backing the digest feature's cross-feed
// "important" bucket. It never runs concurrently with active summarization
// -- see isIdle -- so it doesn't compete for Ollama capacity.
type ClusteringScheduler struct {
	db         *sql.DB
	config     *config.Config
	httpClient *http.Client
	summarizer *SummarizationScheduler
}

// NewClusteringScheduler creates a new story-clustering scheduler.
func NewClusteringScheduler(db *sql.DB, cfg *config.Config, summarizer *SummarizationScheduler) *ClusteringScheduler {
	return &ClusteringScheduler{
		db:         db,
		config:     cfg,
		httpClient: &http.Client{Timeout: cfg.OLLAMA.Timeout},
		summarizer: summarizer,
	}
}

// Start begins the ticker-driven background loop. Blocks until ctx is done.
func (c *ClusteringScheduler) Start(ctx context.Context) {
	log.Printf("Starting story-clustering scheduler (interval: %v)", c.config.Clustering.Interval)
	ticker := time.NewTicker(c.config.Clustering.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Story-clustering scheduler stopping")
			return
		case <-ticker.C:
			c.runCycle(ctx)
		}
	}
}

// isIdleFromStats reports whether the summarization scheduler is idle, given
// its GetStats() snapshot -- extracted as a pure function so the decision
// logic is testable without a real SummarizationScheduler.
func isIdleFromStats(stats map[string]interface{}) bool {
	depth, _ := stats["queue_depth"].(int)
	current, _ := stats["current_request"].(bool)
	return depth == 0 && !current
}

func (c *ClusteringScheduler) isIdle() bool {
	return isIdleFromStats(c.summarizer.GetStats())
}

// runCycle runs one embed-then-cluster pass, skipping entirely if
// summarization is active this tick.
func (c *ClusteringScheduler) runCycle(ctx context.Context) {
	if !c.isIdle() {
		log.Println("Story-clustering: summarization active, skipping this cycle")
		return
	}

	if err := c.embedBatch(ctx); err != nil {
		log.Printf("Story-clustering: embed batch failed: %v", err)
		return
	}
	if err := c.clusterBatch(ctx); err != nil {
		log.Printf("Story-clustering: cluster batch failed: %v", err)
	}
}

// embedBatch embeds up to BatchSize titles in the clustering window that
// don't have an embedding yet.
func (c *ClusteringScheduler) embedBatch(ctx context.Context) error {
	since := time.Now().Add(-time.Duration(c.config.Clustering.WindowDays) * 24 * time.Hour)

	rows, err := c.db.QueryContext(ctx, `
		SELECT id, title FROM articles
		WHERE publish_date >= $1 AND title_embedding IS NULL
		ORDER BY publish_date DESC LIMIT $2`,
		since, c.config.Clustering.BatchSize)
	if err != nil {
		return err
	}
	type idTitle struct {
		id    int64
		title string
	}
	var toEmbed []idTitle
	for rows.Next() {
		var it idTitle
		if err := rows.Scan(&it.id, &it.title); err != nil {
			log.Printf("Story-clustering: embed batch scan error: %v", err)
			continue
		}
		toEmbed = append(toEmbed, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, it := range toEmbed {
		emb, err := fetchEmbedding(ctx, c.httpClient, c.config.OLLAMA.URL, c.config.Clustering.EmbedModel, it.title)
		if err != nil {
			log.Printf("Story-clustering: embedding failed for article %d: %v", it.id, err)
			continue
		}
		if _, err := c.db.ExecContext(ctx,
			`UPDATE articles SET title_embedding = $1 WHERE id = $2`,
			pq.Array(emb), it.id,
		); err != nil {
			log.Printf("Story-clustering: failed to store embedding for article %d: %v", it.id, err)
		}
	}
	return nil
}

// clusterBatch assigns every embedded-but-unclustered article in the window
// to the most similar existing cluster seed, or seeds a new cluster.
func (c *ClusteringScheduler) clusterBatch(ctx context.Context) error {
	since := time.Now().Add(-time.Duration(c.config.Clustering.WindowDays) * 24 * time.Hour)

	seedRows, err := c.db.QueryContext(ctx, `
		SELECT id, title_embedding FROM articles
		WHERE publish_date >= $1 AND story_cluster_id = id`,
		since)
	if err != nil {
		return err
	}
	seeds := map[int64][]float32{}
	for seedRows.Next() {
		var id int64
		var emb []float32
		if err := seedRows.Scan(&id, pq.Array(&emb)); err != nil {
			log.Printf("Story-clustering: seed scan error: %v", err)
			continue
		}
		seeds[id] = emb
	}
	seedRows.Close()
	if err := seedRows.Err(); err != nil {
		return err
	}

	unclusteredRows, err := c.db.QueryContext(ctx, `
		SELECT id, title_embedding FROM articles
		WHERE publish_date >= $1 AND story_cluster_id IS NULL AND title_embedding IS NOT NULL`,
		since)
	if err != nil {
		return err
	}
	type idEmbedding struct {
		id  int64
		emb []float32
	}
	var toCluster []idEmbedding
	for unclusteredRows.Next() {
		var it idEmbedding
		if err := unclusteredRows.Scan(&it.id, pq.Array(&it.emb)); err != nil {
			log.Printf("Story-clustering: unclustered scan error: %v", err)
			continue
		}
		toCluster = append(toCluster, it)
	}
	unclusteredRows.Close()
	if err := unclusteredRows.Err(); err != nil {
		return err
	}

	for _, it := range toCluster {
		clusterID, ok := assignCluster(it.emb, seeds, c.config.Clustering.SimilarityThreshold)
		if !ok {
			clusterID = it.id
			seeds[it.id] = it.emb // available as a seed for the rest of this batch
		}
		if _, err := c.db.ExecContext(ctx,
			`UPDATE articles SET story_cluster_id = $1 WHERE id = $2`,
			clusterID, it.id,
		); err != nil {
			log.Printf("Story-clustering: failed to store cluster assignment for article %d: %v", it.id, err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run the idle-gating test to verify it passes**

Run: `go test ./... -run TestIsIdleFromStats -v`
Expected: PASS.

No dedicated test for `embedBatch`/`clusterBatch`/`runCycle` themselves — same justified
gap as `getArticlesDigest` in the original digest plan: no DB-test harness exists in this
repo, and the decision logic each of them relies on (`assignCluster`, `cosineSimilarity`,
`isIdleFromStats`) is already unit-tested in isolation. Verify these three by building
successfully; live verification happens in Task 7.

- [ ] **Step 5: Wire into `main.go`**

In `main.go`, after the summarization scheduler is created (currently line 54:
`summarizationScheduler := NewSummarizationScheduler(db, cfg, metrics)`), add:

```go
	// Create story-clustering scheduler (backs the digest feature's "important" bucket)
	clusteringScheduler := NewClusteringScheduler(db, cfg, summarizationScheduler)
```

Change `wg.Add(3)` (currently line 72) to `wg.Add(4)`.

After the RSS monitor's start goroutine (currently lines 82-86), add:

```go
	// Start story-clustering scheduler
	go func() {
		defer wg.Done()
		clusteringScheduler.Start(ctx)
	}()
```

- [ ] **Step 6: Build and run the full test suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: build succeeds, `go vet` clean, all tests pass.

- [ ] **Step 7: Commit**

```bash
git add clustering_scheduler.go clustering_scheduler_test.go main.go
git commit -m "feat: add ClusteringScheduler, idle-gated on summarization activity"
```

---

## Task 6: Rewrite `buildDigestQuery` to use `story_cluster_id`

**Files:**
- Modify: `/home/pure/Documents/github/Information-Broker/digest.go` (replace
  `normTitleSQL`/the normalized-title `GROUP BY` version of `buildDigestQuery` entirely)
- Modify: `/home/pure/Documents/github/Information-Broker/digest_test.go` (replace
  `TestBuildDigestQuery` and remove `TestNormTitleSQLPatterns` — `normTitleSQL` is deleted)

**Interfaces:**
- Consumes: nothing new
- Produces: `buildDigestQuery(since time.Time) (string, []interface{})` — same signature as
  before, new SQL body. `normTitleSQL` is deleted (no longer used anywhere).

- [ ] **Step 1: Write the failing test**

Replace `TestBuildDigestQuery` and delete `TestNormTitleSQLPatterns` in `digest_test.go`:

```go
func TestBuildDigestQuery(t *testing.T) {
	since := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	q, args := buildDigestQuery(since)

	if strings.Contains(q, "regexp_replace") || strings.Contains(q, "normTitleSQL") {
		t.Fatalf("query must not use the old title-normalization GROUP BY: %s", q)
	}
	if !strings.Contains(q, "GROUP BY story_cluster_id") {
		t.Fatalf("missing GROUP BY on story_cluster_id: %s", q)
	}
	if !strings.Contains(q, "story_cluster_id IS NOT NULL") {
		t.Fatalf("subquery must exclude unclustered rows: %s", q)
	}
	if !strings.Contains(q, "COUNT(DISTINCT feed_url)") {
		t.Fatalf("missing distinct-feed count: %s", q)
	}
	if !strings.Contains(q, "ORDER BY cross_feed_count DESC, a.publish_date DESC") {
		t.Fatalf("missing ORDER BY: %s", q)
	}
	if len(args) != 1 || args[0] != since {
		t.Fatalf("expected single since arg (bound twice via $1), got %v", args)
	}
}
```

(Delete the `TestNormTitleSQLPatterns` function entirely — `normTitleSQL` no longer exists.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestBuildDigestQuery -v`
Expected: FAIL — the old query still contains `regexp_replace`.

- [ ] **Step 3: Rewrite `buildDigestQuery` and delete `normTitleSQL`**

In `digest.go`, delete the entire `normTitleSQL` function and replace `buildDigestQuery` with:

```go
// buildDigestQuery returns the SQL and args for the cross-feed importance
// heuristic: for every article published since `since`, count how many
// *other* feeds have an article in the same precomputed story cluster
// (story_cluster_id, assigned by ClusteringScheduler via title embedding
// similarity) in the same window. This replaces two earlier live-computed
// approaches: a pg_trgm self-join (timed out past ~2k rows -- trigram GIN
// indexes don't accelerate column-to-column joins) and a GROUP BY on
// normalized title (fast, but too strict -- outlets reword headlines for
// the same event, so daily/weekly digests rarely populated "important").
// Precomputing via embeddings (see clustering_scheduler.go) catches those
// reworded-but-same-story cases; this query is now a plain indexed GROUP BY.
//
// ponytail: story_cluster_id is NULL for articles the clustering job hasn't
// reached yet (its own ticker interval, gated further by summarization
// activity) -- they're excluded from cross_feed_count here but still show
// up in the digest's "everything else" bucket via the outer WHERE clause,
// and get a cluster on the next cycle.
func buildDigestQuery(since time.Time) (string, []interface{}) {
	query := `SELECT a.id, a.title, a.url, a.summary, a.full_content, a.publish_date,
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
		ORDER BY cross_feed_count DESC, a.publish_date DESC`
	return query, []interface{}{since}
}
```

- [ ] **Step 4: Run the full test suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all PASS. `TestSplitImportant`, `TestDigestWindowOrDefault`, and everything in
`embeddings_test.go`/`clustering_scheduler_test.go` from Tasks 4-5 are unaffected by this
change and should still be green.

- [ ] **Step 5: Commit**

```bash
git add digest.go digest_test.go
git commit -m "feat: rewrite buildDigestQuery to GROUP BY precomputed story_cluster_id"
```

---

## Task 7: Live verification — tune the similarity threshold, end-to-end check

**Files:** none (verification only, matching this session's established practice of
spot-checking live behavior before considering a heuristic threshold final)

- [ ] **Step 1: Confirm the migration applies cleanly**

Deploy is out of scope for this pass per the Global Constraints, but if/when deployed:
`ssh smellyfeet-host 'cd ~/Information-Broker && git pull && docker compose up -d --build rss-monitor'`,
then `docker exec information-broker-postgres psql -U postgres -d information_broker -c "\d articles"`
to confirm `title_embedding` and `story_cluster_id` exist.

- [ ] **Step 2: Spot-check the similarity threshold against real title pairs**

Pick 4-6 known pairs from the live corpus: at least one true match (two feeds' titles for the
same real event, worded differently -- e.g. search recent `articles` for a story you know ran
on multiple feeds) and at least one true non-match (two unrelated headlines). Embed each via
`curl http://localhost:11434/api/embeddings -d '{"model":"nomic-embed-text","prompt":"<title>"}'`,
then compute cosine similarity by hand or via a one-off script for each pair. Confirm 0.85
separates matches from non-matches with some margin; if it doesn't, adjust
`CLUSTERING_SIMILARITY_THRESHOLD` and re-check before deploying.

- [ ] **Step 3: After deploying, verify daily/weekly/monthly digest views**

Once deployed and the scheduler has had a few cycles to run (15-minute interval — allow at
least 30-45 minutes, or reduce `CLUSTERING_INTERVAL` temporarily for faster verification), hit
`/digest?range=daily`, `?range=weekly`, `?range=monthly` and confirm "important" now populates
for daily/weekly where it previously didn't (per this session's live findings: 0 articles
matched at daily/weekly under both prior approaches).

- [ ] **Step 4: Confirm idle-gating actually skips cycles during active summarization**

Check the app logs (`docker logs information-broker-app` or equivalent) around a period of
active RSS ingestion for the "Story-clustering: summarization active, skipping this cycle"
log line, confirming the gate is functioning, not just present in code.

No commit for this task (verification only) — but note any threshold adjustment made in Step 2
requires committing a `CLUSTERING_SIMILARITY_THRESHOLD` env var change wherever deploy config
lives (`deploy/.env` equivalent for Information-Broker, or the compose file's environment
block), not a code change.

<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Feature Spec: Explicit Entity Document ID, German Stem Prefix FTS, and In-Memory Vector Cache

## 1. Problem & Goal
Three remaining technical items from `data/todos.md` improve schema robustness, German full-text recall, and multi-turn / REST vector search latency:
1. **Explicit `entities.document_id`:** Previously, entities linked to their defining document only by `documents.title = entities.id`, which breaks when a note defines a custom frontmatter `name:` different from its filename.
2. **German Stem / Prefix Search in FTS5:** Inflected German verbs/nouns (`"konfiguriere"` vs `"Konfiguration"`, `"verschlüssele"` vs `"Verschlüsselung"`) do not match exact FTS5 tokens unless prefix wildcards (`"konfigurier"*`) are included alongside the exact token.
3. **In-Memory Vector Cache:** `SQLiteVectors.Search` decoded all `section_embeddings` blobs from SQLite on every query. In `md-api` and `md-ai chat`, caching decoded float32 vectors in memory (invalidated automatically when the embedding row count or latest `created_at` timestamp changes) eliminates repeated SQLite scans and BLOB decoding.

## 2. Scope & Acceptance Criteria

### 2.1 Explicit `document_id` on `entities` (`internal/repository/sqlite.go`, `internal/ai/hybrid/entity.go`, `internal/ai/hybrid/graph.go`)
*   `migrate` adds `document_id TEXT` to `entities` (including `ALTER TABLE entities ADD COLUMN document_id TEXT` for existing databases, ignoring duplicate-column errors) and backfills `document_id` from `documents` where `documents.title = entities.id`.
*   `UpsertDocument` writes `doc.ID` into `entities.document_id` when `doc.IsEntity` is true.
*   `SQLiteEntities` and `SQLiteGraph` resolve an entity's defining document via `COALESCE(NULLIF(e.document_id, ''), (SELECT d.id FROM documents d WHERE d.title = e.id LIMIT 1))`, preserving compatibility with older databases and forward-referenced stub entities.

### 2.2 Stem / Prefix FTS Matching (`internal/ai/hybrid/query.go`)
*   `ftsMatch` emits `"term"` for short tokens (< 5 runes) and `("term" OR "stem"*)` for longer words (where `stem` strips common German/English inflectional suffixes such as `-iere`, `-ieren`, `-ung`, `-ungen`, `-keit`, `-en`, `-er`, `-es`, `-em` when at least 5 runes remain, or uses `"term"*` directly).

### 2.3 In-Memory Embedding Cache (`internal/ai/hybrid/vector.go`)
*   `SQLiteVectors` caches decoded `[]cachedEmbedding{sectionID, vec}` per `(model, dimensions)` along with `(count, maxCreatedAt)` from `section_embeddings`.
*   On each `Search`, a lightweight single-row query checks `SELECT count(*), COALESCE(max(created_at), '') FROM section_embeddings WHERE model = ?`. If unchanged, `Search` computes cosine similarity directly over the in-memory slice without re-reading or decoding BLOBs.

## 3. Tasks
*   [x] Task 1: Add `entities.document_id` column, migration backfill, write path, and hybrid entity/graph lookup.
*   [x] Task 2: Add stem-aware prefix matching in `ftsMatch`.
*   [x] Task 3: Add in-memory vector cache with automatic invalidation in `SQLiteVectors`.
*   [x] Task 4: Add unit tests for all three enhancements and update `data/todos.md`.

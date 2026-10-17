<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Architecture Plan: Vault Re-Index without Embedding Loss, Case-Insensitive Graph Resolution & CLI Retrieval Ergonomics

**Issue:** [#18](https://github.com/Zheng-Bote/md_indexer-searcher/issues/18)  
**Spec:** [spec.md](file:///run/media/zb_bamboo/home/zb_bamboo/DEV/__NEW__/Go/md_indexer/specs/features/vault-reindex-and-cli-ergonomics/spec.md)

## 1. Architectural Alignment (`md-indexer.sdd`)

- **Read-Only Source of Truth:** Markdown files in the vault remain strictly immutable (`FileSystemAccess: Read-Only`). Force re-indexing only reads `.md` files and updates the materialized SQLite store (`knowledge.db`).
- **CQRS-Light Separation:**
  - Write Path (`cmd/indexer` $\rightarrow$ `internal/service` $\rightarrow$ `internal/repository`): Handles `-force` re-parsing and stale/skipped document removal in `knowledge.db`.
  - Read Path (`cmd/md-ai` $\rightarrow$ `internal/ai/assistant` $\rightarrow$ `internal/ai/hybrid` / `internal/graph`): Queries `knowledge.db` in read-only mode (`--retrieve-only`, `md-ai gaps`, `--stream`).
- **CGO-Free SQLite:** Uses `modernc.org/sqlite` with standard SQLite `COLLATE NOCASE` comparisons.
- **Clean Architecture & Dependency Injection:** `StreamingLLMProvider` extends `provider.LLMProvider` via an optional interface so existing `LLMProvider` implementations and mocks remain 100% compatible.
- **Code Standards:** Apache-2.0 SPDX headers and English comments/docstrings in all modified and new files.

---

## 2. Technical Design

### A. Force Re-Index & Stale Document Cleanup (`internal/repository/sqlite.go`, `internal/service/indexer.go`, `cmd/indexer/main.go`)

1. **Repository Helper (`ListDocumentPaths`):**
   - Add `ListDocumentPaths(ctx context.Context) ([]string, error)` on `SQLiteRepository` returning `SELECT path FROM documents`.
2. **Indexer Service (`WithForce(bool)` & Stale Cleanup):**
   - Add `force bool` field and `WithForce(force bool) *IndexerService` builder method on `IndexerService`.
   - In `indexFile(ctx, path)`: only skip `existingDoc.Hash == doc.Hash` when `!s.force`.
   - In `IndexAll(ctx)`:
     - Track the set of visited valid `.md` paths on disk (`seenPaths map[string]bool`).
     - After walking `s.dir`, query `s.repo.ListDocumentPaths(ctx)` and call `s.repo.RemoveDocument(ctx, dbPath)` for any indexed path that either matches `ShouldSkipPath(dbPath)` or is not in `seenPaths` (when `dbPath` is within `s.dir`).
   - **Embedding Preservation:** `UpsertDocument` replaces `sections` with deterministic IDs (`fmt.Sprintf("%s#%d", doc.ID, idx)`) and does **not** delete `section_embeddings`. `EmbeddingService.EmbedPending` subsequently checks `(section_id, model, content_hash)` and only embeds sections whose heading or content actually changed.
3. **CLI Flag (`cmd/indexer/main.go`):**
   - Add `-force` boolean flag (`"Force re-parsing all Markdown files and purge skipped/deleted entries without dropping unchanged embeddings"`) and wire `indexerService.WithForce(*force)`.

### B. Case-Insensitive Link & Entity Resolution (`COLLATE NOCASE`)

1. **Hybrid Entity Retriever (`internal/ai/hybrid/entity.go`):**
   - Update `entityMatchQuery` and `entityMatchQueryLegacy`:
     `LEFT JOIN documents d ON d.id = e.document_id OR d.title = e.id COLLATE NOCASE`
   - Update `detectEntityGaps` ADR link check:
     `WHERE l.source_document_id = ? AND (LOWER(l.target_document_id) LIKE 'adr%' OR LOWER(d.path) LIKE '%/adr/%')` with `LEFT JOIN documents d ON d.title = l.target_document_id COLLATE NOCASE`.
2. **Hybrid Graph Retriever (`internal/ai/hybrid/graph.go`):**
   - Update `neighbours` link join:
     `JOIN documents d ON d.title = l.target_document_id COLLATE NOCASE`
   - Update `neighbours` relation join:
     `LEFT JOIN entities es ON es.id = r.source_entity_id COLLATE NOCASE`
     `LEFT JOIN entities et ON et.id = r.target_entity_id COLLATE NOCASE`
     `JOIN documents ds ON ds.id = es.document_id OR ds.title = r.source_entity_id COLLATE NOCASE`
     `JOIN documents dt ON dt.id = et.document_id OR dt.title = r.target_entity_id COLLATE NOCASE`
3. **Classic Retriever & Graph Engine (`internal/ai/retriever/retriever.go`, `internal/graph/engine.go`, `internal/repository/sqlite.go`):**
   - Add `COLLATE NOCASE` to `d.title = l.target_document_id` and `d.title = entities.id` backfill/lookup queries.

### C. Dry-Run Retrieval Mode (`md-ai ask --retrieve-only`)

1. **Model & Assistant (`internal/ai/assistant/assistant.go`):**
   - Add `RetrieveOnly(ctx context.Context, query string) (*model.Answer, error)` on `Assistant`:
     - Classifies intent (`a.classifier.Classify(query)`).
     - Runs `a.retriever.Retrieve(ctx, query, intent)`.
     - Builds budgeted context (`a.builder.Build(retResult, a.cfg.MaxContextTokens)`).
     - Populates `Answer` with `Intent`, `Model: "none (retrieve-only)"`, `Plan: retResult.Plan`, deterministic `Sources: sourcesFromBlocks(budgeted.Blocks)`, and `Gaps: retResult.Gaps`, plus a concise summary in `Answer.Answer` (e.g. token estimate, block count, reasoning chain count, and architecture summary if present).
2. **CLI (`cmd/md-ai/main.go`):**
   - Add `--retrieve-only` flag on `askCmd`. When set, call `asst.RetrieveOnly(ctx, query)` instead of `asst.Ask(ctx, query, nil)`.

### D. Vault-Wide Knowledge Gap Audit (`md-ai gaps`)

1. **Audit Query (`internal/ai/hybrid/entity.go`):**
   - Add `AuditEntityGaps(ctx context.Context, db *sql.DB) ([]model.KnowledgeGap, error)` (and `SQLiteEntities.AuditGaps(ctx)`):
     - Loads all non-`unresolved` entities from `entities` (with `document_id`, `type_id`, `name`, `documents.path`, and `entity_properties`) ordered by `name`.
     - Runs `detectEntityGaps(ctx, matched)` to check for missing `description`, missing `ADR` relation/link (`Service`/`Component`/`Pipeline`), and missing `API` section (`Service`/`Component`).
2. **CLI Subcommand (`cmd/md-ai/main.go`):**
   - Register `gapsCmd` (`Use: "gaps"`, `Short: "Audit the indexed knowledge base for entity documentation gaps"`).
   - Supports `--json` and human-readable grouped output.

### E. Token Streaming (`--stream`)

1. **Provider Layer (`internal/ai/provider/provider.go`, `internal/ai/provider/ollama.go`, `internal/ai/provider/mock.go`):**
   - Define optional interface in `internal/ai/provider/provider.go`:
     ```go
     type StreamingLLMProvider interface {
         LLMProvider
         GenerateStream(ctx context.Context, req *model.GenerateRequest, onToken func(token string)) (*model.GenerateResponse, error)
     }
     ```
   - Implement `GenerateStream` on `OllamaProvider` (`Stream: true` in `ollamaGenerateRequest`, reading newline-delimited JSON chunks via `json.NewDecoder(resp.Body)` until `chunk.Done` or `io.EOF`, invoking `onToken(chunk.Response)` on each non-empty chunk and accumulating full response + token counts).
   - Implement `GenerateStream` on `MockProvider` (invoking `onToken` with the mock response) for deterministic unit tests.
2. **Assistant Layer (`internal/ai/assistant/assistant.go`):**
   - Add `AskStream(ctx context.Context, query string, session *conversation.Session, onPlan func(*model.RetrievalPlanInfo, int), onToken func(string)) (*model.Answer, error)`:
     - Executes retrieval and context building, calls `onPlan(retResult.Plan, len(budgeted.Blocks))` if provided, then uses `StreamingLLMProvider.GenerateStream` (or falls back to `Generate` + single `onToken` call if provider does not implement `StreamingLLMProvider`), and post-processes citations, deterministic sources, and gaps identically to `Ask`.
3. **CLI (`cmd/md-ai/main.go`):**
   - Add `--stream` persistent/command flag for `ask` and `chat`. When `--stream` is set and `--json` is false, print the plan header (`[Plan: <type> | Context blocks: <n>]`), stream tokens directly to `os.Stdout`, and print `Sources:` and `Knowledge Gaps:` afterwards.

---

## 3. Verification Strategy

- `internal/service/indexer_test.go`: Test that `IndexAll` with `WithForce(true)` re-indexes unchanged files, removes previously indexed `_templates_` documents, and preserves `section_embeddings`.
- `internal/ai/hybrid/retriever_test.go`: Test case-insensitive wikilinks and entity relations (`[[encryption service]]` $\rightarrow$ `Encryption Service`), and test `AuditEntityGaps` across multiple entity types.
- `internal/ai/provider/provider_test.go` & `internal/ai/assistant/assistant_test.go`: Test `OllamaProvider.GenerateStream` (via `httptest.Server` streaming NDJSON chunks), `Assistant.RetrieveOnly`, and `Assistant.AskStream`.

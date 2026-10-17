<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Feature Specification: Vault Re-Index without Embedding Loss, Case-Insensitive Graph Resolution & CLI Retrieval Ergonomics

**Issue:** [#18](https://github.com/Zheng-Bote/md_indexer-searcher/issues/18)  
**Status:** Specified (`/speckit.specify`)

## 1. Feature Intent

Improve real-world vault maintenance and local CPU ergonomics for `md-indexer` and `md-ai` when working with large Obsidian/Markdown vaults (e.g. 2,000+ notes):

1. **Force Re-Indexing without Embedding Loss (`md-indexer -force`):** Allow re-parsing all Markdown documents in an existing `knowledge.db` (applying normalized wikilinks, code-block exclusions, `metadata` population, `entities.document_id`, and removing newly skipped `_templates_` / hidden directories) without deleting `knowledge.db` or losing existing `section_embeddings` (`bge-m3`).
2. **Case-Insensitive Link & Entity Resolution (`COLLATE NOCASE`):** Resolve wikilinks (`[[mqtt broker]]` $\rightarrow$ `MQTT Broker`) and entity names case-insensitively in SQLite joins so existing vault links connect even when letter casing differs from note titles.
3. **Dry-Run Retrieval Inspection (`md-ai ask --retrieve-only`):** Execute Intent Classifier, Query Planner, Hybrid Retriever, and Context Builder and output the `RetrievalPlan`, `Sources`, `ReasoningChains`, `ArchitectureContext`, `Gaps`, and estimated token count in `<50 ms` without invoking the LLM on CPU.
4. **Vault-Wide Knowledge Gap Audit (`md-ai gaps`):** Scan all indexed `Service`, `Component`, and `Pipeline` entities in `knowledge.db` deterministically and report missing descriptions, missing ADR links/relations, and missing API sections.
5. **Token Streaming (`md-ai ask --stream` / `md-ai chat --stream`):** Stream generated tokens incrementally from Ollama in text mode while immediately displaying retrieval plan telemetry before generation starts.

## 2. User Scenarios

- **Scenario 1 — Upgrading an Existing `knowledge.db` (`md-indexer -force`):**
  A user has already indexed 2,056 notes and computed `bge-m3` embeddings (taking ~10 minutes on CPU). After parser improvements or template exclusions are deployed, running `md-indexer -vault /path/to/vault -db knowledge.db -force` re-parses all Markdown files in ~2 seconds, purges previously indexed `_templates_` notes, and retains all unchanged `section_embeddings`.
- **Scenario 2 — Case-Insensitive Obsidian Links:**
  A note links to `[[encryption service]]` or declares `uses: ["[[mqtt broker]]"]` while the target documents are titled `Encryption Service` and `MQTT Broker`. Both `SQLiteGraph`, `SQLiteEntities`, and `GraphEngine` resolve the target documents without requiring manual casing fixes in the vault.
- **Scenario 3 — Fast Retrieval Debugging (`md-ai ask --retrieve-only`):**
  While curating notes with `_templates_/entity.md`, the user runs `md-ai ask --retrieve-only "Wie funktioniert die Ingestion Pipeline?"` (or with `--json`) to inspect the query plan, fused snippets, reasoning chains, architecture context, and knowledge gaps in `<50 ms` without waiting 2 minutes for CPU LLM generation.
- **Scenario 4 — Vault Curation Audit (`md-ai gaps`):**
  The user runs `md-ai gaps` (or `md-ai gaps --json`) to get a complete checklist of all indexed `Service`, `Component`, and `Pipeline` entities in `knowledge.db` that lack a `description`, an `ADR` link/relation, or an `API` section.
- **Scenario 5 — Interactive Streaming on CPU (`--stream`):**
  The user runs `md-ai ask --stream "..."` or `md-ai chat --stream`. The CLI immediately prints the retrieval summary (`[Plan: architecture | Sources: 5 | Chains: 2]`) and streams tokens to `stdout` as Ollama generates them, followed by the `Sources` and `Knowledge Gaps` sections.

## 3. Requirements (EARS Syntax)

1. **Force Re-Indexing & Stale Skipped-Path Cleanup:**
   **WHEN** `md-indexer` is invoked with `-force` (or `IndexerService.SetForce(true)` is enabled), **THE SYSTEM SHALL** re-parse and upsert all non-skipped Markdown files regardless of their stored SHA-256 hash, remove any previously indexed documents whose path matches `ShouldSkipPath` (`_templates_`, `_template_`, hidden directories) or no longer exists on disk, and preserve valid rows in `section_embeddings`.
2. **Case-Insensitive Link & Entity Joins:**
   **WHEN** `SQLiteGraph`, `SQLiteEntities`, `SQLiteRetriever` (classic), or `GraphEngine` resolve `links.target_document_id`, `entities.id`, or `entity_relations` endpoints against `documents.title`, **THE SYSTEM SHALL** compare titles case-insensitively using `COLLATE NOCASE`.
3. **Retrieve-Only Dry-Run Mode:**
   **WHEN** `md-ai ask` is invoked with `--retrieve-only`, **THE SYSTEM SHALL** execute classification, query planning, hybrid retrieval, and context building, and output the structured retrieval result (plan, budgeted sources, reasoning chains, architecture summary, and deterministic knowledge gaps) in text or `--json` format without calling `LLMProvider.Generate`.
4. **Vault-Wide Deterministic Gap Scan (`md-ai gaps`):**
   **WHEN** `md-ai gaps` is executed, **THE SYSTEM SHALL** query all indexed entities in `knowledge.db`, evaluate deterministic documentation rules (`missing description`, `no ADR linked` for `Service`/`Component`/`Pipeline`, `no API section documented` for `Service`/`Component`), and print the resulting `[]model.KnowledgeGap` list in text or `--json` format.
5. **Streaming LLM Generation (`--stream`):**
   **WHERE** `--stream` is enabled on `md-ai ask` or `md-ai chat` (and `--json` is false), **THE SYSTEM SHALL** print a concise retrieval plan header before generation, stream tokens incrementally via `StreamingLLMProvider.GenerateStream` (`stream: true` on Ollama `/api/generate`), and print `Sources` and `Knowledge Gaps` once generation completes.

## 4. Scope & Affected Components

- `cmd/indexer/main.go` & `internal/service/indexer.go`: Add `-force` CLI flag, `SetForce(bool)` on `IndexerService`, and cleanup of previously indexed documents that are now skipped or deleted.
- `internal/ai/hybrid/entity.go`, `internal/ai/hybrid/graph.go`, `internal/ai/retriever/retriever.go`, `internal/graph/engine.go`, `internal/repository/sqlite.go`: Add `COLLATE NOCASE` to title-based joins and entity document backfill.
- `internal/ai/provider/provider.go` & `internal/ai/provider/ollama.go`: Add optional `StreamingLLMProvider` interface (`GenerateStream(ctx, req, onToken func(string)) (*GenerateResponse, error)`) and NDJSON streaming support in `OllamaProvider`.
- `internal/ai/assistant/assistant.go`: Add `RetrieveOnly(ctx, query)` and `AskStream(ctx, query, session, onToken)` methods on `Assistant`.
- `internal/ai/hybrid/entity.go`: Expose `AuditAllEntityGaps(ctx, db)` helper reusing the deterministic entity gap rules across all entities in `knowledge.db`.
- `cmd/md-ai/main.go`: Add `--retrieve-only` and `--stream` flags, and add the `md-ai gaps` subcommand.

## 5. SpecDD Architecture Alignment (Drift Control)

- [x] **Architecture:** The layered architecture is maintained (`cmd` $\rightarrow$ `service`/`assistant` $\rightarrow$ `hybrid`/`repository` $\rightarrow$ SQLite).
- [x] **Security:** Markdown vault files remain strictly read-only (`md-indexer.sdd`); secret redaction in `contextbuilder` applies equally to `--retrieve-only` and `--stream`.
- [x] **Data Model:** Core SQLite schemas (`knowledge.db`, pure Go `modernc.org/sqlite`, no CGO) remain intact and backwards-compatible.
- [x] **Standards:** SPDX headers (`Apache-2.0`) and English source documentation are maintained.

## 6. Acceptance Criteria

- [x] `md-indexer -force` re-indexes all vault notes and cleans up `_templates_` or deleted entries in `knowledge.db` while keeping existing `section_embeddings` intact.
- [x] Wikilinks and entity relations with differing letter case (e.g. `[[encryption service]]` $\rightarrow$ `Encryption Service`) resolve to target documents in `SQLiteGraph`, `SQLiteEntities`, and `GraphEngine`.
- [x] `md-ai ask --retrieve-only` outputs plan, budgeted sources, reasoning chains, and gaps in both text and `--json` mode without calling Ollama.
- [x] `md-ai gaps` lists all deterministic entity documentation gaps across `knowledge.db` in both text and `--json` mode.
- [x] `md-ai ask --stream` and `md-ai chat --stream` stream tokens live from `LLMProvider` and append sources and gaps after completion.
- [x] Unit tests cover all new flags, queries, and streaming paths (`go test ./...`).
- [x] `CHANGELOG.md` and `README.md` are up-to-date.

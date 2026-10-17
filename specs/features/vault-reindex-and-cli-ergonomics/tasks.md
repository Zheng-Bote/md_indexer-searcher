<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Task Breakdown: Vault Re-Index without Embedding Loss, Case-Insensitive Graph Resolution & CLI Retrieval Ergonomics

**Issue:** [#18](https://github.com/Zheng-Bote/md_indexer-searcher/issues/18)  
**Spec:** [spec.md](file:///run/media/zb_bamboo/home/zb_bamboo/DEV/__NEW__/Go/md_indexer/specs/features/vault-reindex-and-cli-ergonomics/spec.md) · **Plan:** [plan.md](file:///run/media/zb_bamboo/home/zb_bamboo/DEV/__NEW__/Go/md_indexer/specs/features/vault-reindex-and-cli-ergonomics/plan.md)

## Package & Implementation Tasks

### Task 1: Force Re-Indexing & Stale Skipped-Path Cleanup (`internal/repository`, `internal/service`, `cmd/indexer`)
- [x] Add `ListDocumentPaths(ctx context.Context) ([]string, error)` in `internal/repository/sqlite.go`.
- [x] Add `WithForce(force bool) *IndexerService` in `internal/service/indexer.go` so `indexFile` bypasses the SHA-256 unchanged check when `force` is true, and `IndexAll` removes stale/skipped paths (`_templates_`, `.obsidian`, or removed files) via `RemoveDocument` while preserving `section_embeddings` for existing sections.
- [x] Add `-force` CLI flag in `cmd/indexer/main.go`.
- [x] Add unit tests in `internal/service/indexer_test.go` verifying `-force` re-parses unchanged files, purges previously indexed `_templates_` documents, and retains `section_embeddings`.

### Task 2: Case-Insensitive Link & Entity Resolution (`internal/ai/hybrid`, `internal/ai/retriever`, `internal/graph`, `internal/repository`)
- [x] Update SQLite queries in `internal/ai/hybrid/entity.go`, `internal/ai/hybrid/graph.go`, `internal/ai/retriever/retriever.go`, `internal/graph/engine.go`, and `internal/repository/sqlite.go` to join `documents.title` and `entities.id` with `COLLATE NOCASE`.
- [x] Add unit tests in `internal/ai/hybrid/retriever_test.go` verifying case-insensitive wikilink and entity-relation traversal (`[[encryption service]]` $\rightarrow$ `Encryption Service`).

### Task 3: Vault-Wide Entity Documentation Gap Audit (`internal/ai/hybrid`, `cmd/md-ai`)
- [x] Implement `AuditEntityGaps(ctx context.Context, db *sql.DB) ([]model.KnowledgeGap, error)` in `internal/ai/hybrid/entity.go` scanning all non-`unresolved` entities in `knowledge.db` for missing `description`, missing `ADR` reference (`Service`/`Component`/`Pipeline`), and missing `API` section (`Service`/`Component`).
- [x] Add `gaps` subcommand (`md-ai gaps` and `md-ai gaps --json`) in `cmd/md-ai/main.go`.
- [x] Add unit test `TestAuditEntityGaps` in `internal/ai/hybrid/retriever_test.go`.

### Task 4: Dry-Run Retrieval Mode (`internal/ai/assistant`, `cmd/md-ai`)
- [x] Implement `RetrieveOnly(ctx context.Context, query string) (*model.Answer, error)` in `internal/ai/assistant/assistant.go` returning `Plan`, deterministic `Sources`, `Gaps`, and a retrieval summary (token estimate, block count, reasoning chains, architecture summary) without calling `LLMProvider.Generate`.
- [x] Add `--retrieve-only` flag to `md-ai ask` in `cmd/md-ai/main.go`.
- [x] Add unit test `TestAssistant_RetrieveOnlyAndAskStream` in `internal/ai/assistant/assistant_test.go`.

### Task 5: Token Streaming Support (`internal/ai/provider`, `internal/ai/assistant`, `cmd/md-ai`)
- [x] Define `StreamingLLMProvider` interface in `internal/ai/provider/provider.go` and implement `GenerateStream` on `OllamaProvider` (`internal/ai/provider/ollama.go`) and `MockProvider` (`internal/ai/provider/mock.go`).
- [x] Implement `AskStream` in `internal/ai/assistant/assistant.go`.
- [x] Add `--stream` flag to `md-ai ask` and `md-ai chat` in `cmd/md-ai/main.go`.
- [x] Add unit tests in `internal/ai/provider/provider_test.go` and `internal/ai/assistant/assistant_test.go`.

### Task 6: Verification, Documentation & Converge (`/speckit.converge`)
- [x] Run `go test ./...` and `gofmt -l cmd/ internal/ examples/`.
- [x] Update `README.md`, `docs/Hybrid_GraphRAG_DE.md`, and `CHANGELOG.md`.
- [x] Verify SpecDD convergence against `md-indexer.sdd` and `specs/features/vault-reindex-and-cli-ergonomics/`.

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Hybrid GraphRAG Retriever (`internal/ai/hybrid`, `#14`, `#15`, `#17`):**
  - Added query-type planner (`fact`, `entity`, `impact`, `architecture`, `exploration`) and German intent classification patterns.
  - Added section-level FTS5 virtual table (`fts_sections`) with automatic migration backfill, stopword filtering (DE/EN), and column weights (`title: 1.0`, `heading: 0.8`, `tags: 0.7`, `content: 0.5`, `metadata: 0.4`).
  - Added local Ollama vector embeddings (`internal/embedding`, `section_embeddings` table, incremental embedding in `md-indexer -embed-model`, brute-force cosine similarity in `VectorRetriever` with default `bge-m3`).
  - Added weighted BFS graph expansion over `entity_relations` (bidirectional), wikilinks, and shared tags (`2–10` notes) with explicit multi-hop `ReasoningChain`s.
  - Added plan-aware Reciprocal Rank Fusion (`Weights.ForPlan`) boosting graph contributions on `impact` and `architecture` plans.
  - Added `ArchitectureContext` synthesis for architecture questions, retrieval-time structured `KnowledgeGap{Entity, Reason}` detection, deterministic source attribution from budgeted context blocks, and exposed `Answer.Plan` debug info.
- **Context Compression & Safety (`internal/ai/contextbuilder`):**
  - Added Jaccard near-duplicate section filtering ($\ge 0.80$), score-preserving priority sorting, conservative `~3 bytes/token` estimation, and automatic secret redaction (`*_API_KEY`, `*_SECRET`, `*_TOKEN`, `sk-...`, Bearer tokens).
- **Configuration & Documentation:**
  - Added YAML configuration loading (`~/.config/md-indexer/ai.yaml`, `./md-ai.yaml`, `--config` / `MD_AI_CONFIG`), full `MD_AI_*` environment variable support in `md-api`, `md-ai.example.yaml`, and `docs/Hybrid_GraphRAG_DE.md`.
- **Indexer Data Quality & Retrieval Performance:**
  - Added explicit `entities.document_id` column (with automatic migration backfill and fallback to `documents.title`), case-insensitive title and entity resolution (`COLLATE NOCASE`), German stem/prefix matching in FTS5 (`stemTerm`), and an automatically invalidated in-memory vector cache in `SQLiteVectors`.
  - Added `-force` flag to `md-indexer` (`IndexerService.WithForce`) to re-parse all Markdown files and purge stale/excluded paths (`_templates_`, `.obsidian`) from `knowledge.db` while preserving existing `section_embeddings`.
  - Normalized Obsidian wikilink targets (`[[Note|Alias]]`, `[[Note#Heading]]`) in inline links and frontmatter relations, excluded code blocks and inline code spans from link/tag extraction, skipped `_templates_` and hidden folders during indexing/watching, populated the `metadata` table, and added full cascade cleanup in `RemoveDocument`.
- **CLI Retrieval Ergonomics (`cmd/md-ai`, `#18`):**
  - Added `md-ai ask --retrieve-only` dry-run mode to inspect query plans, budgeted context sources, reasoning chains, architecture summaries, and knowledge gaps in `<50 ms` without calling Ollama.
  - Added `md-ai gaps` subcommand (`AuditEntityGaps`) to audit all indexed entities in `knowledge.db` for missing descriptions, missing ADR links/relations, and missing API sections.
  - Added `--stream` token streaming (`StreamingLLMProvider` / `OllamaProvider.GenerateStream`) for `md-ai ask` and `md-ai chat`.

- **Desktop App (`apps/desktop`):**
  - Added a Tauri v2 + Angular 22 + Spartan-ng desktop client with vault selection (native folder dialog), generation/embedding model selection, and grounded Q&A rendering (sanitized Markdown, sources, knowledge gaps).
  - The app runs `md-indexer` and `md-api` as sidecars on `127.0.0.1` with one database per vault; changing the vault or a model restarts them. Settings are restored on start.
  - Added `make desktop-sidecars`, `make desktop-dev` and `make desktop`.
- **Model Listing (`md-api`):** Added `GET /api/ai/models` (`provider.ModelLister`, `OllamaProvider.ListModels`) returning installed Ollama models and the active models (`501` without `-ai`, `502` when Ollama is unreachable).
- **Bind Address:** Added `-host` flag to `md-indexer` and `md-api` (e.g. `127.0.0.1`); the default still listens on all interfaces.

### Changed
- **SQLite Concurrency:** File databases are opened with `journal_mode=WAL` and `busy_timeout=5000` so `md-indexer` and `md-api` can share one database. WAL mode persists in the database file (`-wal`/`-shm` files appear next to it).
- **Ollama & Model Defaults (`#12`, `#13`, `#16`):**
  - Aligned classic and hybrid retrievers with the real indexer SQLite schema (`entity_properties`, `documents.title` link resolution).
  - Changed default LLM model to `qwen3:14b`, lowered default `max_context_tokens` to `4000` for responsive local CPU inference, configured dynamic `num_ctx` window sizing, disabled `think` mode for non-thinking calls, and required answering in the language of the user's question.

## [v0.2.0] - 2026-10-11

### Added
- **Local Grounded AI Assistant & RAG Layer (`cmd/md-ai`):**
  - Added new standalone binary `cmd/md-ai` providing grounded question answering over Markdown notes indexed in SQLite `knowledge.db`.
  - Added native Ollama LLM provider (`internal/ai/provider`) defaulting to `gemma3:1b` with configurable temperature, token limits, and deterministic mock providers for testing.
  - Implemented Intent Classifier (`internal/ai/classifier`) supporting `Search`, `Entity`, `Relationship`, `ADR`, `Impact`, `Summary`, `Compare`, and `KnowledgeGap`.
  - Implemented multi-strategy Knowledge Retriever (`internal/ai/retriever`) combining FTS5 BM25 search, entity lookups, entity relations, and recursive SQLite BFS graph neighborhood queries (`graph_depth: 3`).
  - Added Context Builder (`internal/ai/contextbuilder`) enforcing strict token budgeting (default 16,000 tokens) with priority ordering (Entity > Doc > ADR > Graph > FTS).
  - Enforced strict anti-hallucination prompt templates (`internal/ai/prompt`), inline citation extraction (`internal/ai/citations`), and knowledge gap detection (`internal/ai/gaps`).
  - Added interactive multi-turn chat REPL session manager (`internal/ai/conversation`).
  - Integrated optional AI Q&A endpoint into `md-api` (`POST /api/ai/ask`) enabled via `-ai` flag or `MD_API_ENABLE_AI=true`.
  - Provided comprehensive in-memory test coverage (>81%–100%) across all `internal/ai/...` subpackages while leaving existing components (`cmd/md-indexer`, `cmd/md-search`, `cmd/md-api`) untouched.

- **Comprehensive Test Suite & In-Memory SQLite Coverage >80% (#3):**
  - Established in-memory SQLite (`:memory:`) test architecture for repository, search repo, graph engine, and search engine facade.
  - Added recursive CTE graph traversal tests covering `DependencyTree`, `ImpactAnalysis`, and `ShortestPath` (BFS).
  - Added HTTP API integration tests using `net/http/httptest` covering all REST routes (`/health`, `/api/search`, `/api/documents`, `/api/entities`, `/api/backlinks`, `/api/impact`, `/api/graph`).
  - Added unit tests for `IndexerService` (crawl, hash comparison, update, removal) and `Watcher` lifecycle.
  - Exceeded 80% statement test coverage across all core `internal` packages (`api/rest` 93.5%, `graph` 92.6%, `service` 90.5%, `parser` 88.4%, `search` 85.0%, `repository` 83.3%).
  - Reconciled Apache-2.0 SPDX licensing headers across all source files.
- **Comprehensive Section Content Extraction in AST Walker (#2):**
  - Implemented dynamic `Overview` section initialization for introductory lead content appearing before the first heading.
  - Added full line extraction for fenced code blocks (`ast.FencedCodeBlock`), indented code blocks (`ast.CodeBlock`), and list items (`ast.TextBlock`).
  - Unified section and document content aggregation with real newlines for improved SQLite FTS5 search indexing.
  - Added unit tests covering lead sections, code blocks, lists, and wikilinks inside list items.
- **Robust Entity Extraction & Relation Linking (#1):**
  - Enhanced AST parser to extract inline `[[Wikilinks]]` and `#hashtags` anywhere in Markdown body text (paragraphs, headings, list items).
  - Automatically mapped inline Wikilinks to `LINKS_TO` entity relations when the document is an Entity.
  - Implemented forward reference resilience in SQLite `UpsertDocument` with `unresolved` stub entities, ensuring relational foreign keys are satisfied without race conditions or insertion failures.
  - Persisted document tags into `document_tags` relational table.
  - Added table-driven parser tests and in-memory SQLite repository tests.
  - Added Apache-2.0 SPDX headers to parser and repository modules.

## [v0.1.0] - 2026-10-08

### Added
- **Core Architecture:** Etablished Clean Architecture with a strict CQRS separation (Write-path via `md-indexer`, Read-path via `md-search` and `md-api`).
- **Markdown Indexing:** `md-indexer` uses a file watcher (`fsnotify`) and SHA256 hashing for incremental indexing of Markdown files.
- **SQLite & FTS5:** Integrated SQLite as a disposable, materialized view. Full-Text Search (FTS5) using BM25 ranking.
- **Entity Extraction:** Parser automatically identifies entities from Frontmatter (`type` keyword) and parses list properties (e.g., `uses`, `affects`) into graph relations.
- **Section Content Extraction:** The AST parser now aggregates the full text of each section, enabling deep content search via FTS5.
- **Recursive Graph Engine:** Implemented `WITH RECURSIVE` Common Table Expressions (CTEs) in SQLite for fast, deep graph traversals (`ImpactAnalysis`, `DependencyTree`) without N+1 query problems in Go.
- **CLI & REST API:** Fully functional CLI Explorer (`md-search`) and REST API (`md-api`) exposing search, document details, entity relations, and impact analysis.
- **Agentic Workflow:** Established `SpecDD` and GitHub Spec-Kit conventions in `docs/` for safe, iterative AI-driven feature development.

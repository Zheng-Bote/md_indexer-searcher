# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Local Grounded AI Assistant & RAG Layer (`cmd/md-ai`):**
  - Added new standalone binary `cmd/md-ai` providing grounded question answering over Markdown notes indexed in SQLite `knowledge.db`.
  - Added native Ollama LLM provider (`internal/ai/provider`) defaulting to `gemma3:1b` with configurable temperature, token limits, and deterministic mock providers for testing.
  - Implemented Intent Classifier (`internal/ai/classifier`) supporting `Search`, `Entity`, `Relationship`, `ADR`, `Impact`, `Summary`, `Compare`, and `KnowledgeGap`.
  - Implemented multi-strategy Knowledge Retriever (`internal/ai/retriever`) combining FTS5 BM25 search, entity lookups, entity relations, and recursive SQLite BFS graph neighborhood queries (`graph_depth: 3`).
  - Added Context Builder (`internal/ai/contextbuilder`) enforcing strict token budgeting (default 16,000 tokens) with priority ordering (Entity > Doc > ADR > Graph > FTS).
  - Enforced strict anti-hallucination prompt templates (`internal/ai/prompt`), inline citation extraction (`internal/ai/citations`), and knowledge gap detection (`internal/ai/gaps`).
  - Added interactive multi-turn chat REPL session manager (`internal/ai/conversation`).
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

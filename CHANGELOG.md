# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

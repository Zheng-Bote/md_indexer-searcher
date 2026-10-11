# Feature Specification: Comprehensive Test Suite & In-Memory SQLite Coverage (>80%)

**Issue:** [#3](https://github.com/Zheng-Bote/md_indexer-searcher/issues/3)  
**Status:** In Progress (Specify / Plan Phase)

## 1. Feature Intent

Establish a robust and comprehensive automated test suite utilizing in-memory SQLite instances to achieve >80% statement test coverage across all `internal` packages (`internal/repository`, `internal/graph`, `internal/search`, `internal/api/rest`, `internal/service`).

Tests must execute without external dependencies or CGO, running cleanly in both local and CI environments.

## 2. Requirements (EARS Syntax)

1. **Repository Test Suite:** The repository test suite shall validate all CRUD operations, full-text search (FTS5 BM25 queries), document removal, metadata handling, and entity property lookups using in-memory SQLite (`:memory:`).
2. **Graph Engine Test Suite:** The graph engine test suite shall verify recursive CTE queries including `DirectRelations`, `ReverseRelations`, `DependencyTree` (forward traversal), `ImpactAnalysis` (reverse traversal), and `ShortestPath` (BFS) using controlled graph fixture data.
3. **Search Engine Test Suite:** The search engine test suite shall test search coordination, backlinks resolution, document retrieval, and entity query orchestration against in-memory repository doubles.
4. **REST API Test Suite:** The API test suite shall test all HTTP routes (`/health`, `/api/search`, `/api/documents/{name}`, `/api/entities/{name}`, `/api/backlinks/{name}`, `/api/impact/{name}`, `/api/graph/{name}`) using `httptest.NewRecorder`.
5. **Coverage Threshold:** The test suite shall achieve at least 80% total statement coverage across `internal` core packages.
6. **Code Standards & Drift Alignment:** All source files shall include Apache-2.0 SPDX identifiers and English comments according to `md-indexer.sdd`.

## 3. Scope & Affected Components

* `internal/repository/sqlite_test.go`: Expand with FTS5 search, removal, and search repo tests.
* `internal/graph/engine_test.go`: In-memory SQLite tests for all recursive CTE traversals.
* `internal/search/engine_test.go`: Integration tests for search engine facade.
* `internal/api/rest/server_test.go`: HTTP handler tests for all Chi routes.
* `internal/service/indexer_test.go`: Tests for indexer service crawling and change detection.
* `specs/features/test-coverage/`: Spec Kit artifacts (`spec.md`, `plan.md`, `tasks.md`).

## 4. Acceptance Criteria

- [x] In-memory SQLite repository tests cover CRUD, Search, and Backlinks.
- [x] Graph engine tests verify recursive CTEs (Shortest Path, Impact, Dependency Tree).
- [x] REST API routes are tested via `httptest`.
- [x] `go test -cover ./...` statement coverage exceeds 80% across core `internal` packages.
- [x] Missing SPDX headers in legacy files are reconciled.
- [x] CHANGELOG.md is updated.

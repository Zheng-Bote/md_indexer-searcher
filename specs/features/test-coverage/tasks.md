# Task Breakdown: Test Suite & In-Memory SQLite Coverage (>80%)

## Tasks

- [x] **Task 1: Repository & SearchRepo Tests (`internal/repository`)**
  - Add tests for FTS5 search, `RemoveDocument`, `GetDocumentByPath`, `GetAllDocuments`, and `SearchRepository` methods (`FindDocumentByTitle`, `FindEntityByName`, `GetBacklinks`).

- [x] **Task 2: Recursive CTE Graph Engine Tests (`internal/graph`)**
  - Create `internal/graph/engine_test.go` verifying `DirectRelations`, `ReverseRelations`, `DependencyTree`, `ImpactAnalysis`, and `ShortestPath` using fixture data.

- [x] **Task 3: Search Engine Facade Tests (`internal/search`)**
  - Create `internal/search/engine_test.go` testing search, backlinks, relations, impact, and entity finding.

- [x] **Task 4: REST API Integration Tests (`internal/api/rest`)**
  - Create `internal/api/rest/server_test.go` testing all HTTP endpoints with `net/http/httptest`.

- [x] **Task 5: Indexer Service Tests (`internal/service`)**
  - Create `internal/service/indexer_test.go` testing full vault crawling, incremental hashing, and skipping unchanged files.

- [x] **Task 6: Reconcile SPDX Headers**
  - Add `// SPDX-License-Identifier: Apache-2.0` header to all legacy source files.

- [x] **Task 7: Quality Gate & Converge**
  - Run `go test -cover ./...` and verify statement coverage > 80% across internal packages.
  - Update `CHANGELOG.md`
  - Prepare PR

# Architecture Plan: Test Suite & In-Memory SQLite Coverage (>80%)

## 1. Architectural Alignment (`md-indexer.sdd`)

* **Dependency Injection:** Tests leverage existing dependency injection constructors (`NewSQLiteRepository`, `NewGraphEngine`, `NewSearchEngine`, `NewServer`) without mocks or global variables.
* **CGO-Free In-Memory Database:** Uses `modernc.org/sqlite` with `dsn = ":memory:"` or `file::memory:?cache=shared`.
* **Clean Architecture:** Tests test each layer's contract cleanly.
* **Licensing Standards:** Add missing `// SPDX-License-Identifier: Apache-2.0` headers to all files in `internal/` and `cmd/`.

## 2. Test Packages Design

### A. `internal/repository`
* Test cases for:
  - `NewSQLiteRepository(":memory:")` & `Close()`
  - `UpsertDocument` with FTS index verification
  - `Search(ctx, query)` with snippet and rank verification
  - `GetDocumentByPath(ctx, path)` (found and not found)
  - `GetAllDocuments(ctx)`
  - `RemoveDocument(ctx, path)`
  - `SearchRepository.FindDocumentByTitle`, `FindEntityByName`, `GetBacklinks`

### B. `internal/graph`
* Setup in-memory repository with a graph fixture:
  - Nodes: `A` -> `B` -> `C`, and `D` -> `B`.
* Test cases for:
  - `DirectRelations(ctx, "A")` -> expects `B`
  - `ReverseRelations(ctx, "B")` -> expects `A` and `D`
  - `DependencyTree(ctx, "A")` -> expects traversal to `B` and `C`
  - `ImpactAnalysis(ctx, "C")` -> expects reverse impact on `B` and `A`
  - `ShortestPath(ctx, "A", "C")` -> expects path `["A", "B", "C"]`
  - `ShortestPath` when no path exists -> expects `nil`

### C. `internal/search`
* Integration tests with in-memory repository:
  - `Search`, `Backlinks`, `Relations`, `Impact`, `Graph`, `FindEntity`, `FindDocument`.

### D. `internal/api/rest`
* HTTP tests using `httptest.NewServer` or `httptest.NewRecorder`:
  - `GET /health` -> 200 OK
  - `GET /api/search?q=...` -> 200 OK JSON
  - `GET /api/documents/NonExistent` -> 200 OK / 500 error handled
  - `GET /api/entities/{name}` -> 200 OK JSON
  - `GET /api/backlinks/{name}` -> 200 OK JSON
  - `GET /api/impact/{name}` -> 200 OK JSON
  - `GET /api/graph/{name}` -> 200 OK JSON

### E. `internal/service`
* Test `IndexerService.IndexAll` and `IndexFile` using `t.TempDir()`.

### F. SPDX Header Reconcile
* Add Apache-2.0 SPDX header to legacy files (`models.go`, `engine.go`, `search_repo.go`, `indexer.go`, `watcher.go`, `config.go`, `server.go`, `main.go`).

<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Task Breakdown: Inline Markdown Document Viewer (Desktop)

**Issue:** [#22](https://github.com/Zheng-Bote/md_indexer-searcher/issues/22)  
**Spec:** [spec.md](./spec.md) · **Plan:** [plan.md](./plan.md)

## Decisions

- [x] Raw Markdown is stored in SQLite; `md-api` never reads vault files (confirmed 2026-10-18).
- [x] Title tie-break: same folder -> shortest path -> alphabetical, case-insensitive.
- [x] External links and images are rendered as text in v1.
- [x] Embeds (`![[…]]`) are out of scope.

## Package & Implementation Tasks

### Task 1: Raw Markdown Storage (`internal/entities`, `internal/parser`, `internal/repository`, `internal/service`)
- [x] Add `Markdown` to `entities.Document`; set it in `Parser.ParseFile`.
- [x] Add `documents.markdown` (create statement + `ALTER TABLE` migration); store it in `UpsertDocument`.
- [x] Re-parse unchanged documents whose `markdown` is `NULL`.
- [x] Tests: markdown stored; pre-feature DB migrated and re-parsed; `section_embeddings` kept.

### Task 2: Document Lookup (`internal/entities`, `internal/repository/search_repo.go`, `internal/search`)
- [x] Add `DocumentView` and `ResolvedLink`.
- [x] Implement `DocumentByPath`, `DocumentByTitle` (tie-break) and outgoing link resolution.
- [x] Add `Document` and `ResolveDocument` to `SearchEngine`.
- [x] Tests: lookup by path, case-insensitive title, tie-break order, unresolved links omitted.

### Task 3: REST Endpoint (`internal/api/rest`, `openapi.yaml`)
- [x] Add `GET /api/document` (`200` / `400` / `404`).
- [x] Tests in `internal/api/rest/server_test.go`.
- [x] Document the endpoint and `DocumentView` schema in `openapi.yaml`.

### Task 4: Frontend Rendering (`apps/desktop/src/app/core`, `apps/desktop/src/app/workspace`)
- [x] Add `DocumentView` / `ResolvedLink` types and `documentByPath` in `ApiClient` (wikilinks carry the resolved path).
- [x] Implement `stripFrontmatter` and `renderDocument` (wikilink + embed extensions, external links/images as text).
- [x] Tests: frontmatter, alias/heading wikilinks, unresolved links, code blocks untouched, sanitizing.

### Task 5: Viewer & Integration (`apps/desktop/src/app/workspace`)
- [x] Implement `DocumentViewer` (history, Back, Close/`Esc`, load error, section jump, dialog semantics, focus).
- [x] Make sources in `AnswerView` buttons emitting `sourceSelected`.
- [x] Overlay the viewer in `AskPanel`; restore focus to the source on close.
- [x] Tests: navigation history, error state, answer unchanged after close.

### Task 6: Verification & Docs
- [x] `go test ./...`, `ng test`, `ng build`, `cargo check` pass.
- [x] Manual API check against a real vault (`~/Dokumente/Snippets`, pre-feature desktop DB): migration (13/13 rows, 108 embeddings kept), `200`/`400`/`404`, title lookup.
- [ ] Manual GUI check in the running desktop app (`make desktop-dev`).
- [x] Update `README.md` (desktop section), `apps/desktop/README.md` and `CHANGELOG.md`.
- [x] Set spec status to Implemented.

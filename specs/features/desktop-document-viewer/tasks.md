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
- [ ] Add `Markdown` to `entities.Document`; set it in `Parser.ParseFile`.
- [ ] Add `documents.markdown` (create statement + `ALTER TABLE` migration); store it in `UpsertDocument`.
- [ ] Re-parse unchanged documents whose `markdown` is `NULL`.
- [ ] Tests: markdown stored; pre-feature DB migrated and re-parsed; `section_embeddings` kept.

### Task 2: Document Lookup (`internal/entities`, `internal/repository/search_repo.go`, `internal/search`)
- [ ] Add `DocumentView` and `ResolvedLink`.
- [ ] Implement `DocumentByPath`, `DocumentByTitle` (tie-break) and outgoing link resolution.
- [ ] Add `Document` and `ResolveDocument` to `SearchEngine`.
- [ ] Tests: lookup by path, case-insensitive title, tie-break order, unresolved links omitted.

### Task 3: REST Endpoint (`internal/api/rest`, `openapi.yaml`)
- [ ] Add `GET /api/document` (`200` / `400` / `404`).
- [ ] Tests in `internal/api/rest/server_test.go`.
- [ ] Document the endpoint and `DocumentView` schema in `openapi.yaml`.

### Task 4: Frontend Rendering (`apps/desktop/src/app/core`, `apps/desktop/src/app/workspace`)
- [ ] Add `DocumentView` / `ResolvedLink` types and `documentByPath` / `documentByTitle` in `ApiClient`.
- [ ] Implement `stripFrontmatter` and `renderDocument` (wikilink + embed extensions, external links/images as text).
- [ ] Tests: frontmatter, alias/heading wikilinks, unresolved links, code blocks untouched, sanitizing.

### Task 5: Viewer & Integration (`apps/desktop/src/app/workspace`)
- [ ] Implement `DocumentViewer` (history, Back, Close/`Esc`, load error, section jump, dialog semantics, focus).
- [ ] Make sources in `AnswerView` buttons emitting `sourceSelected`.
- [ ] Overlay the viewer in `AskPanel`; restore focus to the source on close.
- [ ] Tests: navigation history, error state, answer unchanged after close.

### Task 6: Verification & Docs
- [ ] `go test ./...`, `ng test`, `ng build`, `cargo check` pass.
- [ ] Manual check against the test vault (`~/Dokumente/Tech`) in the desktop app.
- [ ] Update `README.md` (desktop section), `apps/desktop/README.md` and `CHANGELOG.md`.
- [ ] Set spec status to Done.

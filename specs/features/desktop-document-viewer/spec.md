<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Feature Specification: Inline Markdown Document Viewer (Desktop)

**Issue:** [#22](https://github.com/Zheng-Bote/md_indexer-searcher/issues/22)  
**Status:** Specified (`/speckit.specify`)

## 1. Feature Intent

Answers in the desktop app list their sources only as file names. To verify an answer, the user has to switch to Obsidian or an editor.

This feature lets the user open a cited source directly in the app as rendered Markdown, follow wikilinks to other documents of the current vault, and return to the unchanged answer.

### Current State (relevant findings)

- `GET /api/documents/{name}` (`FindDocumentByTitle`) returns only `id`, `path` and `title`, not the content.
- The index does not keep the original Markdown. `Document.Content` and `sections.content` hold text extracted by the AST walker; formatting, tables, wikilinks and frontmatter are lost.
- The desktop app must not read vault files itself (`md-indexer.sdd`: client apps use `md-api` over HTTP only).

So the original Markdown must be stored by the indexer and served by `md-api`.

## 2. User Scenarios

- **Scenario 1 — Open a Source:**
  The answer cites `/vault/Docker/How to restart containers automatically in docker.md`, section `Restart policies`. The user clicks the source. The app shows the rendered document over the answer and scrolls to the heading `Restart policies`.
- **Scenario 2 — Follow a Wikilink:**
  The document contains `[[Docker Installation Linux|Installation]]`. The user clicks "Installation" and the viewer shows `Docker Installation Linux.md` from the same vault.
- **Scenario 3 — Back and Close:**
  After following two wikilinks, the user clicks "Back" twice and is at the originally opened source again. One more "Back" returns to the answer. "Close" (or `Esc`) returns to the answer at any time. Question, answer, sources and scroll position are unchanged and no new request is sent.
- **Scenario 4 — Unresolvable Link:**
  `[[Kubernetes Cluster]]` has no matching note in the vault. It is shown as plain text and is not clickable.
- **Scenario 5 — Same Note Name in Two Folders:**
  `Docker/README.md` and `MQTT/README.md` both exist. Opening the source `MQTT/README.md` shows exactly that file.
- **Scenario 6 — Outdated Index:**
  A cited file was deleted after indexing. The viewer shows "Dokument nicht gefunden" and "Close" still returns to the answer.

## 3. Requirements (EARS Syntax)

1. **Raw Markdown Storage:**
   **WHEN** `md-indexer` indexes a Markdown file, **THE SYSTEM SHALL** store its original Markdown content unchanged in the SQLite index alongside the document.
2. **Migration of Existing Indexes:**
   **WHEN** `md-indexer` opens an index created before this feature, **THE SYSTEM SHALL** add the raw Markdown storage without dropping data and **SHALL** re-parse documents whose raw Markdown is missing even if their hash is unchanged, while keeping valid `section_embeddings`.
3. **Document Content Endpoint:**
   **WHEN** `md-api` receives `GET /api/document?path=<absolute path>`, **THE SYSTEM SHALL** return `id`, `path`, `title`, the raw `markdown` and the resolved outgoing wikilinks of exactly that indexed document in the standard `APIResponse` envelope.
4. **Lookup by Title:**
   **WHEN** `md-api` receives `GET /api/document?title=<title>&from=<path of the linking document>`, **THE SYSTEM SHALL** resolve the title case-insensitively and, if several documents share the title, prefer the one in the same folder as `from`, otherwise the one with the shortest path, then alphabetical order.
5. **Unknown Document:**
   **IF** no indexed document matches the requested path or title, **THEN THE SYSTEM SHALL** respond with `404` and an `ErrorResponse`.
6. **Index-Only Access:**
   **THE SYSTEM SHALL** serve document content only from the SQLite index and **SHALL NOT** read files from disk or accept paths that are not indexed documents.
7. **Open Source:**
   **WHEN** the user clicks a source in the answer view, **THE SYSTEM SHALL** load it by path and show the rendered Markdown in a viewer that overlays the answer without destroying the answer state.
8. **Section Jump:**
   **WHERE** the source names a section, **THE SYSTEM SHALL** scroll to the first heading whose text matches the section.
9. **Rendering:**
   **THE SYSTEM SHALL** hide YAML frontmatter, render wikilinks `[[Target]]`, `[[Target|Alias]]` and `[[Target#Heading]]` as links showing the alias or target, and sanitize the rendered HTML (no scripts, no event handler attributes).
10. **Wikilink Navigation:**
    **WHEN** the user clicks a resolved wikilink, **THE SYSTEM SHALL** open the target document of the current vault in the viewer and push the current document onto a back history.
11. **Unresolved Wikilinks:**
    **IF** a wikilink target is not an indexed document of the current vault, **THEN THE SYSTEM SHALL** render it as non-clickable text.
12. **Back and Close:**
    **WHEN** the user clicks "Back", **THE SYSTEM SHALL** show the previous document, or the answer when the history is empty. **WHEN** the user clicks "Close" or presses `Esc`, **THE SYSTEM SHALL** return directly to the answer and clear the history.
13. **Load Errors:**
    **IF** a document cannot be loaded, **THEN THE SYSTEM SHALL** show the error inside the viewer and keep "Back" and "Close" usable.
14. **Accessibility:**
    **THE SYSTEM SHALL** move focus into the viewer when it opens, return focus to the clicked source when it closes, and make sources, wikilinks, "Back" and "Close" keyboard-operable.

## 4. Non-Goals (this iteration)

- Embedded content: `![[image.png]]`, `![[Note]]` transclusions and relative image paths are shown as plain text.
- Editing notes. The vault remains read-only.
- External links open nothing inside the viewer. They are rendered as non-navigating text.
- Search inside the viewer, backlinks panel, graph view.
- Changing the existing `GET /api/documents/{name}` contract.

## 5. Scope & Affected Components

- `internal/repository/sqlite.go`: Store and read raw Markdown (new column or table via `migrate`), lookup by path and by title with the tie-break rule, resolved outgoing links.
- `internal/service/indexer.go`, `internal/parser/markdown.go`: Pass the raw file content to the repository; re-parse documents with missing raw Markdown.
- `internal/search/engine.go`: Engine methods for document content by path / title (API handlers do not access the DB directly).
- `internal/api/rest/server.go`: `GET /api/document`.
- `openapi.yaml`: Document `GET /api/document`.
- `apps/desktop/src/app/core/api-client.service.ts`, `api.types.ts`: Document fetch.
- `apps/desktop/src/app/workspace/answer-view.ts`: Clickable sources.
- `apps/desktop/src/app/workspace/`: New document viewer component with back history; wikilink/frontmatter preprocessing for `marked`.

## 6. SpecDD Architecture Alignment (Drift Control)

- [x] **Architecture:** Layering is maintained: Indexer -> Repository -> Search Engine -> `md-api` -> desktop app. The handler uses the search engine, not the DB.
- [x] **Architecture:** The desktop app reads documents via `md-api` only; no vault or DB access from `apps/`.
- [ ] **Architecture:** Feature affects architecture: no `md-indexer.sdd` update required.
- [x] **Read-Only Vault:** Markdown files are only read by the indexer, as before.
- [x] **Security:** Rendered HTML is sanitized; the endpoint serves only indexed documents and does not touch the file system.
- [x] **Data Model:** Additive, backwards-compatible schema change (`modernc.org/sqlite`, no CGO); the index stays rebuildable and embeddings survive the migration.
- [x] **Standards:** SPDX headers (`Apache-2.0`) and English source documentation.

## 7. Acceptance Criteria

- [ ] Re-indexing stores the raw Markdown; an existing index is migrated without losing embeddings.
- [ ] `GET /api/document` returns the document by path and by title (tie-break as specified), `404` for unknown documents; covered by tests in `internal/api/rest/server_test.go` and `internal/repository`.
- [ ] Clicking a source opens the rendered document and scrolls to the cited section.
- [ ] Frontmatter is hidden, wikilinks render with alias, HTML is sanitized (test with `<script>` and `onerror` payloads).
- [ ] Resolved wikilinks open other documents of the current vault; unresolved ones are not clickable.
- [ ] "Back" walks the history back to the answer; "Close"/`Esc` returns directly; the answer is unchanged and no new request is sent.
- [ ] Same-named documents in different folders open correctly.
- [ ] `go test ./...`, `ng test`, `ng build` pass.
- [ ] `README.md`, `openapi.yaml` and `CHANGELOG.md` are up-to-date.

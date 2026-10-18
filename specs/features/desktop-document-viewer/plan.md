<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Architecture Plan: Inline Markdown Document Viewer (Desktop)

**Issue:** [#22](https://github.com/Zheng-Bote/md_indexer-searcher/issues/22)  
**Spec:** [spec.md](./spec.md)

## 1. Architectural Alignment (`md-indexer.sdd`)

- **Read-Only Source of Truth:** Only the indexer reads vault files (`Parser.ParseFile` already holds the raw bytes). The raw Markdown is stored as a materialized copy in SQLite; the index stays rebuildable from the vault.
- **Layering:** `md-api` handler -> `search.SearchEngine` -> `repository.SearchRepository` -> SQLite. The handler never touches `*sql.DB`; no code path reads from disk at request time.
- **Thin Client:** The desktop app fetches documents from `md-api` only. Wikilink resolution happens in Go; the frontend only renders.
- **CGO-Free, Additive Schema:** One nullable column, added with the existing `ALTER TABLE` migration pattern in `migrate`.
- **No Global State:** Viewer navigation state lives in component signals.

```
Indexer:  ParseFile(path) ── raw bytes ──► Document.Markdown ──► documents.markdown
md-api:   GET /api/document?path=P | ?title=T&from=F
            └─ SearchEngine.Document / ResolveDocument ─► SearchRepository ─► documents (+ links)
Desktop:  AnswerView (source click) ─► AskPanel.openSource ─► DocumentViewer overlay
            └─ ApiClient.document() ─► marked (wikilink ext., frontmatter strip) ─► [innerHTML] (sanitized)
```

---

## 2. Technical Design

### A. Go: Raw Markdown Storage (`internal/entities`, `internal/parser`, `internal/repository`, `internal/service`)

1. **Model:** Add `Markdown string` to `entities.Document`.
2. **Parser:** `ParseFile` sets `doc.Markdown = string(content)` (bytes are already read for hashing; no extra I/O).
3. **Schema (`migrate`):** `ALTER TABLE documents ADD COLUMN markdown TEXT` (error ignored when it exists, same as `entities.document_id`). The `CREATE TABLE documents` statement gets the column for new databases.
4. **Upsert:** `INSERT INTO documents (..., markdown)` stores `doc.Markdown`.
5. **Migration re-parse:** `GetDocumentByPath` additionally selects `markdown IS NOT NULL` into a new field `HasMarkdown bool` (`json:"-"`) on `entities.Document`. `indexFile` skips an unchanged hash only when `HasMarkdown` is true. Section IDs and content hashes stay identical, so `section_embeddings` are kept (same mechanism as `-force`, `#18`).

### B. Go: Document Lookup (`internal/repository/search_repo.go`, `internal/search/engine.go`)

1. **Types (`internal/entities`):**
   ```go
   type DocumentView struct {
       ID       string         `json:"id"`
       Path     string         `json:"path"`
       Title    string         `json:"title"`
       Markdown string         `json:"markdown"`
       Links    []ResolvedLink `json:"links"`
   }
   type ResolvedLink struct {
       Target string `json:"target"` // normalized title as stored in links.target_document_id
       Path   string `json:"path"`   // resolved document path in the current vault
   }
   ```
2. **Repository:**
   - `DocumentByPath(ctx, path) (*DocumentView, error)`: `SELECT id, path, title, markdown FROM documents WHERE path = ?`; `nil` when missing.
   - `DocumentByTitle(ctx, title, from) (*DocumentView, error)`: `SELECT ... WHERE title = ? COLLATE NOCASE`, then pick in Go: same directory as `from` -> shortest path -> lexicographic path.
   - `resolveLinks(ctx, docID, fromPath)`: `SELECT DISTINCT target_document_id FROM links WHERE source_document_id = ?`, resolve each target with the same tie-break; unresolved targets are omitted.
3. **Engine:** Add `Document(ctx, path)` and `ResolveDocument(ctx, title, from)` to `SearchEngine` and `engineImpl`. `FindDocument` and `/api/documents/{name}` remain unchanged.

### C. Go: REST Endpoint (`internal/api/rest/server.go`, `openapi.yaml`)

`GET /api/document`:
- `path` given -> `Document`; else `title` given -> `ResolveDocument(title, from)`; neither -> `400`.
- not found -> `404` `{"success": false, "error": "document not found"}`.
- success -> `200` envelope with `DocumentView`.
- Only exact indexed paths match (`WHERE path = ?`), so arbitrary file paths cannot be read; there is no file-system access in the handler.

### D. Desktop Frontend (`apps/desktop/src/app`)

1. **API (`core/api.types.ts`, `core/api-client.service.ts`):** `DocumentView`, `ResolvedLink`; `documentByPath(path)`, `documentByTitle(title, from)`.
2. **Rendering (`workspace/markdown-document.ts`, pure functions, unit-tested):**
   - `stripFrontmatter(md)`: remove a leading `---\n…\n---` block.
   - `renderDocument(md, links)`: own `Marked` instance (not the global one used by `AnswerView`) with
     - inline extension `wikilink` for `[[Target(#Heading)?(|Alias)?]]` -> `<a href="#wiki/<encoded target>">alias|target</a>` when the normalized target is in `links` (case-insensitive), else plain text. Normalization mirrors Go `normalizeLinkTarget`.
     - inline extension for `![[…]]` -> plain text (embeds out of scope).
     - renderer overrides: `link` (external) and `image` -> text only.
   - Output is bound via `[innerHTML]`, so Angular's sanitizer removes scripts and event handlers (same approach as `AnswerView`). `#wiki/...` hrefs survive sanitizing.
3. **Viewer (`workspace/document-viewer.ts`):**
   - Inputs: `source` (`{ path, section? }`); output: `closed`.
   - State: `history = signal<Entry[]>([])`, `current = signal<Entry>`, `doc = signal<DocumentView | null>`, `error`, `loading`.
   - Click delegation on the content container: `#wiki/<target>` -> push `current`, load `documentByTitle(target, current.path)`.
   - "Zurück": pop history or emit `closed` when empty. "Schließen" and `Esc`: emit `closed`.
   - Section jump: after render, find the first `h1`–`h6` whose trimmed text equals `section` and `scrollIntoView()`.
   - Accessibility: `role="dialog"`, `aria-modal="true"`, `aria-labelledby` the document title; focus the title on open.
4. **Integration:**
   - `AnswerView`: sources become `<button>`s; new `output` `sourceSelected` emits `{ path, section }`.
   - `AskPanel`: `openSource = signal<SourceRef | null>(null)`; the answer stays rendered (scroll position kept), the viewer overlays the panel; on `closed` focus returns to the clicked source button.

---

## 3. Verification Strategy

- **Go:** repository tests (in-memory SQLite) for markdown storage, migration of a pre-feature DB (column added, re-parse triggered, embeddings kept), tie-break rule and link resolution; REST tests for `200` (path/title), `400`, `404`.
- **Frontend:** unit tests for `stripFrontmatter`, wikilink rendering (alias, heading, unresolved, code block untouched), sanitizing (`<script>`, `onerror`); `DocumentViewer` tests for history (open link, Back, Back to answer, Close/`Esc`), load error and section jump; `AskPanel` test that the answer is unchanged after closing.
- **Manual:** desktop app against the real test vault (`~/Dokumente/Tech`): open a source, follow a wikilink, Back/Close.
- `go test ./...`, `ng test`, `ng build`, `cargo check`.

## 4. Risks

- **Index size:** grows by roughly the vault's Markdown size (test vault: a few MB) — acceptable.
- **First start after update:** every document is re-parsed once (no re-embedding). On the 2,000-note test vault this takes seconds.
- **Title collisions:** the tie-break can differ from Obsidian's own resolution in edge cases; documented in the spec.

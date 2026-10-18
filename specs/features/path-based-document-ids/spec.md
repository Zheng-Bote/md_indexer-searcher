<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Feature Specification: Path-Based Document IDs

**Issue:** [#29](https://github.com/Zheng-Bote/md_indexer-searcher/issues/29)  
**Status:** Implemented (`/speckit.implement`) · Decisions confirmed 2026-10-18

## 1. Feature Intent

Every Markdown file must be indexed as its own document. Today the document ID is the SHA-256 of the file content, so notes with identical content collide and only one of them is indexed.

Decisions (2026-10-18, issue #29):

- The document ID is derived from the **vault-relative path**. The content hash stays in `documents.hash` for change detection. Renaming or moving a note becomes "remove old document + add new document".
- Existing `section_embeddings` are **re-keyed** to the new IDs during the migration; unchanged sections are not re-embedded.
- Repeated headings within one note are fixed in the same change.

### Current State (verified on `main`)

- `Parser.ParseFile` sets `ID = sha256(content)`. Section IDs are `<docID>-overview` and `<docID>-<heading text>`; `sections`, `fts_documents`, `fts_sections`, `links.source_document_id`, `document_tags`, `metadata`, `entities.document_id` and (via section IDs) `section_embeddings` depend on it.
- `UpsertDocument` first deletes all rows of the new ID, which belong to the other note with the same content, and then `INSERT INTO documents` fails on the primary key. All `tx.ExecContext` errors are ignored, so the transaction commits a partial state without logging.
- `RemoveDocument` deletes by document ID, so removing one of two identical notes also removes the shared rows.
- **Related collision inside one note:** two headings with the same text (e.g. two `## Example`) produce the same section ID; the second section is silently dropped (reproduced: only `Example` / `first` is stored).

## 2. User Scenarios

- **Scenario 1 — Duplicate Notes:** `Daily/2026-10-17.md` and `Daily/2026-10-18.md` were both created from the same template and not edited yet. Both appear in search and can be opened in the desktop viewer.
- **Scenario 2 — Edit One Duplicate:** The user edits `2026-10-18.md`. `2026-10-17.md` stays indexed unchanged.
- **Scenario 3 — Rename:** `Broker.md` is renamed to `MQTT Broker.md`. The old document disappears from the index, the new one is indexed; nothing else is affected, regardless of the order of the watcher events.
- **Scenario 4 — Repeated Headings:** A note has two `## Example` sections. Both are searchable and retrievable by `md-ai`.
- **Scenario 5 — Upgrade:** An existing index with content-hash IDs and 3,980 embedded sections is opened by the new `md-indexer`. All documents get path-based IDs, all embeddings are kept under the new section IDs, and no section is sent to the embedding model again.

## 3. Requirements (EARS Syntax)

1. **Unique Documents:**
   **THE SYSTEM SHALL** index every non-skipped Markdown file as its own document, regardless of whether other files have identical content.
2. **Path-Based ID:**
   **THE SYSTEM SHALL** derive the document ID as the hex SHA-256 of the file path relative to the vault root, using `/` as separator on every platform.
3. **Content Hash:**
   **THE SYSTEM SHALL** keep storing the SHA-256 of the file content in `documents.hash` and use it to skip unchanged files.
4. **Unique Section IDs:**
   **THE SYSTEM SHALL** keep section IDs of the form `<docID>-<heading text>` (and `<docID>-overview`) and, **WHEN** the same heading text occurs again in a note, append `#<n>` for the n-th occurrence (`n ≥ 2`).
5. **Isolated Upserts:**
   **WHEN** a document is upserted, **THE SYSTEM SHALL NOT** delete or overwrite rows that belong to a document at a different path.
6. **Atomic Upserts:**
   **IF** any statement inside `UpsertDocument` or `RemoveDocument` fails, **THEN THE SYSTEM SHALL** roll back the transaction and return the error, and the indexer **SHALL** log it with the file path.
7. **Rename and Move:**
   **WHEN** a note is renamed or moved, **THE SYSTEM SHALL** remove the document of the old path and index the new path as a new document, independent of the order in which the watcher reports both events.
8. **Migration:**
   **WHEN** `md-indexer` finds an indexed document whose ID differs from the path-based ID of its path, **THE SYSTEM SHALL** re-key its `section_embeddings` from the old to the new section IDs before re-indexing the document, so that sections with unchanged content are not re-embedded.
9. **Readers During Migration:**
   **WHILE** an index has not been migrated yet, **THE SYSTEM SHALL** keep serving it unchanged to `md-api`, `md-search` and `md-ai` (they do not depend on the ID format).

## 4. Non-Goals

- Detecting renames to carry embeddings over to the new path. A renamed note is re-embedded (only that note).
- Deduplicating identical content in search results or AI context.
- Changing the API: `id` fields stay opaque strings.

## 5. Scope & Affected Components

- **Parser & Extractor:** `internal/parser/markdown.go` – ID passed in by the caller instead of computed from content; unique section IDs for repeated headings.
- **Service:** `internal/service/indexer.go` – compute the path-based ID from the vault root (`IndexerService.dir`), trigger re-keying for legacy IDs, log upsert errors.
- **Repository:** `internal/repository/sqlite.go` – error-checked, atomic `UpsertDocument` / `RemoveDocument`; `RekeyEmbeddings(oldDocID, newDocID)`.
- **AI layer:** no change expected (`internal/ai/hybrid` treats IDs as opaque; to be confirmed by tests).
- `CHANGELOG.md`; README data model note on document IDs.

## 6. SpecDD Architecture Alignment (Drift Control)

- [x] **Architecture:** Layering unchanged (Watcher -> Service -> Parser/Repository).
- [x] **Architecture:** No client-app change.
- [x] **Architecture:** Feature affects architecture: no `md-indexer.sdd` update required.
- [x] **Read-Only Vault:** Unchanged.
- [x] **Security:** Unchanged.
- [x] **Data Model:** Schema unchanged; ID values change via an in-place migration; the index stays rebuildable and embeddings survive.
- [x] **Go Standards:** No global state; the vault root is already injected into `IndexerService`.
- [x] **Standards:** SPDX headers and English documentation.

## 7. Acceptance Criteria

- [x] Two notes with identical content are both indexed, searchable and openable in the desktop viewer; changing or deleting one does not affect the other.
- [x] Repeated headings in one note produce distinct sections.
- [x] Rename/move removes the old and adds the new document in both event orders.
- [x] A failing statement in `UpsertDocument` / `RemoveDocument` rolls back and is logged.
- [x] Migration of an existing index: same number of documents, all embeddings re-keyed, zero existing sections re-embedded (verified on a copy of the 2,000-note test vault index; only the 275 previously dropped repeated-heading sections were embedded).
- [x] `go test ./...` with regression tests for all of the above.
- [x] `CHANGELOG.md` and `README.md` are up-to-date.

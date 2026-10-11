# Architecture Plan: Robust Entity Extraction & Relation Linking

## 1. Architectural Alignment (`md-indexer.sdd`)

* **Source of Truth:** Markdown files remain strictly Read-Only. No modifications or back-syncing to files.
* **CQRS-Light:** `internal/parser` and `internal/repository` handle the write path (indexing). The search/query path (`internal/search`, `internal/graph`) reads exclusively from SQLite.
* **No CGO:** Uses `modernc.org/sqlite` without CGO.
* **Clean Architecture:** Changes are confined to `internal/parser`, `internal/repository`, and tested with unit tests.
* **Code Standards:** English documentation, comments, and Apache-2.0 SPDX identifiers.

## 2. Technical Design

### A. Parser: Wikilink and Tag Extraction
* **Current Issue:** The AST walker checks `strings.HasPrefix(val, "[[") && strings.HasSuffix(val, "]]")`. If a sentence contains `link to [[Encryption]] and...`, the link is never detected.
* **Fix:** Use a compiled regular expression `regexp.MustCompile(`\[\[(.*?)\]\]`)` on text nodes to extract all wikilinks within sentences.
* **Entity Relation Integration:** If `doc.IsEntity` is true, each target extracted from a wikilink is appended to `doc.EntityRelations` with type `"LINKS_TO"`.
* **Tag Extraction:** Also extract tags matching `#[a-zA-Z0-9_\-]+` into `doc.Tags`.

### B. Repository: Relational Resilience & Forward References
* **Foreign Key Challenge:**
  `entity_relations` references `entities(id)` for both `source_entity_id` and `target_entity_id`.
  When indexing document `A` which references `B`, document `B` might not have been indexed yet.
* **Solution:**
  In `UpsertDocument`:
  1. Ensure entity type `unresolved` exists:
     `INSERT OR IGNORE INTO entity_types (id, name) VALUES ('unresolved', 'unresolved')`
  2. For every relation `rel` in `doc.EntityRelations`:
     Insert a stub entity for `rel.TargetID` if it doesn't already exist:
     `INSERT OR IGNORE INTO entities (id, type_id, name) VALUES (?, 'unresolved', ?)`
  3. When `B.md` is indexed later, `INSERT OR REPLACE` (or update) sets its real `type_id` and `name`.

### C. Testing Strategy
* Test table in `internal/parser/markdown_test.go` checking:
  - Document with `type: component`, scalar properties, list relations (`uses: [A, B]`).
  - Document containing multiple inline wikilinks (`[[Alpha]]`, `[[Beta]]`) in body text.
  - Verification that `doc.Links` and `doc.EntityRelations` contain the expected entries.
* In-memory SQLite test verifying that `UpsertDocument` succeeds with forward relations.

# Task Breakdown: Robust Entity Extraction & Relation Linking

## Tasks

- [x] **Task 1: AST Parser Wikilink & Tag Extraction**
  - Update `internal/parser/markdown.go` to extract inline `[[Wikilinks]]` and `#tags` from sentences.
  - If `doc.IsEntity`, automatically append Wikilinks to `doc.EntityRelations` with type `"LINKS_TO"`.
  - Add Apache-2.0 SPDX header and English comments.

- [x] **Task 2: Repository Forward Reference Handling**
  - Update `internal/repository/sqlite.go` `UpsertDocument` to ensure `unresolved` entity type and stub target entities are created when inserting relations.
  - Ensure entities can be cleanly updated when the real entity definition is indexed later (`INSERT ... ON CONFLICT(id) DO UPDATE ...` or `INSERT OR REPLACE`).
  - Add Apache-2.0 SPDX header.

- [x] **Task 3: Unit & Integration Tests**
  - Expand `internal/parser/markdown_test.go` with table-driven tests for entities, frontmatter lists, and inline Wikilinks.
  - Add test in `internal/repository` to verify in-memory SQLite insertion of entities and relations.

- [x] **Task 4: Quality Gate & Converge**
  - Run `go test -v ./...`
  - Update `CHANGELOG.md`
  - Prepare PR

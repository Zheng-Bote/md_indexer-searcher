# Feature Spec: Section Content Extraction

## 1. Problem & Goal
Currently, the markdown parser extracts heading names to create `Section` entities, but the actual text content under each heading is discarded. Additionally, the SQLite FTS5 index receives an empty string for document content, making full-text search less effective.

## 2. Scope & Acceptance Criteria
*   The AST walker must maintain state to aggregate text segments into the `Content` field of the current `Section`.
*   The `entities.Document` struct must expose a `Content` field holding the aggregated text of the entire document.
*   The `UpsertDocument` function must insert the full document `Content` into the `fts_documents` table.

## 3. Architecture Plan
*   **Target:** `internal/entities/models.go`, `internal/parser/markdown.go`, `internal/repository/sqlite.go`.

## 4. Tasks
*   [ ] Task 1: Add `Content string` to `entities.Document`.
*   [ ] Task 2: Modify `ast.Walk` in `markdown.go` to append text to the current section and the full document content.
*   [ ] Task 3: Update `UpsertDocument` in `sqlite.go` to insert `doc.Content` instead of an empty string into `fts_documents`.

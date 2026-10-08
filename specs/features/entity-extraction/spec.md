# Feature Spec: Entity Extraction from Markdown

## 1. Problem & Goal
The parser currently parses frontmatter into a generic `Metadata` map but does not identify or extract structured Entities and their relations (e.g., `uses`, `affects`) to populate the `entities`, `entity_types`, `entity_properties`, and `entity_relations` tables.

## 2. Scope & Acceptance Criteria
*   If a Markdown file has `type: <string>` in its frontmatter, it is treated as an Entity.
*   The entity name defaults to the document title (or `name` in frontmatter if provided).
*   All other scalar frontmatter fields are stored as `entity_properties`.
*   List fields like `uses` or `affects` in the frontmatter are treated as outgoing `entity_relations` where the target is the string value (target name).
*   The `UpsertDocument` repository method must persist these entities and relations safely.

## 3. Architecture Plan
*   **Target:** `internal/entities/models.go`, `internal/parser/markdown.go`, `internal/repository/sqlite.go`.
*   No external dependencies. Uses existing `goldmark-meta`.

## 4. Tasks
*   [ ] Task 1: Update `entities.Document` struct.
*   [ ] Task 2: Update `internal/parser/markdown.go` to populate entity fields from `metaData`.
*   [ ] Task 3: Update `internal/repository/sqlite.go` `UpsertDocument` to wipe and insert entity records.

# Feature Specification: Robust Entity Extraction and Relation Linking

**Issue:** [#1](https://github.com/Zheng-Bote/md_indexer-searcher/issues/1)  
**Status:** In Progress (Specify / Plan Phase)

## 1. Feature Intent

The goal is to provide robust extraction of typed domain entities, scalar metadata properties, list-based relationships (e.g., `uses`, `depends_on`, `implements`), and inline Wikilinks (`[[Target]]`) from Markdown documents into the SQLite knowledge graph.

Markdown files remain strictly Read-Only and the Single Source of Truth. The SQLite database acts as a materialized knowledge graph.

## 2. Requirements (EARS Syntax)

1. **Frontmatter Entity Extraction:** When a Markdown file containing `type: <string>` in its YAML frontmatter is parsed, the parser shall mark the document as an Entity (`doc.IsEntity = true`), identify its `EntityType`, resolve its `EntityName` (from `name` or fallback to document title), and record all scalar metadata fields into `EntityProperties`.
2. **List-Based Relationship Extraction:** When a frontmatter field contains a list of strings (e.g., `uses: [A, B]`), the parser shall extract an `EntityRelation` for each item with `SourceID = EntityName`, `TargetID = item`, and `Type = UPPERCASE(field_name)`.
3. **Wikilink Parsing & Relation Mapping:** When a Markdown document contains inline Wikilinks (`[[Target]]`), the parser shall extract each link into `doc.Links`. If the document is an Entity, the parser shall also extract an `EntityRelation` with `Type = "LINKS_TO"`.
4. **Resilient Graph Ingestion (Forward References):** When storing entity relations in `UpsertDocument`, if a target entity does not yet exist in the database, the repository shall insert a stub entity (type: `unresolved`) to satisfy relational foreign key constraints without error, allowing eventual consistency as other Markdown files are indexed.
5. **Entity Deletion & Update Cleanliness:** When an existing document or entity is re-indexed or removed, all previous entity records, properties, and outgoing relations for that document/entity shall be wiped and re-created in an atomic transaction.

## 3. Scope & Affected Components

* `internal/entities/models.go`: Verify model fields for entities and relations.
* `internal/parser/markdown.go`: Improve Wikilink regex extraction and entity relation mapping.
* `internal/repository/sqlite.go`: Ensure `UpsertDocument` handles forward references cleanly and respects schema constraints.
* `internal/parser/markdown_test.go`: Unit tests for frontmatter entity extraction and Wikilink extraction.

## 4. Acceptance Criteria

- [x] Frontmatter entity properties and relations are fully extracted into `entities`, `entity_properties`, and `entity_relations`.
- [x] Inline Wikilinks (`[[Target]]`) anywhere in text are captured in `doc.Links` and as `LINKS_TO` relations for entities.
- [x] Foreign key constraints in SQLite are satisfied via stub entities for deferred/forward-referenced targets.
- [x] Unit and repository tests pass.
- [x] CHANGELOG.md is updated.

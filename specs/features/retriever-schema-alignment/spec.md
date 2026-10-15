<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Feature Spec: Retriever Schema Alignment

## 1. Problem & Goal
`internal/ai/retriever` queries tables and columns that the indexer never creates
(`documents_fts`, `documents.body`, `entities.type/document_uri/description`,
`entity_relations.source_entity/target_entity`, `links.source_path/target_path`).
Its error paths swallow every SQL error, so against a real `knowledge.db` md-ai retrieves nothing.
The unit tests did not catch this because they build their own fake schema.

Goal: the retriever reads the schema owned by `internal/repository/sqlite.go` and the tests prove it.

## 2. Scope & Acceptance Criteria
*   **Entities:** matched on `entities.name`; type resolved via `entity_types`; description read from
    `entity_properties` (`property_key = 'description'`); defining document resolved via `documents.title = entities.id`.
    Unresolved stub entities (`type_id = 'unresolved'`) are not reported as entity matches. Each entity is reported once.
*   **Relations:** read from `entity_relations.source_entity_id / target_entity_id / relation_type`.
*   **FTS:** queries `fts_documents` (joined via `fts_documents.id = documents.id`), ranks with `bm25`,
    selects the top matching documents and returns their non-empty sections.
*   **FTS query sanitizing:** keeps Unicode letters and digits (e.g. German umlauts).
*   **LIKE fallback:** searches `documents.title` and `sections.content`.
*   **Graph neighbors:** traverses `links`; `links.target_document_id` holds the raw wikilink target and is
    resolved to a document via `documents.title`.
*   `model.*` types, the `Retriever` API and md-ai / md-api behavior stay backward compatible.
*   Tests create their database through `repository.NewSQLiteRepository` and populate it through
    `UpsertDocument`, so schema drift breaks the tests.

## 3. Non-Goals
*   No change to the indexer write path or the schema.
*   No new retrieval strategies (handled by the later hybrid-retriever feature).

## 4. Known Data Limitations (documented, not fixed here)
*   Entity → document mapping only works when the entity name equals the document title.
*   Wikilinks with alias or heading (`[[Note|Alias]]`, `[[Note#H]]`) are stored verbatim and do not resolve.

## 5. Tasks
*   [x] Task 1: Rewrite retriever SQL against the real schema.
*   [x] Task 2: Unicode-aware `sanitizeFTSQuery`.
*   [x] Task 3: Retriever and assistant tests use the real migration and write path.
*   [x] Task 4: Verify against a real vault index.

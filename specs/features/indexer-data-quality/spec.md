<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Feature Spec: Indexer & Parser Data Quality Improvements

## 1. Problem & Goal
Several pre-existing parser and repository issues degrade index and graph quality on real Obsidian vaults:
1. Wikilinks with alias or heading (`[[Note|Alias]]`, `[[Note#Heading]]`) are stored verbatim instead of resolving to `Note`.
2. Frontmatter relation lists containing Obsidian links (`uses: ["[[MQTT Broker|Broker]]"]`) keep `[[ ]]` and `|Alias` and fail to join against `documents.title` / `entities.id`.
3. `markdown.go` runs `extractLinksAndTags` on `FencedCodeBlock` and `CodeBlock` nodes and inline code spans, turning `#include`, `#define`, `#1`, and `` `[[which treefrog]]` `` into bogus tags and links.
4. `RemoveDocument` only deletes from `documents`, leaving orphaned rows in `sections`, `links`, `document_tags`, `metadata`, `fts_documents`, `fts_sections`, and `section_embeddings`.
5. `metadata` table is created in SQLite but never populated during `UpsertDocument`.
6. Template folders (`_templates_`, `.obsidian`, `.git`) are indexed as normal notes, turning Templater files into bogus entities.
7. Several pre-existing source files were not formatted with `gofmt`.

## 2. Scope & Acceptance Criteria

### 2.1 Parser (`internal/parser/markdown.go`)
*   `normalizeLinkTarget(raw string) string` strips surrounding `[[...]]`, strips `#heading` fragments and `|alias` suffixes, strips optional `.md` extension, and trims path prefixes so `[[folder/Note#Section|Alias]]` resolves to `Note`.
*   Both inline wikilinks and frontmatter relation list targets pass through `normalizeLinkTarget`.
*   `FencedCodeBlock` and `CodeBlock` AST nodes are still included in section text content, but `extractLinksAndTags` is **not** called on them.
*   Before `extractLinksAndTags` scans a paragraph, heading, or list item, inline code spans (`` `...` ``) are stripped from the text inspected for links/tags.
*   Purely numeric hashtags (e.g. `#1`, `#42`) are ignored by tag extraction.

### 2.2 Template & Hidden Folder Skipping (`internal/service/indexer.go`, `internal/watcher/watcher.go`)
*   `ShouldSkipPath(path string) bool` skips `_templates_`, `templates` (when prefixed with `_`), and hidden directories (`.obsidian`, `.git`, `.trash`, `.specdd`) during `IndexAll`, `IndexFile`, and `Watcher` directory traversal.

### 2.3 Repository Cleanup & Metadata Persistence (`internal/repository/sqlite.go`)
*   `UpsertDocument` populates the `metadata` table (`document_id`, `key`, `value`) for scalar frontmatter metadata keys (and wipes old rows for `doc.ID` on upsert).
*   `RemoveDocument` deletes all dependent rows for the document (`sections`, `links`, `document_tags`, `metadata`, `fts_documents`, `fts_sections`, `section_embeddings`, and `documents`) in a transaction.

### 2.4 Formatting
*   Run `gofmt -w` on all Go files in `cmd/` and `internal/`.

## 3. Tasks
*   [x] Task 1: Normalize wikilink and frontmatter relation targets and skip code blocks/inline code/numeric tags in `internal/parser`.
*   [x] Task 2: Skip `_templates_` and hidden folders in `internal/service` and `internal/watcher`.
*   [x] Task 3: Populate `metadata` and cascade-clean dependent tables in `RemoveDocument` in `internal/repository/sqlite.go`.
*   [x] Task 4: Run `gofmt -w` across the repository and add unit tests for parser, indexer, and repository cleanup.

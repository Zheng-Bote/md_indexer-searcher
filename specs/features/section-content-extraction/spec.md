# Feature Specification: Comprehensive Section Content Extraction in AST Walker

**Issue:** [#2](https://github.com/Zheng-Bote/md_indexer-searcher/issues/2)  
**Status:** In Progress (Specify / Plan Phase)

## 1. Feature Intent

The goal is to ensure complete and lossless extraction of Markdown section contents into `Section.Content` and `Document.Content`. This includes capturing leading intro text prior to any heading, as well as code blocks, lists, blockquotes, and other rich AST blocks, so that the SQLite `sections` table and FTS5 search index contain the complete text representation of all notes.

Markdown files remain strictly Read-Only and the Single Source of Truth.

## 2. Requirements (EARS Syntax)

1. **Lead / Introductory Content Capture:** While parsing Markdown content prior to the first heading, the parser shall dynamically initialize and assign text to an `Overview` section (`Level: 1`, `ID: doc.ID + "-overview"`) instead of discarding it.
2. **Rich AST Elements Extraction:** When traversing non-text block AST nodes (such as `ast.FencedCodeBlock`, `ast.CodeBlock`, `ast.ListItem`), the parser shall extract their text lines and append them to the active section's content.
3. **Lossless Document Content Aggregation:** When all sections are finalized, `Document.Content` shall aggregate both section headings and their full contents, ensuring FTS5 indexing has complete text coverage for BM25 ranking.
4. **Relational Persistence Integrity:** When `UpsertDocument` runs, each section (including introductory sections and rich content) shall be persisted into the `sections` table.

## 3. Scope & Affected Components

* `internal/parser/markdown.go`: AST walking logic for lead paragraphs, code blocks, lists, and section content aggregation.
* `internal/parser/markdown_test.go`: Tests for documents with lead text before headings, fenced code blocks, and lists.
* `specs/features/section-content-extraction/`: Spec Kit artifacts (`spec.md`, `plan.md`, `tasks.md`).

## 4. Acceptance Criteria

- [x] Content before the first heading is captured in an `Overview` section in `doc.Sections`.
- [x] Code blocks (`ast.FencedCodeBlock`, `ast.CodeBlock`) and lists are retained in `Section.Content`.
- [x] Complete section content is stored in the `sections` table in SQLite.
- [x] Unit tests verify lead content and rich block extraction.
- [x] CHANGELOG.md is updated.

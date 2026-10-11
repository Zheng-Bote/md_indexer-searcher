# Architecture Plan: Comprehensive Section Content Extraction

## 1. Architectural Alignment (`md-indexer.sdd`)

* **Source of Truth:** Markdown files remain strictly Read-Only. No filesystem writes.
* **Materialized Index:** SQLite `sections` table and `fts_documents` receive full, structured content extracted from the AST.
* **No CGO:** Uses existing Go dependencies (`goldmark`, `modernc.org/sqlite`).
* **Clean Architecture:** Changes are localized to `internal/parser/markdown.go` and verified through unit tests in `internal/parser/markdown_test.go`.
* **Standards:** English comments, Apache-2.0 SPDX identifiers.

## 2. Technical Design

### A. Dynamic Initial Section (Lead / Overview)
* In `extractAST`:
  - Initialize `currentSectionIdx := -1`.
  - Provide a helper `ensureCurrentSection()`:
    If `currentSectionIdx == -1`, automatically append an `Overview` section:
    ```go
    doc.Sections = append(doc.Sections, entities.Section{
        ID:      doc.ID + "-overview",
        Heading: "Overview",
        Level:   1,
        Content: "",
    })
    currentSectionIdx = len(doc.Sections) - 1
    ```
  - When encountering content before the first heading, call `ensureCurrentSection()` and append the content.
  - When encountering a heading, set `currentSectionIdx = len(doc.Sections) - 1`.

### B. Rich AST Node Traversal
* `ast.FencedCodeBlock`:
  - Traverse lines using `cb.Lines()`. Extract lines and append with code block formatting or plain text into `doc.Sections[idx].Content`.
* `ast.CodeBlock`:
  - Traverse indented code block lines using `cb.Lines()`.
* `ast.ListItem`:
  - Traverse lines using `li.Lines()`. Append to `doc.Sections[idx].Content`. Also extract any wikilinks or tags present in list items.
* `ast.Paragraph`:
  - Continue extracting lines using `para.Lines()`. Ensure newline separation.

### C. Testing Strategy
* Test case in `markdown_test.go` with:
  1. A document having lead paragraphs BEFORE the first `# Heading`.
  2. Fenced code block (```go ... ```).
  3. Bullet lists (`- item 1 with [[Link]]`).
  4. Assertions on `doc.Sections`:
     - Section 0 is "Overview" with lead content.
     - Following sections contain their headings, code blocks, and list items.
     - `doc.Content` contains the full combined text for FTS indexing.

# Task Breakdown: Comprehensive Section Content Extraction

## Tasks

- [x] **Task 1: AST Parser Lead Section & Rich Blocks**
  - Implement dynamic `ensureCurrentSection()` in `internal/parser/markdown.go` to capture lead text before the first heading in an `Overview` section.
  - Implement extraction for `ast.FencedCodeBlock`, `ast.CodeBlock`, and `ast.ListItem` lines into `Section.Content`.
  - Maintain clean spacing and formatting in `Section.Content` and `Document.Content`.

- [x] **Task 2: Unit Testing for Comprehensive Sections**
  - Add test in `internal/parser/markdown_test.go` checking:
    - Lead paragraphs before heading are captured in `Overview`.
    - Fenced code blocks are preserved in section content.
    - List items with wikilinks are preserved in section content and link list.
    - Full `doc.Content` aggregation is complete.

- [x] **Task 3: Quality Gate & Converge**
  - Run `go test -v ./...`
  - Update `CHANGELOG.md`
  - Prepare PR

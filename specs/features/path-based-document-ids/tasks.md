<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Task Breakdown: Path-Based Document IDs

**Issue:** [#29](https://github.com/Zheng-Bote/md_indexer-searcher/issues/29)  
**Spec:** [spec.md](./spec.md) · **Plan:** [plan.md](./plan.md)

## Decisions

- [x] Document ID from the vault-relative path; rename/move = remove + add (2026-10-18).
- [x] Embeddings are re-keyed during the migration (2026-10-18).
- [x] Repeated headings within one note are fixed in the same change (`#n` suffix from the second occurrence) (2026-10-18).

## Package & Implementation Tasks

### Task 1: Parser (`internal/parser`)
- [ ] `ParseFile(path, id)`; content hash stays in `Hash`.
- [ ] Unique section IDs for repeated headings (incl. a heading named `Overview`).
- [ ] Tests updated and extended.

### Task 2: Repository (`internal/repository`)
- [ ] `RekeyEmbeddings(oldDocID, newDocID)`.
- [ ] Error-checked, atomic `UpsertDocument` and `RemoveDocument`.
- [ ] Tests: prefix exactness, rollback on a failing statement.

### Task 3: Service (`internal/service`)
- [ ] `DocumentID(root, path)` (absolute/relative roots, `/` separators).
- [ ] `indexFile`: path-based ID, re-key legacy IDs before upsert, skip logic uses ID + hash + markdown, log upsert errors.
- [ ] `IndexAll` continues after upsert errors.
- [ ] Tests: duplicates, edit/remove one duplicate, rename in both event orders, migration with zero re-embedded sections.

### Task 4: Docs
- [ ] README data model note, CHANGELOG.

### Task 5: Verification
- [ ] `go vet ./...`, `go test ./...`.
- [ ] Migration of a copy of the test vault index (`tech.db`) with `-embed-model bge-m3`: documents and embeddings kept, no embedding batch.
- [ ] Set spec status to Implemented.

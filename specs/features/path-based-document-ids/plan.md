<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Architecture Plan: Path-Based Document IDs

**Issue:** [#29](https://github.com/Zheng-Bote/md_indexer-searcher/issues/29)  
**Spec:** [spec.md](./spec.md)

## 1. Architectural Alignment (`md-indexer.sdd`)

- **Layering:** The ID is a concern of the indexing service, which already owns the vault root (`IndexerService.dir`). The parser receives the ID instead of deriving it; the repository stays storage-only and gains one re-keying operation.
- **Rebuildable Index / Embeddings Survive:** No schema change. Legacy rows are migrated lazily per document during the normal indexing pass; embeddings are re-keyed in SQL before the re-upsert, so `EmbedPending` finds matching `content_hash` values and embeds nothing.
- **No Global State:** No new state; the occurrence counter for headings is local to one parse.

```
IndexerService.indexFile(path)
  id   := DocumentID(vaultRoot, path)            // sha256(slash(rel(abs(root), abs(path))))
  doc  := parser.ParseFile(path, id)              // sections: <id>-<heading>[#n], <id>-overview
  prev := repo.GetDocumentByPath(path)
  if prev != nil && prev.ID != id:                // legacy content-hash ID
       repo.RekeyEmbeddings(prev.ID, id)          // section_embeddings prefix rewrite
  skip if prev.ID == id && hash unchanged && HasMarkdown && !force
  repo.UpsertDocument(doc)                        // atomic; deletes only rows of prev.ID / id of THIS path
  on error: log path + error, continue
```

---

## 2. Technical Design

### A. Document ID (`internal/service/indexer.go`)

```go
// DocumentID returns the hex SHA-256 of path relative to the vault root, with "/" separators.
func DocumentID(root, path string) (string, error)
```

- `root` and `path` are made absolute with `filepath.Abs` (the walk and the watcher may report relative paths when `-dir .` is used), then `filepath.Rel` + `filepath.ToSlash`.
- Exported so tests and future tools can compute IDs; lives in `service` next to `ShouldSkipPath`.

### B. Parser (`internal/parser/markdown.go`)

1. Signature `ParseFile(path, id string)`; `doc.ID = id`, `doc.Hash` stays the content SHA-256.
2. Unique section IDs: a per-parse `map[string]int` counts suffixes; `ensureSection` registers `overview`. The first occurrence keeps `<id>-<suffix>`, later ones get `<id>-<suffix>#<n>`. Because the first occurrence is unchanged, re-keying by prefix stays exact for every section that existed before.
3. Callers: `IndexerService.indexFile` and four parser tests.

### C. Repository (`internal/repository/sqlite.go`)

1. **`RekeyEmbeddings(ctx, oldDocID, newDocID) (int64, error)`:**
   ```sql
   UPDATE section_embeddings
      SET section_id = ? || substr(section_id, ?)          -- newDocID, len(oldDocID)+1
    WHERE substr(section_id, 1, ?) = ? || '-'              -- len(oldDocID)+1, oldDocID
   ```
   Legacy IDs are 64-hex content hashes, so the prefix plus `-` cannot match another document. Rows whose new key already exists are left alone (`UPDATE OR IGNORE`) and later removed by `DeleteOrphanEmbeddings`.
2. **Atomic `UpsertDocument` / `RemoveDocument`:** a small closure `exec(query, args...)` records the first error and turns later calls into no-ops; before `Commit` the error is returned (the deferred `Rollback` undoes everything). All existing statements already tolerate re-runs (`INSERT OR IGNORE`, `ON CONFLICT`, prior deletes), so only real failures surface.
3. Wipe step unchanged in shape: it deletes by the previous ID stored for **this path** and by the new ID. With path-based IDs both belong to this path only, which removes the cross-document deletion.

### D. Service Error Handling (`internal/service/indexer.go`)

- `indexFile` logs upsert errors with the path (`log.Error().Err(err).Str("path", path)`), returns the error.
- `IndexAll` continues with the next file on upsert errors (previously they were invisible, so aborting the whole run now would be a regression); parse errors keep their current behaviour.
- The watcher already ignores the return value; the log line in `indexFile` makes failures visible there too.

### E. Docs

- README data model: `documents.id` = SHA-256 of the vault-relative path, `documents.hash` = content hash.
- CHANGELOG (`Fixed`): duplicates, repeated headings, atomic upserts, migration note (one-time re-parse, embeddings kept).

---

## 3. Verification Strategy

- **Parser:** repeated headings (`Example`, `Example#2`), heading literally named `Overview` next to the implicit overview section, ID passed through.
- **Service:**
  - two identical notes both indexed; editing/removing one leaves the other intact;
  - rename in both event orders (create-then-remove, remove-then-create);
  - `DocumentID` for absolute and relative (`-dir .`) roots;
  - migration: index built with legacy IDs (rows rewritten to content-hash IDs in the test) plus seeded embeddings → after `IndexAll` same document count, every embedding re-keyed, `EmbedPending` with a counting stub embedder embeds **0** sections.
- **Repository:** `RekeyEmbeddings` prefix exactness; a forced failing statement (e.g. a `CHECK`-violating temp trigger) rolls back the whole upsert and returns the error.
- **Real data:** copy of the 2,000-note test vault index (`tech.db`, 3,980 embeddings) migrated with the new `md-indexer` and `-embed-model bge-m3`: document count unchanged (plus previously colliding duplicates), embedding count unchanged, log shows no `Embedding sections` batch.
- `go vet ./...`, `go test ./...`.

## 4. Risks

- **One-time re-parse:** every legacy document is re-upserted once (fast: seconds for 2,000 notes, no embedding calls).
- **Rename cost:** a renamed note is re-embedded (accepted in the spec).
- **Paths outside the root:** `filepath.Rel` can return `../…` if a watcher reports a path outside the vault (e.g. via symlinks); the ID is still unique per path, so this is harmless.

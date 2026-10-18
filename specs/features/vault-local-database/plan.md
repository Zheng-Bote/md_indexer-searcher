<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Architecture Plan: Vault-Local Index Database

**Issue:** [#23](https://github.com/Zheng-Bote/md_indexer-searcher/issues/23)  
**Spec:** [spec.md](./spec.md)

## 1. Architectural Alignment (`md-indexer.sdd`)

- **Read-Only Vault:** Only `md-indexer` writes into the vault, and only below `<vault>/.md_indexer/` (database, WAL files, `.gitignore`). This boundary becomes an explicit rule in `md-indexer.sdd`.
- **Single Source of Path Logic:** All resolution lives in a new `internal/vaultdb` package. `cmd/` binaries call it during wiring; the desktop app stops deriving database paths and only passes the vault.
- **No Global State / DI:** `vaultdb` functions take the user data directory as a parameter; only `cmd/` calls `vaultdb.UserDataDir()`. This keeps tests independent of the real home directory.
- **Readers Stay Passive:** Reader resolution only stats files; it never creates folders or databases.

```
md-indexer -dir V [-db P] [-migrate-from L]
   └─ P given? ──yes──► P
          no ─► vaultdb.ForWriter(V, dataDir) ─► V/.md_indexer/md_indexer.db  (+ .gitignore)
                                                └─ not writable ─► dataDir/md-indexer/vaults/<key>.db
             └─ L given? ─► vaultdb.MigrateLegacy(L, resolved)  (db, -wal, -shm; only if target missing)

md-api -vault V | md-search --vault V | md-ai --vault V   [explicit DB path wins]
   └─ vaultdb.ForReader(V, dataDir) ─► local db if it exists, else fallback db if it exists, else ErrNoIndex

Desktop (sidecar.rs):
   md-indexer -dir V -migrate-from <appData>/vaults/<hash>.db (only if that file exists)
   md-api     -vault V
```

---

## 2. Technical Design

### A. `internal/vaultdb` (new)

```go
const (
    DirName  = ".md_indexer"
    FileName = "md_indexer.db"
)

var ErrNoIndex = errors.New("no index found for vault")

// Vault returns the absolute, symlink-resolved vault path.
func Vault(dir string) (string, error)
// LocalPath: <vault>/.md_indexer/md_indexer.db
func LocalPath(vault string) string
// FallbackPath: <dataDir>/md-indexer/vaults/<first 16 hex of sha256(vault)>.db
func FallbackPath(vault, dataDir string) string
// UserDataDir: $XDG_DATA_HOME | ~/.local/share (Linux/BSD), ~/Library/Application Support (macOS), %LocalAppData% (Windows)
func UserDataDir() (string, error)

// ForWriter creates <vault>/.md_indexer (+ .gitignore "*" if missing) and probes writability
// with a temp file; on failure it creates the fallback directory instead.
func ForWriter(dir, dataDir string) (path string, fallback bool, err error)
// ForReader returns the first existing of LocalPath and FallbackPath, or an error wrapping
// ErrNoIndex that names the vault and both paths.
func ForReader(dir, dataDir string) (string, error)
// MigrateLegacy moves legacy (+ "-wal", "-shm") to target when legacy exists and target does not.
// os.Rename is tried first; on cross-device errors the files are copied and the originals removed.
func MigrateLegacy(legacy, target string) (moved bool, err error)
```

- The fallback key uses the same scheme as the desktop app today (SHA-256 of the absolute path, 8 bytes as hex), so the logic is familiar; the directory itself is new (`md-indexer`, not the Tauri app identifier).
- `.gitignore` is written only when `ForWriter` creates the folder or the file is missing; an existing file is never overwritten.

### B. CLI Wiring

1. **`cmd/indexer`:** `-db` default `""`. When empty: `vaultdb.ForWriter(*dir, dataDir)`, log `"Using vault-local database"` or a warning `"Vault not writable, using fallback database"`. New flag `-migrate-from` (path of a legacy database): after resolution, `MigrateLegacy(from, resolved)`; log when moved. With an explicit `-db`, `-migrate-from` still migrates into that path.
2. **`cmd/md-api`:** `-db` default `""`, new `-vault` (default `.`). When `-db` is empty: `ForReader`; on error `log.Fatal` with the message. `aiCfg.DBPath` is set from the resolved path (unchanged: `md-api` ignores `db_path` from YAML, the flag/resolution wins).
3. **`cmd/md-search`:** `--db` default `""`, new persistent `--vault` (default `.`). Resolution in `PersistentPreRunE` so every subcommand gets it; on error print and exit `1`. (Existing subcommands ignore repository errors today; the new error path is only for resolution.)
4. **`cmd/md-ai` + `internal/ai/config`:** `DefaultConfig().DBPath` becomes `""` (explicit sources keep precedence: `--db` > `MD_AI_DB_PATH` > `db_path` in YAML). New persistent `--vault` (default `.`). In `initAssistant`, before `Validate`: if `DBPath == ""`, resolve with `ForReader`. `Validate` keeps rejecting an empty path, which can then only happen for programmatic callers.
5. **`md-ai.example.yaml`:** `db_path` commented out with a note that it overrides the vault-local default.

### C. Desktop (`apps/desktop/src-tauri/src/sidecar.rs`)

1. Rename `database_path` → `legacy_database_path`; it only computes the path (no `create_dir_all`).
2. Extract pure functions `indexer_args(vault, legacy: Option<&Path>, port, embedding)` and `api_args(vault, port, model, embedding)`; `start` uses them. `-migrate-from` is added only if the legacy file exists.
3. `md-api` gets `-vault <vault>` instead of `-db`.
4. Unit tests for both argument builders (`cargo test`).

### D. Specs & Docs

- `md-indexer.sdd`: add "**MUST** limit writes inside a vault to `<vault>/.md_indexer/` (index database and its housekeeping files)."
- `specs/features/desktop-app/spec.md`: requirement 4 (Vault Database Isolation) updated to the vault-local location with migration note.
- `README.md` (usage, defaults, sync-exclusion hint for `.md_indexer/`), `apps/desktop/README.md`, `CHANGELOG.md` (changed default; `./index.db` is not migrated automatically, use `-db index.db` or re-index).

---

## 3. Verification Strategy

- **Go unit tests (`internal/vaultdb`):** writer uses local path and writes `.gitignore`; existing `.gitignore` untouched; read-only vault (`chmod 0555`, skipped when running as root) falls back and creates the fallback dir; reader prefers local, then fallback; reader error names both paths, wraps `ErrNoIndex` and creates nothing; `MigrateLegacy` moves db + `-wal` + `-shm`, does nothing when the target exists or the legacy file is missing; symlinked vault resolves to the same paths.
- **Go config test:** `DefaultConfig().DBPath == ""`; `MD_AI_DB_PATH` still wins.
- **Rust:** argument builder tests.
- **Manual:** CLI run against a copy of a real vault (writable and read-only via `chmod`), `md-search --vault`, `md-ai ask --retrieve-only --vault`; desktop app migration of an existing legacy DB (embeddings count before/after).
- `go vet ./...`, `go test ./...`, `ng test`, `cargo test`, `cargo clippy`.

## 4. Risks

- **Changed CLI default:** existing `./index.db` / `knowledge.db` setups silently get a new empty location unless `-db` is passed. Mitigation: CHANGELOG entry and README note; readers fail loudly instead of creating an empty DB.
- **Cross-device move:** the app data dir and the vault are often on different file systems; covered by the copy fallback.
- **Sync tools:** a vault-local SQLite file can still be synced by tools that do not ignore dot-folders; documented, out of scope to detect.

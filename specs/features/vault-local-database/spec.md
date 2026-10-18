<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Feature Specification: Vault-Local Index Database

**Issue:** [#23](https://github.com/Zheng-Bote/md_indexer-searcher/issues/23)  
**Status:** Implemented (`/speckit.implement`) · Decisions confirmed 2026-10-18

## 1. Feature Intent

The index database should live with the vault it belongs to, so it is easy to find, back up or delete, for the desktop app and the CLI tools alike.

Decisions (2026-10-18, issue #23):

- Default location: `<vault>/.md_indexer/md_indexer.db`.
- Fallback when the vault is not writable: the local user data folder.
- Applies to the desktop app **and** the CLI tools (`md-indexer`, `md-api`, `md-search`, `md-ai`). An explicitly given database path keeps working unchanged.

### Current State

| Component | Today's database location |
|---|---|
| Desktop app (`sidecar.rs` `database_path`) | `<app data>/vaults/<sha256(abs vault)[:16 hex]>.db` |
| `md-indexer` | `-db`, default `index.db` in the working directory; vault via `-dir` (default `.`) |
| `md-api`, `md-search` | `-db` / `--db`, default `index.db`; no notion of a vault |
| `md-ai` | `--db`, `db_path` (YAML), `MD_AI_DB_PATH`; default `knowledge.db` |

The readers (`md-api`, `md-search`, `md-ai`) do not know the vault, so they need a way to name it.

## 2. User Scenarios

- **Scenario 1 — CLI, Writable Vault:**
  `md-indexer -dir ~/Obsidian/Work` creates `~/Obsidian/Work/.md_indexer/md_indexer.db`. Afterwards `md-search --vault ~/Obsidian/Work search mqtt` and `md-api -vault ~/Obsidian/Work -ai` use that database without `-db`.
- **Scenario 2 — Read-Only Vault:**
  The vault is a read-only mount. `md-indexer` stores the database in the local data folder (`~/.local/share/md-indexer/vaults/<hash>.db` on Linux) and logs that the fallback is used. The readers find it there with the same `--vault`.
- **Scenario 3 — Explicit Path:**
  `md-indexer -dir V -db /tmp/test.db` and `md-ai --db /tmp/test.db` behave exactly as today.
- **Scenario 4 — Desktop App Upgrade:**
  A vault was indexed by an earlier app version into `<app data>/vaults/<hash>.db`. After the update the database (incl. `-wal`/`-shm`) is moved into `<vault>/.md_indexer/` once, without re-indexing or re-embedding.
- **Scenario 5 — Vault Under Git:**
  The vault is a git repository. The created `.md_indexer/` folder contains a `.gitignore` with `*`, so the database is never committed.
- **Scenario 6 — No Index Yet:**
  `md-search --vault ~/Notes search x` is run before the vault was indexed. The tool reports that no index exists for the vault and suggests running `md-indexer`; it does not create an empty database.

## 3. Requirements (EARS Syntax)

1. **Default Location:**
   **WHEN** a tool resolves the database for a vault and no explicit database path is given, **THE SYSTEM SHALL** use `<vault>/.md_indexer/md_indexer.db`, where `<vault>` is the absolute, symlink-resolved vault path.
2. **Writer Fallback:**
   **IF** `md-indexer` cannot create `<vault>/.md_indexer/` or write into it, **THEN THE SYSTEM SHALL** use `<user data dir>/md-indexer/vaults/<key>.db`, where `<key>` is the first 16 hex characters of the SHA-256 of the absolute vault path, and **SHALL** log the fallback location.
3. **User Data Directory:**
   **THE SYSTEM SHALL** use `$XDG_DATA_HOME` or `~/.local/share` on Linux, `~/Library/Application Support` on macOS and `%LocalAppData%` on Windows as `<user data dir>`.
4. **Reader Resolution:**
   **WHEN** `md-api`, `md-search` or `md-ai` resolve the database for a vault, **THE SYSTEM SHALL** use the vault-local database if it exists, otherwise the fallback database if it exists.
5. **No Index:**
   **IF** a reader finds neither database, **THEN THE SYSTEM SHALL** exit with an error naming the vault and both checked paths, and **SHALL NOT** create a database or folder.
6. **Vault Flag:**
   **THE SYSTEM SHALL** accept `-vault <dir>` (`md-api`) and `--vault <dir>` (`md-search`, `md-ai`), defaulting to the current working directory, consistent with `md-indexer -dir` (default `.`).
7. **Explicit Path Precedence:**
   **WHEN** a database path is given explicitly (`-db` / `--db`, `MD_AI_DB_PATH`, or `db_path` in the `md-ai` YAML config), **THE SYSTEM SHALL** use it unchanged and skip vault resolution.
8. **Git Protection:**
   **WHEN** `md-indexer` creates `<vault>/.md_indexer/`, **THE SYSTEM SHALL** write a `.gitignore` containing `*` into it, unless that file already exists.
9. **Index Exclusion:**
   **THE SYSTEM SHALL NOT** index or watch anything inside `<vault>/.md_indexer/` (already covered by skipping dot-folders in `ShouldSkipPath`; to be kept under test).
10. **Read-Only Vault Content:**
    **THE SYSTEM SHALL** write inside the vault only below `<vault>/.md_indexer/`; Markdown files stay read-only.
11. **Desktop Integration:**
    **WHEN** the desktop app starts the sidecars, **THE SYSTEM SHALL** pass the vault (`md-indexer -dir`, `md-api -vault`) instead of a database path, so the resolution rules above are implemented once in Go.
12. **Desktop Migration:**
    **WHEN** a legacy desktop database `<app data>/vaults/<hash>.db` exists and no database exists at the resolved location, **THE SYSTEM SHALL** move it there together with its `-wal` and `-shm` files before indexing starts.

## 4. Non-Goals

- Automatic migration of CLI databases at arbitrary explicit paths (e.g. `./index.db`); users pass `-db` to keep using them or re-index.
- Detecting or handling sync conflicts (Syncthing, Nextcloud, iCloud). The hidden folder makes exclusion easy; the README documents it.
- Sharing one database between several machines.
- Changing the database schema.

## 5. Scope & Affected Components

- New `internal/vaultdb` package: resolution for writer and readers, user data dir, fallback key, `.gitignore`, legacy move.
- `cmd/indexer/main.go`: default `-db` empty → resolve from `-dir`; optional `-migrate-from <legacy db>` used by the desktop app.
- `cmd/md-api/main.go`, `cmd/md-search/main.go`, `cmd/md-ai/main.go`, `internal/ai/config/config.go`: `vault` flag; default DB path empty → resolve.
- `apps/desktop/src-tauri/src/sidecar.rs`: pass `-dir` / `-vault` and `-migrate-from`; drop the local path derivation.
- `specs/features/desktop-app/spec.md` (requirement 4, "Vault Database Isolation") and `md-indexer.sdd` (vault write boundary).
- `README.md`, `apps/desktop/README.md`, `md-ai.example.yaml`, `CHANGELOG.md` (default location change is user-visible).

## 6. SpecDD Architecture Alignment (Drift Control)

- [x] **Architecture:** Layering unchanged; path resolution is configuration done in `cmd/` via a shared `internal` package.
- [x] **Architecture:** The desktop app still only passes arguments and talks to `md-api`; it no longer derives database paths.
- [x] **Architecture:** Feature affects architecture: `md-indexer.sdd` gets the rule that writes inside a vault are limited to `<vault>/.md_indexer/`.
- [x] **Read-Only Vault:** Markdown files are never written.
- [x] **Security:** No secrets involved; readers never create files.
- [x] **Data Model:** Schema unchanged; only the file location changes.
- [x] **Go Standards:** No global state; resolution is a pure function plus small file-system helpers, injected into `cmd/` wiring.
- [x] **Standards:** SPDX headers and English documentation.

## 7. Acceptance Criteria

- [x] Without `-db`, `md-indexer -dir V` writes `V/.md_indexer/md_indexer.db` and `V/.md_indexer/.gitignore`.
- [x] A read-only vault falls back to the user data dir with a log message.
- [x] `md-api -vault V`, `md-search --vault V`, `md-ai --vault V` open the same database; without an index they fail with a clear message and create nothing.
- [x] Explicit `-db` / `--db` / `MD_AI_DB_PATH` / `db_path` behave as before.
- [x] The desktop app uses the vault-local database and migrates an existing legacy database once, keeping embeddings.
- [x] Nothing inside `.md_indexer/` is indexed.
- [x] Tests: `go test ./...` (resolution, fallback, readers, gitignore, legacy move) and `cargo test` (sidecar arguments).
- [x] `README.md`, `apps/desktop/README.md`, `md-ai.example.yaml`, `CHANGELOG.md`, `md-indexer.sdd` and the desktop-app spec are up-to-date.

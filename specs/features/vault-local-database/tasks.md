<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Task Breakdown: Vault-Local Index Database

**Issue:** [#23](https://github.com/Zheng-Bote/md_indexer-searcher/issues/23)  
**Spec:** [spec.md](./spec.md) · **Plan:** [plan.md](./plan.md)

## Decisions

- [x] Location `<vault>/.md_indexer/md_indexer.db`, fallback in the user data dir; applies to desktop and CLI (2026-10-18).
- [x] `.md_indexer/.gitignore` with `*` is written by `md-indexer` (confirmed 2026-10-18).
- [x] Existing CLI databases (`./index.db`, `knowledge.db`) are not migrated automatically; `-db` keeps them usable (confirmed 2026-10-18).
- [x] Flags: `-vault` / `--vault` for readers, `-dir` stays for `md-indexer` (confirmed 2026-10-18).

## Package & Implementation Tasks

### Task 1: `internal/vaultdb`
- [x] `Vault`, `LocalPath`, `FallbackPath`, `UserDataDir`.
- [x] `ForWriter` (folder, `.gitignore`, writability probe, fallback).
- [x] `ForReader` (local -> fallback -> `ErrNoIndex`, no side effects).
- [x] `MigrateLegacy` (db, `-wal`, `-shm`; rename with cross-device copy fallback).
- [x] Tests as listed in the plan.

### Task 2: CLI Wiring (`cmd/indexer`, `cmd/md-api`, `cmd/md-search`, `cmd/md-ai`, `internal/ai/config`)
- [x] `md-indexer`: `-db` default empty, writer resolution, `-migrate-from`, log messages.
- [x] `md-api`: `-vault`, reader resolution when `-db` is empty.
- [x] `md-search`: `--vault`, reader resolution in `PersistentPreRunE`.
- [x] `md-ai`: `DefaultConfig().DBPath = ""`, `--vault`, resolution before `Validate`; config test updated.
- [x] `md-ai.example.yaml`: `db_path` commented out.

### Task 3: Desktop Sidecars (`apps/desktop/src-tauri/src/sidecar.rs`)
- [x] `legacy_database_path` without directory creation.
- [x] `indexer_args` / `api_args` builders; `-migrate-from` only if the legacy file exists; `md-api -vault`.
- [x] `cargo test` for the builders; `cargo clippy` clean.

### Task 4: Specs & Docs
- [x] `md-indexer.sdd`: vault write boundary rule.
- [x] `specs/features/desktop-app/spec.md`: requirement 4 updated.
- [x] `README.md`, `apps/desktop/README.md`, `CHANGELOG.md`.

### Task 5: Verification
- [x] `go vet ./...`, `go test ./...`, `cargo test`, `cargo clippy` pass (`ng test` not affected: no frontend change).
- [x] Manual CLI check (writable and read-only vault copy, readers with `--vault`, missing index creates nothing, explicit `-db`).
- [x] Migration check with a copy of the real legacy desktop DB across file systems (home -> tmpfs): 13 docs and 108 embeddings kept, legacy file removed. GUI run of the desktop app not done.
- [x] Set spec status to Implemented.

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
- [ ] `Vault`, `LocalPath`, `FallbackPath`, `UserDataDir`.
- [ ] `ForWriter` (folder, `.gitignore`, writability probe, fallback).
- [ ] `ForReader` (local -> fallback -> `ErrNoIndex`, no side effects).
- [ ] `MigrateLegacy` (db, `-wal`, `-shm`; rename with cross-device copy fallback).
- [ ] Tests as listed in the plan.

### Task 2: CLI Wiring (`cmd/indexer`, `cmd/md-api`, `cmd/md-search`, `cmd/md-ai`, `internal/ai/config`)
- [ ] `md-indexer`: `-db` default empty, writer resolution, `-migrate-from`, log messages.
- [ ] `md-api`: `-vault`, reader resolution when `-db` is empty.
- [ ] `md-search`: `--vault`, reader resolution in `PersistentPreRunE`.
- [ ] `md-ai`: `DefaultConfig().DBPath = ""`, `--vault`, resolution before `Validate`; config test updated.
- [ ] `md-ai.example.yaml`: `db_path` commented out.

### Task 3: Desktop Sidecars (`apps/desktop/src-tauri/src/sidecar.rs`)
- [ ] `legacy_database_path` without directory creation.
- [ ] `indexer_args` / `api_args` builders; `-migrate-from` only if the legacy file exists; `md-api -vault`.
- [ ] `cargo test` for the builders; `cargo clippy` clean.

### Task 4: Specs & Docs
- [ ] `md-indexer.sdd`: vault write boundary rule.
- [ ] `specs/features/desktop-app/spec.md`: requirement 4 updated.
- [ ] `README.md`, `apps/desktop/README.md`, `CHANGELOG.md`.

### Task 5: Verification
- [ ] `go vet ./...`, `go test ./...`, `ng test`, `cargo test` pass.
- [ ] Manual CLI check (writable and read-only vault copy, readers with `--vault`, missing index).
- [ ] Manual desktop migration check (legacy DB moved, embeddings kept).
- [ ] Set spec status to Implemented.

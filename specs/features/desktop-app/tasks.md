<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Task Breakdown: Desktop App (Tauri + Angular + Spartan-ng)

**Issue:** TBD  
**Spec:** [spec.md](./spec.md) · **Plan:** [plan.md](./plan.md)

## Open Decisions

- [x] **Root spec scope:** `md-indexer.sdd` now lists `apps/` as the client-app layer (consumes `md-api` only, no direct DB or vault access).
- [ ] **Issue number:** Create a GitHub issue and reference it in `spec.md`, `plan.md` and `tasks.md`.

## Package & Implementation Tasks

### Task 1: Model Listing Endpoint (`internal/ai/model`, `internal/ai/provider`, `internal/api/rest`, `cmd/md-api`)
- [x] Add `ModelInfo` to `internal/ai/model`.
- [x] Add `ModelLister` interface and implement `ListModels` on `OllamaProvider` (`GET /api/tags`) and `MockProvider`.
- [x] Add `WithModelLister` and `GET /api/ai/models` (`200` / `501` / `502`) in `internal/api/rest/server.go`.
- [x] Wire the lister in `cmd/md-api/main.go` when `-ai` is enabled.
- [x] Tests: `ListModels` against an `httptest` Ollama stub in `internal/ai/provider/provider_test.go`; all three status codes in `internal/api/rest/server_test.go`.
- [x] Document `GET /api/ai/models` in `openapi.yaml`.

### Task 2: Sidecar Readiness (`cmd/md-api`, `cmd/indexer`, `internal/repository`)
- [x] Add `-host` flag to `md-api` and `md-indexer` (default keeps current behaviour).
- [x] Enable WAL and `busy_timeout` in `NewSQLiteRepository` for plain file DSNs.
- [x] Test: concurrent write (indexer) and read (search) on one DB file without `SQLITE_BUSY`.

### Task 3: Tauri Shell (`apps/desktop/src-tauri`)
- [ ] Initialise Tauri v2 in `apps/desktop` (dev URL, frontend dist path, app identifier).
- [ ] Add plugins: shell, dialog, store, http (scope `http://127.0.0.1:*`).
- [ ] Configure `externalBin` for `md-indexer` and `md-api`; git-ignore `binaries/` and `target/`.
- [ ] Implement `SidecarManager` (start/stop, free ports, readiness polling, `backend-status` events, stderr tail, kill on window close).
- [ ] Implement commands `select_vault`, `apply_config`, `backend_status`.

### Task 4: Angular Frontend (`apps/desktop/src`)
- [ ] Add Spartan-ng helm components: button, select, textarea, card, badge, sonner.
- [ ] Implement `BackendService`, `SettingsService`, `ApiClient`.
- [ ] Implement `VaultPicker`, `ModelSelector`, `AskPanel`, `AnswerView` and the main layout.
- [ ] Restore saved settings on startup and auto-start the backend.
- [ ] Vitest unit tests for services (Tauri APIs mocked) and `AnswerView` rendering.

### Task 5: Build Integration (`Makefile`)
- [ ] Add `desktop-sidecars`, `desktop-dev` and `desktop` targets.

### Task 6: Verification, Documentation & Converge (`/speckit.converge`)
- [ ] Run `go test ./...`, `gofmt -l cmd/ internal/ examples/`, `ng build`, `ng test`, `cargo check`.
- [ ] Manual check against the test vault: select vault, index, switch model, ask question, switch vault and back, close app (no remaining processes).
- [ ] Update `README.md` (desktop section), `apps/desktop/README.md` and `CHANGELOG.md`.
- [ ] Verify SpecDD convergence against `md-indexer.sdd` and `specs/features/desktop-app/`.

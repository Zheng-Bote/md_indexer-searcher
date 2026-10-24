<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Task Breakdown: Robust Model Discovery in the Desktop App

**Issue:** [#36](https://github.com/Zheng-Bote/md_indexer-searcher/issues/36)  
**Spec:** [spec.md](./spec.md) · **Plan:** [plan.md](./plan.md)

## Decisions

- [x] Missing embedding model: warn only (2026-10-24).
- [x] Missing answer model: reset to md-api's default local model with notice; block only if the default is missing too (2026-10-24).

## Package & Implementation Tasks

### Task 1: AI layer (`internal/ai/provider`)
- [x] `LocalModelsError`; `Registry.ListModels` returns cloud models with it on local failure.
- [x] Registry test with an unreachable daemon.

### Task 2: API layer (`internal/api/rest`, `openapi.yaml`)
- [x] `/api/ai/models`: 200 with `local_error` for `*LocalModelsError`.
- [x] REST test; `openapi.yaml`.

### Task 3: Desktop logic (`model-options.ts`, `workspace.service.ts`, `api.types.ts`)
- [x] `sameModel`, `checkSelection` with tests.
- [x] `WorkspaceService`: notices, `localError`, `answerBlocked`, public guarded `refreshModels()`; tests.

### Task 4: Desktop UI (`model-selector.ts`, `ask-panel.ts`)
- [x] Reload button, notices, Ollama note.
- [x] Asking disabled while no answer model is available.

### Task 5: Docs & Verification
- [x] `README.md`, `CHANGELOG.md`.
- [x] `go vet ./...`, `go test ./...`, `npm test`, `ng build`, `cargo test`.
- [x] Manual API check: `md-api` with a real and an unreachable `-ollama-host` → 200; without Ollama only cloud models plus `local_error`.
- [ ] Manual GUI check of scenarios 1–5 (user).
- [x] Set spec status to Implemented.

<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Task Breakdown: Show Loaded Local Models

**Issue:** [#37](https://github.com/Zheng-Bote/md_indexer-searcher/issues/37)  
**Spec:** [spec.md](./spec.md) · **Plan:** [plan.md](./plan.md)

## Decisions

- [x] Only a "geladen" marker, no keep-alive time (2026-10-24).
- [x] Background refresh after each answer (2026-10-24).

## Package & Implementation Tasks

### Task 1: AI layer (`internal/ai/model`, `internal/ai/provider`)
- [x] `ModelInfo.Loaded`; `/api/ps` in `OllamaProvider.ListModels`.
- [x] Provider tests (marked, cloud never marked, `/api/ps` failure).

### Task 2: Desktop (`api.types.ts`, `model-options.ts`, `model-selector.ts`, `ask-panel.ts`)
- [x] `modelLabel` with "geladen"; embedding labels.
- [x] Refresh after an answer; tests.

### Task 3: Docs & Verification
- [x] `openapi.yaml`, `README.md`, `CHANGELOG.md`.
- [x] `go vet ./...`, `go test ./...`, `npm test`, `ng build`.
- [x] Manual API check: warm `bge-m3` → only `bge-m3:latest` has `loaded: true`.
- [x] GUI check of scenarios 1–3 (user, 2026-10-24).
- [x] Set spec status to Implemented.

<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Feature Specification: Show Loaded Local Models

**Issue:** [#37](https://github.com/Zheng-Bote/md_indexer-searcher/issues/37)  
**Status:** Implemented (`/speckit.implement`) · Decisions confirmed 2026-10-24

## 1. Feature Intent

Show which local Ollama models are currently loaded in memory, so the user sees which answer model responds without a cold start. On CPU-only machines loading a model is a noticeable part of the answer time (`qwen3:14b` ≈ 2 min per answer, benchmark 2026-10-18).

### Current State (verified on `main`)

- `OllamaProvider.ListModels` only calls `GET /api/tags`; `model.ModelInfo` has no loaded flag.
- Ollama reports loaded models via `GET /api/ps` (`name`, `expires_at`, `size_vram`, …; verified with Ollama 0.40.3).
- Since #36 the desktop app can reload the model list without restart (`WorkspaceService.refreshModels()`).

## 2. User Scenarios

- **Scenario 1 — Warm model:** After asking with `qwen3:14b`, the answer selector shows "qwen3:14b · 14.8B · geladen" without a manual reload.
- **Scenario 2 — Keep-alive expired:** After the keep-alive expired (or `ollama stop qwen3:14b`) and "Modelle neu laden", the marker is gone.
- **Scenario 3 — Embedding model:** While indexing or retrieval keeps `bge-m3` loaded, the embedding selector marks it as loaded.

## 3. Requirements (EARS Syntax)

1. **WHEN** `GET /api/ai/models` is answered, **THE SYSTEM SHALL** query the local Ollama daemon's `GET /api/ps` and mark each listed local model as loaded (`loaded: true`) if it appears there.
2. **IF** `GET /api/ps` fails, **THEN THE SYSTEM SHALL** still return the model list without `loaded` marks and **SHALL NOT** fail the request.
3. **THE SYSTEM SHALL** never mark cloud models (including `:cloud` models of the local daemon) as loaded.
4. **WHILE** a local model is loaded, the desktop app **SHALL** show a "geladen" marker next to it in the answer and embedding model selectors.
5. **WHEN** the model list is refreshed (#36), **THE SYSTEM SHALL** update the `loaded` marks.
6. **WHEN** an answer has been received, the desktop app **SHALL** reload the model list in the background, so the marks reflect the model just used without a manual refresh.

## 4. Non-Goals

- Remaining keep-alive time, memory usage or unloading models from the app.
- Periodic polling.

## 5. Decisions (2026-10-24)

1. **Only a "geladen" marker, no keep-alive time.** The remaining time goes stale while the list is open; the relevant question is only "answers now or loads first".
2. **Background refresh after each answer** (requirement 6).

## 6. Scope & Affected Components

- `internal/ai/model`: `ModelInfo.Loaded` (`json:"loaded,omitempty"`).
- `internal/ai/provider/ollama.go`: `/api/ps` query in `ListModels`.
- `openapi.yaml`: `loaded` in the model schema.
- `apps/desktop/src/app/core/api.types.ts`, `workspace/model-options.ts` (labels), `workspace/model-selector.ts` (embedding labels), `workspace/ask-panel.ts` (refresh after an answer).
- `README.md`, `CHANGELOG.md`.

## 7. SpecDD Architecture Alignment (Drift Control)

- [x] **Architecture:** Layering maintained; the Ollama protocol code stays in `internal/ai/provider`.
- [x] **Architecture:** Client app uses md-api over HTTP only.
- [x] **Architecture:** No change to `md-indexer.sdd` needed.
- [x] **Read-Only Vault:** Unchanged.
- [x] **Security:** No secrets involved.
- [x] **Data Model:** Unchanged (SQLite untouched).
- [x] **Go Standards:** No global state.
- [x] **Standards:** SPDX headers and English documentation.

## 8. Acceptance Criteria

- [x] After asking with `qwen3:14b`, the selector marks `qwen3:14b` as "geladen" without a manual refresh; other local models are not marked.
- [x] After the keep-alive expired (or `ollama stop`) and a refresh, the mark disappears.
- [x] A failing `/api/ps` does not break `GET /api/ai/models`.
- [x] Tests added/updated: `go test ./...` and `npm test` in `apps/desktop`.
- [x] CHANGELOG.md and README.md are up-to-date.

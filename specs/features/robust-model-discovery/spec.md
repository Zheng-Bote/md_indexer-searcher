<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Feature Specification: Robust Model Discovery in the Desktop App

**Issue:** [#36](https://github.com/Zheng-Bote/md_indexer-searcher/issues/36)  
**Status:** Implemented (`/speckit.implement`) · Decisions confirmed 2026-10-24

## 1. Feature Intent

The desktop app discovers its models dynamically, but reacts badly when the installed models change or the local Ollama daemon is down. Align it with `sdd_studio/apps/desk`: keep what works selectable, say clearly what is missing, and allow reloading the list.

### Current State (verified on `main`)

- `WorkspaceService.refreshModels()` calls `GET /api/ai/models` once whenever md-api becomes ready. It adopts md-api's default model (`active.model`) only if no answer model is saved; a saved model is never checked against the list.
- `Registry.ListModels` returns an error as soon as the local daemon's `GET /api/tags` fails; `/api/ai/models` then answers 502 and the selectors are empty, although the cloud models from `.env` would work.
- There is no way to reload the list without a backend restart (changing vault, embedding model or `.env`).
- Ollama treats `bge-m3` and `bge-m3:latest` as the same model; `/api/tags` always reports the tag.

## 2. User Scenarios

- **Scenario 1 — Removed answer model:** `qwen3:32b` was saved, then removed with `ollama rm`. On start the app switches to md-api's default `qwen3:14b` and keeps a notice "`qwen3:32b` nicht verfügbar – `qwen3:14b` wird verwendet" visible until the user picks a model. Asking works immediately.
- **Scenario 2 — Default missing too:** Neither the saved model nor `qwen3:14b` is installed. The notice says so, asking is disabled until an available model is selected.
- **Scenario 3 — Removed embedding model:** `bge-m3` is saved but no longer installed. A notice suggests `ollama pull bge-m3` or another model; the setting stays unchanged.
- **Scenario 4 — Ollama stopped:** Only cloud providers work. The selectors offer the cloud models and show "Ollama nicht erreichbar: …"; saved local models are kept untouched.
- **Scenario 5 — New model pulled:** After `ollama pull gemma3:12b` the user clicks "Modelle neu laden" and the model appears without restarting the app or the sidecars.

## 3. Requirements (EARS Syntax)

**Stale selection**

1. **WHEN** the model list has been loaded and the saved answer model is not contained in it, **THE SYSTEM SHALL** reset the answer model to md-api's default local model (`active.model`) and show a persistent notice (not only a toast) naming the missing and the replacement model.
2. **IF** md-api's default local model is not contained in the list either, **THEN THE SYSTEM SHALL NOT** reset the answer model, **SHALL** keep the notice visible and **SHALL** disable asking until the user selects an available model.
3. **WHEN** the model list has been loaded, the local daemon was reachable and the saved embedding model is not contained in it, **THE SYSTEM SHALL** show a persistent, actionable notice (e.g. "`bge-m3` nicht installiert – `ollama pull bge-m3` ausführen oder anderes Modell wählen") and **SHALL NOT** change the embedding model setting automatically.
4. **IF** the local daemon is unreachable, **THEN THE SYSTEM SHALL NOT** treat saved models of the local daemon as missing (no reset, no notice per requirements 1–3).
5. **THE SYSTEM SHALL NOT** select a cloud model automatically when resetting a missing model.

**Ollama unreachable**

6. **IF** the local Ollama daemon cannot be reached or returns an error, **THEN** `GET /api/ai/models` **SHALL** answer 200 with the enabled cloud models and a field `local_error` (string) describing the failure, instead of 502.
7. **WHILE** `local_error` is set, the desktop app **SHALL** show "Ollama nicht erreichbar: …" next to the model selectors and keep the cloud models selectable.

**Refresh**

8. **THE SYSTEM SHALL** offer a "Modelle neu laden" action next to the model selectors that calls `GET /api/ai/models` again without restarting the sidecars, applying requirements 1–7 to the result.
9. **WHILE** a refresh is running, **THE SYSTEM SHALL** disable the refresh action and keep the previous list visible.

**Matching**

10. **THE SYSTEM SHALL** compare Ollama model names with an implicit `:latest` tag (`bge-m3` equals `bge-m3:latest`).

## 4. Non-Goals

- Loaded-model markers (`/api/ps`) — #37.
- Capability-based classification (`/api/show`) — #38.
- Listing models before a vault is selected (needs md-api without a vault).
- Periodic polling; reloading is explicit.

## 5. Decisions (2026-10-24)

1. **Missing embedding model: warn only, no automatic change.** Switching restarts the sidecars and re-embeds the vault on the next index run; "Keins" would silently disable vector retrieval.
2. **Missing answer model: reset to md-api's default local model with a persistent notice, do not block asking**, except when the default is missing too (requirement 2).

## 6. Scope & Affected Components

- `internal/ai/provider/registry.go`: on local failure return the cloud models together with a `*LocalModelsError`.
- `internal/api/rest/server.go`: map `*LocalModelsError` to 200 with `local_error`.
- `openapi.yaml`: `AIModelsResponse.local_error`.
- `apps/desktop/src/app/core/api.types.ts`: `ModelsResponse.local_error`.
- `apps/desktop/src/app/workspace/model-options.ts`: pure selection check and name matching.
- `apps/desktop/src/app/workspace/workspace.service.ts`: apply the check, notices, public refresh.
- `apps/desktop/src/app/workspace/model-selector.ts`: notices, Ollama note, refresh button.
- `apps/desktop/src/app/workspace/ask-panel.ts`: disable asking while no answer model is available.
- `README.md`, `CHANGELOG.md`.

## 7. SpecDD Architecture Alignment (Drift Control)

- [x] **Architecture:** Layering maintained; model discovery stays in md-api, provider HTTP code stays in `internal/ai/provider`.
- [x] **Architecture:** Client app uses md-api over HTTP only; it does not call Ollama.
- [x] **Architecture:** No change to `md-indexer.sdd` needed.
- [x] **Read-Only Vault:** Unchanged.
- [x] **Security:** No secrets involved; `local_error` contains the local daemon's error only. No automatic switch to a cloud model.
- [x] **Data Model:** Unchanged.
- [x] **Go Standards:** No global state.
- [x] **Standards:** SPDX headers and English documentation.

## 8. Acceptance Criteria

- [x] With a saved answer model that is no longer installed, the app shows a persistent notice and uses md-api's default local model; asking works without manual action.
- [x] If md-api's default local model is missing too, the notice stays and asking is disabled until an available model is selected.
- [x] With a saved embedding model that is no longer installed, the app shows an actionable notice (`ollama pull …`) and does not change the setting.
- [x] With Ollama stopped, `GET /api/ai/models` returns 200 with cloud models and `local_error`; the app shows the note and cloud models stay selectable; saved local models are not reset.
- [x] After `ollama pull <model>`, "Modelle neu laden" shows the new model without restarting the app or the sidecars.
- [x] Tests added/updated: `go test ./...` and `npm test` / `cargo test` in `apps/desktop`.
- [x] CHANGELOG.md and README.md are up-to-date.

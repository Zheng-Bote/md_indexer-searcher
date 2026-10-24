<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Architecture Plan: Robust Model Discovery in the Desktop App

**Issue:** [#36](https://github.com/Zheng-Bote/md_indexer-searcher/issues/36)  
**Spec:** [spec.md](./spec.md)

## 1. Architectural Alignment (`md-indexer.sdd`)

- **AI Layer:** Only `Registry.ListModels` changes; the Ollama provider and the cloud providers are untouched.
- **API Layer:** `/api/ai/models` maps a typed partial-result error to 200; no new endpoint.
- **Client App:** Validation is a pure function in `model-options.ts`; `WorkspaceService` keeps the state. The app still talks to md-api only.
- **Cloud rule:** A reset only ever selects md-api's default local model (`provider: "ollama"`, `active.model`).

```
GET /api/ai/models
  Registry.ListModels
    local.ListModels ── ok ──> local + cloud models, nil
                    └─ err ──> cloud models, &LocalModelsError{Err}
  handler: nil -> 200 {models, active}
           *LocalModelsError -> 200 {models, active, local_error}
           other -> 502

WorkspaceService.refreshModels()   (backend ready, or "Modelle neu laden")
  -> listModels() -> checkSelection(settings, response) -> persist reset, set notices
```

---

## 2. Technical Design

### A. `internal/ai/provider/registry.go`

```go
// LocalModelsError reports that the local daemon could not list its models; the
// accompanying model list still holds the cloud models.
type LocalModelsError struct{ Err error }
```

`ListModels` keeps going on a local error, appends the cloud models and returns them with `&LocalModelsError{Err: err}` (partial result plus error, documented on the method).

### B. `internal/api/rest/server.go`

`errors.As(err, &localErr)` → `respondJSON(200, {models, active, local_error: localErr.Err.Error()})`; other errors stay 502. `openapi.yaml`: optional `local_error` string in `AIModelsResponse`.

### C. Desktop – `model-options.ts`

```ts
/** Ollama names without tag mean ":latest". */
export function sameModel(a: string, b: string): boolean

export interface SelectionCheck {
  answerModel: ModelRef | null;       // value to persist (reset or unchanged)
  answerNotice: string | null;
  answerBlocked: boolean;             // no available answer model at all
  embeddingNotice: string | null;
}
export function checkSelection(settings: AppSettings, response: ModelsResponse): SelectionCheck
```

Rules:
- Local daemon models (`provider === 'ollama'`, incl. `:cloud`) are only checked when `local_error` is absent (requirement 4). Cloud provider models are always checked: they come from `.env` and are listed even when Ollama is down.
- Answer: effective = saved ?? default (`{ollama, active.model}`). Saved is available → unchanged. Saved missing and default available → reset with notice. Saved null and default available → adopt silently (current first-start behaviour). Default missing → keep, notice, blocked.
- Embedding: saved non-null, local reachable and not among `embeddingModels()` → notice; never changed.

### D. Desktop – `WorkspaceService`

- New signals: `localError`, `answerNotice`, `embeddingNotice`, `answerBlocked`, `refreshing`.
- `refreshModels()` becomes public; guarded by `refreshing` (requirement 9); keeps the previous list on failure.
- `selectAnswerModel` clears `answerNotice`/`answerBlocked`; `selectEmbeddingModel` clears `embeddingNotice`.

### E. Desktop – UI

- `model-selector.ts`: "Modelle neu laden" button (disabled while refreshing or backend not ready); notices as `role="status"` texts under the respective selector; Ollama note when `localError`. `disabled` no longer depends on the local list only (cloud models stay selectable).
- `ask-panel.ts`: `canAsk` also requires `!answerBlocked()`; short hint pointing to the settings while blocked.

### F. Docs

`README.md` (desktop section: reload button, behaviour without Ollama), `CHANGELOG.md` (`Added`, `Changed`).

---

## 3. Verification Strategy

- **Go:** registry test with an unreachable daemon (cloud models + `*LocalModelsError`); REST test "Ollama unreachable via registry" → 200 with `local_error`; plain provider error stays 502.
- **Desktop unit:** `checkSelection` table (available, reset, blocked, cloud saved while Ollama down, embedding missing, `:latest` matching, local error suppresses checks); `WorkspaceService` refresh applies the reset and clears notices on selection.
- **Manual (user, GUI):** scenarios 1–5 with `examples/vault` (`ollama stop`/`systemctl stop ollama`, `ollama pull`).
- `go vet ./...`, `go test ./...`, `npm test`, `cargo test`.

## 4. Risks

- **Partial result + error** is less common in Go; documented on `ListModels`, the only caller is the REST handler.
- **Name normalisation** only adds `:latest`; other aliases (digests, registry prefixes) are not resolved.

<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Architecture Plan: Show Loaded Local Models

**Issue:** [#37](https://github.com/Zheng-Bote/md_indexer-searcher/issues/37)  
**Spec:** [spec.md](./spec.md)

## 1. Architectural Alignment (`md-indexer.sdd`)

- **AI Layer:** `OllamaProvider.ListModels` adds one request to `/api/ps`; the registry and the REST handler are unchanged.
- **Client App:** Labels only; the reload after an answer reuses `WorkspaceService.refreshModels()` from #36.

```
OllamaProvider.ListModels
  GET /api/tags  -> models (error -> return error, as before)
  GET /api/ps    -> loaded names (error -> ignored, no marks)
  local model in loaded names -> Loaded = true
```

---

## 2. Technical Design

### A. `internal/ai/model`

`ModelInfo.Loaded bool `json:"loaded,omitempty"``.

### B. `internal/ai/provider/ollama.go`

- `loadedModels(ctx) (map[string]bool, error)`: `GET /api/ps`, decode `models[].name`.
- `ListModels`: after `/api/tags`, `loaded, err := p.loadedModels(ctx)`; on error no marks (requirement 2); `Loaded = loaded[m.Name] && locality == local` (requirement 3).

### C. Desktop

- `api.types.ts`: `loaded?: boolean`.
- `model-options.ts`: `modelLabel(m)` = name, parameter size, "geladen" joined by " · "; used for local answer options and embedding options.
- `model-selector.ts`: embedding items use `modelLabel`.
- `ask-panel.ts`: after a successful answer `void this.workspace.refreshModels()` (requirement 6).

### D. Docs

`openapi.yaml` (`loaded`), `README.md`, `CHANGELOG.md`.

---

## 3. Verification Strategy

- **Go:** provider test with a test server serving `/api/tags` and `/api/ps` (loaded local model marked, `:cloud` model never marked); `/api/ps` returning 500 → list without marks, no error.
- **Desktop unit:** labels with and without `loaded`; ask panel triggers a refresh after an answer.
- **Manual:** `curl /api/ai/models` with a loaded model; GUI scenarios 1–3 (user).
- `go vet ./...`, `go test ./...`, `npm test`, `ng build`.

## 4. Risks

- One more request per listing; `/api/ps` is local and cheap.
- Ollama reports names with tag; `/api/tags` does too, so plain equality is enough.

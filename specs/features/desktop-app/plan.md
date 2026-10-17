<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Architecture Plan: Desktop App (Tauri + Angular + Spartan-ng)

**Issue:** TBD  
**Spec:** [spec.md](./spec.md)

## 1. Architectural Alignment (`md-indexer.sdd`)

- **Thin Client:** The desktop app contains no retrieval or indexing logic. It orchestrates the existing binaries and talks to `md-api` over HTTP. `knowledge.db` is only touched by the Go binaries.
- **Read-Only Source of Truth:** The vault directory is only read by `md-indexer`. The app never writes into the vault; databases live in the Tauri app data directory.
- **CGO-Free SQLite:** WAL and `busy_timeout` are set via `modernc.org/sqlite` DSN pragmas.
- **Clean Architecture & DI:** Model listing is added as an optional `ModelLister` interface (same pattern as `StreamingLLMProvider`), injected into `rest.Server` via a builder method.
- **No Global State:** Sidecar handles live in Tauri managed state, not in globals.

```
┌──────────────── Tauri app ─────────────────┐
│ Angular 22 + Spartan-ng (WebView)          │
│   │ invoke()            │ plugin-http fetch │
│   ▼                     ▼                   │
│ Rust: SidecarManager    http://127.0.0.1:P  │
│   ├─ spawn md-indexer -dir V -db D -host 127.0.0.1 -port Pi [-embed-model E]
│   └─ spawn md-api -db D -ai -host 127.0.0.1 -port Pa -model M -embedding-model E
└─────────────────────────────────────────────┘
            D = <appData>/vaults/<sha256(abs(V))[:16]>.db
```

---

## 2. Technical Design

### A. Go: Model Listing (`internal/ai/provider`, `internal/api/rest`, `cmd/md-api`)

1. **Provider (`provider.go`, `ollama.go`):**
   - Add `model.ModelInfo { Name string; Size int64; Family string }` in `internal/ai/model`.
   - Add optional interface `ModelLister { ListModels(ctx) ([]model.ModelInfo, error) }`.
   - Implement `OllamaProvider.ListModels` via `GET {host}/api/tags`, mapping `models[].name`, `size`, `details.family`.
   - Implement it on `MockProvider` for tests.
2. **REST (`server.go`):**
   - Add `WithModelLister(l provider.ModelLister, active ActiveModels) *Server`, where `ActiveModels` holds `Model` and `EmbeddingModel`.
   - `GET /api/ai/models`:
     - no lister → `501` (same message style as `/api/ai/ask`).
     - lister error → `502` with `"ollama not reachable at <host>"`.
     - success → `{"success": true, "data": {"models": [...], "active": {"model": "...", "embedding_model": "..."}}}`.
3. **Wiring (`cmd/md-api/main.go`):** Inside the `-ai` block, call `server.WithModelLister(prov, ...)`. The active embedding model is reported as empty when vectors are disabled.

### B. Go: Sidecar Readiness (`cmd/md-api`, `cmd/indexer`, `internal/repository`)

1. **Bind host:** Add `-host` flag (default `""` = all interfaces, current behaviour) to `md-api` and `md-indexer`; `Addr = host + ":" + port`. The desktop app always passes `127.0.0.1`.
2. **Concurrent access:** `NewSQLiteRepository` appends `_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)` to the DSN when it is a plain file path (no existing query string). Existing callers keep working.
3. **Readiness signal:** `md-indexer` starts its HTTP server only after `IndexAll` completes, so `GET http://127.0.0.1:Pi/health` responding is the "initial index done" signal. No Go change needed for this.

### C. Tauri Shell (`apps/desktop/src-tauri`)

1. **Setup:** Tauri v2 via `npm create tauri-app`-equivalent init inside `apps/desktop` (`frontendDist: ../dist/md-indexer-desktop/browser`, `devUrl: http://localhost:4200`).
2. **Plugins:** `tauri-plugin-shell` (sidecars), `tauri-plugin-dialog` (folder picker), `tauri-plugin-store` (settings), `tauri-plugin-http` (fetch scoped to `http://127.0.0.1:*`, avoids the missing CORS preflight handling in `md-api`).
3. **Sidecars:** `bundle.externalBin: ["binaries/md-indexer", "binaries/md-api"]`; the Makefile copies `bin/*` to `src-tauri/binaries/<name>-<target-triple>`.
4. **`SidecarManager` (Rust, managed state):**
   - `apply(vault, model, embedding_model)`: compute DB path, kill running sidecars, then in the background pick free ports, spawn `md-indexer`, poll its `/health`, spawn `md-api`, poll `/health`, and emit `backend-status` events (`indexing`, `starting`, `ready`, `error`). Every apply starts a new generation; work of superseded generations is discarded.
   - Any vault or model change restarts both sidecars. The indexer pass is incremental (hash check, missing embeddings only).
   - Without an embedding model, `md-indexer` runs without `-embed-model` and `md-api` with `-vectors=false`.
   - `stop()`: kill both children; also called on `WindowEvent::Destroyed`.
   - Forward the last N stderr lines per sidecar for the error view.
5. **Commands:** `apply_config(vault, model, embedding_model)`, `backend_status() -> BackendStatus`. The folder picker runs in the frontend via `@tauri-apps/plugin-dialog`.

### D. Angular Frontend (`apps/desktop/src`)

- Zoneless, standalone components, signals; Spartan-ng helm components (`button`, `select`, `textarea`, `card`, `badge`, `sonner`).
- **Services:**
  - `BackendService`: wraps Tauri `invoke`/`listen`, exposes `status` and `apiBaseUrl` as signals.
  - `SettingsService`: reads/writes `tauri-plugin-store` (`vault`, `model`, `embeddingModel`).
  - `ApiClient`: `listModels()`, `ask(question)` using `@tauri-apps/plugin-http` `fetch`.
- **Components:**
  - `VaultPicker`: current vault path + "Select vault" button.
  - `ModelSelector`: two selects (generation, embedding) fed by `/api/ai/models`; change → `apply_config`.
  - `AskPanel`: textarea + submit, busy state, disabled unless `status === 'ready'`.
  - `AnswerView`: Markdown answer rendered with `marked` and bound via `[innerHTML]` (Angular's built-in sanitizer), sources list, gaps list.
- **Bootstrapping a first model list:** On first start without saved models, `md-api` starts with the `md-ai` config defaults (`qwen3:14b`, `bge-m3`), and the user can change them afterwards.
- **Dev mode:** `ng serve` alone has no Tauri APIs; development runs through `npm run tauri dev`.

### E. Build (`Makefile`)

- `desktop-sidecars`: `go build` `md-indexer` and `md-api`, copy to `apps/desktop/src-tauri/binaries/` with the `rustc -vV` host triple suffix.
- `desktop-dev`: `desktop-sidecars` + `npm --prefix apps/desktop run tauri dev`.
- `desktop`: `desktop-sidecars` + `npm --prefix apps/desktop run tauri build`.
- `apps/desktop/src-tauri/binaries/` and `target/` are git-ignored.

## 3. Risks

- **First index duration:** With embeddings, a large vault can take ~10 minutes on CPU. The indexing state must make this visible; it is not an error.
- **Embedding model change:** Changing the embedding model leaves existing embeddings of the old model in the DB; `EmbedPending` computes the missing ones. Disk usage grows, which is acceptable for this iteration.
- **Long requests:** `/api/ai/ask` can take several minutes on CPU. The HTTP plugin request must not use a shorter timeout than `md-api -timeout` (600 s).

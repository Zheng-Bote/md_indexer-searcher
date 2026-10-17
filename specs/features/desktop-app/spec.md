<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Feature Specification: Desktop App (Tauri + Angular + Spartan-ng)

**Issue:** TBD  
**Status:** Specified (`/speckit.specify`)

## 1. Feature Intent

Provide a local desktop interface for `md-indexer` so that users can work with a Markdown vault without using the CLI:

1. **Vault Selection:** Pick a vault directory via a native folder dialog. The app indexes it into a per-vault SQLite database and serves it via `md-api`.
2. **LLM Selection:** Choose the generation model and the embedding model from the models available in the local Ollama instance.
3. **Grounded Q&A:** Ask questions against the selected vault and show the grounded answer with its sources and knowledge gaps.

The app is a thin client. All indexing, retrieval and generation stays in the existing Go binaries (`md-indexer`, `md-api`), which the app runs as Tauri sidecars.

## 2. User Scenarios

- **Scenario 1 — First Start / Vault Selection:**
  The user opens the app, clicks "Select vault" and picks `/home/user/Obsidian/Work`. The app starts `md-indexer` for that vault, shows an "Indexing…" state until the initial index is complete, then starts `md-api` and enables the question input.
- **Scenario 2 — Switching Vaults:**
  The user picks a different vault. The app stops both sidecars and restarts them with the new vault and its own database. The previous vault's database is kept, so switching back does not require a full re-index.
- **Scenario 3 — Model Selection:**
  The user opens the model selector and sees all models from the local Ollama instance (e.g. `qwen3:14b`, `bge-m3`). After choosing a generation or embedding model, the app restarts both sidecars. The indexer pass only re-parses changed notes and computes embeddings that are missing for the selected embedding model. Choosing no embedding model disables vector retrieval.
- **Scenario 4 — Asking a Question:**
  The user types "Wie funktioniert die Ingestion Pipeline?" and submits. The app shows a busy state (CPU generation can take minutes), then renders the Markdown answer, a list of cited source documents, and any detected knowledge gaps.
- **Scenario 5 — Ollama Not Reachable:**
  Ollama is not running. The model list request fails, and the app shows a clear error with the configured Ollama host instead of an empty selector.
- **Scenario 6 — Restart Persistence:**
  The user closes and reopens the app. The last vault, generation model and embedding model are restored and the sidecars start automatically.

## 3. Requirements (EARS Syntax)

1. **Model Listing Endpoint (`GET /api/ai/models`):**
   **WHEN** `md-api` runs with `-ai` and receives `GET /api/ai/models`, **THE SYSTEM SHALL** return the models installed in the configured Ollama instance (name, size, family) together with the active generation model and embedding model, in the standard `APIResponse` envelope.
2. **Model Listing Without AI:**
   **WHEN** `md-api` runs without `-ai` and receives `GET /api/ai/models`, **THE SYSTEM SHALL** respond with `501` and an `ErrorResponse`, consistent with `/api/ai/ask`.
3. **Ollama Failure:**
   **IF** Ollama cannot be reached while listing models, **THEN THE SYSTEM SHALL** respond with `502` and an `ErrorResponse` naming the Ollama host.
4. **Vault Database Isolation:**
   **WHEN** a vault is selected in the desktop app, **THE SYSTEM SHALL** use a database file in the app data directory that is derived deterministically from the absolute vault path, so each vault keeps its own index and embeddings.
5. **Sidecar Restart on Configuration Change:**
   **WHEN** the vault, generation model or embedding model changes, **THE SYSTEM SHALL** stop both sidecars and restart them with the new arguments, and **SHALL NOT** accept questions until `md-api` reports healthy on `/health`.
6. **Indexing State:**
   **WHILE** `md-indexer` is performing its initial index of the selected vault, **THE SYSTEM SHALL** show an indexing state and keep the question input disabled.
7. **Question Answering:**
   **WHEN** the user submits a question, **THE SYSTEM SHALL** send it to `POST /api/ai/ask` and render the answer as sanitized Markdown together with its sources and knowledge gaps.
8. **Settings Persistence:**
   **WHEN** the app starts, **THE SYSTEM SHALL** restore the last selected vault, generation model and embedding model.
9. **Sidecar Lifecycle:**
   **WHEN** the app window closes, **THE SYSTEM SHALL** terminate all sidecar processes it started.

## 4. Non-Goals (this iteration)

- Token streaming (`--stream`) in the UI. This is planned for a later iteration and will require an SSE variant of `/api/ai/ask`.
- Multi-turn chat sessions. Each question is answered independently.
- Editing or creating Markdown notes. The vault remains read-only.
- Remote or shared `md-api` instances. The app only manages local sidecars on `127.0.0.1`.
- Runtime vault or model switching inside `md-api`. Configuration changes are applied by restarting the sidecars.

## 5. Scope & Affected Components

- `internal/ai/provider/provider.go`, `internal/ai/provider/ollama.go`: Optional `ModelLister` interface and `ListModels` via Ollama `GET /api/tags`.
- `internal/api/rest/server.go`: `GET /api/ai/models` route.
- `cmd/md-api/main.go`: Wire the model lister when `-ai` is enabled; listen on `127.0.0.1` when `-host` is set.
- `cmd/indexer/main.go`: Accept the bind host so that the sidecar does not listen on all interfaces.
- `internal/repository/sqlite.go`: Enable WAL and `busy_timeout`, because `md-indexer` (writer) and `md-api` (reader) run concurrently on the same database.
- `openapi.yaml`: Document `GET /api/ai/models`.
- `apps/desktop/`: Angular 22 + Spartan-ng frontend and `src-tauri/` (Tauri v2) shell with sidecar management.
- `Makefile`: `desktop` targets that build the Go sidecars for the host target triple and run `tauri build`.

## 6. SpecDD Architecture Alignment (Drift Control)

- [ ] **Architecture:** Layering is unchanged. The desktop app consumes `md-api` over HTTP only and never opens `knowledge.db` directly.
- [ ] **Security:** Markdown vault files remain read-only (`md-indexer.sdd`). Sidecars bind to `127.0.0.1` only. Rendered answers are sanitized.
- [ ] **Data Model:** SQLite schema is unchanged; WAL mode is a connection setting and stays CGO-free (`modernc.org/sqlite`).
- [ ] **Standards:** SPDX headers (`Apache-2.0`) and English source documentation in all new Go, TypeScript and Rust files.
- [x] **Project Layout:** `md-indexer.sdd` lists `apps/` as the client-app layer that consumes `md-api` only.

## 7. Acceptance Criteria

- [ ] `GET /api/ai/models` returns installed Ollama models plus the active models, `501` without `-ai`, and `502` when Ollama is down; covered by tests in `internal/api/rest/server_test.go` and `internal/ai/provider/provider_test.go`.
- [ ] `md-indexer` and `md-api` can run concurrently on one database without `SQLITE_BUSY` errors.
- [ ] Selecting a vault in the app indexes it into its own database and enables questions once `md-api` is healthy.
- [ ] Switching the vault or a model restarts the affected sidecars; switching back to a previous vault reuses its database.
- [ ] Questions return a rendered answer with sources and knowledge gaps.
- [ ] Last vault and models are restored after an app restart.
- [ ] No sidecar process remains after closing the app.
- [ ] `go test ./...`, `ng build`, `ng test` and `cargo check` pass.
- [ ] `README.md`, `openapi.yaml` and `CHANGELOG.md` are up-to-date.

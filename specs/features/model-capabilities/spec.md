<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Feature Specification: Classify Local Models by Ollama Capabilities

**Issue:** [#38](https://github.com/Zheng-Bote/md_indexer-searcher/issues/38)  
**Status:** Implemented (`/speckit.implement`) · Decisions confirmed 2026-10-24

## 1. Feature Intent

Classify local Ollama models as embedding or generation models from Ollama's own metadata instead of name/family heuristics.

### Current State (verified on `main`)

- `ModelKind` (`internal/ai/provider/ollama.go`) marks a model as embedding model if its name contains `embed` or its family is `bert`/`nomic-bert`; everything else is a generation model. Embedding models of other families end up in the answer selector and are missing from the embedding selector.
- `POST /api/show` reports `capabilities` (verified with Ollama 0.40.3: `bge-m3` → `["embedding"]`, `qwen3:14b` → `["completion","tools","thinking"]`); `/api/tags` reports a `digest` per model.
- The desktop filters use `kind !== 'embedding'` (answer) and `kind !== 'generation'` (embedding), so a third kind value appears in both.

## 2. User Scenarios

- **Scenario 1 — Unusual embedding model:** An embedding model whose name lacks `embed` and whose family is not `bert` appears only in the embedding selector.
- **Scenario 2 — Older Ollama:** Without `capabilities` the classification stays as today.
- **Scenario 3 — Many models:** Repeated listings (reload, refresh after each answer) do not query `/api/show` again for unchanged models.

## 3. Requirements (EARS Syntax)

1. **WHEN** md-api lists local models, **THE SYSTEM SHALL** determine each model's kind from the `capabilities` reported by `POST /api/show`: `embedding` → embedding model, `completion` → generation model.
2. **IF** `/api/show` fails or reports no `capabilities` (older Ollama), **THEN THE SYSTEM SHALL** fall back to the current `ModelKind` name/family rules for that model.
3. **THE SYSTEM SHALL** cache the capabilities per model name and digest, so a repeated `GET /api/ai/models` does not call `/api/show` again for unchanged models.
4. **THE SYSTEM SHALL** bound the `/api/show` calls of one listing (concurrency and timeout) so `GET /api/ai/models` stays responsive with many installed models.
5. **THE SYSTEM SHALL NOT** query `/api/show` for `:cloud` models of the local daemon (they are generation models listed as cloud).
6. **IF** a model reports both `completion` and `embedding`, **THEN THE SYSTEM SHALL** offer it in both the answer-model and the embedding-model selector.
7. **THE SYSTEM SHALL** keep the capabilities internal to md-api and **SHALL NOT** add them to the `GET /api/ai/models` response.

## 4. Non-Goals

- Exposing `tools`/`thinking`/`vision` capabilities.
- Changing how cloud provider models are classified (always generation).

## 5. Decisions (2026-10-24)

1. **Capabilities stay internal.** No feature needs them yet; a `capabilities` field can be added later without breaking clients.
2. **Models with both capabilities appear in both selectors** via the new kind value `both`.

## 6. Scope & Affected Components

- `internal/ai/model/types.go`: `KindBoth = "both"`.
- `internal/ai/provider/ollama.go`: `/api/show` with cache (per provider instance, mutex), bounded concurrency and timeout; fallback to `ModelKind`.
- `openapi.yaml`: `kind` enum gains `both`; description.
- `apps/desktop/src/app/core/api.types.ts`: `kind` type; `workspace/model-options.spec.ts`: case for `both`.
- `README.md`, `CHANGELOG.md`.

## 7. SpecDD Architecture Alignment (Drift Control)

- [x] **Architecture:** Layering maintained; Ollama protocol code stays in `internal/ai/provider`.
- [x] **Architecture:** Client app uses md-api over HTTP only.
- [x] **Architecture:** No change to `md-indexer.sdd` needed.
- [x] **Read-Only Vault:** Unchanged.
- [x] **Security:** No secrets involved.
- [x] **Data Model:** Unchanged (SQLite untouched).
- [x] **Go Standards:** The cache lives in the provider instance (no global state).
- [x] **Standards:** SPDX headers and English documentation.

## 8. Acceptance Criteria

- [x] An embedding model without `embed` in its name and outside `bert`/`nomic-bert` appears only in the embedding selector (test with a test server).
- [x] `qwen3:14b` and `bge-m3` keep their current classification.
- [x] With `/api/show` unavailable, the classification falls back to the current rules.
- [x] A second `GET /api/ai/models` does not call `/api/show` again (test with a counting test server).
- [x] A model reporting both `completion` and `embedding` is offered in both selectors.
- [x] Tests added/updated: `go test ./...` and `npm test` in `apps/desktop`.
- [x] CHANGELOG.md and README.md are up-to-date.

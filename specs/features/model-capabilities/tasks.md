<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Task Breakdown: Classify Local Models by Ollama Capabilities

**Issue:** [#38](https://github.com/Zheng-Bote/md_indexer-searcher/issues/38)  
**Spec:** [spec.md](./spec.md) · **Plan:** [plan.md](./plan.md)

## Decisions

- [x] Capabilities stay internal (2026-10-24).
- [x] Models with both capabilities appear in both selectors (`kind: both`) (2026-10-24).

## Package & Implementation Tasks

### Task 1: AI layer (`internal/ai/model`, `internal/ai/provider`)
- [x] `KindBoth`; digest in `/api/tags`; `/api/show` with cache, bounded concurrency and timeout; `ModelKind` fallback.
- [x] Provider tests (classification, `:cloud` skipped, fallback, cache, digest change).

### Task 2: Desktop (`api.types.ts`, `model-options.spec.ts`)
- [x] `kind` type with `both`; test for both selectors.

### Task 3: Docs & Verification
- [x] `openapi.yaml`, `README.md`, `CHANGELOG.md`.
- [x] `go vet ./...`, `go test ./...`, `npm test`, `ng build`.
- [x] Manual API check against local Ollama: kinds unchanged (`qwen3:32b`, `gemma3:12b`, `qwen3:14b` generation, `bge-m3` embedding); first listing 0.09 s, cached listing < 0.01 s.
- [x] GUI check (user, confirmed 2026-10-24).
- [x] Set spec status to Implemented.

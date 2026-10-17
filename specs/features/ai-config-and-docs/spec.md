<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Feature Spec: AI Configuration Loading, Runtime Budget, and Hybrid GraphRAG Documentation

## 1. Problem & Goal
1. `md-ai` defines YAML tags on `config.Config`, but never loads `~/.config/md-indexer/ai.yaml` or `./md-ai.yaml`.
2. `md-api` only accepts `-model` and `-ollama-host` and ignores `MD_AI_*` environment variables, YAML config, and retriever/vector/timeout flags.
3. On CPU-only inference (~28 tokens/s prefill), the previous default `MaxContextTokens: 16000` causes ~10–15 min latency per query when many sections match. Lowering the default context budget to `4000` tokens keeps answers fast while preserving full configurability.
4. Deliverables 13 & 14 from `data/prompt.md` (example configuration `md-ai.example.yaml`, per-query-class walkthroughs in `docs/`, architecture & ER diagrams in `README.md`, and `CHANGELOG.md` entries for `#12`–`#18`) need to be completed.

## 2. Scope & Acceptance Criteria

### 2.1 YAML & Environment Configuration (`internal/ai/config`)
*   `Config.LoadFromFile(path string) error` parses a YAML file into `Config` via `gopkg.in/yaml.v2`.
*   `Config.LoadDefaultFiles()` loads `~/.config/md-indexer/ai.yaml` (if present) followed by `./md-ai.yaml` (if present), before `LoadFromEnv()` and CLI flags.
*   `cmd/md-ai` supports `--config <path>` (or `MD_AI_CONFIG`) in addition to automatic discovery.
*   `LoadFromEnv()` also supports `MD_AI_TOP_K_FTS`, `MD_AI_TOP_K_VECTOR`, `MD_AI_MIN_VECTOR_SCORE`, `MD_AI_FUSION_FTS_WEIGHT`, `MD_AI_FUSION_VECTOR_WEIGHT`, `MD_AI_FUSION_GRAPH_WEIGHT`, and `MD_AI_FUSION_ENTITY_WEIGHT`.
*   `DefaultConfig().MaxContextTokens` is lowered to `4000` for responsive local CPU operation.

### 2.2 `cmd/md-api` AI Wiring
*   `cmd/md-api` loads `aiCfg.LoadDefaultFiles()` and `aiCfg.LoadFromEnv()` and exposes flags (`-ai-config`, `-ollama-host`, `-model`, `-retriever`, `-vectors`, `-embedding-model`, `-max-context-tokens`, `-timeout`).

### 2.3 Deliverables & Documentation
*   `md-ai.example.yaml` at repository root with documented settings.
*   `docs/Hybrid_GraphRAG_DE.md` documenting architecture, pipeline stages, configuration, and concrete walkthroughs for all 5 query classes (`fact`, `entity`, `impact`, `architecture`, `exploration`).
*   `README.md` updated with `section_embeddings`/`fts_sections` in the ER diagram and the Hybrid GraphRAG architecture diagram.
*   `CHANGELOG.md` updated with `[Unreleased]` covering PRs `#12`–`#17` and the Stage e / config enhancements.

## 3. Tasks
*   [x] Task 1: Add YAML file loading, extra env vars, and `4000` default context budget in `internal/ai/config`.
*   [x] Task 2: Wire YAML/env/flags into `cmd/md-ai` and `cmd/md-api`.
*   [x] Task 3: Create `md-ai.example.yaml` and `docs/Hybrid_GraphRAG_DE.md`, and update `README.md` and `CHANGELOG.md`.
*   [x] Task 4: Add unit tests in `internal/ai/config/config_test.go` and verify all tests pass.

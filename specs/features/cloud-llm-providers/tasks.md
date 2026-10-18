<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Task Breakdown: Local Model Filtering and Cloud LLM Providers

**Issue:** [#24](https://github.com/Zheng-Bote/md_indexer-searcher/issues/24)  
**Spec:** [spec.md](./spec.md) · **Plan:** [plan.md](./plan.md)

## Decisions

- [x] Own `.env` in the `ai_txt-system` format; providers `OLLAMA` (Cloud), `OPENROUTER`, `NVIDIA`, `CLAUDE`, `GEMINI`; Groq dropped; key `none` disables (2026-10-18).
- [x] Gemini via Google's OpenAI-compatible endpoint (confirmed in Google's docs, beta) (2026-10-18).
- [x] No size filter for local models; size label + sort; filter only embedding/generation/cloud mismatches (2026-10-18).
- [x] Claude effort fixed in configuration, default `medium` (2026-10-18).
- [x] Per-request routing with a provider registry in `md-api`; answer-model changes no longer restart the sidecars (plan, 2026-10-18).
- [x] Claude server-side refusal fallback (`fallbacks: "default"`) enabled; the answering model is reported (confirmed 2026-10-18).

## Package & Implementation Tasks

### Task 1: Configuration (`internal/ai/config`)
- [x] `.env` loader (path resolution, parsing, precedence, permission warning).
- [x] Provider configs (`none`/empty disabled, default endpoints, `<P>_LLM_xx` order, unknown prefixes ignored).
- [x] Config fields: cloud context budget, cloud max tokens, cloud timeout, Claude effort (+ per model), `env_file`.
- [x] Tests.

### Task 2: Providers (`internal/ai/model`, `internal/ai/provider`)
- [x] `ModelInfo` (provider, locality, kind, parameter size); `Answer` (provider, locality).
- [x] Ollama: classification and parameter size in `ListModels`; Bearer key and `/api/chat` mode for Ollama Cloud.
- [x] OpenAI-compatible provider (streaming SSE, usage, error mapping) for OpenRouter, NVIDIA, Gemini.
- [x] Claude provider (`anthropic-sdk-go`, streaming, effort, refusal/max_tokens handling, optional server-side fallback).
- [x] Typed provider errors.
- [x] Registry (`List`, `Resolve`).
- [x] Tests with `httptest` servers for every provider.

### Task 3: Assistant, API, CLI (`internal/ai/assistant`, `internal/api/rest`, `cmd/md-api`, `cmd/md-ai`)
- [x] `Ask` with model reference; budgets by locality; actual model in the answer.
- [x] `POST /api/ai/ask` `model` field; error JSON with `error_kind`; `GET /api/ai/models` from the registry.
- [x] `md-api -env-file`, `md-ai --env-file --provider`; provider summary log without keys; CLI cloud notice.
- [x] Tests (routing, no fallback, keys absent from responses).
- [x] `openapi.yaml`.

### Task 4: Desktop (`apps/desktop`)
- [x] Settings: `answerModel {provider,name}` with migration from `model`, `envFile`.
- [x] Sidecars: `MD_AI_ENV_FILE` as environment; answer model removed from `md-api` args; no restart on answer-model change.
- [x] Model selector: grouped/filtered/sorted lists with size labels; `.env` path setting.
- [x] Ask panel: `model` per request, consent once per app run, provider-specific errors; answer badge.
- [x] Tests (`ng test`, `cargo test`).

### Task 5: Docs & Specs
- [x] `example.env`, `md-ai.example.yaml`, `README.md`, `CHANGELOG.md`.
- [x] `md-indexer.sdd` (cloud provider rules).
- [x] `docs/architektur_erweiterung.md` (4.1, 5.2 Stage 1, 14).

### Task 6: Verification
- [x] `go vet ./...`, `go test ./...`, `ng test`, `ng build`, `cargo test`, `cargo clippy`.
- [x] Manual `md-api` run on the sample vault (`examples/vault`) with a test `.env` (invalid keys): model list classified (local/cloud, generation/embedding, parameter size), `none`/`GROQ` ignored, permission warning, no key in `ps`/logs/responses; real endpoints of OpenRouter, Claude and Gemini return `auth` (502); unknown model 400; local `qwen3:14b` answer via `model` with `provider`/`locality`.
- [x] Manual question per provider with valid keys (user's `.env`, sample vault, 2026-10-18): Ollama Cloud `gpt-oss:120b` 1.7 s, Gemini `gemini-3.8-flash` 47 s, OpenRouter `liquid/lfm-2.5-2.6b:free` 2.0 s, NVIDIA `meta/llama-3.2-90b-vision-instruct` 19 s — all `locality: cloud` with sources. Outdated example IDs (OpenRouter `deepseek/deepseek-r1:free`, NVIDIA `meta/llama-3.3-70b-instruct`) were reported as `unavailable` with the provider's message; the free reasoning model `nvidia/nemotron-3.5-lightning:free` exceeded the 180 s cloud timeout.
- [ ] Claude with a valid key (no key available).
- [ ] GUI run of the desktop app.
- [x] Set spec status to Implemented.

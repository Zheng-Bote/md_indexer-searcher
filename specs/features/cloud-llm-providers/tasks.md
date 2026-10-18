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
- [ ] Claude server-side refusal fallback (`fallbacks: "default"`) enabled with the answering model reported — confirm or decline.

## Package & Implementation Tasks

### Task 1: Configuration (`internal/ai/config`)
- [ ] `.env` loader (path resolution, parsing, precedence, permission warning).
- [ ] Provider configs (`none`/empty disabled, default endpoints, `<P>_LLM_xx` order, unknown prefixes ignored).
- [ ] Config fields: cloud context budget, cloud max tokens, cloud timeout, Claude effort (+ per model), `env_file`.
- [ ] Tests.

### Task 2: Providers (`internal/ai/model`, `internal/ai/provider`)
- [ ] `ModelInfo` (provider, locality, kind, parameter size); `Answer` (provider, locality).
- [ ] Ollama: classification and parameter size in `ListModels`; Bearer key and `/api/chat` mode for Ollama Cloud.
- [ ] OpenAI-compatible provider (streaming SSE, usage, error mapping) for OpenRouter, NVIDIA, Gemini.
- [ ] Claude provider (`anthropic-sdk-go`, streaming, effort, refusal/max_tokens handling, optional server-side fallback).
- [ ] Typed provider errors.
- [ ] Registry (`List`, `Resolve`).
- [ ] Tests with `httptest` servers for every provider.

### Task 3: Assistant, API, CLI (`internal/ai/assistant`, `internal/api/rest`, `cmd/md-api`, `cmd/md-ai`)
- [ ] `Ask` with model reference; budgets by locality; actual model in the answer.
- [ ] `POST /api/ai/ask` `model` field; error JSON with `error_kind`; `GET /api/ai/models` from the registry.
- [ ] `md-api -env-file`, `md-ai --env-file --provider`; provider summary log without keys; CLI cloud notice.
- [ ] Tests (routing, no fallback, keys absent from responses).
- [ ] `openapi.yaml`.

### Task 4: Desktop (`apps/desktop`)
- [ ] Settings: `answerModel {provider,name}` with migration from `model`, `envFile`.
- [ ] Sidecars: `MD_AI_ENV_FILE` as environment; answer model removed from `md-api` args; no restart on answer-model change.
- [ ] Model selector: grouped/filtered/sorted lists with size labels; `.env` path setting.
- [ ] Ask panel: `model` per request, consent once per app run, provider-specific errors; answer badge.
- [ ] Tests (`ng test`, `cargo test`).

### Task 5: Docs & Specs
- [ ] `example.env`, `md-ai.example.yaml`, `README.md`, `CHANGELOG.md`.
- [ ] `md-indexer.sdd` (cloud provider rules).
- [ ] `docs/architektur_erweiterung.md` (4.1, 5.2 Stage 1, 14).

### Task 6: Verification
- [ ] `go vet ./...`, `go test ./...`, `ng test`, `ng build`, `cargo test`, `cargo clippy`.
- [ ] Manual question per configured provider with the user's `.env`; no key in `ps`, logs or responses.
- [ ] Set spec status to Implemented.

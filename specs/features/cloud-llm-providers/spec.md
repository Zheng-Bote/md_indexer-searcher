<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Feature Specification: Local Model Filtering and Cloud LLM Providers

**Issue:** [#24](https://github.com/Zheng-Bote/md_indexer-searcher/issues/24)  
**Status:** Specified (`/speckit.specify`) · Decisions confirmed 2026-10-18  
**Architecture context:** `docs/architektur_erweiterung.md` (4.1, 4.2, 5.1, 5.2 Stage 1, 12)

## 1. Feature Intent

The desktop app offers one model choice for answers that contains

- the **local** Ollama models that can actually generate answers, and
- **cloud** models from the providers configured in md_indexer's own `.env`: **Ollama Cloud**, **OpenRouter**, **NVIDIA NIM**, **Claude** and **Gemini**.

Local CPU generation takes minutes (2026-10-18 benchmark: `qwen3:14b` ≈ 2 min per answer); cloud models answer in seconds and allow larger contexts. Retrieval and embeddings (`bge-m3`) always stay local; only answer generation goes to the selected cloud model. This is Stage 1 of the hybrid extension: the user chooses the model explicitly; `auto_context`, `cloud_only` and the comparison run are out of scope.

### Current State (verified on `main`)

- **One provider:** `md-api` and `md-ai` build a single `OllamaProvider` at startup (`cmd/md-api/main.go`, `cmd/md-ai/main.go`); the model comes from `-model` / `model`. The desktop app restarts the sidecars when the model changes.
- **Model list:** `GET /api/ai/models` returns Ollama's `/api/tags` as `{name, size, family}` plus the active models. The desktop shows **every** installed model in both selectors, e.g. `bge-m3` (family `bert`) as answer model and `qwen3:14b` as embedding model.
- **No `.env` support:** there is no loader; keys can only come from the process environment, and no provider reads them. `.gitignore` already ignores `.env` and `.env.*`.
- **Context budget:** one `MaxContextTokens` (default 4,000, tuned for local CPU) for every model.
- **Redaction:** `contextbuilder.RedactSecrets` already runs on every context block before generation.

## 2. User Scenarios

- **Scenario 1 — Clean Local Lists:** Installed are `qwen3:14b`, `qwen3:32b`, `gemma3:12b` and `bge-m3`. "Antwort-Modell" shows `qwen3:32b · 32B`, `qwen3:14b · 14B`, `gemma3:12b · 12B` (sorted by size); "Embedding-Modell" shows `bge-m3` only.
- **Scenario 2 — Cloud Models from `.env`:** The `.env` sets `CLAUDE_API_KEY=sk-ant-…` with `CLAUDE_LLM_01=claude-opus-5-5`, `OPENROUTER_API_KEY=…` with two models, and `GEMINI_API_KEY=none`. The selector shows a "Cloud" group with Claude and OpenRouter models; Gemini, NVIDIA and Ollama Cloud do not appear.
- **Scenario 3 — Asking a Cloud Model:** The user selects `claude-sonnet-5-5`. On the first cloud question of the session a notice explains that excerpts of the vault are sent to Anthropic; after confirming, the answer arrives in seconds with sources and a "Cloud · Claude" badge.
- **Scenario 4 — Ollama `:cloud` Model:** The local daemon also lists a cloud model (e.g. `gpt-oss:120b-cloud`). It appears in the cloud group, not among the local models, and triggers the same notice.
- **Scenario 5 — Errors:** The OpenRouter key is invalid → "OpenRouter: API-Key ungültig (401)". A free model is rate limited → "OpenRouter: Rate-Limit erreicht (429), bitte später erneut versuchen". Claude declines a request → "Claude hat die Anfrage abgelehnt (refusal: <category>)". In no case is another provider or a local model used silently.
- **Scenario 6 — No `.env`:** Without a `.env` the app behaves exactly as today with filtered local lists.

## 3. Requirements (EARS Syntax)

**Local models**

1. **Answer-Model List:**
   **WHEN** the model list is requested, **THE SYSTEM SHALL** offer as local answer models only installed Ollama models that are not embedding models.
2. **Embedding-Model List:**
   **THE SYSTEM SHALL** offer as embedding models only models recognised as embedding models (family `bert` or `nomic-bert`, or `embed` in the name).
3. **Locality:**
   **THE SYSTEM SHALL** classify a model as local only if it is served by an Ollama host on `localhost`/`127.0.0.1`/`::1` and is not a cloud model of the daemon (tag ending in `cloud`, or reported as remote by Ollama); all other models are cloud models.
4. **Size Display:**
   **THE SYSTEM SHALL** show the parameter size reported by Ollama (`details.parameter_size`) next to each local model and sort local answer models by size, largest first; no model is hidden because of its size.
5. **Defaults:**
   **THE SYSTEM SHALL** keep `qwen3:14b` as the default answer model and `bge-m3` as the default embedding model.

**Configuration (`.env`)**

6. **`.env` Loading:**
   **WHEN** `md-api` or `md-ai` start, **THE SYSTEM SHALL** read md_indexer's `.env` from the path given by `-env-file` / `--env-file`, else `MD_AI_ENV_FILE`, else `~/.config/md-indexer/.env`, and apply the precedence `process environment > .env > md-ai.yaml > defaults`.
7. **Format:**
   **THE SYSTEM SHALL** read per provider `<PROVIDER>_API_KEY`, optional `<PROVIDER>_ENDPOINT` and `<PROVIDER>_LLM_01 … _LLM_nn` for `OLLAMA` (Ollama Cloud), `OPENROUTER`, `NVIDIA`, `CLAUDE` and `GEMINI`; other providers (e.g. `GROQ`) are ignored.
8. **Disabled Providers:**
   **IF** a provider's API key is missing, empty or `none` (case-insensitive), **THEN THE SYSTEM SHALL** not offer that provider or its models.
9. **File Permissions:**
   **IF** the `.env` is readable by group or others, **THEN THE SYSTEM SHALL** log a warning naming the file (not its content).

**Providers**

10. **OpenAI-Compatible Provider:**
    **THE SYSTEM SHALL** provide one provider for OpenAI-compatible `/chat/completions` APIs (`Authorization: Bearer`, streaming, `usage`) and use it for OpenRouter, NVIDIA NIM and Gemini (Google's OpenAI-compatible endpoint).
11. **Ollama Cloud:**
    **WHERE** `OLLAMA_API_KEY` is set, **THE SYSTEM SHALL** call `OLLAMA_ENDPOINT` with the key as bearer token through the existing Ollama provider.
12. **Claude Provider:**
    **THE SYSTEM SHALL** provide a Claude provider that calls the Anthropic Messages API through the official Go SDK (`github.com/anthropics/anthropic-sdk-go`) with the key from `CLAUDE_API_KEY`, streams answers, uses a configured effort per model (default `medium`), and **SHALL** report `stop_reason: "refusal"` as an error with its category instead of an empty answer.
13. **Routing:**
    **WHEN** the user has selected a model, **THE SYSTEM SHALL** answer every following question with the provider that owns that model.
14. **Context Budget:**
    **THE SYSTEM SHALL** use a separate, configurable context budget for cloud models (default `16000` tokens) and keep `max_context_tokens` (default `4000`) for local models.
15. **No Silent Fallback:**
    **IF** a cloud call fails (authentication, HTTP 429, timeout, refusal, network), **THEN THE SYSTEM SHALL** return a specific error naming the provider and cause, and **SHALL NOT** retry with another provider or a local model.

**Security & transparency**

16. **Key Handling:**
    **THE SYSTEM SHALL** pass API keys to sidecars only via environment variables and **SHALL NOT** put them into command-line arguments, logs, API responses or the desktop settings file.
17. **Redaction:**
    **THE SYSTEM SHALL** keep applying `RedactSecrets` to every context block before it is sent to any provider.
18. **Cloud Marking:**
    **WHILE** a cloud model is selected, **THE SYSTEM SHALL** mark it as "Cloud · <Provider>" in the selector and on the answer.
19. **Consent:**
    **WHEN** the first question of an app session would be sent to a cloud model, **THE SYSTEM SHALL** show a notice naming the provider and stating that vault excerpts leave the machine, and send the question only after confirmation; the confirmation lasts until the app is closed.
20. **Answer Metadata:**
    **THE SYSTEM SHALL** include `provider` and `locality` (`local` | `cloud`) in the answer of `POST /api/ai/ask`.

## 4. Non-Goals

- `auto_context`, `cloud_only`, `local_only` enforcement modes, the comparison run (`/api/ai/compare`) and cost limits (architecture RFC, later stages).
- Groq.
- Cloud embeddings; embeddings stay local with `bge-m3`.
- A UI control for Claude effort (fixed per model in configuration).
- Editing the `.env` from the desktop app (only its path is configurable there).
- Remote access to `md-api` (blocker 4.3).

## 5. Open Points for `/speckit.plan`

- Verify Gemini's OpenAI-compatible endpoint (URL, streaming, `usage`) against current Google documentation; switch to the native API only if something essential is missing.
- Verify how Ollama marks cloud models in `/api/tags` (tag suffix vs. remote fields) for requirement 3.
- Routing mechanism: keep "restart `md-api` with the selected provider/model" (today's pattern) or pass the model per request. Requirement 13 only fixes the behaviour.

## 6. Scope & Affected Components

- **AI layer:** `internal/ai/provider/` (new `openai_compatible.go`, `claude.go`; bearer key in `ollama.go`; parameter size and locality in the model list), `internal/ai/model/types.go` (`ModelInfo` with provider, locality, parameter size; `Answer` with provider and locality), `internal/ai/config/` (`.env` loader, provider registry, cloud context budget, per-model effort).
- **API / CLI:** `cmd/md-api/main.go`, `cmd/md-ai/main.go` (`-env-file`, provider registry, routing), `internal/api/rest/server.go` (model list grouping, error mapping), `openapi.yaml`.
- **Client app:** `apps/desktop/src-tauri/src/sidecar.rs` / `lib.rs` (`.env` path setting passed as `MD_AI_ENV_FILE`), `apps/desktop/src/app/core/` (types, settings), `workspace/model-selector.ts` (filtered and grouped lists, size labels, Cloud badge), `ask-panel.ts` / `answer-view.ts` (consent notice, badge, specific errors).
- **Docs & config:** `example.env` (all five providers, `none` convention, example models), `md-ai.example.yaml`, `README.md`, `CHANGELOG.md`, `docs/architektur_erweiterung.md` (4.1, 5.2 Stage 1, 14), `md-indexer.sdd`.
- **Dependency:** `github.com/anthropics/anthropic-sdk-go` (pure Go, no CGO).

## 7. SpecDD Architecture Alignment (Drift Control)

- [x] **Architecture:** Layering maintained: assistant → `LLMProvider` interface; retrieval and embeddings unchanged and local.
- [x] **Architecture:** The desktop app talks to `md-api` over HTTP only; keys reach the sidecar as environment variables.
- [x] **Architecture:** Feature affects architecture: `md-indexer.sdd` gets rules for cloud providers (explicit user choice, locality per model, no silent fallback, keys only via environment).
- [x] **Read-Only Vault:** Unchanged.
- [x] **Security:** Redaction before every provider call; keys never in CLI arguments, logs, API responses or settings; consent before the first cloud call.
- [x] **Data Model:** SQLite schema unchanged.
- [x] **Go Standards:** Providers injected into the assistant; no global state; protocol code only in `internal/ai/provider`.
- [x] **Standards:** SPDX headers and English documentation.

## 8. Acceptance Criteria

- [ ] Answer selector: only generation models (local, sorted by size, with size label) plus enabled cloud models; embedding selector: only embedding models.
- [ ] Ollama cloud models of the local daemon are listed as cloud models.
- [ ] Providers with missing, empty or `none` keys are not offered; `GROQ_*` entries are ignored.
- [ ] Questions are answered by Ollama Cloud, OpenRouter, NVIDIA NIM, Claude and Gemini models (mocked in tests; one manual run per configured provider).
- [ ] Claude refusals, 401 and 429 produce specific messages; no fallback to another provider or local model.
- [ ] Keys do not appear in process arguments (`ps`), logs, API responses or `settings.json`.
- [ ] Consent notice appears once per app session before the first cloud question; cloud answers carry the badge and `provider`/`locality`.
- [ ] Without `.env` the app works as before (local models only).
- [ ] `go test ./...` (providers against `httptest` servers, `.env` parsing incl. precedence and `none`, model classification, routing), `ng test` (selectors, notice, badge), `cargo test`.
- [ ] `example.env`, `README.md`, `CHANGELOG.md`, `openapi.yaml`, `md-indexer.sdd`, `docs/architektur_erweiterung.md` are up-to-date.

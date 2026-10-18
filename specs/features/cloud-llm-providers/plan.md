<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Architecture Plan: Local Model Filtering and Cloud LLM Providers

**Issue:** [#24](https://github.com/Zheng-Bote/md_indexer-searcher/issues/24)  
**Spec:** [spec.md](./spec.md)

## 0. Resolved Open Points (2026-10-18)

1. **Gemini via OpenAI compatibility** — confirmed in Google's docs: base URL `https://generativelanguage.googleapis.com/v1beta/openai/`, `Authorization: Bearer <key>`, `stream: true` supported, usage in streams via `stream_options: {include_usage: true}`. Google labels the compatibility layer *beta*; we only use chat completions, streaming and usage. → Gemini uses the OpenAI-compatible provider; `GEMINI_ENDPOINT` defaults to that URL.
2. **Ollama cloud models** — the local daemon names them with the tag `cloud` (e.g. `gemma4:cloud`); older releases used a `-cloud` tag suffix (e.g. `gpt-oss:120b-cloud`). Ollama's docs don't document any remote field in `/api/tags`. → A local model is a cloud model when its tag equals `cloud` or ends in `-cloud`. Direct Ollama Cloud access uses `https://ollama.com` with `Authorization: Bearer` and documents `/api/chat` (not `/api/generate`) and model names **without** a suffix.
3. **Routing** → **per request**, not by restarting `md-api`. `md-api` builds a provider registry once at startup; `POST /api/ai/ask` takes an optional model reference. Reasons: switching the answer model no longer restarts both sidecars (today it also restarts `md-indexer`); keys are loaded once by `md-api` itself; the registry is the base for the later comparison run. Changing the **embedding** model still restarts the sidecars (the index must be re-embedded).

## 1. Architectural Alignment (`md-indexer.sdd`)

- **Provider interfaces:** every backend implements `LLMProvider` (+ `StreamingLLMProvider`); protocol code lives only in `internal/ai/provider`. The assistant receives a resolver instead of a single provider (DI, no globals).
- **Keys never cross the desktop app:** `md-api` / `md-ai` read the `.env` themselves. The desktop app only stores and passes the **path** (`MD_AI_ENV_FILE` environment variable of the sidecar). Keys are never in CLI arguments, logs, API responses or `settings.json`.
- **Redaction:** unchanged — `ContextBuilder` redacts before any provider is called.
- **Locality is per model** (architecture RFC 5.1).
- **No silent fallback** across providers or to local models.

```
.env ──► envfile.Load ──► ProviderConfig[OLLAMA|OPENROUTER|NVIDIA|CLAUDE|GEMINI]  (key "none"/empty → disabled)
                                   │
md-api start ──► provider.Registry ┼─ local:  OllamaProvider(ollama_host)                   models: /api/tags (filtered, sized)
                                   ├─ cloud:  OllamaProvider(ollama.com, key, /api/chat)     models: OLLAMA_LLM_xx
                                   ├─ cloud:  OpenAICompatProvider(OpenRouter|NVIDIA|Gemini)  models: <P>_LLM_xx
                                   └─ cloud:  ClaudeProvider(anthropic-sdk-go)               models: CLAUDE_LLM_xx
GET  /api/ai/models ──► Registry.List()  → [{name, provider, locality, kind, size, parameter_size}]
POST /api/ai/ask {question, model?: {provider, name}}
        └─ Assistant.Ask(ctx, q, ref) ─► retrieval (local) ─► context (budget by locality) ─► Registry.Resolve(ref).Generate
```

---

## 2. Technical Design

### A. `.env` loading (`internal/ai/config/envfile.go`)

- `LoadEnvFile(path string) (map[string]string, error)`: `KEY=VALUE` lines, `#` comments, optional `export ` prefix, single/double quotes stripped; no variable expansion. Missing file at the default path → empty map, no error; missing file at an explicit path → error.
- Path: `-env-file` / `--env-file` flag > `MD_AI_ENV_FILE` > `~/.config/md-indexer/.env`.
- Precedence: a key present in the process environment wins over the file (`process env > .env > md-ai.yaml > defaults`).
- Permission check: warn (path only) if mode `& 0o077 != 0`.
- `Providers(env) []ProviderConfig` with `Name`, `APIKey`, `Endpoint` (default per provider), `Models []string` (`<P>_LLM_01…_LLM_99`, ordered); providers whose key is empty or `none` (case-insensitive) are skipped; unknown prefixes (e.g. `GROQ`) are ignored.

| Provider | Default endpoint | Protocol |
|---|---|---|
| `OLLAMA` (Ollama Cloud) | `https://ollama.com` | Ollama `/api/chat` + Bearer |
| `OPENROUTER` | `https://openrouter.ai/api/v1` | OpenAI-compatible |
| `NVIDIA` | `https://integrate.api.nvidia.com/v1` | OpenAI-compatible |
| `GEMINI` | `https://generativelanguage.googleapis.com/v1beta/openai` | OpenAI-compatible |
| `CLAUDE` | Anthropic SDK default | Anthropic Messages API via `anthropic-sdk-go` |

### B. Configuration (`internal/ai/config/config.go`)

New fields (YAML + `MD_AI_*` env + flags where useful):

- `cloud_max_context_tokens` (default `16000`), `cloud_max_tokens` (default `16000`; reasoning models need room beyond the local `2048`), `cloud_timeout_seconds` (default `180`).
- `claude_effort` (default `medium`) and optional `claude_effort_by_model` map (model → `low|medium|high|xhigh|max`).
- `env_file` (path).

### C. Providers (`internal/ai/provider`)

1. **Model metadata** (`internal/ai/model`): `ModelInfo` gains `Provider`, `Locality` (`local|cloud`), `Kind` (`generation|embedding`), `ParameterSize` (e.g. `14.8B`). JSON is additive (`name`, `size`, `family` unchanged).
2. **Ollama (local):** `ListModels` additionally reads `details.parameter_size` and `details.families`; classification:
   - `Kind = embedding` if a family is `bert`/`nomic-bert` or the name contains `embed`, else `generation`;
   - `Locality = cloud` if the tag is `cloud` or ends in `-cloud`, else `local` (the host is always `localhost` for this instance; a non-loopback `ollama_host` makes all its models `cloud`).
3. **Ollama Cloud:** `OllamaProvider` gets options `WithAPIKey(key)` (Bearer header) and `WithChatAPI()` (uses `/api/chat` with `system` + `user` messages; streaming reads `message.content`). Local behaviour (`/api/generate`) is unchanged.
4. **OpenAI-compatible (`openai_compatible.go`):** `POST {endpoint}/chat/completions`, messages `system` + `user`, `stream: true`, `stream_options: {include_usage: true}`, SSE parsing of `choices[0].delta.content`, `usage.prompt_tokens/completion_tokens`. `ProviderName` set per instance (`openrouter`, `nvidia`, `gemini`).
5. **Claude (`claude.go`):** `anthropic.NewClient(option.WithAPIKey(key), option.WithRequestTimeout(...), option.WithMaxRetries(2))`; `Messages.NewStreaming` with `Model`, `MaxTokens = cloud_max_tokens`, `System` = system prompt, one user message, `OutputConfig.Effort` from config; text deltas forwarded to `onToken`; final message accumulated (`message.Accumulate`). `StopReason == refusal` → `ErrRefusal` with `StopDetails.Category`; `StopReason == max_tokens` → error "answer truncated". Temperature is not sent (rejected by current Claude models). Thinking blocks are ignored (only text is used).
   - **Refusal fallback (vendor guidance):** Anthropic recommends enabling server-side fallbacks (`fallbacks: "default"`, beta `server-side-fallback-2026-07-01`) for `claude-opus-5-5`/`claude-sonnet-5-5`. This re-serves a declined request with another **Claude** model inside the same call. Planned: enabled, and the answer reports the model that actually answered (`response.model`), so it is never silent; it stays within the Claude provider (spec requirement 15 is about provider/locality switches). Exact Go parameter shape verified against the SDK during implementation; if the SDK lacks the `"default"` form, the feature is left off and documented.
6. **Typed errors (`errors.go`):** `provider.Error{Provider, Kind (auth|rate_limit|refusal|truncated|timeout|unavailable), Status, Detail}`; all providers map HTTP 401/403 → `auth`, 429 → `rate_limit`, context deadline → `timeout`.
7. **Registry (`registry.go`):** `NewRegistry(local *OllamaProvider, cloud []ProviderConfig, httpClient...)`; `List(ctx)` merges local `/api/tags` with configured cloud models; `Resolve(ref)` returns the provider and locality for `{provider, name}`; unknown references → `400`.

### D. Assistant & API

- `Assistant.Ask(ctx, question, history, ref *ModelRef)`: `ref == nil` → configured default model/provider (CLI behaviour unchanged). Context budget = `cloud_max_context_tokens` for `cloud`, `max_context_tokens` for `local`; `max_tokens` analogous. `Answer` gains `Provider`, `Locality`; `Model` is the model that actually answered.
- `POST /api/ai/ask`: body `{question, model?: {provider, name}}`. Errors of kind `auth|rate_limit|refusal|truncated|timeout|unavailable` → `502` with `{success:false, error:"<Provider>: <message>", error_kind:"…"}`; unknown model → `400`.
- `GET /api/ai/models`: registry list; `active` unchanged (default local models).
- `cmd/md-api`: `-env-file`; registry wiring; log of enabled providers (names and model counts only).
- `cmd/md-ai`: `--env-file`, `--provider` (default `ollama`); cloud use prints a one-line notice to stderr (explicit flag = explicit choice).

### E. Desktop

- **Settings:** `AppSettings` gains `answerModel: {provider, name} | null` (replaces `model`; migration: old string → `{provider:"ollama", name}`) and `envFile: string | null`.
- **Sidecars (`sidecar.rs`):** `md-api` gets `MD_AI_ENV_FILE` via the shell plugin's `env()` when `envFile` is set; the answer model is **no longer** a sidecar argument (per-request routing), so changing it does not restart the sidecars. Embedding model and vault changes restart as before.
- **Model selector:** "Antwort-Modell" with groups "Lokal" (generation models, label `· 14.8B`, sorted by parameter size desc) and "Cloud · <Provider>" (configured models); "Embedding-Modell" lists `kind = embedding` only. A settings field "`.env`-Datei" (path, folder dialog).
- **Ask panel:** sends `model` with every question; on the first cloud question of an app run a confirmation dialog names the provider; confirmation is kept in memory only (signal). Errors show the provider-specific message.
- **Answer view:** badge "Cloud · <Provider>" / "Lokal", actual model name.

### F. Docs & config

- `example.env` (all five providers, `none` convention, example models: Claude `claude-opus-5-5`, `claude-sonnet-5-5`, `claude-haiku-5-5`; Gemini `gemini-3.8-flash` as shown in Google's docs at planning time).
- `md-ai.example.yaml` (cloud budget, timeouts, Claude effort, `env_file`), `README.md`, `CHANGELOG.md`, `openapi.yaml` (`model` in ask request, model list fields, `provider`/`locality` in answer, error kinds).
- `md-indexer.sdd`: cloud LLM calls only after an explicit model choice; locality per model; no silent provider/locality fallback; keys only from environment / `.env` read by the Go binaries.
- `docs/architektur_erweiterung.md`: 4.1 loader and 5.2 Stage 1 → implemented, 14 updated.

---

## 3. Verification Strategy

- **Go unit tests:**
  - `envfile`: parsing (comments, quotes, `export`), `none`/empty disables, `GROQ_*` ignored, process env precedence, permission warning, default vs. explicit missing path.
  - Ollama: classification (`bge-m3` → embedding; `gemma4:cloud`, `gpt-oss:120b-cloud` → cloud; parameter size), `/api/chat` + Bearer against `httptest`.
  - OpenAI-compatible: streaming SSE with usage chunk, 401/429 mapping, endpoint per provider.
  - Claude: SDK client pointed at an `httptest` server (`option.WithBaseURL`) returning SSE for a normal answer, a `refusal` and `max_tokens`; effort value in the request body.
  - Registry/assistant: routing by reference, budgets by locality, unknown model → 400, no fallback on error.
  - REST: `model` in ask body, error JSON with `error_kind`, keys never present in any response.
- **Frontend (`ng test`):** selector grouping/filtering/sorting, settings migration, consent once per run, badge, error message.
- **Rust (`cargo test`):** `md-api` args no longer contain the answer model; env path passed as environment.
- **Manual:** one question per configured provider with the user's `.env` (Ollama Cloud, OpenRouter, NVIDIA, Claude, Gemini); `ps -ef` and logs contain no key; app restart keeps the selection; without `.env` only local models.
- `go vet ./...`, `go test ./...`, `ng test`, `ng build`, `cargo test`, `cargo clippy`.

## 4. Risks

- **Gemini compatibility layer is beta** (Google) — isolated behind the OpenAI-compatible provider; switching to the native API later only touches one provider.
- **Ollama Cloud `/api/chat` vs. local `/api/generate`** — two request shapes in one provider; covered by tests for both.
- **Claude reasoning tokens** — thinking counts against `max_tokens`; `cloud_max_tokens` defaults to 16,000 and truncation is reported explicitly.
- **Desktop settings migration** — old `model` string is converted once; covered by a unit test.
- **Free-tier rate limits** — surfaced as `rate_limit`; no automatic retry beyond the SDK's / client's 2 retries on the same provider.

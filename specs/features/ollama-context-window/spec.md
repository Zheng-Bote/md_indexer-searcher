<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Feature Spec: Ollama Context Window, Thinking & Timeout

## 1. Problem & Goal
Running md-ai against a real knowledge base with local Ollama models produced ungrounded answers or failed:
*   Ollama allocates a default `num_ctx` of 4096 and **silently truncates** longer prompts
    (observed: `truncating input prompt limit=2050 prompt=24117`). The knowledge context and grounding rules were dropped.
*   Thinking models (e.g. `qwen3:14b`) run a hidden reasoning phase by default that consumes the `num_predict` budget.
*   The Ollama HTTP client had a fixed 120 s timeout, too short for 14B models on CPU.

Goal: the full prompt reaches the model, thinking models answer directly, and slow hardware does not time out.

## 2. Scope & Acceptance Criteria
*   `GenerateRequest.ContextWindow` carries the required window; the Ollama provider sends it as `options.num_ctx` when > 0.
*   The assistant sizes the window as `prompt bytes / 3 + max_tokens`, rounded up to the next multiple of 8192
    (coarse steps avoid Ollama model reloads on small prompt changes).
*   The Ollama provider always sends `"think": false`; non-thinking models accept it.
*   `Config.TimeoutSeconds` (default 600, env `MD_AI_TIMEOUT`, flag `--timeout`) sets the LLM HTTP client timeout
    for md-ai and md-api; values < 1 are invalid.

## 3. Non-Goals
*   No change of the default model or the context token budget.
*   No streaming responses.

## 4. Tasks
*   [x] Task 1: `num_ctx` sized from the prompt.
*   [x] Task 2: `think: false`.
*   [x] Task 3: Configurable timeout wired into md-ai and md-api.
*   [x] Task 4: Unit tests for request payload, window sizing and config.

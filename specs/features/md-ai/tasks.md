<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Task Breakdown: Local Grounded AI Assistant (md-ai)

## Package & Implementation Tasks

### Task 1: Core Models & Configuration (`internal/ai/model`, `internal/ai/config`)
- [x] Define data models in `internal/ai/model/types.go` (`Intent`, `RetrievalResult`, `ContextBlock`, `GenerateRequest`, `GenerateResponse`, `Answer`, `SourceCitation`).
- [x] Implement configuration parser in `internal/ai/config/config.go` with defaults (Ollama URL `http://localhost:11434`, default model `gemma3:1b`, `max_context_tokens: 16000`, `graph_depth: 3`).
- [x] Add unit tests for configuration loading and validation in `internal/ai/config/config_test.go`.

### Task 2: LLM Provider Layer (`internal/ai/provider`)
- [x] Define `LLMProvider` interface in `internal/ai/provider/provider.go`.
- [x] Implement `OllamaProvider` in `internal/ai/provider/ollama.go` handling native HTTP communication with `/api/generate` and `/api/chat`.
- [x] Implement `MockProvider` in `internal/ai/provider/mock.go` for deterministic test executions.
- [x] Add unit tests in `internal/ai/provider/provider_test.go` covering Ollama request serialization, response parsing, error handling, and mock assertions.

### Task 3: Intent Classifier (`internal/ai/classifier`)
- [x] Define `Classifier` interface and implementation in `internal/ai/classifier/classifier.go`.
- [x] Implement pattern/heuristic-based classification into: `Search`, `Entity`, `Relationship`, `ADR`, `Impact`, `Summary`, `Compare`, `KnowledgeGap`.
- [x] Add unit tests in `internal/ai/classifier/classifier_test.go` validating classification accuracy across various query styles.

### Task 4: Knowledge Retriever (`internal/ai/retriever`)
- [x] Implement `Retriever` in `internal/ai/retriever/retriever.go`:
  - FTS5 BM25 search across documents and sections.
  - Entity & relation lookups.
  - Multi-hop graph neighborhood search using SQLite recursive CTE / BFS up to `graph_depth`.
- [x] Add comprehensive in-memory SQLite tests in `internal/ai/retriever/retriever_test.go` verifying multi-strategy retrieval without disk dependencies.

### Task 5: Context Builder & Token Budgeter (`internal/ai/contextbuilder`)
- [x] Implement `ContextBuilder` in `internal/ai/contextbuilder/builder.go`:
  - Prioritize evidence: 1. Exact Entity > 2. Exact Doc > 3. Linked ADRs > 4. Linked Components > 5. Graph Docs > 6. FTS hits.
  - Enforce token budgeting (`max_context_tokens`) using word/token approximations.
- [x] Add unit tests in `internal/ai/contextbuilder/builder_test.go` verifying priority ordering and token truncation.

### Task 6: Prompt Engineering, Anti-Hallucination & Gaps (`internal/ai/prompt`, `internal/ai/gaps`, `internal/ai/citations`)
- [x] Implement system prompt templates in `internal/ai/prompt/template.go` (strict grounding rules, inline citation instructions `[Doc: <Path>#<Section>]`, refusal to fabricate unknown facts).
- [x] Implement citation parser and validator in `internal/ai/citations/citations.go`.
- [x] Implement knowledge gap detection and extraction in `internal/ai/gaps/gaps.go`.
- [x] Add unit tests for prompt formatting, citation extraction, and gap detection.

### Task 7: Conversation Manager (`internal/ai/conversation`)
- [x] Implement multi-turn session history in `internal/ai/conversation/session.go`.
- [x] Add unit tests in `internal/ai/conversation/session_test.go`.

### Task 8: Assistant Orchestrator (`internal/ai/assistant`)
- [x] Wire Classifier, Retriever, ContextBuilder, Prompt, Provider, Gaps, and Citations in `internal/ai/assistant/assistant.go`.
- [x] Add end-to-end integration tests in `internal/ai/assistant/assistant_test.go` using in-memory SQLite and `MockProvider`.

### Task 9: CLI Interface (`cmd/md-ai`)
- [x] Implement Cobra CLI in `cmd/md-ai/main.go` supporting:
  - `ask <question>` (with `--model`, `--db`, `--ollama-host`, `--temperature`, `--max-tokens`, `--json`)
  - `chat` (interactive REPL session)
  - `version`
- [x] Verify build and execution: `go build -o md-ai ./cmd/md-ai`.

### Task 10: Verification & Quality Gate
- [x] Run complete test suite (`go test -race ./internal/ai/...`).
- [x] Verify test coverage >= 80% across `internal/ai/...`.
- [x] Verify zero modification to `cmd/md-indexer`, `cmd/md-search`, `cmd/md-api`.
- [x] Update `CHANGELOG.md`.

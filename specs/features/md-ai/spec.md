<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Feature Specification: Local Grounded AI Assistant (md-ai)

## 1. Objective & Overview
`md-ai` is an intelligent, local, grounded AI assistant and Retrieval-Augmented Generation (RAG) CLI service that queries a Markdown knowledge base materialized in SQLite (`knowledge.db`).
It provides grounded answers to user questions, supports deep graph-aware context retrieval, enforces strict anti-hallucination policies, formats source citations, identifies knowledge gaps, and supports local LLMs via Ollama (specifically tested with `gemma3:1b` and other supported models) as well as extensible cloud LLM providers.

## 2. Key Constraints & Guarantees
- **Strict Read-Only:** Markdown files remain strictly immutable. Knowledge is sourced strictly from `knowledge.db`.
- **Untouched Components:** `cmd/md-indexer`, `cmd/md-search`, and `cmd/md-api` are untouched.
- **CGO-Free:** Built strictly without CGO using `modernc.org/sqlite`.
- **Architecture:** Clean Architecture, Dependency Injection, structured logging (`log/slog`), no global mutable state, Apache-2.0 SPDX headers, English comments and docs.

## 3. User Scenarios & Capabilities

### User Story 1 - Single Question CLI Execution
As a developer or researcher, I want to query my knowledge base from the CLI (`md-ai ask "What decisions were made regarding authentication?"`) using a local model (`gemma3:1b`) and get an answer grounded strictly in my indexed notes with clear citations.

### User Story 2 - Interactive Chat Session
As a user exploring my vault, I want to enter an interactive REPL mode (`md-ai chat`) to converse with my knowledge base across multiple turns, preserving conversation context and asking follow-up questions.

### User Story 3 - Intent Classification & Multi-Strategy Retrieval
As a user asking varied queries (search queries, entity lookups, ADR decisions, impact analysis, comparative queries), the assistant must classify my query intent and retrieve relevant context via:
- Full-Text Search (FTS5 BM25 `MATCH`)
- Entity and Entity Relation lookups
- Recursive BFS graph traversal (`graph_depth: 3`) across backlinks, forward links, and tags.

### User Story 4 - Token Budgeting & Context Prioritization
As a user with a large knowledge base, the context builder must assemble evidence without exceeding a configurable token budget (`max_context_tokens: 16000`), prioritizing:
1. Exact Entity Matches
2. Exact Document Matches
3. Linked Architectural Decision Records (ADRs)
4. Linked Components
5. Graph-connected documents (hops <= 3)
6. Relevant FTS5 content chunks

### User Story 5 - Anti-Hallucination & Knowledge Gap Detection
When queried on topics not covered in the knowledge base, the assistant must explicitly state that the information is missing from the indexed vault, identify the knowledge gap, and decline to speculate or hallucinate.

## 4. Functional Requirements

### 4.1 CLI Interface (`cmd/md-ai`)
- Subcommands:
  - `ask <question>`: Single-shot question answering. Flags: `--model`, `--db`, `--ollama-host`, `--temperature`, `--max-tokens`, `--json`.
  - `chat`: Interactive conversation session in the terminal.
  - `version`: Print component version.
- Configuration via CLI flags, environment variables (`MD_AI_*`), and config file (`~/.config/md-indexer/ai.yaml` or `./md-ai.yaml`).

### 4.2 LLM Provider Layer (`internal/ai/provider`)
- Pluggable interface:
  ```go
  type LLMProvider interface {
      Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error)
      Name() string
  }
  ```
- Ollama Provider:
  - Connects to Ollama REST API (default `http://localhost:11434/api/generate` or `/api/chat`).
  - Supports configurable models (default `gemma3:1b`, compatible with `llama3`, `qwen3`, etc.).
  - Configurable timeouts and error handling.
- Mock Provider for unit and integration testing without running a live Ollama instance.

### 4.3 Intent Classifier (`internal/ai/classifier`)
- Classifies queries into:
  - `Search`: General keyword or topic search.
  - `Entity`: Inquiries about a specific entity, component, or person.
  - `Relationship`: How entity A connects to entity B.
  - `ADR`: Architectural decisions, rationale, consequences.
  - `Impact`: What changes or breaks if X is modified.
  - `Summary`: Summaries of documents or topics.
  - `Compare`: Comparing two concepts, architectures, or files.
  - `KnowledgeGap`: Identifying missing information.

### 4.4 Retriever & Context Builder (`internal/ai/retriever`, `internal/ai/contextbuilder`)
- Queries SQLite materialized view (`documents`, `sections`, `entities`, `entity_relations`, `links`, `tags`, `documents_fts`).
- Executes BFS graph traversal for connected context up to configurable depth.
- Assembles prompt with system instructions enforcing:
  - Grounding in provided context only.
  - Inclusion of citation identifiers `[Doc: <Path>#<Section>]`.
  - Recognition of knowledge gaps.

### 4.5 Output & Citations (`internal/ai/citations`, `internal/ai/gaps`)
- Parses generated answer or post-processes response to extract and validate cited document paths.
- Provides optional JSON output structure:
  ```json
  {
    "answer": "...",
    "sources": [{"path": "...", "section": "...", "snippet": "..."}],
    "gaps": ["..."],
    "intent": "ADR",
    "model": "gemma3:1b"
  }
  ```

## 5. Non-Functional Requirements
- **Performance:** Context retrieval < 100ms on SQLite.
- **Resilience:** Graceful error messages when Ollama daemon is offline or model is missing.
- **Testability:** In-memory SQLite tests for retriever, mock LLM provider for deterministic end-to-end testing, statement coverage >= 80%.
- **Portability:** Pure Go, no CGO, cross-platform Linux/macOS/Windows.

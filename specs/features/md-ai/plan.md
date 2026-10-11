<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Architectural Plan: Local Grounded AI Assistant (md-ai)

## 1. Alignment with SpecDD (`md-indexer.sdd`)

| Rule / Constraint | Verification in `md-ai` Plan |
| :--- | :--- |
| **Markdown Read-Only** | `md-ai` never accesses the file system to write or modify markdown files. All knowledge is read directly from SQLite `knowledge.db`. |
| **No CGO** | Uses `modernc.org/sqlite` via `internal/storage/sqlite.go` or dedicated read-only connection. |
| **Clean Architecture & DI** | Core layers (Provider, Retriever, ContextBuilder, Classifier, Generator) defined as interfaces with constructor-based dependency injection. |
| **No Global State** | All configs, db connections, and clients are encapsulated in structs (`Config`, `Assistant`, `Engine`). |
| **Untouched Components** | Zero modifications to `cmd/md-indexer`, `cmd/md-search`, or `cmd/md-api`. |
| **Code Standards** | All comments in English, Apache-2.0 SPDX headers, structured `log/slog` logging. |

---

## 2. Component Architecture & Package Structure

```
cmd/
└── md-ai/
    └── main.go                 # Entrypoint (Cobra CLI: ask, chat, version)

internal/
└── ai/
    ├── config/
    │   └── config.go           # CLI / YAML / Env configuration (default model: gemma3:1b)
    ├── model/
    │   └── types.go            # Data contracts (Intent, RetrievalResult, ContextBlock, Answer)
    ├── classifier/
    │   ├── classifier.go       # Intent classifier interface & rule/pattern-based implementation
    │   └── classifier_test.go
    ├── retriever/
    │   ├── retriever.go        # Multi-strategy retriever (FTS5, entities, relations, graph BFS)
    │   └── retriever_test.go   # In-memory SQLite tests
    ├── contextbuilder/
    │   ├── builder.go          # Token budgeter & priority sorter (Entity > Doc > ADR > Graph > FTS)
    │   └── builder_test.go
    ├── provider/
    │   ├── provider.go         # LLMProvider interface & factory
    │   ├── ollama.go           # Native HTTP Ollama provider (supports gemma3:1b, llama3, etc.)
    │   ├── mock.go             # Mock provider for unit & integration tests
    │   └── provider_test.go
    ├── prompt/
    │   ├── template.go         # Grounded system prompts, citation instructions, anti-hallucination guards
    │   └── template_test.go
    ├── citations/
    │   ├── citations.go        # Citation parser & validator against retrieved docs
    │   └── citations_test.go
    ├── gaps/
    │   ├── gaps.go             # Knowledge gap detection & flagging
    │   └── gaps_test.go
    ├── conversation/
    │   ├── session.go          # Multi-turn conversation history manager for interactive chat
    │   └── session_test.go
    └── assistant/
        ├── assistant.go        # Orchestrator binding Retriever, Builder, Provider, Gaps, Citations
        └── assistant_test.go
```

---

## 3. Data Flow

```
[User Question]
       │
       ▼
1. Classifier (Identifies query intent: Search, Entity, ADR, Impact, etc.)
       │
       ▼
2. Retriever (Executes FTS5, Entity lookups, and BFS Graph traversal against knowledge.db)
       │
       ▼
3. Context Builder (Prioritizes sources, applies token budget e.g. 16k tokens)
       │
       ▼
4. Prompt Engine (Injects strict anti-hallucination system prompt, citations format, and retrieved context)
       │
       ▼
5. LLM Provider (Calls local Ollama API - model: gemma3:1b)
       │
       ▼
6. Post-Processing (Validates citations, verifies gaps, formats markdown/terminal or JSON output)
       │
       ▼
[CLI / Interactive Output]
```

---

## 4. Database Schema Utilization (Read-Only)

`md-ai` connects to `knowledge.db` with read-only pragmas (`mode=ro` or standard read queries):
- `documents`: Document path, title, word count, modified time.
- `sections`: Heading, content, level, line ranges for precise context snippets.
- `documents_fts`: Full-text BM25 ranking (`MATCH`).
- `entities` & `entity_relations`: Extracted entities (components, ADRs, concepts) and their relationships.
- `links`: Backlinks and forward links used for graph neighborhood discovery.
- `tags`: Categorization and topic filtering.

---

## 5. Ollama Integration Strategy

- **Endpoint:** `POST http://localhost:11434/api/generate` (or `/api/chat`).
- **Default Model:** `gemma3:1b` (configured and verified on user system).
- **Fallback / Alternative Models:** Easily switched via `--model <name>` or config file (`llama3`, `qwen3`, etc.).
- **Streaming & Non-Streaming:** Standard non-streaming for scripted/JSON outputs; optional streaming for CLI REPL chat.
- **Mocking:** `MockProvider` simulates Ollama responses without network dependencies to guarantee fast, deterministic tests (`go test ./internal/ai/...`).

---

## 6. Testing & Quality Strategy

1. **Unit Tests:**
   - Every subpackage (`classifier`, `contextbuilder`, `provider`, `prompt`, `citations`, `gaps`, `conversation`) tested independently.
2. **In-Memory Integration Tests:**
   - `retriever_test.go` uses an in-memory SQLite database populated with test documents, sections, entities, and links to verify FTS5 and graph traversal without filesystem access.
3. **End-to-End Orchestrator Tests:**
   - `assistant_test.go` runs the entire pipeline end-to-end with in-memory SQLite + `MockProvider`.
4. **Coverage Target:**
   - Statement coverage across `internal/ai/...` >= 80%.

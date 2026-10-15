<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Feature Spec: Hybrid Retriever (Stage c: Vector Retrieval)

## 1. Problem & Goal
FTS only matches word forms present in the text ("konfiguriere" does not find "Konfiguration", synonyms are missed).
Goal: local semantic retrieval with Ollama embeddings (`bge-m3`, `nomic-embed-text`, `jina-embeddings-v3`),
stored in SQLite and fused with the existing sources. No cloud APIs, no external vector database.
Builds on `specs/features/hybrid-retriever/spec.md`.

## 2. Scope & Acceptance Criteria

### 2.1 Storage
*   Table `section_embeddings(section_id PK, embedding BLOB, dimensions, model, content_hash, created_at)`;
    vectors are little-endian float32. `content_hash` (SHA-256 of the embedded text) detects changed sections.

### 2.2 Indexing (`internal/embedding`, `internal/service`)
*   Embeddings are optional: the indexer writes them only with `-embed-model <model>` (`-ollama-host` configurable).
*   Embedded text per non-empty section: `document title \n heading \n content`, truncated to 8000 runes,
    with the model's document prefix (`search_document: ` for nomic-embed-text).
*   After a full index run and after each watcher-triggered file index, only sections without an embedding for the
    model or with a changed hash are embedded (batches of 32); embeddings of removed sections are deleted.
*   Embedding failures are logged and do not stop text indexing.

### 2.3 Retrieval (`internal/ai/hybrid`)
*   `VectorRetriever` embeds the query (query prefix `search_query: ` for nomic-embed-text), ranks embeddings of the
    configured model by cosine similarity (brute force) and returns the top `TopKVector` (default 25) sections
    with cosine ≥ `MinVectorScore`, so unrelated sections do not fill the context.
*   Embeddings of removed sections never occupy top-k slots.
*   Without embeddings for the model, the query is not embedded and vector retrieval contributes nothing.
*   Plans enable vectors for fact, entity, architecture and exploration queries; fusion weight 0.35.
*   md-ai stays read-only.

### 2.4 Configuration
*   `EnableVectors` (default true, `MD_AI_ENABLE_VECTORS`, `--vectors`), `EmbeddingModel` (default `bge-m3`,
    `MD_AI_EMBEDDING_MODEL`, `--embedding-model`; must match the indexer's `-embed-model`), `TopKVector`,
    `MinVectorScore`, `FusionVectorWeight`.

## 3. Non-Goals
*   Approximate nearest-neighbour indexes or in-memory vector caches.

## 4. Tasks
*   [x] Task 1: `section_embeddings` table and repository access.
*   [x] Task 2: Ollama embedder, vector codec, cosine.
*   [x] Task 3: Incremental embedding service wired into the indexer.
*   [x] Task 4: Vector retriever, plan and fusion integration, configuration.
*   [x] Task 5: Unit tests and verification against a real vault.

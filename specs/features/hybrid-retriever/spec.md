<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Feature Spec: Hybrid Retriever (Stage b: Planning, Section FTS, Fusion)

## 1. Problem & Goal
The classic retriever runs every strategy for every question, searches whole documents, ORs all query words
(including stopwords such as "wie", "ich") and concatenates results without a joint ranking.
On a real vault this floods the context with irrelevant documents.

Goal: a second-generation retriever (`internal/ai/hybrid`) that plans retrieval per query type,
searches at section level with field weights, and fuses all sources into one ranked, deduplicated list.
The classic retriever stays available. Later stages add vectors (c), weighted graph expansion and reasoning
chains (d), architecture mode and context compression (e).

## 2. Scope & Acceptance Criteria

### 2.1 Query planning
*   `QueryType`: `fact`, `entity`, `impact`, `architecture`, `exploration`.
*   The planner derives the type from the classified `model.Intent`; architecture questions
    ("how does … work", "wie funktioniert …", "architecture", "data flow") are detected by the planner itself.
*   `RetrievalPlan{QueryType, UseFTS, UseEntities, UseGraph, UseVectors, GraphDepth, MaxResults}`.
    FTS is enabled for every type because it seeds entity and graph retrieval. `UseVectors` stays false until stage (c).
    `GraphDepth` is capped by `Config.GraphDepth`.
*   The rule classifier also recognises German phrasings for all intents.

### 2.2 Section-level FTS (indexer)
*   New FTS5 table `fts_sections(section_id UNINDEXED, document_id UNINDEXED, title, heading, tags, content, metadata)`,
    written by `UpsertDocument` for every non-empty section. `metadata` holds the document's frontmatter property values.
*   The migration backfills `fts_sections` from existing tables when it is empty, so existing databases need no re-index.
*   Ranking: `bm25` column weights title 1.0, heading 0.8, tags 0.7, content 0.5, metadata 0.4.
*   If `fts_sections` is missing (database built by an older indexer, md-ai is read-only), FTS falls back to `fts_documents`.

### 2.3 Query sanitizing
*   FTS and entity matching drop German and English stopwords; terms are split like the FTS5 `unicode61` tokenizer.

### 2.4 Candidates and fusion
*   Every source returns `Candidate`s keyed by section with per-source scores (`FTSScore`, `EntityScore`, `GraphScore`;
    `VectorScore` reserved).
*   Sources: FTS, entities (entities whose full name occurs in the query — partial name matches are left to FTS;
    sections of the entity's defining document; also fills `Entities`/`Relations`),
    graph (link neighbors of the top FTS/entity documents, `1/depth`).
*   Fusion: weighted Reciprocal Rank Fusion, `score = Σ w_source / (60 + rank_source)`, weights from config
    (FTS 0.35, graph 0.20, entity 0.10; vector 0.35 reserved). Duplicate sections merge; at most 3 sections per document;
    at most `MaxResults` candidates.
*   Output is the existing `model.RetrievalResult`; `Snippet.Score` carries the fused score and `Strategy` the strongest source.

### 2.5 Wiring
*   `assistant` depends on a `Retriever` interface; md-ai and md-api select the implementation via
    `Config.Retriever` (`hybrid` default, `classic`), env `MD_AI_RETRIEVER`, md-ai flag `--retriever`.

## 3. Non-Goals
*   Embeddings, weighted relation types, reasoning chains, architecture context, context-builder changes (stages c–e).

## 4. Tasks
*   [x] Task 1: `fts_sections` table, write path and backfill.
*   [x] Task 2: Stopword-aware FTS query builder.
*   [x] Task 3: German intent patterns.
*   [x] Task 4: Planner, candidate model, FTS/entity/graph sources, RRF fusion.
*   [x] Task 5: Retriever interface and config wiring.
*   [x] Task 6: Unit tests and verification against a real vault.

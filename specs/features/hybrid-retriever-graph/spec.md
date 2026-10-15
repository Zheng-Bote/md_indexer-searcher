<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Feature Spec: Hybrid Retriever (Stage d: Weighted Graph Expansion & Reasoning Chains)

## 1. Problem & Goal
The stage-b graph source only followed wikilinks and scored neighbours by `1/depth`.
Typed entity relations (`uses`, `depends_on`, ... from frontmatter lists) and shared tags were ignored, and the
LLM saw no explicit path explaining *why* a related document matters.

Goal: expand seed documents over all graph edges with relation-specific weights and hand explicit multi-hop
reasoning chains to the context builder. Builds on `specs/features/hybrid-retriever/spec.md`.

## 2. Scope & Acceptance Criteria

### 2.1 Edges (document level)
*   Wikilinks (`LINKS_TO`), resolved via `documents.title`.
*   Entity relations in both directions, resolved via `documents.title`; `TAGS` (handled as shared tags) and
    `ATTACHMENTS` (files, not notes) are excluded.
*   Shared tags (`SHARES_TAG #tag`) for tags used by 2–10 documents; purely numeric tags are ignored.
*   Folder/notebook membership is not an edge (notebooks hold ~100–400 notes and would connect everything).

### 2.2 Weights
`USES` 1.0, `DEPENDS_ON` 0.9, `ADR` 0.8, `CONTAINS` 0.5, `LINKS_TO` 0.4, `RELATION` 0.3, shared tag 0.3,
`REFERENCES`/`REFERENCE` 0.2, other relation lists 0.3.

### 2.3 Traversal
*   Breadth-first up to the plan's `GraphDepth`, edges traversed in both directions.
*   `GraphScore` of a document = best product of edge weights over its paths; seeds are never candidates.
*   At most 50 nodes are expanded per hop (highest scores); at most `TopKFTS` documents and 2 sections per document
    become candidates.

### 2.4 Reasoning chains
*   `model.ReasoningChain{Steps []ReasoningStep{Source, Relation, Target}}` is the best path from a seed to a document.
    Steps keep the semantic direction of the relation even when traversed backwards
    (e.g. reaching "REST Delivery" from "Encryption" yields `REST Delivery DEPENDS_ON Encryption`).
*   `RetrievalResult.Chains` holds the chains of fused graph documents in fused order, at most 5;
    chains that are a prefix of another selected chain are dropped.
*   The context builder renders each chain as a priority-2 block (source `knowledge-graph`).

### 2.5 ADR marker
*   Snippets are marked `adr` only when "ADR" occurs as a word in path or title (not in "Badradio").

## 3. Non-Goals
*   Architecture context and context compression (stage e).

## 4. Tasks
*   [x] Task 1: Weighted multi-source edges and beam-limited BFS.
*   [x] Task 2: Reasoning chains in the retrieval model and context builder.
*   [x] Task 3: Word-boundary ADR detection.
*   [x] Task 4: Unit tests (chain, weights, depth, tags) and verification on the real and a template-based demo vault.

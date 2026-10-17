<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Feature Spec: Hybrid Retriever (Stage e: Architecture Context, Compression, Knowledge Gaps & Safety)

## 1. Problem & Goal
After stages (b)–(d), the hybrid retriever ranks sections across FTS, entities, vectors, and weighted graph traversal.
Several quality, context, and safety gaps remain before the hybrid GraphRAG pipeline is complete:
1. Graph candidates with default RRF weight `0.20` are pushed down on `impact` and `architecture` plans where graph dependencies are primary evidence.
2. Architecture questions ("Wie funktioniert …?") only receive prose snippets without a structured overview of involved services, components, pipelines, ADRs, and dependencies.
3. Context builder re-sorts all snippets by coarse strategy priority (overwriting RRF score order), keeps near-duplicate sections, underestimates tokens (`len/4` vs ~3 bytes/token), and leaks raw API keys/tokens from vault notes into LLM prompts.
4. `Answer.Sources` only lists `[Doc: …]` strings emitted by the LLM instead of deterministically including the context documents used.
5. `KnowledgeGap` is only heuristic string-matching on LLM output (`[]string`) instead of structured retrieval-time detection (`KnowledgeGap{Entity, Reason}` for missing ADR, description, or API) combined with answer-level gap detection.
6. The system prompt lacks an explicit rule to answer in the language of the user's question, and JSON output lacks the executed `QueryType`/`RetrievalPlan` for debugging.

Builds on `specs/features/hybrid-retriever/spec.md`, `specs/features/hybrid-retriever-vectors/spec.md`, and `specs/features/hybrid-retriever-graph/spec.md`.

## 2. Scope & Acceptance Criteria

### 2.1 Plan-Aware Graph Fusion Weight (Stage d follow-up)
*   For `QueryImpact` and `QueryArchitecture`, `Weights.ForPlan(plan)` boosts the graph weight to at least `max(w.Graph*2, w.FTS)` (e.g. `0.40` with default weights) so multi-hop dependencies compete on equal footing with FTS and vector hits.

### 2.2 Architecture Mode (`QueryArchitecture`)
*   When `plan.QueryType == QueryArchitecture`, the retriever synthesizes an `ArchitectureContext` (`Services`, `Components`, `Pipelines`, `ADRs`, `Entities`, `Dependencies`) from the matched/reached documents, their entity types/properties, and relations.
*   The context builder renders `ArchitectureContext` as a high-priority (`Priority: 2`, source `architecture-model`) structured summary block before general snippets.

### 2.3 Context Compression, Score Ordering, Token Estimation & Secret Masking
*   **Score Ordering:** Snippets retain their fused score order; only direct entity definitions (`strategy == "entity"`, Priority 1) and ADRs (`strategy == "adr"`, Priority 3) are grouped ahead of general fused snippets (`Priority: 4`), sorted by `Score` descending within each priority tier.
*   **Near-Duplicate Removal:** Before budgeting, snippets whose normalized token set has Jaccard similarity ≥ `0.80` with an already selected higher-ranked snippet are dropped.
*   **Token Estimation:** `estimateTokens` uses `(len(s) + 2) / 3` (~3 bytes/token, matching `assistant.contextWindow`) so the token budget is strictly respected on mixed German/English Markdown and code.
*   **Secret Masking:** Before context blocks are formatted for the LLM (and in snippet previews), common secret assignments and tokens (`*_API_KEY`, `*_SECRET`, `*_TOKEN`, `*_PASSWORD`, `sk-...`, `ghp_...`, Bearer tokens) have their values replaced with `[REDACTED]`.

### 2.4 Retrieval-Time Knowledge Gap Detection & `[]model.KnowledgeGap`
*   `model.KnowledgeGap{Entity string, Reason string}` replaces `[]string` in `model.Answer.Gaps` and `openapi.yaml`.
*   During entity retrieval (and for entities discovered in architecture/entity/impact queries), the retriever checks each matched entity (`type_id != 'unresolved'`) for:
    *   missing description (`description` property empty and no `Overview`/`Beschreibung` section),
    *   missing ADR (`no ADR relation or linked ADR document` for `service`/`component`/`pipeline` entities),
    *   missing API documentation (`no api property and no API section` for `service`/`component` entities).
*   `gaps.DetectGaps(answer)` returns `[]model.KnowledgeGap` (with `Entity: "general"` for unattributed LLM-reported gaps) and is merged with retrieval-time `RetrievalResult.Gaps` without duplicates.

### 2.5 Deterministic Sources, Planner Debug Info & Answer Language
*   **Deterministic Sources:** `assistant.Ask` populates `Answer.Sources` from the `citations.ExtractCitations` matches **plus** all document snippets that were included in the budgeted context blocks (deduplicated by path, preserving section and a redacted snippet preview).
*   **Planner Debug Output:** `model.RetrievalPlanInfo` (`query_type`, `use_fts`, `use_entities`, `use_graph`, `use_vectors`, `graph_depth`, `max_results`) is stored on `RetrievalResult.Plan` and exposed on `Answer.Plan` (`json:"plan,omitempty"`).
*   **Language Rule:** `prompt.BuildPrompt` instructs the model to always answer in the same language as the user's question (e.g. German question → German answer).

## 3. Non-Goals
*   Modifying external vault files or existing note frontmatter.

## 4. Tasks
*   [x] Task 1: Extend `model` types (`KnowledgeGap`, `RetrievalPlanInfo`, `ArchitectureContext`, `ContextBlock.Score`) and `openapi.yaml`.
*   [x] Task 2: Plan-aware fusion weight, retrieval-time `KnowledgeGap` detection, and `ArchitectureContext` assembly in `internal/ai/hybrid`.
*   [x] Task 3: Context compression (Jaccard near-duplicate filter), score-preserving priority sort, `/3` token estimate, and secret redaction in `internal/ai/contextbuilder`.
*   [x] Task 4: Deterministic sources, merged `KnowledgeGap`s, plan propagation, and language-aware prompt in `internal/ai/assistant`, `internal/ai/gaps`, `internal/ai/prompt`, and `cmd/md-ai`.
*   [x] Task 5: Unit tests across all modified packages.

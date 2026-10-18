# SpecDD project specific overrides

## Project Specs
- The root spec is `md-indexer.sdd`. It defines system intent, layering (Watcher -> Parser -> Repository -> Search/Graph Engine -> API/CLI, plus client apps under `apps/`), strict rules and code standards for the whole repository.
- Nested `.sdd` files may refine the root spec for their directory but MUST NOT weaken its `Must not` or `Forbids` constraints.

## Spec Kit Integration
- SpecDD remains the Single Source of Truth for system architecture, global governance, and security (read-only vault, secret redaction, no direct database or vault access from client apps).
- GitHub Spec Kit is used for feature-level specs (e.g. new retrieval strategies, LLM providers, API endpoints, CLI flags, or desktop app features).
- Feature specs are created as GitHub issues from `.github/ISSUE_TEMPLATE/feature_spec.md` (labels `feature`, `spec-kit`) and written in English with EARS requirements.
- Spec Kit features MUST NOT violate SpecDD `Must not` or `Forbids` constraints. If a feature changes the architecture, `md-indexer.sdd` MUST be updated in the same change.
- For more details on the integration and mapping, refer to `docs/SpecDD_SpecKit_Integration_EN.md`.
- For instructions on how AI Agents should interact with the SpecDD framework and resolve constraints, refer to `docs/AI_Workflow_EN.md`.
- For the hybrid GraphRAG retrieval design of `md-ai`, refer to `docs/Hybrid_GraphRAG_DE.md`.

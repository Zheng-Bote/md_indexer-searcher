# AI Agent Instructions: SpecDD × GitHub Spec Kit Workflow

Practical guide for feature development in the `md-indexer` project using AI agents.

## The Agentic Workflow (Flow-Forward)

1.  **Specify (`/speckit.specify`):** Create feature branch. Write PRD in `specs/features/<name>/`.
2.  **Plan (`/speckit.plan`):** Validate against `md-indexer.sdd`. Ensure Read-Only constraints and SQLite rules are respected.
3.  **Tasks (`/speckit.tasks`):** Break plan into atomic packages. Explicitly name target components (`internal/parser`, `cmd/md-search`).
4.  **Implement (`/speckit.implement`):** Write code. **Mandatory Quality Gate:** Check for English comments, SPDX headers, and CGO-free dependencies.
5.  **Converge (`/speckit.converge`):** Compare code against Specs and `.sdd`. Fix drift.
6.  **Release:** Update `CHANGELOG.md` and prepare PR.

| Phase | AI Command | Main Artifact | AI Quality Gate |
| :--- | :--- | :--- | :--- |
| **1. Requirements** | `/speckit.specify` | Feature Spec (MD) | Scope is testable. |
| **2. Design** | `/speckit.plan` | Architecture Design | Alignment with `.sdd` (Read-only FS, SQLite). |
| **3. Tasking** | `/speckit.tasks` | Task Plan | Tasks are atomic and reviewable. |
| **4. Implementation**| `/speckit.implement` | Code & Tests | Code fulfills `.sdd` baselines (English, SPDX, No CGO). |
| **5. Review** | `/speckit.converge` | Delta Report | Code matches Specs. |

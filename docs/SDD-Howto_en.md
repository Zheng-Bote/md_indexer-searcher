# Developer Guide: SpecDD & GitHub Spec Kit

Welcome to the `md-indexer` project! We use **SpecDD** (Architectural source of truth) and **GitHub Spec Kit** (Feature development) to safely and iteratively build features.

## Why Two Frameworks?
*   **SpecDD (`.sdd` files):** The constitution. Found in `md-indexer.sdd`, it dictates the architecture (SQLite materialized view, Read-Only FS).
*   **GitHub Spec Kit (Flow-Forward Spec):** Structured work orders in `specs/`. We use Agentic commands to safely guide feature implementation.

## The Flow-Forward Workflow
1.  **Check Constraints:** Read `md-indexer.sdd` before planning.
2.  **Specify & Plan:** Run `/speckit.specify` to define the feature in `specs/features/<name>/`. Run `/speckit.plan` to map it against the `.sdd`.
3.  **Tasks:** Run `/speckit.tasks` to break down work (e.g., "Update parser", "Update DB schema").
4.  **Implement & Converge:** Loop between `/speckit.implement` (writing code) and `/speckit.converge` (validating against the spec and `.sdd`).
5.  **Quality Assurance:** Update the `CHANGELOG.md` and create a PR.

## Example: New Recursive Graph Feature
1. **Plan:** Specify a BFS shortest-path feature.
2. **Tasks:** 1. Write recursive SQL in `sqlite.go`. 2. Expose in `SearchEngine`. 3. Add to `md-api`.
3. **Implement:** Build it incrementally, ensuring we never write back to the markdown files (per `.sdd`).

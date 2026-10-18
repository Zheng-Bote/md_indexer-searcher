# Feature: [Feature Name]

**Status:** Draft | In Progress | Done
**Author:** [Name]
**Issue:** #[number]

## 1. Feature Intent
*What is the goal of this feature? What value does it add?*

## 2. Affected Components
*List the layers from `md-indexer.sdd` that will be modified: FS/Watcher, Parser & Extractor, Repository, Search/Graph Engine, AI Layer, API/CLI, Client Apps (`apps/desktop`).*

## 3. SpecDD Constraints & Architecture Impact
*Describe how this feature integrates with the existing SpecDD architecture. Explicitly address:*
- **Read-Only Vault:** Does it touch vault files? Markdown files must never be written.
- **Persistence:** Does it require new SQLite tables or columns? Is the index still rebuildable and are embeddings preserved on re-index?
- **Security:** Does vault content leave the machine (e.g. online LLM)? Is secret redaction applied? Are secrets kept out of CLI arguments, logs and API responses?
- **Interfaces:** New `md-api` endpoints, CLI flags, provider implementations or Tauri commands?
- **Architecture Change:** Does `md-indexer.sdd` need an update?

## 4. Implementation Tasks (AI-Friendly)
*List concrete, actionable tasks for implementation.*
- [ ] Task 1: Implement logic in the affected layer(s) with SPDX headers and English comments.
- [ ] Task 2: Add unit tests (`go test ./...`; `npm test` / `cargo test` for `apps/desktop`).
- [ ] Task 3: ...

## 5. Acceptance Criteria
*How do we know this is done?*
- [ ] ...
- [ ] CHANGELOG.md and README.md are up-to-date

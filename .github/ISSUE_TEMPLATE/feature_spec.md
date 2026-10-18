---
name: Feature Specification (Spec Kit)
about: Propose a new feature or rule for md-indexer, md-ai, md-api, md-search or the desktop app.
title: "Feature: "
labels: "feature, spec-kit"
assignees: ""
---

## Feature Intent

_Describe what the feature does and why it is needed. Keep it concise. Reference current behaviour (file/function) where relevant._

## Requirements (EARS Syntax)

_Use EARS patterns: `THE SYSTEM SHALL …` (ubiquitous), `WHEN <trigger>, THE SYSTEM SHALL …` (event), `WHILE <state>, …` (state), `WHERE <feature/option>, …` (optional), `IF <condition>, THEN THE SYSTEM SHALL …` (unwanted behaviour)._

1. **WHEN** _trigger_, **THE SYSTEM SHALL** _response_.
2. **IF** _condition_, **THEN THE SYSTEM SHALL** _response_.
3. **THE SYSTEM SHALL** _response_.

## Scope

_Which layers and files are affected? Delete the lines that do not apply._

- **FS / Watcher:** `internal/watcher/`
- **Parser & Extractor:** `internal/parser/`, `internal/service/`
- **Repository (SQLite / FTS5 / embeddings):** `internal/repository/`, `internal/embedding/`
- **Search / Graph Engine:** `internal/search/`, `internal/graph/`
- **AI (md-ai):** `internal/ai/` (`hybrid`, `assistant`, `provider`, `contextbuilder`, …)
- **API / CLI:** `internal/api/`, `cmd/indexer`, `cmd/md-api`, `cmd/md-search`, `cmd/md-ai`
- **Client app:** `apps/desktop/` (Angular frontend, Tauri `src-tauri`)

### Open Decisions

_Questions that must be answered before or during implementation (delete if none)._

## SpecDD Architecture Alignment (Drift Control)

Please confirm that this feature respects the constraints defined in `md-indexer.sdd`:

- [ ] **Architecture:** The layered architecture is maintained (Watcher -> Parser -> Repository -> Search/Graph Engine -> API/CLI; API handlers do not access the database directly).
- [ ] **Architecture:** Client apps under `apps/` use `md-api` over HTTP only and do not access the SQLite database or vault Markdown files directly.
- [ ] **Architecture:** Feature affects architecture: SpecKit feature forces update of `md-indexer.sdd`.
- [ ] **Read-Only Vault:** Markdown files remain the immutable single source of truth (no writes, no bidirectional sync).
- [ ] **Security:** Secret redaction in `contextbuilder` remains enforced; no secrets (API keys, tokens) in logs, CLI arguments or API responses.
- [ ] **Data Model:** SQLite remains a disposable, backwards-compatible index (`modernc.org/sqlite`, no CGO); existing embeddings survive re-indexing.
- [ ] **Go Standards:** Standard project layout (`cmd/`, `internal/`), Clean Architecture with Dependency Injection, no global state.
- [ ] **Standards:** SPDX headers (`Apache-2.0`) and English source documentation will be maintained.

## Acceptance Criteria

- [ ] Criterion 1
- [ ] Criterion 2
- [ ] Tests added/updated: `go test ./...` and, if the desktop app is affected, `npm test` / `cargo test` in `apps/desktop`.
- [ ] CHANGELOG.md and README.md are up-to-date

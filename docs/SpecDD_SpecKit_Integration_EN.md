# Integration Matrix: SpecDD × GitHub Spec Kit for md-indexer

This defines how architecture specification (SpecDD) and feature specification (GitHub Spec Kit) work together in `md-indexer`.

## 1. System Level vs Feature Level
*   **System Intent (`md-indexer.sdd`):** Architecture, DB (SQLite), Parsers, Read-Only norms.
*   **Feature Intent (`specs/`):** Isolated changes (e.g., new API endpoint, new CLI flag).

## 2. Integration Logic
*   **Architecture:** SpecDD is the Single Source of Truth.
*   **Database:** SpecKit features may add tables/queries but must not break the core `documents`/`fts5` structure defined in SpecDD.
*   **File System:** SpecDD enforces Read-Only. SpecKit features cannot introduce write capabilities to Markdown files.

## 3. Code Standards Baseline
All Spec Kit features MUST adhere to:
*   English documentation and comments.
*   SPDX Apache-2.0 headers.
*   No CGO dependencies.

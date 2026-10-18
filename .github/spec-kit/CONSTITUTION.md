# Spec Kit Constitution for md-indexer

**IMPORTANT: This project uses a hybrid SpecDD & Spec Kit architecture.**

## 1. System Intent & Single Source of Truth (SpecDD)
The architectural intent, global constraints, security norms (read-only vault, secret redaction, no direct database or vault access from client apps), and directory ownership are governed strictly by **SpecDD** (`.sdd` files).

Do NOT rely on this Constitution file for system rules. Instead, you MUST read the `.sdd` files (root spec: `md-indexer.sdd`) and the `.specdd/bootstrap.md` / `.specdd/bootstrap.project.md` files to understand what you are allowed to do.

## 2. Role of Spec Kit (Feature Intent)
Spec Kit is used exclusively for **Feature-Level Intent** (isolated changes such as retrieval strategies, LLM providers, API endpoints, CLI flags, desktop app features, bug fixes).
Feature specs are tracked as GitHub issues created from `.github/ISSUE_TEMPLATE/feature_spec.md`; `.github/spec-kit/feature_template.md` is the equivalent for file-based specs.
When implementing a Spec Kit feature:
- Read the feature's Markdown spec or the GitHub Issue.
- Verify that your proposed changes do not violate any `Must not` or `Forbids` rules from the SpecDD hierarchy.
- If the feature changes the architecture, update `md-indexer.sdd` in the same change.
- Ensure all Code Quality baselines (SPDX headers, English docs, standard Go layout, unit tests) are met.

## 3. Integration Reference
For a full breakdown of how these frameworks integrate, read:
[docs/SpecDD_SpecKit_Integration_EN.md](../../docs/SpecDD_SpecKit_Integration_EN.md)

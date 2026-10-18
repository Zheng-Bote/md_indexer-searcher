<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Feature Spec: Clear Previous Answer on New Question (Desktop)

**Issue:** [#21](https://github.com/Zheng-Bote/md_indexer-searcher/issues/21)  
**Status:** Done

## 1. Problem & Goal
`AskPanel.submit()` only replaced `answer` on success. While the next answer was generated (minutes on CPU) and after a failed request, the previous answer, its sources and its duration stayed visible and appeared to belong to the new question.

Goal: submitting a question removes the previous result from the view immediately.

## 2. Requirements (EARS)
1. **WHEN** the user submits a question via the "Fragen" button or `Ctrl+Enter`, **THE SYSTEM SHALL** remove the previously displayed answer, its sources and its duration before the request is sent.
2. **IF** the new request fails, **THEN THE SYSTEM SHALL** show only the error alert and no previous answer.
3. **WHEN** a question is submitted, **THE SYSTEM SHALL** keep the question text in the textarea.

## 3. Scope
*   `apps/desktop/src/app/workspace/ask-panel.ts` – reset `answer` and `durationMs` at the start of `submit()`.
*   `apps/desktop/src/app/workspace/ask-panel.spec.ts` – unit tests.

## 4. Non-Goals
*   No backend changes.
*   No answer history.

## 5. Tasks
*   [x] Task 1: Reset answer and duration on submit.
*   [x] Task 2: Unit tests for busy state, failure and kept question text.

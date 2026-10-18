<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# md-indexer-desktop

Desktop client for md-indexer (Tauri v2, Angular 22, Spartan-ng). Pick a vault, choose the models, ask questions.

## How it works

- The app starts two sidecars per vault, both bound to `127.0.0.1` on free ports:
  1. `md-indexer -dir <vault> -db <appData>/vaults/<hash>.db` builds and watches the index. Its `/health` only answers after the initial index is done, so the app shows "Indexiere Vault…" until then.
  2. `md-api -ai -db <same db>` serves `/api/ai/ask`, `/api/ai/models` and `/api/document`.
- Changing the vault or a model restarts both sidecars. Unchanged notes are not re-parsed, and embeddings are only computed for sections that are missing them.
- Without an embedding model, `md-api` runs with `-vectors=false` (full-text and graph retrieval only).
- `md-api` reads `~/.config/md-indexer/ai.yaml` as usual (Ollama host, context budget, …). Values chosen in the app take precedence.
- Clicking a source opens the document viewer. It loads the original Markdown via `/api/document` (stored in the index, so the app never reads vault files), hides frontmatter, scrolls to the cited section and turns resolvable wikilinks into links. "Zurück" walks the link history back to the answer, "Schließen"/`Esc` returns directly. External links, images and `![[embeds]]` are shown as text.
- Settings are stored in `settings.json` in the app data directory.

## Development

From the repository root:

```bash
make desktop-dev   # go build sidecars -> src-tauri/binaries, then `tauri dev`
make desktop       # release bundles
```

In `apps/desktop`:

```bash
npm test           # Vitest unit tests (ng test)
npm run build      # Angular production build
cd src-tauri && cargo test
```

`ng serve` alone has no Tauri APIs; use `npm run tauri dev` (or `make desktop-dev`).

## Layout

- `src/app/core/`: Tauri bridge (`BackendService`), md-api client (`ApiClient`), settings store
- `src/app/workspace/`: vault picker, model selector, question panel, answer view, document viewer (`document-viewer.ts`, rendering in `markdown-document.ts`)
- `libs/ui/`: generated Spartan-ng helm components
- `src-tauri/src/sidecar.rs`: sidecar lifecycle (`SidecarManager`)

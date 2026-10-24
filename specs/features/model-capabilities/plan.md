<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Architecture Plan: Classify Local Models by Ollama Capabilities

**Issue:** [#38](https://github.com/Zheng-Bote/md_indexer-searcher/issues/38)  
**Spec:** [spec.md](./spec.md)

## 1. Architectural Alignment (`md-indexer.sdd`)

- **AI Layer:** Only `OllamaProvider` changes; the cache is a field of the provider instance (md-api creates one local provider), guarded by a mutex.
- **API Layer / Client App:** The response shape is unchanged except for the new `kind` value `both`; the desktop filters already treat it correctly.

```
OllamaProvider.ListModels
  GET /api/tags                     -> name, digest, family
  for local, non-":cloud" models    (max 4 parallel, 5 s each)
    cache[name@digest] hit?         -> capabilities
    else POST /api/show {model}     -> capabilities (cached only on success)
  kindOf(capabilities) or ModelKind(name, families) as fallback
  GET /api/ps                       -> loaded marks (#37)
```

---

## 2. Technical Design

### A. `internal/ai/model/types.go`

`KindBoth = "both"` with a comment (generation and embedding).

### B. `internal/ai/provider/ollama.go`

- `ollamaTagsResponse`: add `Digest`.
- `OllamaProvider`: `capsMu sync.Mutex`, `caps map[string][]string` (key `name + "@" + digest`).
- `capabilities(ctx, name, digest) ([]string, error)`: cache lookup, else `POST /api/show` with a 5 s timeout; a non-empty result is cached.
- `ListModels`: after `/api/tags`, a bounded worker loop (`showConcurrency = 4`) fills a slice of capabilities for non-`:cloud` models; then `kindOf(caps)`: `embedding` + `completion` → `both`, `embedding` → embedding, `completion` → generation, otherwise `ModelKind` fallback.
- No change for models listed by the registry from `.env`.

### C. Desktop

`api.types.ts`: `kind?: 'generation' | 'embedding' | 'both'`; spec case in `model-options.spec.ts` (answer groups and `embeddingModels` both contain a `both` model).

### D. Docs

`openapi.yaml` (`kind` enum + description), `README.md`, `CHANGELOG.md`.

---

## 3. Verification Strategy

- **Go:** test server counting `/api/show` calls: kinds from capabilities (unusual embedding model, `both`, generation), `:cloud` not queried, fallback on `/api/show` 500 and on missing capabilities, second listing without new `/api/show` calls, changed digest queries again.
- **Desktop unit:** `both` in both selectors.
- **Manual:** `curl /api/ai/models` against local Ollama: kinds of `qwen3:14b`, `qwen3:32b`, `gemma3:12b`, `bge-m3` unchanged.
- `go vet ./...`, `go test ./...`, `npm test`, `ng build`.

## 4. Risks

- First listing with many models costs one `/api/show` per model (bounded, cached afterwards).
- Failed lookups are not cached, so a daemon without `capabilities` is asked again on each listing; acceptable for a local call, keeps the behaviour correct after an Ollama upgrade.

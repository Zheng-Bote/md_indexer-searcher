<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Feature Specification: Loopback by Default, No Wildcard CORS

**Issue:** [#31](https://github.com/Zheng-Bote/md_indexer-searcher/issues/31)  
**Status:** Implemented (`/speckit.implement`) · Decisions confirmed 2026-10-18

## 1. Feature Intent

`md-api` and `md-indexer` must not expose the vault beyond the local machine unless the user explicitly asks for it, and no web page in the user's browser may read the local API.

### Current State (verified on `main`)

| | `md-api` | `md-indexer` |
|---|---|---|
| Bind without `-host` | all interfaces, `:8081` | all interfaces, `:8080` |
| Address building | `*host + ":" + *port` (breaks for IPv6 literals) | same |
| CORS | `Access-Control-Allow-Origin: *` on every response | none |
| Routes | `/health`, `/api/search`, `/api/document`, `/api/documents/{name}`, `/api/entities/…`, `/api/backlinks/…`, `/api/impact/…`, `/api/graph/…`, `/api/ai/ask`, `/api/ai/models` | `/health`, `/documents` (all documents incl. extracted text), `/search` |

- The desktop app passes `-host 127.0.0.1` to both sidecars on random ports and talks to `md-api` through the Tauri HTTP plugin (no browser CORS involved). It only uses `/health` of `md-indexer` as its readiness signal.
- Nothing in the repository calls `md-indexer`'s `/documents` or `/search`. Both handlers query the repository directly, which contradicts `md-indexer.sdd` ("MUST NOT bypass the Search/Graph Engine … from API handlers").
- `Dockerfile` runs `CMD ["./md-api"]` and the README publishes ports `8080`/`8081`. Inside a container a loopback bind is not reachable through a published port.

## 2. User Scenarios

- **Scenario 1 — CLI Default:** `md-api -vault ~/Notes -ai` is started without `-host`. Another machine in the LAN cannot connect; `curl http://127.0.0.1:8081/health` on the same machine works.
- **Scenario 2 — Explicit Exposure:** `md-api -vault ~/Notes -host 0.0.0.0` is started deliberately. It listens on all interfaces and logs a warning that the API is reachable without authentication.
- **Scenario 3 — Malicious Web Page:** While `md-api` runs on `127.0.0.1:8081`, the user visits a page that runs `fetch("http://127.0.0.1:8081/api/search?q=password")`. The browser blocks reading the response because no `Access-Control-Allow-Origin` header is sent.
- **Scenario 4 — Desktop App:** Vault selection, questions, sources and the document viewer work unchanged.
- **Scenario 5 — Docker:** `docker run -p 8081:8081 …` still reaches `md-api`, because the container command binds explicitly to `0.0.0.0` inside the container's own network namespace.

## 3. Requirements (EARS Syntax)

1. **Loopback Default:**
   **WHEN** `md-api` or `md-indexer` is started without `-host`, **THE SYSTEM SHALL** listen on `127.0.0.1` only.
2. **Explicit Exposure Warning:**
   **WHEN** `-host` is set to an address that is not a loopback address (`127.0.0.0/8`, `::1`, `localhost`), **THE SYSTEM SHALL** log a warning that the API is reachable from the network without authentication.
3. **Address Formatting:**
   **THE SYSTEM SHALL** build the listen address with `net.JoinHostPort`, so IPv6 literals such as `::1` work.
4. **No Wildcard CORS:**
   **THE SYSTEM SHALL NOT** send `Access-Control-Allow-Origin` (or any other CORS header) from `md-api`, so browsers deny cross-origin reads.
5. **Index-Only Readiness Server:**
   **THE SYSTEM SHALL** serve only `/health` from `md-indexer`; querying the index is the job of `md-api`.
6. **Container Exposure:**
   **WHEN** `md-api` runs from the project `Dockerfile`, **THE SYSTEM SHALL** start it with `-host 0.0.0.0` so published ports keep working.
7. **Desktop Compatibility:**
   **THE SYSTEM SHALL** keep the desktop sidecars working without changes to their arguments (`-host 127.0.0.1` stays explicit).

## 4. Non-Goals

- Authentication for remote access (bearer token, mTLS) — tracked as blocker 4.3 in `docs/architektur_erweiterung.md`.
- A configurable CORS allowlist. No browser-based client exists; it can be added when one does (see Decisions).
- Fixing the outdated `Dockerfile` base image (`golang:1.21` vs. `go 1.27` in `go.mod`) or adding a vault/indexer setup to the container; only the `md-api` command changes.

## 5. Decisions (2026-10-18)

1. **No CORS allowlist** for now; requirement 4 drops CORS entirely. An allowlist (`-cors-origin`) can be added when a browser-based client exists.
2. **`md-indexer` serves only `/health`** (requirement 5): `/documents` and `/search` are unused, bypass the search engine layer, and `/documents` dumps the whole index.

## 6. Scope & Affected Components

- `cmd/md-api/main.go`, `cmd/indexer/main.go`: `-host` default `127.0.0.1`, `net.JoinHostPort`, warning for non-loopback hosts (shared helper, e.g. in `internal/api` or a small `internal/netaddr` package).
- `internal/api/rest/server.go`: remove the CORS middleware.
- `internal/api/server.go`: `/health` only.
- `Dockerfile`: `CMD ["./md-api", "-host", "0.0.0.0"]`.
- `md-indexer.sdd`: loopback default for all binaries, no wildcard CORS.
- `README.md` (`-host` section, Docker), `CHANGELOG.md` (behaviour change for users relying on LAN access), `docs/architektur_erweiterung.md` 4.4 → implemented.

## 7. SpecDD Architecture Alignment (Drift Control)

- [x] **Architecture:** Layering maintained; removing `md-indexer`'s `/documents` and `/search` removes handlers that bypassed the search engine.
- [x] **Architecture:** Client apps unchanged; they keep using `md-api` over HTTP on loopback.
- [x] **Architecture:** Feature affects architecture: `md-indexer.sdd` extended (loopback default for every binary, no wildcard CORS).
- [x] **Read-Only Vault:** Unchanged.
- [x] **Security:** Reduces exposure; no secrets involved.
- [x] **Data Model:** Unchanged.
- [x] **Go Standards:** No global state; small pure helper for address handling.
- [x] **Standards:** SPDX headers and English documentation.

## 8. Acceptance Criteria

- [x] Without `-host`, both binaries bind to `127.0.0.1` (checked via the listen address, and manually from another interface address).
- [x] A non-loopback `-host` logs the warning; loopback hosts (`127.0.0.1`, `::1`, `localhost`) do not.
- [x] No response of `md-api` carries an `Access-Control-Allow-Origin` header.
- [x] `md-indexer` answers `/health`; `/documents` and `/search` return `404`.
- [x] Desktop app works unchanged (`cargo test`; sidecar arguments unchanged, they already pass `-host 127.0.0.1`).
- [x] `go test ./...` with tests for the address helper and the missing CORS header.
- [x] `md-indexer.sdd`, `README.md`, `CHANGELOG.md`, `Dockerfile` are up-to-date.
- [x] `docs/architektur_erweiterung.md` 4.4 marked implemented (follow-up commit on PR #32).

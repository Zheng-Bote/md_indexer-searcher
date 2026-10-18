<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Task Breakdown: Loopback by Default, No Wildcard CORS

**Issue:** [#31](https://github.com/Zheng-Bote/md_indexer-searcher/issues/31)  
**Spec:** [spec.md](./spec.md) · **Plan:** [plan.md](./plan.md)

## Decisions

- [x] No CORS allowlist for now; `md-api` sends no CORS headers (2026-10-18).
- [x] `md-indexer` serves only `/health`; `/documents` and `/search` are removed (2026-10-18).

## Package & Implementation Tasks

### Task 1: `internal/netaddr`
- [x] `Listen(host, port)` and `IsLoopback(host)`.
- [x] Table tests.

### Task 2: Binaries (`cmd/md-api`, `cmd/indexer`)
- [x] `-host` default `127.0.0.1`, `net.JoinHostPort`, warning for exposed hosts.

### Task 3: HTTP Layer (`internal/api/rest`, `internal/api`)
- [x] Remove the CORS middleware from `md-api`.
- [x] `md-indexer` server: `/health` only; remove unused handlers, fields and constructor parameters.
- [x] Tests: no CORS header; `/health` 200, removed routes 404.

### Task 4: Container, Specs & Docs
- [x] `Dockerfile`: `md-api -host 0.0.0.0`.
- [x] `md-indexer.sdd`: loopback default for all binaries, no wildcard CORS.
- [x] `README.md`, `CHANGELOG.md`.
- [ ] `docs/architektur_erweiterung.md` 4.4 → implemented (after PR #32 is merged; follow-up).

### Task 5: Verification
- [x] `go vet ./...`, `go test ./...`, `cargo test`.
- [x] Manual: without `-host` both listen on `127.0.0.1` only (`ss -ltn`), request via LAN address refused, no CORS header with a foreign `Origin`, `/documents` 404; warning with `-host 0.0.0.0`.
- [x] Set spec status to Implemented.

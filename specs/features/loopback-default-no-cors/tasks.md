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
- [ ] `Listen(host, port)` and `IsLoopback(host)`.
- [ ] Table tests.

### Task 2: Binaries (`cmd/md-api`, `cmd/indexer`)
- [ ] `-host` default `127.0.0.1`, `net.JoinHostPort`, warning for exposed hosts.

### Task 3: HTTP Layer (`internal/api/rest`, `internal/api`)
- [ ] Remove the CORS middleware from `md-api`.
- [ ] `md-indexer` server: `/health` only; remove unused handlers, fields and constructor parameters.
- [ ] Tests: no CORS header; `/health` 200, removed routes 404.

### Task 4: Container, Specs & Docs
- [ ] `Dockerfile`: `md-api -host 0.0.0.0`.
- [ ] `md-indexer.sdd`: loopback default for all binaries, no wildcard CORS.
- [ ] `README.md`, `CHANGELOG.md`.
- [ ] `docs/architektur_erweiterung.md` 4.4 → implemented (after PR #32 is merged).

### Task 5: Verification
- [ ] `go vet ./...`, `go test ./...`, `cargo test`.
- [ ] Manual: listen sockets without `-host` are loopback-only; warning with `-host 0.0.0.0`.
- [ ] Set spec status to Implemented.

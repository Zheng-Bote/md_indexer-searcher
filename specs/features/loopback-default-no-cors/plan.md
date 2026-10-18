<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# Architecture Plan: Loopback by Default, No Wildcard CORS

**Issue:** [#31](https://github.com/Zheng-Bote/md_indexer-searcher/issues/31)  
**Spec:** [spec.md](./spec.md)

## 1. Architectural Alignment (`md-indexer.sdd`)

- **API / CLI Layer:** Only wiring in `cmd/` and the HTTP layer change; repository, search engine and AI layer are untouched.
- **Layering Repaired:** `md-indexer`'s `/documents` and `/search` handlers read the repository directly; removing them removes the only API handlers that bypass the search engine.
- **Client Apps:** The desktop sidecars already pass `-host 127.0.0.1` and use the Tauri HTTP plugin; no change needed.
- **No Global State:** The address logic is a pure function.

```
cmd/md-api, cmd/indexer
  -host (default "127.0.0.1"), -port
      └─ netaddr.Listen(host, port) -> addr = net.JoinHostPort(host, port), exposed = !loopback(host)
            exposed? -> log.Warn("listening on <addr>: reachable from the network without authentication")
  http.Server{Addr: addr}

md-api router: RequestID, Logger, Recoverer           (CORS middleware removed)
md-indexer router: Logger, Recoverer, GET /health     (/documents, /search removed)
```

---

## 2. Technical Design

### A. `internal/netaddr` (new, tiny)

```go
// Listen returns the address to listen on and whether it is reachable from other hosts.
func Listen(host, port string) (addr string, exposed bool)

// IsLoopback reports whether host is "localhost" or a loopback IP (127.0.0.0/8, ::1).
func IsLoopback(host string) bool
```

- `exposed` is true for every non-loopback host, including an explicitly empty `-host ""` (all interfaces) and `0.0.0.0` / `::`.
- A hostname other than `localhost` counts as exposed; no DNS lookups.

### B. Binaries

1. `cmd/md-api/main.go` and `cmd/indexer/main.go`:
   - `-host` default `"127.0.0.1"`; help text "use 0.0.0.0 to listen on all interfaces (no authentication)".
   - `addr, exposed := netaddr.Listen(*host, port)`; warn when `exposed`; `http.Server{Addr: addr}`.
2. `cmd/indexer/main.go`: `api.NewServer()` without arguments (see C).

### C. HTTP Layer

1. `internal/api/rest/server.go`: drop `middleware.SetHeader("Access-Control-Allow-Origin", "*")`.
2. `internal/api/server.go`: router with `/health` only; `getDocuments`, `search`, and the now unused `cfg`/`repo` fields and constructor parameters are removed.

### D. Container

`Dockerfile`: `CMD ["./md-api", "-host", "0.0.0.0"]`. The README's `docker run` example keeps working. (The outdated base image and the missing vault/indexer setup in the image are out of scope.)

### E. Specs & Docs

- `md-indexer.sdd`: replace the client-app loopback rule by "**MUST** bind HTTP servers to loopback by default; listening on other interfaces requires an explicit `-host` and logs a warning" and add "**MUST NOT** send wildcard CORS headers".
- `README.md`: `-host` paragraph (default loopback, `0.0.0.0` for exposure), md-indexer HTTP server = `/health` only, Docker note.
- `CHANGELOG.md` (`Security`, `Changed`): bind default, CORS removal, removed `md-indexer` routes.
- `docs/architektur_erweiterung.md` 4.4 → `[implementiert]` and blocker list: only after PR #32 is merged (the section does not exist on `main` yet); otherwise a follow-up commit on #32's branch.

---

## 3. Verification Strategy

- **Unit:** `netaddr` table test (`127.0.0.1`, `127.1.2.3`, `::1`, `localhost` → not exposed; `""`, `0.0.0.0`, `::`, `192.168.1.10`, `example.org` → exposed; `::1` formatted as `[::1]:8081`).
- **REST:** `md-api` responses (`/health`, `/api/search`, an error response) carry no `Access-Control-Allow-Origin`.
- **md-indexer server:** `/health` = 200, `/documents` and `/search` = 404.
- **Manual:**
  - start both binaries without `-host`, check the listen socket (`ss -ltn`) shows `127.0.0.1:<port>` only, and a request to the machine's LAN address is refused;
  - start with `-host 0.0.0.0`, check the warning;
  - desktop app (`make desktop-dev` is not required: the sidecar arguments are unchanged and covered by `cargo test`).
- `go vet ./...`, `go test ./...`, `cargo test`.

## 4. Risks

- **Behaviour change for LAN users:** anyone who reached `md-api` from another machine without `-host` must now pass `-host 0.0.0.0`. Documented in CHANGELOG and README; the startup warning makes the exposure explicit.
- **Removed `md-indexer` routes:** unused in the repository; external scripts using them must switch to `md-api` (`/api/search`).

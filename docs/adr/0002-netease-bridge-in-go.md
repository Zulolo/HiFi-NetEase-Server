# ADR-0002 · One Go service (`hifid`) with the NetEase adapter built on a pure-Go MIT library

- Status: accepted
- Date: 2026-09-13
- Requirements addressed: FR-1, FR-1.6, C-1, C-2, C-4, NFR-3, NFR-6

## Context

The official NetEase client cannot run on the boards (docs/02 §1.1). An open-source
implementation of the NetEase web/mobile API is needed on the server, plus an HTTP API, a
stream proxy, uploads and MPD control. Targets are Debian arm64 and riscv64 with 1 GB RAM.

## Options considered

| Option | Pros | Cons |
|---|---|---|
| **Go service using `chaunsin/netease-cloud-music`** (MIT, v0.8.0 2026-09, arm64 + riscv64 binaries, QR/cookie login, all levels, cloud disk) | One static binary per arch, ≈ 30–60 MB RSS, cross-compiles from Windows, active upstream, MIT-compatible | API library may lag NetEase changes (mitigated by the adapter interface) |
| Go service using `go-musicfox/netease-music` (MIT) | Same benefits, 160+ endpoints | Less active (2026-04); fewer login options |
| Node.js `api-enhanced` sidecar + thin Go/other front | Reference implementation, most complete, fastest to track NetEase changes | Node ≥ 22, 100–200 MB RSS, no official riscv64 Node binaries |
| Python (`musicbox` internals) | Active in 2026 | QR-only, TUI-centred, Python runtime + external players |
| `4fuu/net-mpd` (NetEase as an MPD-protocol server) | Clever precedent | Created 2026-07, 0 stars; plays through its own engine, not through MPD's DAC pipeline |
| Rust `ncm-api-rs` | Tiny RSS | Third toolchain; young |

## Decision

Implement `hifid` in Go as one binary that embeds the PWA, the REST/WebSocket API, the
NetEase adapter, the stream proxy, tus uploads, the MPD adapter and the output manager. The
NetEase adapter is an interface implemented first with `chaunsin/netease-cloud-music`;
`api-enhanced` is used as the endpoint specification and can be plugged in as a sidecar
implementation if the Go library breaks.

## Consequences

- Pure-Go dependencies only (SQLite via a pure-Go driver) so `CGO_ENABLED=0` cross-builds work.
- go-musicfox (GPL-3) is used only as a separate interim program in M1, never linked.
- A recorded-fixture test suite for the adapter is mandatory to detect API drift early.
- Repository licence should be MIT (open question Q10).

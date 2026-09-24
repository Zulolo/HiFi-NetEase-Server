# server/ · `hifid` (Go)

The single custom service of the project: one static binary running next to MPD
(ADR-0001 — MPD owns playback, `hifid` never touches samples).

## Status · M1 skeleton (v0.1.0-m1)

Implemented and verified on the Orange Pi Zero 3:

| Area | Endpoints |
|---|---|
| System | `GET /api/v1/system/status` |
| Player | `GET /player`, `POST /player/play\|pause\|stop\|next\|prev\|seek`, `PUT /player/volume\|options` |
| Queue | `GET /queue`, `DELETE /queue`, `DELETE /queue/{qid}` |
| Outputs | `GET /outputs`, `PUT /outputs/{id}/active` |
| Events | `GET /api/v1/ws` (MPD idle fan-out) |
| UI | mobile PWA embedded with `go:embed`, served at `/` |

`format.delivery` is reported from MPD's `audio` field cross-checked against
`/proc/asound/card*/pcm*p/sub*/hw_params`, so the UI shows what the DAC really
receives (FR-4.6). Verified: a DSD256 file reports
`native-dsd, 11289600, DSD_U32_BE @ 352800`, matching the kernel exactly.

Not yet implemented: the NetEase adapter and its stream proxy (M2, and note
ADR-0008 — the proxy must pipe bytes and correct `Content-Type`), the library
and upload modules, discovery, and per-DAC capability probing.

## Layout

```
server/
├── cmd/hifid/            main package: flags, wiring, graceful shutdown
└── internal/
    ├── config/           YAML config with defaults matching install.sh
    ├── player/           MPD adapter: status, transport, queue, outputs, idle watcher
    ├── api/              REST handlers + WebSocket hub
    └── web/              embedded PWA (go:embed static/)
```

Planned packages (docs/03): `netease/`, `library/`, `upload/`, `discovery/`.

## Build

Go 1.24+ and pure-Go dependencies only, so it cross-compiles from any host:

```sh
cd server
go build -trimpath -ldflags "-s -w -X main.version=$(git describe --tags --always)" -o dist/hifid ./cmd/hifid
# or for another target:
GOOS=linux GOARCH=arm64  CGO_ENABLED=0 go build -o dist/hifid-arm64  ./cmd/hifid
GOOS=linux GOARCH=riscv64 CGO_ENABLED=0 go build -o dist/hifid-riscv64 ./cmd/hifid
```

## Run

```sh
hifid --config /etc/hifid/config.yaml            # defaults work without the file
hifid --listen 0.0.0.0:8080                      # override the listen address
```

`HIFID_TOKEN` supplies the bearer token; `auth.mode: all` then requires it on
every call, `admin` (the default) leaves reads open on the trusted LAN.
MPD is reached through `/run/mpd/socket` when present, else TCP 6600.

## Cross-cutting rules (docs/03)

- The MPD queue never holds raw NetEase CDN URLs; it holds
  `http://127.0.0.1:<port>/stream/ncm/<id>` proxy URLs (C-2, ADR-0008).
- `netease` and `player` are interfaces: a broken NetEase API must not stop
  local playback (NFR-6).
- Nothing here touches audio samples; format decisions live in the generated
  `mpd.conf`.

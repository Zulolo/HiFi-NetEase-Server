# server/ · `hifid` (Go)

The single custom service of the project: one static binary running next to MPD
(ADR-0001 — MPD owns playback, `hifid` never touches samples).

## Status · daily-use complete (0.15, September 2026)

Verified on an Orange Pi Zero 3 (arm64, Debian 12); nothing in it is board-specific.

| Area | Endpoints (all under `/api/v1`) |
|---|---|
| System | `GET /system/status`, `GET /system/stats` (CPU/RAM/temperature/Wi-Fi), `POST /system/poweroff` |
| Player | `GET /player`, `POST /player/play\|pause\|stop\|next\|prev\|seek\|jump`, `PUT /player/volume\|options` |
| Queue | `GET /queue`, `POST /queue` (refs `ncm:<id>`, `ncm:playlist:<id>`, `local:<path>`), `DELETE /queue[/{qid}]` |
| Outputs | `GET /outputs`, `PUT /outputs/{id}/active` |
| Library | `GET /library?path=`, `/library/search`, `/library/stats`, `/library/tags?tag=artist\|album`, `/library/find` |
| NetEase | QR login, `GET /netease/playlists[/{id}/tracks]`, `/netease/search`, `/netease/daily`, `POST /netease/playlists/{id}/export` |
| Downloads | `POST /netease/download[/playlist]`, `GET\|DELETE /netease/downloads`, `POST …/pause\|resume\|retag` |
| Stream | `GET /stream/ncm/{id}` (loopback, token-free; what MPD fetches) |
| Events | `GET /ws` (MPD idle fan-out) |
| UI | mobile PWA embedded with `go:embed`, served at `/` |

`format.delivery` is reported from MPD's `audio` field cross-checked against
`/proc/asound/card*/pcm*p/sub*/hw_params`, so the UI shows what the DAC really
receives (FR-4.6). Verified: a DSD256 file reports
`native-dsd, 11289600, DSD_U32_BE @ 352800`, matching the kernel exactly.

Not implemented (by choice, see docs/11): browser upload (Samba is the import path),
UDP discovery (mDNS suffices), and per-DAC capability probing / `mpd.conf` generation
inside hifid (the installer's shell scripts do it; deferred until a second DAC is in use).

## Layout

```
server/
├── cmd/hifid/            main package: flags, wiring, graceful shutdown
└── internal/
    ├── config/           YAML config with defaults matching install.sh
    ├── player/           MPD adapter: status, transport, queue, outputs, idle watcher, library browse/list/find
    ├── netease/          session (cookie jar), catalogue, search, daily, quality ladders, stream proxy (pipe mode),
    │                     download + tagging, explicit download list (queue.go), playlist export
    ├── sysinfo/          /proc and /sys sampling for the header strip (Linux build tag, stub elsewhere)
    ├── api/              REST handlers + WebSocket hub, auth, disk stats
    └── web/              embedded PWA (go:embed static/), ETag-versioned
```

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
hifid --listen 0.0.0.0:80                        # override the listen address
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

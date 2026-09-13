# server/ · `hifid` (Go)

The single custom service of the project. One static binary, cross-compiled for
`linux/arm64` and `linux/riscv64`, running as a systemd service next to MPD.

No code yet. The layout below is the agreed structure (see docs/03 and docs/09).

```
server/
├── cmd/hifid/            main package: flag/env parsing, wiring, graceful shutdown
└── internal/
    ├── config/           YAML config loading + validation (paths, ports, tokens, per-DAC settings)
    ├── netease/          NetEase Cloud Music adapter: login (QR/phone/cookie), library browsing,
    │                     URL resolution with quality ladder, download with tagging
    ├── player/           MPD adapter (libmpdclient protocol over TCP/unix socket): queue, transport,
    │                     outputs, mixer, idle-event fan-out; capability probe of ALSA cards
    ├── library/          Local library on the USB disk: paths, scan trigger, duplicate check,
    │                     M3U/extm3u generation for NetEase playlists (so plain MPD clients see them)
    ├── upload/           tus resumable upload endpoint + finalize-into-library pipeline
    ├── discovery/        mDNS/DNS-SD advertisement (_hifid._tcp, _http._tcp) + UDP beacon fallback
    ├── api/              REST + WebSocket handlers (versioned under /api/v1), auth token middleware
    └── web/              Embedded PWA assets (go:embed of ../../web/dist)
```

Cross-cutting rules (from docs/03):

- The MPD queue never contains raw NetEase CDN URLs; it contains `http://127.0.0.1:<port>/stream/ncm/<id>` proxy URLs served by `netease` + `api`.
- `netease` and `player` are interfaces; a broken NetEase API must not stop local playback (NFR-6).
- Nothing in this service touches audio samples. Format decisions live in MPD configuration generated from `config` + `player` probes.

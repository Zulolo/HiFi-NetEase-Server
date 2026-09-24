# ADR-0008 · NetEase stream delivery: pipe the bytes and correct the Content-Type, not a 302 redirect

- Status: accepted (supersedes the "redirect by default" note in docs/08 §5)
- Date: 2026-09-24
- Requirements addressed: FR-1.4, FR-4.1, FR-4.6, NFR-1, C-2

## Context

`hifid` enqueues every NetEase track into MPD as a local proxy URL
(`http://127.0.0.1:8080/stream/ncm/<songID>`) because raw CDN URLs expire and are bound to the
session (C-2). The original design left two ways for that endpoint to deliver audio, and chose
the cheaper one as the default:

- **redirect** — answer `302 Location: <cdn url>` and let MPD's `curl` input fetch the CDN
  directly. Zero copy, no bytes through `hifid`.
- **pipe** — read the CDN response in `hifid` and stream it on to MPD.

Bench measurement on the target board (Orange Pi Zero 3, MPD 0.23.x, ES9039 dongle) on
2026-09-24 invalidated that default.

Resolving track 22605222 at `level=jymaster` returns a `.flac` object of 164 723 846 bytes that
`file` and `ffprobe` both confirm is **FLAC 24-bit/192 kHz**. The CDN nevertheless serves it with:

```
HTTP/1.1 206 Partial Content
Server: Tengine
Content-Type: audio/mpeg; charset=UTF-8
```

MPD selects its decoder from the MIME type. Given `audio/mpeg` it hands a FLAC bitstream to the
`mad` decoder and fails:

```
mad: input does not appear to be a mp3 bit stream
exception: Failed to decode "http://m801.music.126.net/...flac";
  avformat_open_input() failed: Invalid data found when processing input
```

A 302 does not avoid this: MPD follows the redirect to the CDN and reads the same wrong header.
So redirect mode cannot play any `lossless`, `hires` or `jymaster` track — that is, everything a
HiFi server exists for. Only the genuinely lossy levels (`standard`, `exhigh`), which really are
MP3, happen to match the advertised type.

## Options considered

| Option | Pros | Cons |
|---|---|---|
| **Pipe the bytes and re-send a correct `Content-Type`** | Fixes the only real failure; keeps `Range`/206 so seeking and read-ahead work; enables "cache while playing" and a future tee to disk; one place to add a browser-like User-Agent | `hifid` sits in the audio path: ~1.5 MB/s of copying for a 24/192 stream, plus its socket buffers |
| Keep redirect and force a file extension hint | No bytes through `hifid` | MPD still trusts the MIME type over the URL; does not fix it |
| Keep redirect and patch MPD to sniff content | Cheapest at runtime | Forking the playback engine for one CDN's bad header; breaks the "use stock Debian MPD" premise (ADR-0001) |
| Download every track before playing | Always correct | Defeats instant play; the offline sync job (docs/08 §7.1) already covers the durable case |

## Decision

`GET /stream/ncm/{id}` **pipes** the CDN response by default and rewrites `Content-Type` to match
the level actually granted (`audio/flac` for `lossless`/`hires`/`jymaster`, `audio/mpeg` for
`standard`/`higher`/`exhigh`). `Content-Length` and `Content-Range` pass through unchanged and
`Range` requests are honoured, because the CDN answers `206 Partial Content`.

`stream_mode: redirect` remains a configuration switch for lossy-only setups and for debugging,
but it is no longer the default. The config default in docs/10 §4.1 changes accordingly.

Verified end to end on the bench with a minimal pipe proxy: MPD played the `jymaster` stream and
ALSA reported `access: RW_INTERLEAVED, format: S24_3LE, rate: 192000, channels: 2` on the ES9039
dongle — bit-perfect 24/192 — while MPD used 0.6 % CPU and 80 MB RSS.

## Consequences

- The cost is bounded and affordable: a 24/192 FLAC stream is ≈ 1.5 MB/s. Measured headroom on
  the board is 1.7 GB free RAM and a mostly idle CPU (docs/05 §11), and Wi-Fi already carries the
  same bytes. Local playback of downloaded files never touches this path at all.
- The proxy is the natural place for the tee into `/srv/music/netease` ("cache while playing"),
  for the browser-like User-Agent, and for retrying with a freshly resolved URL when one expires
  mid-track.
- The adapter must record the **granted** level per track (docs/08 §4) since that decides which
  `Content-Type` to emit; it cannot assume the requested level was honoured.
- `hifid` must stream rather than buffer whole objects: a `jymaster` track is ~157 MB, far beyond
  what a 2 GB board should hold in memory.

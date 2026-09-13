# ADR-0001 · Music Player Daemon is the playback engine

- Status: accepted
- Date: 2026-09-13
- Requirements addressed: FR-4, FR-5, FR-2.8, NFR-1, NFR-3, NFR-4, NFR-6

## Context

The server must decode every common PCM codec and DSD, deliver bit-perfect audio to USB
DACs with native DSD → DoP → PCM fallback, switch between several outputs at runtime, keep a
queue and a local library, and be controllable from phones, on Debian arm64 and riscv64 with
1 GB of RAM (docs/01). Writing this ourselves is out of proportion; the question is which
existing engine to build on (survey in docs/02 §2).

## Options considered

| Option | Pros | Cons |
|---|---|---|
| **MPD 0.24** (C++, GPL-2, Debian trixie both arches) | Native DSD automatic, DoP per format, Dsd2Pcm fallback, hw: ALSA, multiple outputs with runtime enable/disable, hardware mixer, gapless, library DB, stored playlists, curl input with Range, `addtagid`, huge client ecosystem (M.A.L.P., myMPD, mpc), ≈ 40–80 MB RSS | C++ project we do not modify; config-file outputs (no runtime add) |
| Custom player on GStreamer 1.26 | Native DSD in alsasink, flexible | DoP not documented; we would write queue, library, protocol, clients |
| Mopidy | Python plugin model, Iris UI | GStreamer DSD problems, ≈ 2× RAM, no NetEase backend, weaker bit-perfect story |
| mpv / ffmpeg-based custom | Simple | DSD only as PCM 352.8 k; no DoP/native |
| squeezelite + Lyrion Music Server | Good DSD (`-D`) | Needs a Perl server, heavier, no NetEase plugin |
| Volumio / moOde image | Turnkey UI | Raspberry Pi only; no Orange Pi RV; fork required for NetEase |

## Decision

Use MPD unmodified as the only component that touches audio samples. `hifid` talks to it over
the MPD protocol (unix socket) and generates its configuration. The MPD TCP port stays open on
the LAN so existing clients keep working.

## Consequences

- DSD policy, format conversion and output switching are configuration, not code (docs/04).
- A new DAC requires one MPD restart (config blocks); known DACs do not.
- We inherit MPD's limits: software volume is a no-op on DSD; `allowed_formats` fallback
  semantics need verification on hardware (R11).
- Third-party MPD clients see the NetEase library only through exported `.m3u` playlists.

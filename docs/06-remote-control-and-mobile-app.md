# 06 · Remote Control and Mobile App

Status: draft v0.1 · 2026-09-13 · addresses FR-2, FR-5.2, NFR-5

## 1. Control paths, layered

The design offers four ways to control the server. They share the same MPD state, so any
mix can be used at the same time.

| Layer | What it gives | Needs our code? | When |
|---|---|---|---|
| L0 · MPD protocol clients | Transport, volume, queue, local library, stored playlists (including the exported NetEase playlists), **output selection** | No | Day 1 (M0) |
| L1 · myMPD (web PWA for MPD) | Same as L0 with a polished installable web UI, smart playlists, web radio | No (install package) | Optional, M0/M1 |
| L2 · `hifid` PWA | Everything in FR-2: NetEase browsing/search, login QR, downloads, per-DAC settings, format badge, upload page | Yes (the project) | M1–M4 |
| L3 · Native Android app | L2 plus lock-screen/media-session controls, LAN auto-discovery, background reliability | Yes, optional | M6 |
| Side path · DLNA renderer (`upmpdcli`) | Official NetEase phone app "casts" to the server; server not logged in | No (install package) | Optional |

## 2. Day-1 Android clients (L0)

| App | Status | Outputs | Playlists | Notes |
|---|---|---|---|---|
| M.A.L.P. | Open source (GPL-3), 1.3.2 (2024-07), on F-Droid and Play | Yes, incl. partitions | Yes | Recommended. Profiles for several servers, album art, queue editing. |
| MAFA | Closed source, 3.2.3 (2026-06), sideloaded APK (removed from Play 2025) | Yes | Yes | Most polished UI; closed source. |
| MPDroid / MPD Remote / Mupeace | Dead (2014–2022) | | | Avoid. |
| Cantata | Desktop only (Linux/Windows/macOS) | Yes | Yes | Useful on the PC. |

What L0 cannot do: search NetEase, log in, download, show delivery format, change per-DAC
DSD settings. That is exactly the scope of the `hifid` PWA.

## 3. myMPD as an optional companion (L1)

- C backend with an embedded HTTP server, no database, no PHP/Node; installable PWA.
- Packages: OBS repository for Debian 13 on arm64; riscv64 must be built from source (small
  CMake project).
- Value: web radio, smart playlists, Lua triggers that can call `hifid` endpoints (for example
  "download this song" as a myMPD script).
- Cost: one more service (≈ 20 MB RSS), one more UI to explain. Decision: document it, do
  not depend on it.

## 4. The `hifid` PWA (L2)

### 4.1 Stack

- Static single-page app embedded in the Go binary (`go:embed`), no build step needed on
  the board; built on the PC with a standard bundler.
- Framework: a small reactive framework (Svelte or Preact) to keep the bundle under 200 kB
  gzipped; the app runs on mid-range phones over Wi-Fi.
- State: one WebSocket (`/api/v1/ws`) delivering `player`, `queue`, `outputs`, `download.*`,
  `upload.*` events (docs/09 §11); REST for actions; optimistic UI for transport buttons.
- Offline behaviour: the service worker caches the shell only; API calls always go to the
  server (the server is on the LAN, no need for offline data).

### 4.2 Screens

| Screen | Content | Requirement |
|---|---|---|
| Now Playing | Cover, title, artist, album, progress/seek, play/pause/next/prev, repeat/shuffle, volume slider (hidden or disabled when the output is in `fixed` mode with a hint), **format badge** ("DSD128 native", "DoP 176.4k", "PCM 24/96", "PCM converted"), output chip that opens the output sheet | FR-2.1, FR-2.2, FR-4.6, FR-5.2 |
| NetEase | Tabs: Playlists, Liked, Daily, Search, Cloud disk; each row has play-next/append/download actions; playlist page has "play all", "shuffle", "download all" | FR-1.3, FR-2.3, FR-2.4 |
| Library | Artists, Albums, Folders, Recently added; same row actions minus download | FR-2.3, FR-3.3 |
| Queue | Reorder (drag), remove, clear, save as playlist | FR-2.3 |
| Downloads | Job list with progress, failed items with retry | FR-2.4 |
| Upload | Drop zone for files and folders, per-file progress, resume after reload, hash check result | FR-3 |
| Settings | NetEase login (QR), outputs table (alias, present, DSD mode, volume mode, max rate), server status, token | FR-1.1, FR-5.5, NFR-7 |

### 4.3 Discovery and first connection (FR-2.6)

Browsers cannot browse mDNS, so the PWA is reached by URL. Three aids:

1. Avahi publishes the host as `hifi.local`; Android 13+ and iOS resolve `.local` names
   system-wide, so `http://hifi.local` works on most phones.
2. The server prints a QR code with its URL on the status page and in the install script.
3. Fallback: the router's DHCP client list or a fixed DHCP lease.

The PWA remembers the last working URL in `localStorage`.

### 4.4 Installability and HTTPS

PWA "Add to Home Screen" with full app behaviour requires a secure context (HTTPS or
`localhost`). Options ranked by convenience:

| Option | How | Trade-off |
|---|---|---|
| A · Plain HTTP (default) | Chrome still offers "Add to Home screen" as a shortcut; no service worker | Good enough for control; no offline shell |
| B · Own CA with mkcert | `hifid` serves TLS with a cert for `hifi.local`; install the CA once on the phone | One-time manual step; full PWA |
| C · Real DNS name + Let's Encrypt (DNS-01) | `music.example.com` → LAN IP; automated renewal | Needs a domain; cleanest long-term |

The Go binary can terminate TLS itself; nginx (`deploy/nginx`) is only kept as an
alternative.

Browser note: Chrome 142+ shows a "local network access" prompt only when a *public* page
calls a *private* address; the PWA is served from the private address itself and is not
affected.

### 4.5 Security (NFR-5)

- `Authorization: Bearer <token>` for admin calls (login, settings, uploads, downloads);
  configurable to require it for everything.
- Token entered once in Settings; stored in `localStorage`; never in the URL.
- Server binds only to the LAN interface; the stream proxy binds to loopback.

## 5. Native Android app (L3, optional)

Trigger for building it: the PWA proves insufficient for lock-screen media controls or for
automatic LAN discovery.

| Approach | Pros | Cons |
|---|---|---|
| Capacitor 7 wrapper around `web/` | Same UI code; add NSD discovery and media-session plugins | Web view performance; plugin maintenance |
| Kotlin + Jetpack Compose | Best media integration (MediaSession, Android Auto), NSD API | Second UI codebase |
| Flutter | One codebase for iOS too; `multicast_dns` package | Larger app, third toolchain |

Platform requirements to plan for:

- Cleartext HTTP to LAN IPs is blocked by default since API 28: ship a
  `network_security_config` allowing cleartext for private ranges, or use TLS (option B/C).
- Android 16 introduces an opt-in local-network permission; from the Android 17 target level
  it is mandatory for mDNS/SSDP. Request `NEARBY_WIFI_DEVICES` and handle denial with the
  manual URL fallback.
- Discovery: Android NSD for `_hifid._tcp`; fallback UDP probe on port 8081 (docs/09 §12).

## 6. DLNA side path (official NetEase app as controller)

**What DLNA is.** DLNA (Digital Living Network Alliance, built on UPnP AV) is the standard
that lets an app on your phone "cast" to a player box on the same Wi-Fi: the phone acts as
the *controller*, the box is the *renderer*. The phone sends "play this URL" plus
play/pause/next/volume commands; the renderer fetches the audio itself and plays it. It is
the same mechanism Xiaomi speakers and many TVs use.

**How it helps here.** The official 网易云音乐 Android app has a cast button (投射到设备) that
lists DLNA renderers on the LAN. If the server runs `upmpdcli`, MPD appears in that list, and
the official app becomes a remote control for the server immediately: browse and search in
the app you already know, tap cast, and the sound comes out of the ES9039Q2M dongle through
MPD's normal bit-perfect path. No server-side login, no custom code, and the DAC selection
made in the PWA still applies. It is a convenient extra for guests and for the first week
before `hifid` exists, not a replacement: the phone must stay on the LAN, the quality is what
the app chooses (a 2024 report shows 96 kHz streams arriving this way), and there is no
download, no local library and no format badge.

- `upmpdcli` turns MPD into a UPnP AV / OpenHome renderer (1.9.x, GPL-2, vendor apt repo for
  Debian arm64; build from source on riscv64). It shares MPD's outputs, so the DAC selection
  made in the PWA applies to casts as well.
- The NetEase Android app has had a DLNA "投射到设备" entry that moved between versions and
  was removed and restored around 2022; the current entry point must be verified on the
  phone in use before relying on it.
- Limits: the server is not logged in, quality is whatever the phone app chooses, no download,
  no local library. Documented as a convenience for guests.

## 7. Subsonic/OpenSubsonic route (rejected)

A Subsonic facade over MPD would let DSub/Ultrasonic use "jukebox mode", but the API has no
output selection, the maintained clients (Symfonium, Tempo) do not implement jukebox, and
prior facades (mpdsub, mpdsonic) are archived. Not pursued.

## 8. Home Assistant (bonus)

The Home Assistant MPD integration gives play/pause/next/volume and stored-playlist selection
for automations; output switching would need `rest_command` calls into `hifid`. Documented,
not built.

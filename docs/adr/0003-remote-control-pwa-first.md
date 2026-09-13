# ADR-0003 · PWA first for phone control; MPD protocol stays open; native app optional

- Status: accepted
- Date: 2026-09-13
- Requirements addressed: FR-2, FR-2.6, FR-2.8, FR-5.2

## Context

The owner needs phone control on the LAN, including NetEase browsing, downloads and DAC
selection. A native Android app was suggested as "maybe needed". Survey in docs/06.

## Options considered

| Option | Pros | Cons |
|---|---|---|
| **Mobile-first PWA served by `hifid`** | One codebase for Android, iOS and the PC upload page; no store; instant updates; WebSocket live state | Install needs HTTPS for full PWA features; no mDNS in browsers; weaker lock-screen media controls |
| Native Android app first (Kotlin/Compose) | Best media-session integration, NSD discovery | Second codebase before the API is stable; Android-only; Android 16/17 local-network permission work |
| Only existing MPD clients (M.A.L.P., myMPD) | Zero code; outputs and volume supported | No NetEase search/login/downloads/DAC settings |
| Subsonic facade for DSub/Ultrasonic jukebox | Existing polished apps | No output selection in the API; maintained clients lack jukebox; prior facades archived |
| DLNA renderer only (`upmpdcli`) | Official NetEase app as the controller | Server not logged in; no downloads/local library |

## Decision

Build the PWA as the primary controller on top of the `/api/v1` REST + WebSocket API. Keep
MPD's port 6600 open so M.A.L.P. works from day 1 and as a fallback. Export NetEase playlists
as MPD stored playlists so plain clients can play them. Defer a native app (Capacitor wrapper
or Kotlin) to M6, triggered only by concrete PWA shortcomings. Install `upmpdcli` as an
optional add-on.

## Consequences

- The API (docs/09) is the contract for all clients; OpenAPI document generated from code.
- Provide the own-CA TLS option so the PWA can be installed with a service worker.
- Discovery relies on `hifi.local` + a QR code; a native app would add NSD later.

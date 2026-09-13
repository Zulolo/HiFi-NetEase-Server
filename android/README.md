# android/ · Optional native app (phase 3)

Not started. Decision in ADR-0003: the PWA is the first controller; a native
Android app is only built if the PWA falls short (background media controls,
lock-screen controls, LAN discovery). It must use the same `/api/v1` REST +
WebSocket API as the PWA and add nothing server-side.

Candidate approaches (docs/06): Kotlin + Jetpack Compose (thin client), or a
Capacitor wrapper around `web/` with an NSD (mDNS) discovery plugin.

Day-1 alternative that needs no code: M.A.L.P. (open-source MPD client) for
transport, volume, output selection and the NetEase playlists exported as MPD
stored playlists.

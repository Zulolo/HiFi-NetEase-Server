# 11 · Roadmap and Milestones

Status: draft v0.1 · 2026-09-13

Each milestone ends in something the owner can use on the real speaker. Effort is a rough
solo-developer estimate; hardware verification steps are explicit because DAC behaviour on
Linux can only be confirmed on the actual dongles.

## M0 · Bench verification · done 2026-09-24 on an Orange Pi Zero 3 (2 GB)

Result: `deploy/scripts/install.sh` brings a fresh Debian 12 board to a working bit-perfect
MPD server with Samba and Avahi in one run; the Comtrue/ES9039 dongle plays PCM to 768 kHz
and native DSD64/128 (after the installer's kernel quirk); everything survives a reboot
(details in ADR-0007 and docs/05 §11). Still running: the 24 h Wi-Fi and playback soak.

Original plan, kept for readers repeating it on other hardware:

0. Record each dongle's `lsusb` ID (docs/12 Q13) and make sure the board has a port for the
   disk and one for the DAC (hub or header ports if needed), a heatsink and its rated supply.
1. Flash the distribution image, check the music disk with `f3probe`, attach it and the
   DAC, join Wi-Fi with power save off. Run `deploy/scripts/install.sh` (it enables the
   watchdog timer when the default route is wireless).
2. `apt install mpd mpc alsa-utils ffmpeg samba`; run `deploy/scripts/probe-dac.sh` on every dongle:
   record VID:PID, ALSA card name, `/proc/asound/cardX/stream0` formats, native DSD flag,
   supported rates. Fill the table in docs/04 §7.
3. Hand-written `mpd.conf` with one `hw:` output; play FLAC 16/44, 24/96, 24/192, DSD64,
   DSD128 from the disk with `mpc`; note delivery mode per dongle (native/DoP/PCM).
4. Run `deploy/scripts/bench-dsd.sh`: CPU % for dsd2pcm and for soxr on this CPU;
   write results into docs/05 §"Measured on hardware".
5. Install M.A.L.P. on the phone, control MPD on port 6600: play/pause/volume/outputs.
6. Thresholds (ADR-0007): 24 h Wi-Fi log (ping loss, outage length, `iperf3` every hour) and
   24 h DSD128/24-192 local playback while a 500 MB album is copied over Samba; record RAM
   headroom. Optionally install `upmpdcli` and test the NetEase app's cast button (docs/06 §6).

Exit criteria: 24 h of 24/192 local playback without dropouts; each dongle's best DSD mode
known; Wi-Fi outages ≤ 60 s and loss ≤ 2 %, otherwise escalate (USB Wi-Fi adapter, then RV).

## M1 · NetEase via MPD, interim tools · ~1 week

Goal: listen to the NetEase library on the Acton IV from the phone, using existing software.

1. Build/install **go-musicfox** on the board (Go, arm64/riscv64), configure `mpd` engine,
   QR login, play own playlists from an SSH terminal (Termux/JuiceSSH on the phone). This
   validates the NetEase API from the board's IP and the account's quality entitlement.
2. Start the `hifid` skeleton: config, MPD adapter, `/api/v1/player` + `/queue`, WebSocket
   events, minimal PWA with now-playing/transport/volume/outputs.
3. Deploy units from `deploy/systemd`, udev names from `deploy/udev`.

Exit criteria: phone PWA controls MPD; NetEase playable through go-musicfox.

## M2 · Native NetEase integration in `hifid` · ~2–3 weeks

1. NetEase adapter (ADR-0002 library), QR login, session persistence.
2. Stream proxy `/stream/ncm/{id}` with redirect mode, URL cache, `addtagid` metadata.
3. Catalogue endpoints + PWA screens: playlists, liked, daily, search, cloud disk.
4. Playlist export to `playlists/NetEase/*.m3u` (extm3u) so M.A.L.P. sees them.
5. Download jobs with tagging and index; proxy prefers local copies.
6. Offline sync scheduler (docs/08 §7.1): subscriptions, night window, pacing, resume,
   "on disk / wanted" status in the PWA. This is the main mode of use.

Exit criteria: FR-1 acceptance test passes; a subscribed playlist is fully on disk after one
night; go-musicfox no longer needed.

## M3 · Import and library · ~1–2 weeks

1. Samba share (installed in M0) wired to `hifid`: inotify watcher, debounced incremental
   MPD updates, "recently added" index, duplicate warning.
2. Library screens (artists/albums/folders/recent), `update` triggers, cover art.
3. tus endpoint, finalize pipeline, hash verification (secondary path).
4. Upload page in the PWA (drag-and-drop folders, parallel chunks, resume after reload).

Exit criteria: FR-3 acceptance test (4 GB DSF folder over Wi-Fi, via Samba and via the web page)
passes.

## M4 · Output manager and DSD polish · ~1–2 weeks

1. ALSA probe → capability JSON per dongle; stable ids from VID:PID:serial.
2. `mpd.conf` generation with per-DAC blocks (dsd_mode, volume_mode, max_rate),
   controlled MPD restart, hot-plug via udev → `hifid` → rescan.
3. `format.delivery` reporting (native/DoP/converted) from MPD status + ALSA hw_params.
4. Volume policy per output (hardware/fixed), UI feedback.
5. Optional: offline PCM twin for DSD256 files, only if the CS43131 dongle is used for them.

Exit criteria: FR-4 and FR-5 acceptance tests pass on the ES9039Q2M dongle (default) and the
CS43131 dongle.

## M5 · Hardening · ~1 week

1. 72 h soak test, RAM/CPU profile against NFR-3, boot-time measurement (NFR-2).
2. Token auth, LAN binding audit, cookie encryption, log rotation.
3. Backup/restore of `/srv/data/hifid` and the MPD DB; disk-full behaviour.
4. Install script `deploy/scripts/install.sh` for a fresh Debian image on both boards.

## M6 · Optional extensions (pick by value)

| Extension | Value | Effort |
|-----------|-------|--------|
| Native Android app (Kotlin or Capacitor wrapper) with media-session lock-screen controls and NSD discovery | Convenience | 2–3 weeks |
| `upmpdcli` DLNA renderer so the official NetEase app can cast to the server | Guests, zero-login use | 1 evening |
| Lyrics (NetEase lyric endpoint) in the PWA | Nice | 2 days |
| Simultaneous outputs (FR-5.6) | Rare | 1 day (MPD already supports) |
| Home Assistant `mpd` integration | Automations | 1 hour |
| Second source (e.g. QQ 音乐) behind the same source interface | Future | unknown |

## Dependencies and ordering

```mermaid
gantt
    dateFormat  YYYY-MM-DD
    title Milestones (indicative, solo developer, part time)
    section Verify
    M0 Bench verification        :m0, 2026-09-15, 5d
    section Build
    M1 Interim NetEase + hifid skeleton :m1, after m0, 7d
    M2 NetEase integration       :m2, after m1, 18d
    M3 Upload and library        :m3, after m2, 10d
    M4 Output manager and DSD    :m4, after m3, 10d
    M5 Hardening                 :m5, after m4, 7d
    section Optional
    M6 Extensions                :m6, after m5, 14d
```

M0 must come first: the DAC probe results decide how much of M4 is needed (if all dongles do
native DSD out of the box, M4 shrinks to configuration generation).

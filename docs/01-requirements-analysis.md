# 01 · Requirements Analysis

Status: draft v0.1 · 2026-09-13

This document restates the five demands from the project owner, interprets each one
in engineering terms, and derives testable acceptance criteria, constraints, and
non-functional requirements. Later documents (architecture, audio pipeline, hardware
evaluation) refer back to the requirement IDs defined here (FR-x, NFR-x, C-x).

## 1. Context

| Item | Given |
|------|-------|
| Speaker | Marshall Acton IV. Inputs: 3.5 mm AUX (analog), RCA (analog), Bluetooth 5.3 (SBC/AAC/LDAC/LC3). No USB, no S/PDIF, no HDMI, no Wi-Fi. |
| Connection to speaker | 3.5 mm AUX cable from a USB DAC dongle. |
| Server board candidates | Orange Pi Zero 3 (Allwinner H618, arm64) and Orange Pi RV (RISC-V). Both run Debian. |
| OS storage | 64 GB microSD (TF) card. |
| Music storage | 512 GB USB disk. |
| DACs | Several "USB-C/USB-A to 3.5 mm" HiFi dongles, mostly ES9039Q2M- or CS43131-based, with different USB bridge chips. |
| Music source 1 | 网易云音乐 (NetEase Cloud Music) account with a large saved library (playlists, liked songs, possibly 云盘 cloud disk). |
| Music source 2 | Local files on a Windows PC, to be imported over the LAN. |
| Controller | A mobile phone (Android assumed; iOS should not be excluded) on the same Wi-Fi/LAN. |
| Network | Home LAN. No requirement for access from the internet. |

## 2. Demands, interpretation, and acceptance criteria

### FR-1 · NetEase Cloud Music on the server, logged in with the owner's account

**Demand (verbatim):** "NetEast Music should be ported to arm linux and I can login using my account."

**Interpretation.** The official NetEase Cloud Music client is closed source; it cannot be
"ported" by a third party, and its Linux builds target x86_64 (older 1.2.x) or UOS/deepin
desktops. A headless server cannot use a desktop GUI client anyway. What can be delivered is
an open-source *NetEase client service* on the server that speaks the NetEase web/mobile API
with the owner's own credentials and exposes the owner's library (playlists, liked songs,
daily recommendations, search, cloud disk) to the playback engine and to the remote control
app. The owner's account is the only account used; nothing is done to bypass licensing
(no "unblock" plugins are part of the baseline).

**Sub-requirements**

| ID | Requirement |
|----|-------------|
| FR-1.1 | Login with the owner's account using QR-code scan from the NetEase phone app (preferred, avoids password/captcha), with phone+password/captcha and cookie import as fallbacks. |
| FR-1.2 | Session persists across reboots (cookie stored on the server, encrypted at rest, LAN-only). |
| FR-1.3 | Browse: my playlists, liked songs (我喜欢的音乐), daily recommendation, playlist detail, album, artist, search, cloud disk (云盘). |
| FR-1.4 | Resolve a playable stream URL at the best quality the account is entitled to (standard → higher → exhigh → lossless → hires → …), with graceful downgrade. |
| FR-1.5 | Download a song / whole playlist to the local library with tags and cover art embedded, so it is playable offline and shows up in the local library. |
| FR-1.6 | Runs on Debian arm64 and riscv64 without a display, as a systemd service, within the RAM budget of the smallest candidate board (see NFR-3). |

**Acceptance.** From a phone on the LAN: scan QR → see own playlists → tap a song → sound
comes out of the Acton IV within 3 s. Reboot the board → still logged in.

### FR-2 · Remote control from the phone on the LAN

**Demand:** "select, play, stop, volume up and down, next, download… maybe one android app."

| ID | Requirement |
|----|-------------|
| FR-2.1 | Transport: play, pause, stop, next, previous, seek, repeat/shuffle. |
| FR-2.2 | Volume up/down/set/mute. Must not break bit-perfect DSD playback (see FR-4 and ADR-0005). |
| FR-2.3 | Select what to play: NetEase playlists/songs/search results, local library (artist/album/folder), the play queue. |
| FR-2.4 | Trigger downloads (FR-1.5) and see progress. |
| FR-2.5 | Choose the output DAC (FR-5). |
| FR-2.6 | Discover the server on the LAN without typing an IP (mDNS/DNS-SD, plus manual fallback). |
| FR-2.7 | Live state: now-playing, position, volume, queue update pushed to the phone (WebSocket/SSE), so two controllers stay in sync. |
| FR-2.8 | Delivery: a mobile-first web app (PWA) is the first deliverable; a native Android app is optional and must use the same API. Off-the-shelf MPD clients (e.g. M.A.L.P.) must keep working for basic transport control. |

**Acceptance.** Two phones open the UI; pressing play on one updates the other within 1 s.
All FR-2.1 to FR-2.5 actions are reachable in at most 2 taps from the now-playing screen.

### FR-3 · Import/upload local music from the PC over the LAN

**Demand:** "maybe one web server is needed on the server and support multi-files uploading."

| ID | Requirement |
|----|-------------|
| FR-3.1 | Browser upload of many files at once (multi-select and drag-and-drop of folders), optionally preserving folder structure. |
| FR-3.2 | Large files (a single DSD256 track can exceed 1 GB; an album 3 to 6 GB) must upload reliably; interrupted uploads resume rather than restart. |
| FR-3.3 | Uploaded files land on the 512 GB USB disk, are scanned into the library automatically, and appear in the phone UI without manual steps. |
| FR-3.4 | Alternative zero-UI import paths are documented and optionally enabled: SMB share (Windows Explorer drag-and-drop), SFTP/rsync. |
| FR-3.5 | Duplicate detection (same path or same audio hash) at least warns. |

**Acceptance.** Drag a 4 GB folder of DSF files onto the web page from Windows; all files
arrive intact (hash-verified), and the album is browsable on the phone within 1 minute
after the last file completes.

### FR-4 · Format support beyond NetEase, including DSD; hardware first, software fallback

**Demand:** "First choice the HiFi USB to 3.5mm DAC adapter HW to decode, if the HW doesn't
support, use CPU to SW decode."

**Interpretation.** A USB Audio Class DAC accepts only PCM (and, for some devices, DSD)
sample streams; it does not decode FLAC/MP3/AAC/ALAC. "Hardware decode" therefore means:
deliver the audio to the DAC in its *native* form so that the DAC chip performs the
digital-to-analog conversion without the CPU altering the samples. Concretely:

| Content | "HW decode" (preferred) | "SW decode" fallback |
|---------|-------------------------|----------------------|
| PCM lossless/lossy (FLAC, ALAC, WAV, AIFF, APE, WavPack, MP3, AAC, OGG, Opus) | CPU unpacks the container/codec (unavoidable, cheap); PCM is passed bit-perfect at the file's own sample rate and bit depth. | Only if the DAC does not support that rate/depth: resample or re-quantize on the CPU with a high-quality resampler. |
| DSD (DSF, DFF, DSD64/128/256/512) | 1) Native DSD over USB (ALSA `DSD_U32_BE`) if kernel + DAC support it; 2) else DoP (DSD-over-PCM) if the DAC supports DoP at the required PCM container rate. | 3) Convert DSD to PCM on the CPU (352.8 kHz or lower) and play as PCM. |

| ID | Requirement |
|----|-------------|
| FR-4.1 | Play at least: FLAC, ALAC, WAV, AIFF, APE, WavPack, MP3, AAC (m4a), OGG Vorbis, Opus, DSF, DFF (DSDIFF), and NetEase-served MP3/FLAC streams. |
| FR-4.2 | Automatic per-DAC capability detection: supported PCM rates/depths, native DSD, DoP, max DSD rate. |
| FR-4.3 | Decision chain native DSD → DoP → software conversion is automatic per output, but the user can pin a mode per DAC. |
| FR-4.4 | Bit-perfect PCM path by default: no resampling, no software mixing, no system sound server in the path. |
| FR-4.5 | Software fallback quality: DSD-to-PCM and any resampling use a high-quality algorithm and must not stutter on the target CPU (see calculations in doc 05). |
| FR-4.6 | The UI shows the actual delivered format (e.g. "DSD128 native", "DoP 176.4k", "PCM 24/96") so the user can verify. |

**Acceptance.** Play a DSD64 file on an ES9039Q2M dongle: its DSD indicator (if any)
lights and the kernel stream shows a DSD format. On a DAC without DSD support the same
file plays as PCM with no dropouts and the UI reports "software conversion".

### FR-5 · Selectable output channel among several USB DACs

**Demand:** "I need to select by which channel the music will be output… different HiFi
USB to 3.5mm DAC adapters, mostly ES9039Q2M or CS43131."

**Interpretation.** "Channel" = output device. Several DAC dongles (and possibly the board's
own analog jack) may be attached at once; the user picks the active one from the phone.

| ID | Requirement |
|----|-------------|
| FR-5.1 | List attached audio outputs with a human name (DAC model or user alias), not just `card1`. |
| FR-5.2 | Switch the active output from the phone; playback continues (a short gap is acceptable). |
| FR-5.3 | Stable identity across reboots and re-plugging (same dongle → same name, regardless of ALSA card index). |
| FR-5.4 | Hot-plug: a newly inserted DAC appears in the list without a reboot; an unplugged active DAC falls back to another one or pauses with a clear message. |
| FR-5.5 | Per-output settings persist: DSD mode, volume mode (hardware / fixed 100 %), max sample rate. |
| FR-5.6 | Optional: play on more than one output simultaneously (nice-to-have, not required). |

## 3. Constraints

| ID | Constraint | Consequence |
|----|-----------|-------------|
| C-1 | Official NetEase client is closed source and desktop-only. | Use an open-source API implementation; keep the NetEase adapter isolated behind an interface so it can be swapped when the API changes. |
| C-2 | NetEase stream URLs expire (minutes) and depend on account entitlement (VIP for lossless/Hi-Res on many tracks). | The playback engine must never hold raw CDN URLs in its queue; resolve lazily through a local proxy. |
| C-3 | Unofficial API use is against NetEase ToS in spirit; endpoints change without notice. | Personal, single-account, LAN-only use; no redistribution; expect maintenance. Documented in 12-risks. |
| C-4 | Two target architectures: arm64 (Zero 3) and riscv64 (RV). | Prefer components packaged in Debian for both, and Go for custom code (trivial cross-compilation, no runtime). Avoid Electron/Node/Chromium on the server. |
| C-5 | Smallest board has 1 GB RAM and USB 2.0 only. | RAM budget ≤ 300 MB for all our services; USB 2.0 is sufficient for audio (see doc 05). |
| C-6 | Acton IV inputs are analog; it digitizes AUX internally for its DSP. | The audible ceiling is set by the speaker's ADC/DSP. The chain is still built bit-perfect up to the DAC, but ultra-high rates (≥ 384 kHz, DSD512) bring no audible gain on this speaker. |
| C-7 | OS on a microSD card. | Keep write-heavy data (library DB, caches, downloads, logs) on the USB disk; keep the card mostly read-only in spirit. |
| C-8 | Headless, no display attached. | All administration via web UI/SSH; QR login rendered in the web UI. |

## 4. Non-functional requirements

| ID | Requirement | Target |
|----|-------------|--------|
| NFR-1 | Audio integrity | Bit-perfect PCM path; no dropouts for ≥ 24 h continuous playback at 24/192 PCM and DSD128. |
| NFR-2 | Startup | Cold boot to "ready to play" ≤ 60 s; services auto-start and auto-restart. |
| NFR-3 | Resource budget | RSS of our services ≤ 300 MB on a 1 GB board; idle CPU < 5 %. |
| NFR-4 | Portability | Same install procedure on Debian 12/13 arm64 and Debian 13 riscv64. |
| NFR-5 | Security | Services bound to LAN interfaces only; NetEase cookie encrypted at rest; upload endpoint requires a shared token; no internet exposure. |
| NFR-6 | Maintainability | NetEase adapter, player adapter, and UI are separate modules with documented interfaces; a broken NetEase API must not break local playback. |
| NFR-7 | Observability | Structured logs, a status page showing DAC capabilities, current stream format, queue, and NetEase session health. |
| NFR-8 | Data safety | Uploaded/downloaded files are never deleted automatically; library rescans are non-destructive. |

## 5. Out of scope (for now)

- Multi-room / synchronized playback to several speakers.
- Bluetooth output to the Acton IV (would re-encode to SBC/AAC/LDAC; contradicts the HiFi goal).
- Access from outside the LAN; user accounts for multiple people.
- Other streaming services (QQ 音乐, Spotify, Tidal). The architecture keeps a "source" abstraction so they could be added.
- Lyrics display, scrobbling, EQ/DSP (possible later, but any DSP breaks bit-perfect by design).

## 6. Glossary

| Term | Meaning |
|------|---------|
| Bit-perfect | Samples reach the DAC unchanged: no resampling, mixing, dithering or volume scaling in software. |
| DSD | Direct Stream Digital, 1-bit sigma-delta audio; DSD64 = 2.8224 MHz, DSD128 = 5.6448 MHz, DSD256 = 11.2896 MHz, DSD512 = 22.5792 MHz. |
| DoP | DSD over PCM: DSD bytes packed into 24-bit PCM frames with marker bytes; a DoP-capable DAC unpacks them. DSD64 needs a 176.4 kHz PCM container, DSD128 352.8 kHz, DSD256 705.6 kHz. |
| Native DSD | DSD sent over USB as raw DSD samples (ALSA format `DSD_U32_BE`); requires kernel support for the specific device. |
| UAC2 | USB Audio Class 2.0, the protocol used by essentially all HiFi USB dongles. |
| MPD | Music Player Daemon, an open-source headless audio player with a network control protocol. |
| PWA | Progressive Web App: a web page installable on the phone home screen. |

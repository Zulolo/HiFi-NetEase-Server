# HiFi-NetEase-Server

A headless HiFi music server for small Debian single-board computers (Orange Pi, Raspberry Pi
and similar) that plays a 网易云音乐 (NetEase Cloud Music) library and local files, including
DSD, bit-perfect through USB DAC dongles into any amplifier or active speaker with an analog
input, controlled from a phone on the LAN, with music imported from a PC over the LAN.

**Status: NetEase integration working (milestones M0–M2 core done).** `deploy/scripts/install.sh`
turns a fresh Debian board into a bit-perfect MPD server with DAC auto-detection (including
native DSD where the kernel allows it) and a Samba share; `deploy/scripts/install-hifid.sh`
adds the `hifid` service and its phone web app at the bare hostname (port 80; myMPD, if installed, sits on 8080). From the phone you can log in to
NetEase by QR code, browse your playlists, play a track live (streamed at lossless) or add it
to an explicit download list (fetched at the best quality your account grants, then played
from disk), and browse everything already on the disk, uploads included. Start with the
[user manual](docs/USER-MANUAL.md); progress and what is still open are in
[docs/11](docs/11-roadmap-and-milestones.md); the design documents are in
[docs/](docs/00-overview.md).

## What it will do

| Need | Delivered by |
|---|---|
| NetEase Cloud Music on ARM / RISC-V Linux, logged in with your own account | `hifid` NetEase adapter (Go, QR-code login) feeding MPD through a local stream proxy; downloads with tags; scheduled offline sync of chosen playlists |
| Remote control from a phone (select, play, stop, volume, next, download) | Mobile PWA served by `hifid`; M.A.L.P. and any other MPD client also work |
| Import local music from a PC over the LAN, many files at once | Samba share for Explorer drag-and-drop (auto-scanned); resumable browser uploads (tus) as a second path |
| All common formats plus DSD, hardware first, software fallback | MPD: native DSD → DoP → DSD-to-PCM conversion; bit-perfect PCM via ALSA `hw:` |
| Choose which DAC plays | One MPD output per DAC, udev-stable names, switch from the phone, hot-plug |

## Repository layout

```
docs/          design documents 00–12 and ADRs (start with docs/00-overview.md)
server/        hifid Go service: MPD adapter, NetEase adapter + pipe proxy, download list, REST/WS API, embedded PWA
web/           (the PWA currently lives in server/internal/web/static and is embedded into hifid)
android/       optional native app (planned, phase 3)
deploy/        mpd.conf template, udev rules, systemd units, probe and watchdog scripts
tools/         bench scripts to fill the measurement tables in docs/05
```

## Recommended hardware

The design is architecture-neutral (armhf, arm64, riscv64). What matters:

| Part | Recommendation |
|---|---|
| Board | Any Debian-capable SBC with ≥ 512 MB RAM and at least one USB 2.0 host port. Verified: Orange Pi Zero 3 (Allwinner H618, 2 GB, Debian 12). Also targeted: Orange Pi RV (riscv64); any Raspberry Pi 3 or newer works the same way. CPU is not the constraint; Wi-Fi driver quality and RAM are (docs/05). |
| Network | Ethernet if available. On Wi-Fi, prefer boards with a mainline Wi-Fi driver (Broadcom `brcmfmac`, MediaTek `mt76`); the design is offline-first so weak Wi-Fi only slows imports and background downloads (ADR-0007). |
| Music storage | USB flash drive or SSD (ext4). Check real capacity with `f3probe` before trusting a large cheap stick. |
| DAC | Any USB Audio Class 2 dongle. Verified: a Comtrue-bridge ES9039 dongle (`2fc6:f802`), PCM to 768 kHz and native DSD after the installer's kernel quirk. ES9039Q2M-class dongles do native DSD up to DSD512; CS43131-class dongles do DoP up to DSD128. Whether Linux offers native DSD depends on the USB bridge chip (docs/04 §4). |
| Speaker / amp | Anything with an analog line input (3.5 mm AUX or RCA). Note that smart speakers digitize the AUX input internally, which caps the audible benefit of very high sample rates (docs/05 §10). |
| NetEase account | Quality ladder follows the account tier (standard → exhigh → lossless/hires with VIP → jymaster with SVIP) and is configurable. |

## Quick start

```
scp -r deploy user@board:~/hifi/
ssh user@board
cd ~/hifi/deploy/scripts && chmod +x *.sh
sudo ./install.sh --disk /dev/sdb1 --format --with-mympd   # or --disk-label hifi / --no-disk
sudo ./test-audio.sh                                        # proves the bit-perfect path
```

Then open `http://<board-ip>/` on the phone (myMPD web client, no app needed) or add the
server (port 6600) in M.A.L.P. from F-Droid, and drop music onto `\\board\music`.
Full instructions and troubleshooting: [docs/USER-MANUAL.md](docs/USER-MANUAL.md).

## Next step

Milestone M1/M2: the `hifid` Go service (NetEase login, stream proxy, offline sync, REST API)
and the phone PWA, per [docs/11](docs/11-roadmap-and-milestones.md).

## Licence

MIT, see [LICENSE](LICENSE).

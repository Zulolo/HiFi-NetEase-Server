# HiFi-NetEase-Server

A headless HiFi music server for small Debian single-board computers (Orange Pi, Raspberry Pi
and similar) that plays a 网易云音乐 (NetEase Cloud Music) library and local files, including
DSD, bit-perfect through USB DAC dongles into any amplifier or active speaker with an analog
input, controlled from a phone on the LAN, with music imported from a PC over the LAN.

**Status: design phase.** This repository currently contains the requirements analysis,
open-source survey, architecture, calculations, decision records and deployment templates.
No application code yet; see [docs/11](docs/11-roadmap-and-milestones.md) for the plan.

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
server/        hifid Go service (layout only, no code yet)
web/           mobile-first PWA (planned)
android/       optional native app (planned, phase 3)
deploy/        mpd.conf template, udev rules, systemd units, probe and watchdog scripts
tools/         bench scripts to fill the measurement tables in docs/05
```

## Recommended hardware

The design is architecture-neutral (armhf, arm64, riscv64). What matters:

| Part | Recommendation |
|---|---|
| Board | Any Debian-capable SBC with ≥ 512 MB RAM and at least one USB 2.0 host port. Tested targets in order of priority: Orange Pi Zero LTS (Allwinner H3, 512 MB), Orange Pi RV (StarFive JH7110, riscv64); any Raspberry Pi 3 or newer works. CPU is not the constraint; Wi-Fi driver quality and RAM are (docs/05). |
| Network | Ethernet if available. On Wi-Fi, prefer boards with a mainline Wi-Fi driver (Broadcom `brcmfmac`, MediaTek `mt76`); the design is offline-first so weak Wi-Fi only slows imports and background downloads (ADR-0007). |
| Music storage | USB flash drive or SSD (ext4). Check real capacity with `f3probe` before trusting a large cheap stick. |
| DAC | Any USB Audio Class 2 dongle. ES9039Q2M-class dongles do native DSD up to DSD512; CS43131-class dongles do DoP up to DSD128. Whether Linux offers native DSD depends on the USB bridge chip (docs/04 §4). |
| Speaker / amp | Anything with an analog line input (3.5 mm AUX or RCA). Note that smart speakers digitize the AUX input internally, which caps the audible benefit of very high sample rates (docs/05 §10). |
| NetEase account | Quality ladder follows the account tier (standard → exhigh → lossless/hires with VIP → jymaster with SVIP) and is configurable. |

## Next step

Milestone M0 (bench verification, no custom code): flash Debian/Armbian on the board, install
`mpd` and `samba`, run `deploy/scripts/probe-dac.sh` on each dongle, fill the DAC matrix in
[docs/04 §7](docs/04-audio-pipeline-dsd-dac.md), and check the Wi-Fi and USB-audio
thresholds of [ADR-0007](docs/adr/0007-primary-target-board.md).

## Licence

MIT, see [LICENSE](LICENSE).

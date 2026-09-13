# HiFi-NetEase-Server

A headless HiFi music server for an Orange Pi (Debian) that plays a 网易云音乐 (NetEase Cloud
Music) library and local files, including DSD, through USB DAC dongles (ES9039Q2M, CS43131)
into a Marshall Acton IV over 3.5 mm AUX, controlled from a phone on the LAN, with browser
uploads from a PC.

**Status: design phase.** This repository currently contains the requirements analysis,
open-source survey, architecture, calculations, decision records and deployment templates.
No application code yet; see [docs/11](docs/11-roadmap-and-milestones.md) for the plan.

## What it will do

| Demand | Delivered by |
|---|---|
| NetEase Cloud Music on ARM/RISC-V Linux, logged in with my account | `hifid` NetEase adapter (Go, QR login) feeding MPD through a local stream proxy; downloads with tags |
| Remote control from my phone (select, play, stop, volume, next, download) | Mobile PWA served by `hifid`; M.A.L.P. and any MPD client also work |
| Upload local music from my PC over the LAN, many files at once | Resumable tus uploads in the PWA (multi-GB safe); optional Samba share |
| All common formats plus DSD, hardware first, software fallback | MPD: native DSD → DoP → DSD-to-PCM, bit-perfect PCM via ALSA `hw:` |
| Choose which DAC/channel plays | MPD output blocks per DAC, udev-stable names, switch from the phone, hot-plug |

## Repository layout

```
docs/          design documents 00–12 and ADRs (start with docs/00-overview.md)
server/        hifid Go service (layout only, no code yet)
web/           mobile-first PWA (planned)
android/       optional native app (planned, phase 3)
deploy/        mpd.conf template, udev rules, systemd units, install/probe scripts
tools/         bench scripts to fill the measurement tables in docs/05
```

## Hardware (owner's setup)

- Orange Pi Zero 3 (Allwinner H618, arm64) — primary target; Orange Pi RV (StarFive JH7110, riscv64) — second target
- 64 GB microSD for the OS, 512 GB USB disk for music
- USB-C/USB-A to 3.5 mm DAC dongles based on ES9039Q2M and CS43131
- Marshall Acton IV (AUX 3.5 mm / RCA analog inputs)

## Next step

Milestone M0 (bench verification, no custom code): flash Armbian Debian 13 on the Zero 3,
install `mpd`, run `deploy/scripts/probe-dac.sh` on every dongle, and fill the DAC matrix in
[docs/04 §7](docs/04-audio-pipeline-dsd-dac.md). Open questions for the owner are listed in
[docs/12 §2](docs/12-risks-and-open-questions.md).

## Licence

Proposed: MIT (compatible with all selected libraries). To be confirmed by the owner.

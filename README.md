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
| Import local music from my PC over the LAN, many files at once | Samba share for Explorer drag-and-drop (primary, auto-scanned); resumable tus uploads in the PWA (secondary, multi-GB safe) |
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

- Orange Pi RV (StarFive JH7110, riscv64, Wi-Fi via Broadcom AP6256) — deployment board; Orange Pi Zero LTS (Allwinner H3, 512 MB) — usable only with a mainline-driver USB Wi-Fi adapter or Ethernet, decided by the M0 bake-off (ADR-0007)
- Wi-Fi only at the speaker's location (no Ethernet)
- 64 GB microSD for the OS, 512 GB USB flash drive for music
- USB-C/USB-A to 3.5 mm DAC dongles based on ES9039Q2M (default output) and CS43131; DSD library up to DSD256
- Marshall Acton IV (AUX 3.5 mm / RCA analog inputs)
- NetEase Cloud Music SVIP account (unlocks 超清母带 `jymaster` quality)

## Next step

Milestone M0 (bench verification, no custom code): install Debian 13 on the Orange Pi RV (and
on the small board once its model is confirmed), install `mpd`, run
`deploy/scripts/probe-dac.sh` on every dongle, fill the DAC matrix in
[docs/04 §7](docs/04-audio-pipeline-dsd-dac.md), and run the Wi-Fi/USB-audio bake-off of
[ADR-0007](docs/adr/0007-primary-target-board.md). Remaining questions for the owner are in
[docs/12 §2](docs/12-risks-and-open-questions.md) (Q11–Q13).

## Licence

MIT, see [LICENSE](LICENSE).

# 00 · Documentation Overview

Design set for a headless HiFi music server on an Orange Pi that plays the owner's 网易云音乐
library and local files (including DSD) through USB DAC dongles into a Marshall Acton IV, controlled
from a phone. Read in order; each document cites requirement IDs from 01.

| # | Document | What it answers |
|---|---|---|
| 01 | [Requirements analysis](01-requirements-analysis.md) | What exactly is being asked (FR/NFR/C IDs), acceptance criteria, glossary |
| 02 | [Open-source survey](02-open-source-survey.md) | Which existing projects exist, their status in 2026, and what was chosen |
| 03 | [Solution and architecture](03-solution-architecture.md) | Components, responsibilities, runtime flows, deployment view |
| 04 | [Audio pipeline: DSD, DACs, outputs](04-audio-pipeline-dsd-dac.md) | Bit-perfect chain, native/DoP/PCM decision, kernel quirks, volume policy, output switching |
| 05 | [Hardware evaluation and calculations](05-hardware-evaluation-and-calculations.md) | Board comparison and decision matrix, bit rates, USB/storage/CPU/RAM/power/network budgets |
| 06 | [Remote control and mobile app](06-remote-control-and-mobile-app.md) | Control layers, day-1 apps, PWA design, native app plan, DLNA side path |
| 07 | [Upload and local library](07-upload-and-local-library.md) | tus upload pipeline, SMB alternative, library layout, formats |
| 08 | [NetEase integration design](08-netease-integration-design.md) | Login, quality ladder, stream proxy, playlist export, downloads, rate limits |
| 09 | [API design](09-api-design.md) | REST + WebSocket contract of `hifid` |
| 10 | [Deployment and operations](10-deployment-and-operations.md) | OS image, install steps, systemd, config files, security, backup |
| 11 | [Roadmap and milestones](11-roadmap-and-milestones.md) | M0 bench verification through M6 extensions |
| 12 | [Risks and open questions](12-risks-and-open-questions.md) | Risk register, questions for the owner, assumptions |
| ADR | [Architecture decision records](adr/) | 0001 MPD engine · 0002 Go service + NetEase library · 0003 PWA first · 0004 tus uploads · 0005 DSD strategy · 0006 output selection · 0007 target board |

Diagrams are Mermaid blocks inside the documents (03 and 11); GitHub and VS Code render them.

## The design in five lines

1. **MPD** plays everything, bit-perfect, with native DSD → DoP → PCM fallback and switchable USB outputs.
2. **`hifid`** (one Go binary) logs into NetEase with the owner's account, proxies streams into MPD, exports playlists, downloads with tags, accepts resumable uploads, probes DACs and generates MPD's config.
3. **PWA** on the phone for everything; M.A.L.P. works from day 1; a native app is optional.
4. **Orange Pi RV** (riscv64, Debian 13) is the deployment board because Wi-Fi is the only link and it has the mature Broadcom driver, more RAM and four USB ports; the Orange Pi Zero LTS (H3, 512 MB) qualifies only with a mainline-driver USB Wi-Fi adapter, decided by the M0 bake-off (ADR-0007).
5. **M0** proves each dongle's DSD mode on the bench before any feature code is written.

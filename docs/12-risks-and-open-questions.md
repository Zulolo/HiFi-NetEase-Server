# 12 · Risks and Open Questions

Status: draft v0.1 · 2026-09-13

## 1. Risk register

| # | Risk | Likelihood | Impact | Mitigation | Owner / when |
|---|---|---|---|---|---|
| R1 | NetEase changes or blocks the unofficial API (weapi/eapi/xeapi); the Go library lags | Medium (happened to pyncm and the original Node API in 2024–2026) | NetEase features stop; local playback unaffected | Adapter behind an interface; three candidate libraries; recorded-fixture tests; downloads make the library partly offline | M2, ongoing |
| R2 | Account flagged by NetEase risk control (风控) | Low–Medium | Login blocked for a period | QR login only, cookie persistence (`MUSIC_U`, `NMTID`), no login loops, no scrobble/task automation, home IP only, request limiter | M2 |
| R3 | Native DSD not offered by the kernel for a given dongle | Medium (Savitech-bridge dongles; unknown bridges) | Falls back to DoP or PCM; DSD256 on CS43131 becomes software | Probe in M0; `quirk_flags` per device; upstream patch; DoP works on all DSD dongles | M0 |
| R4 | USB volume control corrupts DoP on some dongles | Medium | Noise burst when changing volume during DoP | Default `fixed` volume for DoP outputs until tested; per-output policy | M0 |
| R5 | Dropouts on the SBC (USB isochronous + disk + network load) | Low–Medium (no H618/JH7110-specific reports either way) | Audible glitches | Large ALSA buffer, RT priority, governor, Ethernet, DAC and disk on different ports, soak test | M0, M5 |
| R6 | Software DSD fallback overloads the CPU (DSD256 → 80 % of an A53 core measured on Pi 3) | High if triggered | Dropouts | Fallback rate pinned to 352.8 k, `soxr medium`, offline PCM twin for DSD256+ | M4 |
| R7 | Power: bus-powered HDD + dongle exceed the Zero 3's USB supply | Medium if HDD | Disk resets, DAC re-enumeration | Powered hub or SSD; 5 V/3 A PSU; documented in docs/05 §8 | M0 |
| R8 | Orange Pi RV kernel/DTB availability for Debian 13 (mainline DTB only in 6.19) | Medium | Extra work to build a kernel | Zero 3 is primary; RV is secondary; riscv64 support is a build target, not a launch blocker | M5 |
| R9 | Stream URL expiry (1200 s) during long prebuffers or after pause | Low | Song fails to resume from the CDN | Proxy resolves fresh URLs on each open; MPD reopens on seek; downloaded copies bypass it | M2 |
| R10 | PWA install limits (HTTPS) and Android local-network permission changes (Android 16/17) | Medium | "Add to home screen" without full PWA features; native app needs new permission handling | Plain HTTP works for control; own-CA TLS option; permission handling documented for the native app | M1, M6 |
| R11 | MPD `allowed_formats` fallback selection differs from the intended DSD-to-PCM rate | Medium | Unnecessary resampling | Verify in M0; dedicated fallback output block with `format` as backstop | M0 |
| R12 | microSD wear from logs/DB | Low | OS corruption | Journald size cap, MPD DB and all writes on the USB disk, `noatime` | M1 |
| R13 | GPL boundaries: go-musicfox is GPL-3 | n/a | Cannot be linked into `hifid` | Used only as a separate interim program; `hifid` uses MIT libraries | — |
| R14 | Legal/ToS: unofficial API use | Accepted | Personal use | Single account, LAN only, no unblock modules, no redistribution of content | — |
| R15 | Acton IV AUX path limits audible benefit of Hi-Res/DSD | Certain | Expectation | Documented (C-6); the chain is still bit-perfect for future equipment | — |

## 2. Open questions for the project owner

| # | Question | Why it matters | Default assumption if unanswered |
|---|---|---|---|
| Q1 | Which "Orange Pi RV" exactly: RV (StarFive JH7110, 4× U74) or RV2 (Ky X1, 8 cores)? | Kernel path and USB topology differ | RV (JH7110) |
| Q2 | Which Zero 3 RAM variant (1 / 1.5 / 2 / 4 GB)? | 1.5 GB needs an Armbian fix; 1 GB tightens the RAM budget | 2 GB |
| Q3 | Is the 512 GB USB disk an SSD/flash drive or a 2.5" HDD? | Power budget and hub requirement | SSD or flash |
| Q4 | Exact dongle models / `lsusb` IDs of the ES9039Q2M and CS43131 adapters | Determines native DSD prospects and udev names | Probe in M0 |
| Q5 | Phone OS: Android only, or also iOS? | Native app scope; PWA works on both | Android primary, iOS via PWA |
| Q6 | NetEase account tier (free / 黑胶 VIP / SVIP)? | Sets the achievable quality ladder and whether `jymaster` matters | 黑胶 VIP |
| Q7 | Is Ethernet available at the speaker's location? | Wi-Fi is fine for control, poor for uploads and slightly riskier for streaming | Ethernet |
| Q8 | Should the official NetEase app's DLNA casting be supported (install upmpdcli)? | Cheap add-on, but needs verification on the phone's app version | Yes, optional |
| Q9 | Is a Samba share acceptable on the LAN (Windows drag-and-drop)? | Bulk import convenience | Yes |
| Q10 | Licence for this repository (MIT suggested, compatible with all chosen libraries) | Publishing | MIT |

## 3. Assumptions made in this design

- The owner accepts an open-source re-implementation of the NetEase client rather than the
  official binary (C-1).
- Single household, single NetEase account, trusted LAN.
- The 3.5 mm output of the dongle drives the Acton IV AUX at line level (dongle volume at
  100 % is within the speaker's input range; if it clips, the `hardware` volume mode or a
  fixed lower level is used).
- Debian 13 (trixie) or Armbian's Debian 13 is acceptable as the OS baseline.

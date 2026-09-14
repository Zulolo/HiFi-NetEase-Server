# 12 · Risks and Open Questions

Status: draft v0.1 · 2026-09-13

## 1. Risk register

| # | Risk | Likelihood | Impact | Mitigation | Owner / when |
|---|---|---|---|---|---|
| R1 | NetEase changes or blocks the unofficial API (weapi/eapi/xeapi); the Go library lags | Medium (happened to pyncm and the original Node API in 2024–2026) | NetEase features stop; local playback unaffected | Adapter behind an interface; three candidate libraries; recorded-fixture tests; downloads make the library partly offline | M2, ongoing |
| R2 | Account flagged by NetEase risk control (风控) | Low–Medium | Login blocked for a period | QR login only, cookie persistence (`MUSIC_U`, `NMTID`), no login loops, no scrobble/task automation, home IP only, request limiter | M2 |
| R3 | Native DSD not offered by the kernel for a given dongle | Medium (Savitech-bridge dongles; unknown bridges) | Falls back to DoP or PCM; DSD256 on CS43131 becomes software | Probe in M0; `quirk_flags` per device; upstream patch; DoP works on all DSD dongles | M0 |
| R4 | USB volume control corrupts DoP on some dongles | Medium | Noise burst when changing volume during DoP | Default `fixed` volume for DoP outputs until tested; per-output policy | M0 |
| R5 | Dropouts on the SBC (USB isochronous + disk + Wi-Fi interrupt load) | Medium (Wi-Fi is the primary link; no H618/JH7110-specific USB-audio reports either way) | Audible glitches | Large ALSA buffer and 8–16 MB MPD buffer, RT priority, governor, Wi-Fi power save off, DAC and disk on different ports, soak test over Wi-Fi | M0, M5 |
| R6 | Software DSD fallback overloads the CPU (DSD256 → 80 % of an A53 core measured on Pi 3) | Low (ES9039Q2M is the default output and handles DSD256 natively or via DoP) | Dropouts | Fallback rate pinned to 352.8 k, `soxr medium`, optional offline PCM twin | M4 (optional) |
| R7 | Power: USB devices exceed the board's USB supply | Low (flash drive ≈ 0.5 W + dongle ≈ 1 W) | Device resets | 5 V/3 A PSU (Zero 3) or 5 V/4 A (RV); documented in docs/05 §8 | M0 |
| R16 | XR819 Wi-Fi instability on the Zero LTS interrupts phone control, Samba copies or background downloads (accepted: local playback does not depend on Wi-Fi) | High | Annoyance, slow imports; never a playback dropout for local files | Watchdog timer restarts the link; power save off; fixed AP channel; offline-first sync with resumable downloads; PWA auto-reconnect; escalation: `mt76` USB adapter, then the RV | M0 |
| R18 | Bulk NetEase downloading on an always-on box trips risk control (风控) | Low–Medium | Account temporarily blocked | Pace the sync job (≤ 1 song per 15 s, ≤ 600 per day, night window), download only what the account is entitled to, no scrobble/task traffic, exponential back-off on errors | M2 |
| R17 | Counterfeit or slow 512 GB flash drive (fake capacity, 5–10 MB/s sustained writes, thermal throttling) | Medium | Data loss, slow imports | `f3probe`/`f3write` check before use; `noatime`, `commit=60`; keep SQLite/MPD DB writes small; consider a USB SSD later | M0 |
| R8 | Orange Pi RV kernel/DTB availability for Debian 13 (mainline DTB only in 6.19) | Medium | Extra work to build a kernel | Zero 3 is primary; RV is secondary; riscv64 support is a build target, not a launch blocker | M5 |
| R9 | Stream URL expiry (1200 s) during long prebuffers or after pause | Low | Song fails to resume from the CDN | Proxy resolves fresh URLs on each open; MPD reopens on seek; downloaded copies bypass it | M2 |
| R10 | PWA install limits (HTTPS) and Android local-network permission changes (Android 16/17) | Medium | "Add to home screen" without full PWA features; native app needs new permission handling | Plain HTTP works for control; own-CA TLS option; permission handling documented for the native app | M1, M6 |
| R11 | MPD `allowed_formats` fallback selection differs from the intended DSD-to-PCM rate | Medium | Unnecessary resampling | Verify in M0; dedicated fallback output block with `format` as backstop | M0 |
| R12 | microSD wear from logs/DB | Low | OS corruption | Journald size cap, MPD DB and all writes on the USB disk, `noatime` | M1 |
| R13 | GPL boundaries: go-musicfox is GPL-3 | n/a | Cannot be linked into `hifid` | Used only as a separate interim program; `hifid` uses MIT libraries | — |
| R14 | Legal/ToS: unofficial API use | Accepted | Personal use | Single account, LAN only, no unblock modules, no redistribution of content | — |
| R15 | Acton IV AUX path limits audible benefit of Hi-Res/DSD | Certain | Expectation | Documented (C-6); the chain is still bit-perfect for future equipment | — |

## 2. Design inputs settled on 2026-09-13 (formerly open questions)

| # | Question | Input | Effect on the design |
|---|---|---|---|
| Q1 | Which "Orange Pi RV": RV (StarFive JH7110) or RV2 (Ky X1)? | **StarFive JH7110** | RV2 rows in docs/05 are reference only |
| Q2 | RAM of the small board? | **512 MB**, server image (no desktop) | Budget recomputed for 512 MB in docs/05 §6; workable for MPD + `hifid` + Samba but leaves little page cache and rules out extras |
| Q3 | Music disk type? | **USB flash drive** | No powered hub or spin-up budget; sustained write 10–30 MB/s becomes the import ceiling; verify real capacity with `f3probe`; keep write-heavy data small |
| Q4 | Dongles? | **ES9039Q2M-class and CS43131-class; ES9039Q2M preferred; DSD library is DSD256 or lower** | ES9039Q2M block is the default active output. DSD256 native/DoP is expected on ES9039Q2M, so the software DSD fallback and the offline PCM twin move from M4 to "optional" |
| Q5 | Phone OS? | **Android primary** | PWA first, optional Capacitor/Kotlin app unchanged |
| Q6 | NetEase tier assumed? | **SVIP** (configurable) | `jymaster` (超清母带) is unlocked; default ladder becomes `jymaster > hires > lossless > exhigh` |
| Q7 | Ethernet at the speaker? | **No, Wi-Fi only** | Wi-Fi driver quality becomes a board-selection criterion; power save off, AP isolation off; offline-first design; larger stream buffers |
| Q8 | DLNA casting from the official app? | Requested an explanation | Explained in docs/06 §6; kept as an optional add-on to verify in M1 |
| Q9 | Samba share acceptable? | **Yes, Samba primary, web upload second** | ADR-0004 revised: Samba installed by default with an inotify-driven library update; tus web upload built later in M3 |
| Q10 | Licence? | **MIT** | `LICENSE` added |
| Q11 | Exact small board? | **Orange Pi Zero LTS, Allwinner H3, 512 MB**, to be tried first: music is mostly played from local files, the box is always on to download, and another board can replace it if Wi-Fi becomes a nuisance | The H3 CPU is sufficient; 512 MB is tight but workable; the XR819 Wi-Fi is the accepted risk (R16) with mitigations: watchdog, power save off, offline-first sync, retry-tolerant jobs. Escalation path: `mt76` USB Wi-Fi adapter, then the RV. ADR-0007 revised accordingly |
| Q12 | Wi-Fi bands / isolation? | **Separate 2.4 GHz and 5 GHz networks** | The Zero LTS joins 2.4 GHz (XR819 limit), phone and PC may stay on 5 GHz; verify both SSIDs share one LAN without client isolation so mDNS and Samba work across them |
| Q13 | `lsusb` IDs of the ES9039Q2M dongle (and its brand/model) | To be reported after plugging it in | XMOS or Comtrue bridge, both in the kernel's native-DSD list |

### Still open

| # | Question | Why it matters | Assumption until answered |
|---|---|---|---|
| Q13 | see above | | |
| Q14 | Do the 2.4 GHz and 5 GHz networks come from the same router (one LAN) or from separate routers/VLANs? | mDNS and Samba do not cross routed boundaries without extra configuration | Same LAN |

## 3. Assumptions made in this design

- The owner accepts an open-source re-implementation of the NetEase client rather than the
  official binary (C-1).
- Single household, single NetEase account, trusted LAN reached over Wi-Fi.
- The 3.5 mm output of the dongle drives the Acton IV AUX at line level (dongle volume at
  100 % is within the speaker's input range; if it clips, the `hardware` volume mode or a
  fixed lower level is used).
- Debian 13 (trixie), Armbian's Debian 13, or the vendor Debian image for the Orange Pi RV is
  acceptable as the OS baseline.
- The 512 GB flash drive is genuine and formatted ext4 by the install script.

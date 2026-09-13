# 05 · Hardware Evaluation and Calculations

Status: draft v0.1 · 2026-09-13 · addresses C-4, C-5, C-7, NFR-1, NFR-2, NFR-3

All numbers are engineering estimates from public specifications; the section "Measured on
hardware" is filled in during milestone M0 (docs/11) with `tools/bench` results.

## 1. Candidate boards

The project owner has an Orange Pi Zero 3 and an "Orange Pi RV". Two different RISC-V
boards carry the RV name, so both are listed; confirm which one is on the bench.

| | Orange Pi Zero 3 | Orange Pi RV | Orange Pi RV2 |
|---|---|---|---|
| SoC | Allwinner H618, 4× Cortex-A53 @ 1.5 GHz (arm64) | StarFive JH7110, 4× SiFive U74 @ 1.5 GHz (riscv64) | Ky X1 (SpacemiT K1 family), 8× RV64GCV @ 1.6 GHz (riscv64) |
| RAM | 1 / 1.5 / 2 / 4 GB LPDDR4 | 2 / 4 / 8 GB LPDDR4 | 2 / 4 / 8 GB LPDDR4X |
| Storage | microSD, SPI flash | microSD, SPI, M.2 M-key PCIe 2.0 ×1 (NVMe) | microSD, optional eMMC, 2× M.2 M-key |
| USB | 1× USB 2.0 Type-A; 2× USB 2.0 on 13-pin header (expansion board); USB-C power/OTG | 4× USB 3.0 Type-A via one VL805 PCIe xHCI (same controller family as Raspberry Pi 4) | 3× USB 3.0 via GL3523 hub on one DWC3; 1× USB 2.0 Type-A; USB 2.0 header |
| Ethernet | 1× GbE | 1× GbE | 2× GbE |
| Wi-Fi / BT | Wi-Fi 5 + BT 5.0 | Wi-Fi 5 + BT 5.0 | Wi-Fi 5 + BT 5.0 |
| Power input | USB-C 5 V / 3 A | USB-C 5 V / 4 A | USB-C 5 V / 5 A |
| Idle power | ≈ 0.8 W bare, ≈ 2 W with peripherals | ≈ 3.4–4.2 W (VisionFive 2, same SoC) | ≈ 1.5–3 W |
| Kernel status | Mainline DT since 2023; Armbian community image, Debian 13 + Linux 6.18 (USB, GbE, Wi-Fi OK) | Board DT merged in Linux 6.19; Debian 13 riscv64 is an official release arch; no Armbian | Vendor 6.6 kernel (known CPU-hog bug) or Armbian community 6.18; USB 2.0 mainline support still in review |
| Packages (Debian 13) | mpd 0.24, mpc, ffmpeg 7.1, samba, nginx, avahi, golang 1.24, nodejs 20: all present; myMPD (OBS deb) and upmpdcli (vendor apt) available | Same Debian packages present on riscv64; **myMPD and upmpdcli: build from source only** | Same as RV |
| Price (USD) | 15–25 | 30–50 | 30–50 |

Sources are listed in docs/02 §7. Board wiki pages were unreachable during the survey; the
USB VBUS current limit of the Zero 3 could not be verified and is treated as "assume
≤ 1 A total, use a powered hub or self-powered disk".

### 1.1 Decision matrix (weight 1–5)

| Criterion (weight) | Zero 3 | RV (JH7110) | RV2 (Ky X1) |
|---|---|---|---|
| USB audio path maturity (5) | 5 · plain EHCI per port, no hub, arm64 snd-usb-audio widely used | 4 · VL805 xHCI (well known from Pi 4, isochronous firmware fixes exist; EEPROM version unverified) | 2 · hub on one DWC3, USB 2.0 support still landing |
| Kernel freshness for DSD quirks (4) | 5 · 6.18 via Armbian | 4 · 6.19 mainline (self-built) or Debian 6.12 + own DTB | 3 |
| Package availability (4) | 5 | 3 · myMPD/upmpdcli from source | 3 |
| Ports and power for disk + 2 DACs (3) | 2 · needs expansion board or powered hub | 5 · 4× USB 3.0, 4 A supply | 4 |
| Idle power (2) | 5 | 2 | 3 |
| RAM headroom (2) | 3 · 1–4 GB | 4 | 4 |
| Owner already has it (3) | 5 | 5 | ? |
| **Weighted score** | **101 / 115** | **88 / 115** | 68 / 115 |

**Decision (ADR-0007):** Orange Pi Zero 3 is the primary target and the M0 bench board.
The Orange Pi RV is the second target: the same software is built for riscv64, and the RV is
the better chassis if the 512 GB disk is a spinning HDD or if more than two USB devices are
needed. Nothing in the design is arm64-specific.

### 1.2 Zero 3 physical setup

```
USB-C 5 V/3 A PSU ──> Zero 3
Type-A ──> powered USB 2.0 hub ──> 512 GB disk (or self-powered SSD)
                              └──> DAC #1 (ES9039Q2M)
13-pin header expansion board ──> DAC #2 (CS43131)      (optional second output)
GbE ──> router  (Wi-Fi only as fallback; not for uploads)
```

If the disk is a bus-powered 2.5" HDD (spin-up ≈ 1 A), the powered hub is mandatory.

## 2. Audio data rates

Formula: bits per second = sample rate × bit depth × channels.

| Format | Raw bit rate | Bytes / s | MB per 4-min track (raw) | Typical FLAC size (4 min) |
|---|---|---|---|---|
| PCM 16/44.1 | 1.411 Mbps | 176 kB | 42 MB | 22–30 MB |
| PCM 24/48 | 2.304 Mbps | 288 kB | 69 MB | 40–50 MB |
| PCM 24/96 | 4.608 Mbps | 576 kB | 138 MB | 80–100 MB |
| PCM 24/192 | 9.216 Mbps | 1.15 MB | 276 MB | 150–190 MB |
| PCM 32/384 | 24.6 Mbps | 3.07 MB | 737 MB | rare |
| PCM 32/768 | 49.2 Mbps | 6.14 MB | 1.47 GB | rare |
| DSD64 (2.8224 MHz) | 5.645 Mbps | 706 kB | 169 MB | DSF is uncompressed |
| DSD128 (5.6448 MHz) | 11.29 Mbps | 1.41 MB | 339 MB | |
| DSD256 (11.2896 MHz) | 22.58 Mbps | 2.82 MB | 677 MB | |
| DSD512 (22.5792 MHz) | 45.16 Mbps | 5.64 MB | 1.35 GB | |
| NetEase "standard" MP3 | 0.128 Mbps | 16 kB | 3.8 MB | |
| NetEase "exhigh" MP3 | 0.32 Mbps | 40 kB | 9.6 MB | |
| NetEase "lossless" FLAC | ≈ 0.9–1.1 Mbps | ≈ 125 kB | ≈ 30 MB | |
| NetEase "hires" FLAC | ≈ 2–5 Mbps | ≈ 250–600 kB | ≈ 60–150 MB | |

DoP containers: DSD64 → PCM 24/176.4 (8.47 Mbps on the wire), DSD128 → 24/352.8
(16.9 Mbps), DSD256 → 24/705.6 (33.9 Mbps). A DAC whose USB interface stops at 384 kHz PCM
can therefore do at most DSD128 via DoP.

## 3. USB 2.0 budget (Zero 3)

- High-speed USB: 480 Mbps raw; isochronous (audio) traffic may reserve up to 80 % of each
  125 µs microframe, ≈ 384 Mbps; the host schedules isochronous packets before bulk (disk)
  traffic, so audio has priority by protocol design.
- Worst case audio stream: DSD512 native or PCM 32/768 ≈ 49 Mbps ≈ 768 bytes per
  microframe: 13 % of the isochronous budget. DSD256 native ≈ 23 Mbps.
- Disk traffic while playing: reading a 24/192 FLAC ≈ 1.2 MB/s ≈ 10 Mbps, negligible.
- Disk traffic while uploading: USB 2.0 bulk practical ceiling ≈ 30–38 MB/s (240–300 Mbps).
  On the same hub as the DAC this coexists with even the worst-case audio stream, because
  isochronous bandwidth is reserved first. For the least risk, put the active DAC on a
  different port/controller than the disk (Zero 3: DAC on the Type-A, disk on the header
  board, or the reverse).

Conclusion: USB 2.0 is not a limitation for a single-zone HiFi server; USB 3.0 on the RV only
speeds up uploads.

## 4. Storage plan (512 GB disk)

Usable after ext4 formatting with reserved blocks set to 1 %: ≈ 500 GB.

| Content type | Assumed size | Capacity if used alone |
|---|---|---|
| FLAC 16/44.1 (NetEase lossless downloads) | 30 MB/track | ≈ 16,600 tracks |
| FLAC 24/96 | 90 MB/track | ≈ 5,500 tracks |
| FLAC 24/192 | 170 MB/track | ≈ 2,900 tracks |
| DSD64 | 169 MB/track | ≈ 2,950 tracks |
| DSD128 | 339 MB/track | ≈ 1,470 tracks |
| DSD256 | 677 MB/track | ≈ 740 tracks |

Suggested layout for a mixed library:

| Path | Budget | Content |
|---|---|---|
| `/srv/music/netease` | 150 GB | ≈ 5,000 lossless/hires downloads |
| `/srv/music/local` | 300 GB | uploads: ≈ 1,000 Hi-Res tracks + ≈ 500 DSD tracks |
| `/srv/data` | 30 GB | MPD DB (≈ 1 kB per song on disk), SQLite index, cover cache (≈ 50 kB per album), tus incoming (largest album in flight ≈ 6 GB) |
| free | 20 GB | never fill a disk above 95 %; `hifid` warns at 90 % |

The 64 GB microSD holds only the OS (≈ 3 GB used). Journald is capped at 200 MB and MPD's
database, state and sticker files live on the USB disk (C-7).

## 5. CPU budget (Cortex-A53 @ 1.5 GHz, four cores)

MPD uses at least two threads: the decoder thread (codec + format conversion) and the
output thread (ALSA writes, DoP packing, output-side conversion). They run on different
cores, so the figures below are per-core percentages.

| Task | Estimate | Basis |
|---|---|---|
| FLAC decode 16/44.1 | 1–2 % | integer codec, ≈ 20 MB/s decode speed on A53 |
| FLAC decode 24/192 | 4–8 % | scales with sample rate |
| MP3/AAC decode | 1–3 % | |
| DSF/DFF to native DSD or DoP | < 1 % | byte shuffling only; DoP adds marker bytes, no math |
| MPD Dsd2Pcm (96-tap FIR, 8:1 decimation) DSD64 → PCM 352.8 kHz, no resampling | 15–25 % | scaled from the measured DSD256 figure below (÷ 4 in sample rate) |
| Dsd2Pcm DSD128 → PCM 705.6 kHz, then 2:1 resample to 352.8 kHz | 30–50 % | |
| Dsd2Pcm DSD256 → PCM, output forced to 352.8 kHz | ≈ 57 % | measured on a Raspberry Pi 3 B+ (Cortex-A53 @ 1.4 GHz), MPD discussion #2372 |
| Dsd2Pcm DSD256 → PCM, free output rate (1.4112 MHz → device rate via soxr) | ≈ 80 % | same source; **dropout risk** |
| Dsd2Pcm DSD128 → 384 kHz (non-integer ratio) | ≈ 100 % | reported with dropouts; avoid non-44.1-family fallback rates |
| Resampling PCM 44.1 → 192 kHz, soxr "very high" | 30–35 % (Pi 2), ≈ 20 % (A53) | RuneAudio forum measurements; not used in the bit-perfect default |
| `hifid` idle / serving API | < 1 % | Go, event driven |
| tus upload receive at 35 MB/s | 10–20 % | copying + sha256 (≈ 150 MB/s per core on A53) |
| ffmpeg tagging after download | burst, 100 % for < 1 s per file | `-c copy`, no re-encode |

Important consequence of how MPD converts DSD: Dsd2Pcm outputs PCM at one eighth of the
DSD bit rate (DSD64 → 352.8 kHz). If the DAC accepts that rate (ES9039Q2M dongles: up to
768 kHz; CS43131: up to 384 kHz) no resampling is needed and the DSD64 fallback stays well
inside one core. DSD128 and above on a DAC without native DSD/DoP add a resampler stage at
705.6 kHz or more, which is where the measured 57–100 % figures come from.

Mitigations for the expensive case (designed in docs/04):

1. Prefer native DSD or DoP; the ES9039Q2M dongles cover DSD512 natively.
2. For DACs that cannot do a given DSD rate at all, `hifid` can create an offline PCM twin
   (`ffmpeg`, 24/352.8 or 24/176.4) next to the DSD file once, at import time, instead of
   converting in real time.
3. Use `libsamplerate` "Medium" or `soxr` "quick" for the real-time fallback path only.

## 6. RAM budget (1 GB board)

| Component | RSS estimate |
|---|---|
| Debian 13 minimal (Armbian, no desktop) | 90–140 MB |
| MPD 0.24 with a 20,000-song database, 4 MB audio buffer | 40–80 MB |
| `hifid` (Go, embedded PWA, SQLite index, caches) | 30–60 MB |
| avahi-daemon | 3–5 MB |
| samba (optional, idle) | 20–40 MB |
| upmpdcli (optional) | 15–25 MB |
| **Total worst case** | **≈ 350 MB** |

Remaining ≈ 600 MB is page cache for the music files (helps gapless and re-reads). A 2 GB
Zero 3 or the RV removes any pressure. Node.js-based NetEase API sidecars would add
120–200 MB and are avoided in the baseline (ADR-0002).

## 7. Network

| Flow | Rate | Comment |
|---|---|---|
| NetEase streaming (lossless/hires) | 1–5 Mbps down | trivial for any home internet |
| PWA control traffic | < 10 kbps | WebSocket state events |
| Upload PC → board over GbE | ≈ 110 MB/s theoretical | bottleneck is the USB 2.0 disk at ≈ 35 MB/s on the Zero 3, ≈ 100 MB/s on the RV with an SSD |
| Upload over Wi-Fi 5 | 10–25 MB/s typical | acceptable for single albums, avoid for bulk |

Upload time for a 4 GB DSD album: ≈ 2 min (Zero 3, GbE, USB 2.0 SSD), ≈ 40 s (RV, USB 3.0
SSD), 3–7 min over Wi-Fi.

## 8. Power

| Item | Steady | Peak |
|---|---|---|
| Zero 3 board | 1–2 W | 3 W (all cores) |
| USB DAC dongle (ES9039Q2M or CS43131 with amp) | 0.5–1.0 W (100–200 mA) | 1.3 W |
| USB SSD | 1–2 W | 3 W |
| 2.5" USB HDD | 2–2.5 W | 5–6 W at spin-up (≈ 1–1.2 A) |
| USB hub (powered) | 0.5 W | |
| **Total (SSD)** | **≈ 4–5 W** | 7 W |
| **Total (HDD)** | **≈ 5–6 W** | 12 W (needs the hub's own PSU) |

A 5 V / 3 A USB-C supply with a short, thick cable is the minimum for the Zero 3; low input
voltage is the most common cause of USB disk resets on this board class.

## 9. Timing

| Item | Target | Expected |
|---|---|---|
| Cold boot to MPD ready | ≤ 60 s (NFR-2) | Armbian ≈ 20–30 s to network, MPD DB load 2–5 s, `hifid` < 1 s |
| Tap-to-sound for a NetEase track | ≤ 3 s | URL resolution 200–600 ms, CDN first byte 200–500 ms, MPD prebuffer ≈ 1 s |
| Output switch | ≤ 2 s gap | MPD closes/opens ALSA devices; stream resumes from the buffer |
| Library visible after upload | ≤ 60 s | MPD incremental `update` on one folder: 1–5 s |

## 10. Sample-rate sanity check against the speaker (C-6)

The Acton IV digitizes its AUX input for its internal DSP. No specification of that ADC is
published; class-D smart speakers commonly run at 48 kHz internally. Therefore:

- 16/44.1 and 24/96 sources already exceed what the speaker can reproduce; DSD64 native vs
  a DSD64-to-PCM conversion will not be distinguishable through the AUX path.
- The design still keeps the chain bit-perfect up to the DAC because the same server may later
  feed a different amplifier or headphones, and because the requirement asks for it.
- Practical default: allow all rates, but do not spend effort chasing DSD512 or 768 kHz
  support per dongle beyond what the kernel provides out of the box.

## 11. Measured on hardware (to be filled in M0)

| Measurement | Zero 3 | RV | Script |
|---|---|---|---|
| `mpd` RSS with the full library | | | `tools/bench/mem.sh` |
| `hifid` RSS idle / during upload | | | |
| dsd2pcm CPU %: DSD64, DSD128, DSD256 | | | `deploy/scripts/bench-dsd.sh` |
| soxr CPU %: 705.6 k → 352.8 k | | | |
| USB disk sequential write MB/s | | | `tools/bench/disk.sh` |
| tus upload MB/s from Windows over GbE | | | |
| Boot to MPD ready (s) | | | `systemd-analyze` |
| 24 h dropout count at 24/192 and DSD128 | | | MPD log grep |

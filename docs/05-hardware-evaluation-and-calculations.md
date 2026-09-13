# 05 · Hardware Evaluation and Calculations

Status: draft v0.1 · 2026-09-13 · addresses C-4, C-5, C-7, NFR-1, NFR-2, NFR-3

All numbers are engineering estimates from public specifications; the section "Measured on
hardware" is filled in during milestone M0 (docs/11) with `tools/bench` results.

## 1. Candidate boards

The project owner has an **Orange Pi Zero LTS** (confirmed: Allwinner H3, 512 MB) and an
**Orange Pi RV** (confirmed: StarFive JH7110), plus larger boards including a Raspberry Pi 5
that the owner would rather not use for such a simple task. The RV2 is listed for reference
only.

| | Orange Pi Zero LTS (owned) | Orange Pi RV (owned) | Raspberry Pi 5 (owned, reference) | Orange Pi RV2 (reference) |
|---|---|---|---|---|
| SoC | Allwinner H3, 4× Cortex-A7 @ 1.0–1.2 GHz (32-bit armhf) | StarFive JH7110, 4× SiFive U74 @ 1.5 GHz (riscv64) | BCM2712, 4× Cortex-A76 @ 2.4 GHz (arm64) | Ky X1, 8× RV64GCV @ 1.6 GHz |
| RAM | 512 MB DDR3 | 2 / 4 / 8 GB LPDDR4 | 2 / 4 / 8 / 16 GB | 2 / 4 / 8 GB |
| Storage | microSD, SPI flash | microSD, SPI, M.2 PCIe 2.0 ×1 | microSD, PCIe (HAT) | microSD, eMMC, 2× M.2 |
| USB | 1× USB 2.0 Type-A; 2× USB 2.0 on 13-pin header (expansion board); micro-USB OTG (power) | 4× USB 3.0 Type-A via VL805 xHCI (Pi 4 class) | 2× USB 3.0 + 2× USB 2.0 (RP1) | 3× USB 3.0 via hub, 1× USB 2.0 |
| Ethernet | 10/100 Mbps | 1× GbE | 1× GbE | 2× GbE |
| Wi-Fi | **XR819, 802.11n, 2.4 GHz only, out-of-tree driver, widely reported unstable** | Wi-Fi 5 + BT 5.0, Broadcom AP6256 (mainline `brcmfmac`) | Wi-Fi 5 + BT 5.0, Broadcom CYW43455 (`brcmfmac`) | Wi-Fi 5 (AP6256) |
| Power input | micro-USB 5 V / 2 A | USB-C 5 V / 4 A | USB-C 5 V / 5 A (PD) | USB-C 5 V / 5 A |
| Idle power | ≈ 1–1.5 W | ≈ 3.4–4.2 W (VisionFive 2, same SoC) | ≈ 2.5–3 W | ≈ 1.5–3 W |
| Kernel status | H3 well supported in mainline; Armbian community/legacy images; Debian armhf packages | Board DT in Linux 6.19; Debian 13 riscv64 official; vendor Debian image | Raspberry Pi OS / Debian arm64, 6.12 kernel, best-documented USB audio host | Vendor 6.6 or Armbian community 6.18 |
| Packages (Debian 13) | mpd, ffmpeg, samba, avahi, golang: present on armhf; Go cross-build with `GOARCH=arm GOARM=7` | All present on riscv64; myMPD/upmpdcli from source | Everything, including myMPD and upmpdcli packages | Same as RV |
| Price (USD) | 10–15 | 30–50 | 60–120 | 30–50 |

Sources are listed in docs/02 §7 and in the Armbian/Orange Pi community threads on the
XR819 driver. The "adapter does most of the work" intuition is correct for the D/A
conversion and for native DSD (pure pass-through), but the board still decodes FLAC/MP3,
runs the NetEase service, Samba and the Wi-Fi stack; on the H3 those are all light. The
CPU is not the constraint. Wi-Fi quality and RAM are.

### 1.1 Decision matrix (weight 1–5), revised after the owner's answers

New inputs: Wi-Fi is the only network link (C-9), the disk is a flash drive (C-10), the
small board is an Orange Pi Zero LTS (H3, 512 MB).

| Criterion (weight) | Orange Pi Zero LTS | RV (JH7110) | Raspberry Pi 5 (reference) |
|---|---|---|---|
| USB audio path maturity (5) | 3 · H3 EHCI is fine, but one Type-A plus header ports; flash drive and DAC share one USB 2.0 root | 4 · VL805 xHCI (Pi 4 class; isochronous firmware fixes exist, EEPROM version unverified) | 5 · RP1 xHCI, the most-used USB DAC host in the hobby |
| Kernel freshness for DSD quirks (4) | 4 · mainline supports H3 well | 4 · Debian 6.12 + board DTB, or 6.19 mainline | 5 · 6.12 Raspberry Pi kernel |
| Package availability (3) | 4 · armhf | 4 · baseline all present; myMPD/upmpdcli from source | 5 |
| Ports and power for flash drive + 2 DACs (3) | 2 · expansion board or hub, micro-USB 2 A input | 5 · 4× USB 3.0, 4 A supply | 5 |
| **Wi-Fi driver maturity (4)** | 1 · XR819: 2.4 GHz only, out-of-tree, unstable | 4 · Broadcom AP6256 via mainline `brcmfmac` | 5 · Broadcom CYW43455 via `brcmfmac` |
| RAM headroom (3) | 1 · 512 MB | 5 · 2–8 GB | 5 |
| Idle power (2) | 5 | 2 | 3 |
| Owner already has it (3) | 5 | 5 | 5 |
| **Weighted score** | **81 / 135** | **113 / 135** | 129 / 135 |

**Decision (ADR-0007, revised):** the Orange Pi RV is the intended deployment board. The
Zero LTS is kept as an option only if its Wi-Fi problem is removed: a USB Wi-Fi adapter with
a mainline driver (MediaTek MT7612U or MT7921AU, `mt76`, both 5 GHz capable) or Ethernet.
The Raspberry Pi 5 would be the most trouble-free host but is over-specified for the task;
it stays a reference, not a target. The M0 bake-off (24 h Wi-Fi stability, 24 h USB-DAC
playback during Samba copies, native DSD availability, RAM headroom) decides between the RV
and a Zero LTS with a USB Wi-Fi adapter, if the owner wants to try the latter. Nothing in the
design is architecture-specific.

### 1.2 Physical setup

```
Orange Pi RV:   USB-C 5 V/4 A PSU ──> RV
                USB 3.0 #1 ──> 512 GB USB flash drive
                USB 3.0 #2 ──> DAC #1 (ES9039Q2M, default output)  ──> 3.5 mm ──> Acton IV AUX
                USB 3.0 #3 ──> DAC #2 (CS43131, optional)
                Wi-Fi 5 (5 GHz) ──> router   (power save off; no Ethernet at this location)

Zero LTS (only with the Wi-Fi fix):
                micro-USB 5 V/2 A PSU ──> Zero LTS
                Type-A ──> DAC #1 (ES9039Q2M)
                13-pin expansion board USB ×2 ──> flash drive, USB Wi-Fi adapter (mt76)
                (all three share one USB 2.0 root: 20 Mbps Wi-Fi + 10 Mbps disk + 23 Mbps DSD256 is still far below the bus limit)
```

A flash drive (≈ 100 mA) plus one dongle (≈ 100–250 mA) stays well inside any of the boards'
USB budgets, so no powered hub is needed. Check the flash drive once with `f3probe` (fake
capacity is common on large cheap sticks) and format it ext4 with `-m 1`.

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

Usable after ext4 formatting with reserved blocks set to 1 %: ≈ 500 GB. The drive is a USB
flash stick (C-10): expect 10–30 MB/s sustained writes (some sticks drop to 5 MB/s once their
SLC cache is full or they heat up), 100+ MB/s reads; mount with `noatime,commit=60`; keep the
SQLite index and MPD database small and rarely rewritten; a USB SSD is the upgrade path if
import speed matters later.

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

## 5. CPU budget (Cortex-A53 @ 1.5 GHz, four cores; H3 and JH7110 notes below)

MPD uses at least two threads: the decoder thread (codec + format conversion) and the
output thread (ALSA writes, DoP packing, output-side conversion). They run on different
cores, so the figures below are per-core percentages.

For the owner's boards: the Zero LTS's Cortex-A7 @ 1.0 GHz is roughly 2–3× slower than an
A53 @ 1.5 GHz per core, so multiply the decode rows by ≈ 2.5 (FLAC 24/192 ≈ 10–20 % of a core,
still fine) and treat every software DSD conversion row as **not feasible**; the JH7110's U74
is comparable to an A53 for integer decode but has no SIMD unit, so the DSD conversion rows
are worse there too. With the ES9039Q2M dongle doing native DSD, neither board needs them.

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

## 6. RAM budget

| Component | RSS estimate |
|---|---|
| Debian 13 minimal server image (no desktop) | 90–140 MB |
| MPD 0.24 with a 20,000-song database, 16 MB audio buffer | 50–90 MB |
| `hifid` (Go, embedded PWA, SQLite index, caches) | 30–60 MB |
| avahi-daemon | 3–5 MB |
| samba (default, idle; +10–20 MB per active copy session) | 20–40 MB |
| upmpdcli (optional) | 15–25 MB |
| **Total, baseline (no upmpdcli)** | **≈ 195–335 MB** |

- On the **RV (2–8 GB)** there is no pressure; the rest is page cache for music files.
- On the **Zero LTS (512 MB)** the baseline fits with ≈ 150–300 MB left, of which the kernel needs
  ≈ 50 MB and page cache gets the remainder. It works for playback (a 24/192 FLAC needs only
  1.2 MB/s of reads) but: no myMPD, no Node.js, `zram` swap enabled as a safety net, MPD
  `audio_buffer_size` back to 8 MB, and Samba copies during DSD playback should be tested in
  M0 (a large copy fills the page cache and evicts MPD's read-ahead).
- Node.js-based NetEase API sidecars would add 120–200 MB and are avoided (ADR-0002).

## 7. Network

| Flow | Rate | Comment |
|---|---|---|
| NetEase streaming (lossless/hires/jymaster) | 1–9 Mbps down over Wi-Fi | trivial bandwidth; Wi-Fi latency spikes are absorbed by MPD's 16 MB buffer (≈ 30 s at 24/96) |
| PWA control traffic | < 10 kbps | WebSocket state events |
| Samba or upload, PC → board over Wi-Fi 5 (5 GHz, good signal) | 10–25 MB/s typical | comparable to the flash drive's write speed, so neither dominates |
| Same over 2.4 GHz or weak signal | 3–10 MB/s | plan bulk imports near the router or move the board temporarily |
| Ethernet (not available at the speaker) | ≈ 110 MB/s | would be limited by the flash drive at 10–30 MB/s anyway |

Import time for a 4 GB DSD album: 3–7 min over 5 GHz Wi-Fi, 7–20 min over 2.4 GHz. All
control and streaming functions are unaffected by these rates.

## 8. Power

| Item | Steady | Peak |
|---|---|---|
| Orange Pi RV board (Wi-Fi active) | 3–4 W | 6 W (all cores) |
| Zero LTS board (Wi-Fi active) | 1–1.5 W | 2.5 W |
| USB DAC dongle (ES9039Q2M or CS43131 with amp) | 0.5–1.0 W (100–200 mA) | 1.3 W |
| USB flash drive | 0.3–0.5 W | 1 W during writes |
| USB Wi-Fi adapter (mt76, if used on the Zero LTS) | 0.5–1 W | 2 W |
| **Total (RV)** | **≈ 4–5.5 W** | 8 W |
| **Total (Zero LTS + USB Wi-Fi)** | **≈ 2.5–4 W** | 6.5 W |

Use the board's rated supply (RV: 5 V / 4 A; Zero LTS: 5 V / 2 A on micro-USB, which is
marginal with three USB devices, so use a short, thick cable and a good adapter); low input
voltage is the most common cause of USB device resets on these boards. No powered hub is
needed with a flash drive.

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

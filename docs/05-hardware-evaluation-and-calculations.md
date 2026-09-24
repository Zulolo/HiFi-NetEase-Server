# 05 · Hardware Evaluation and Calculations

Status: draft v0.1 · 2026-09-13 · addresses C-4, C-5, C-7, NFR-1, NFR-2, NFR-3

All numbers are engineering estimates from public specifications; the section "Measured on
hardware" is filled in during milestone M0 (docs/11) with `tools/bench` results.

## 1. Candidate boards

The bench board is an **Orange Pi Zero 3 (Allwinner H618, 2 GB)**; the **Orange Pi RV**
(StarFive JH7110) is the riscv64 secondary target. The Zero LTS (an earlier assumption), the
Raspberry Pi 5 and the Orange Pi RV2 are kept as reference points for readers with other
hardware.

| | Orange Pi Zero 3 (verified) | Orange Pi RV (secondary) | Raspberry Pi 5 (reference) | Orange Pi Zero LTS (reference) |
|---|---|---|---|---|
| SoC | Allwinner H618, 4× Cortex-A53 @ 1.5 GHz (arm64) | StarFive JH7110, 4× SiFive U74 @ 1.5 GHz (riscv64) | BCM2712, 4× Cortex-A76 @ 2.4 GHz (arm64) | Allwinner H3, 4× Cortex-A7 @ 1.0–1.2 GHz (armhf) |
| RAM | 1 / 1.5 / 2 / 4 GB LPDDR4 (2 GB on the bench) | 2 / 4 / 8 GB LPDDR4 | 2 / 4 / 8 / 16 GB | 512 MB |
| Storage | microSD, SPI flash | microSD, SPI, M.2 PCIe 2.0 ×1 | microSD, PCIe (HAT) | microSD |
| USB | 1× USB 2.0 Type-A; 2× USB 2.0 on the 13-pin header; USB-C power/OTG; each port on its own EHCI root (no shared hub) | 4× USB 3.0 Type-A via VL805 xHCI | 2× USB 3.0 + 2× USB 2.0 (RP1) | 1× Type-A + header, micro-USB OTG |
| Ethernet | 1× GbE | 1× GbE | 1× GbE | 10/100 |
| Wi-Fi | Wi-Fi 5 + BT 5.0, Unisoc UWE5622 (out-of-tree driver in vendor and Armbian kernels; stable on the bench so far, watchdog enabled) | Wi-Fi 5, Broadcom AP6256 (`brcmfmac`) | Wi-Fi 5, Broadcom CYW43455 (`brcmfmac`) | XR819 2.4 GHz, unstable |
| Power input | USB-C 5 V / 3 A | USB-C 5 V / 4 A | USB-C 5 V / 5 A (PD) | micro-USB 5 V / 2 A |
| Idle power | ≈ 1–2 W | ≈ 3.4–4.2 W | ≈ 2.5–3 W | ≈ 1–1.5 W |
| Kernel status | Vendor Debian 12 image with 6.1.31 (bench); Armbian Debian 13 with 6.18 mainline available | Board DT in Linux 6.19; Debian 13 riscv64 official | Raspberry Pi OS 6.12 | H3 mainline |
| Packages | Debian 12 + backports: mpd 0.24.2, ffmpeg 5.1, samba 4.22 (verified) | All present on riscv64; myMPD/upmpdcli from source | Everything | armhf |
| Price (USD) | 15–25 | 30–50 | 60–120 | 10–15 |

The "adapter does most of the work" intuition is correct for the D/A conversion and for
native DSD (pure pass-through), but the board still decodes FLAC/MP3, runs the NetEase
service, Samba and the Wi-Fi stack; on an A53 these are all light. The CPU is not the
constraint; Wi-Fi driver quality and RAM are, and the Zero 3 has enough of both.

### 1.1 Decision matrix (weight 1–5), revised with the settled design inputs

Inputs: Wi-Fi is the only network link (C-9), the music disk is a microSD card in a USB
reader (C-10), the bench board is an Orange Pi Zero 3 with 2 GB.

| Criterion (weight) | Orange Pi Zero 3 | RV (JH7110) | Raspberry Pi 5 (reference) | Zero LTS (reference) |
|---|---|---|---|---|
| USB audio path maturity (5) | 5 · plain EHCI per port, no hub; verified bit-perfect PCM and native DSD | 4 · VL805 xHCI (Pi 4 class) | 5 · RP1 xHCI | 3 |
| Kernel freshness for DSD quirks (4) | 4 · vendor 6.1 needs the installer's quirk; Armbian 6.18 would not | 4 · Debian 6.12 + board DTB, or 6.19 mainline | 5 | 4 |
| Package availability (3) | 5 · verified (backports mpd 0.24) | 4 · myMPD/upmpdcli from source | 5 | 4 |
| Ports and power for disk + 2 DACs (3) | 3 · Type-A + header ports, 3 A supply | 5 · 4× USB 3.0 | 5 | 2 |
| **Wi-Fi driver maturity (4)** | 3 · UWE5622 out-of-tree; stable so far, watchdog on | 4 · `brcmfmac` | 5 · `brcmfmac` | 1 · XR819 |
| RAM headroom (3) | 5 · 2 GB (1.49 GB free with everything running) | 5 | 5 | 1 |
| Idle power (2) | 5 | 2 | 3 | 5 |
| Already available, no purchase (3) | 5 | 5 | 5 | 5 |
| **Weighted score** | **118 / 135** | 113 / 135 | 129 / 135 | 81 / 135 |

**Decision (ADR-0007, revised 2026-09-24):** the Orange Pi Zero 3 is the verified first
deployment. The RV remains the riscv64 secondary target. The Raspberry Pi 5 and the Zero LTS
rows stay as guidance for readers with other boards. Nothing in the design or scripts is
board-specific.

### 1.2 Physical setup (verified)

```
Orange Pi Zero 3:
                USB-C 5 V/3 A PSU ──> Zero 3
                Type-A (EHCI root 3) ──> DAC (Comtrue/ES9039 dongle, 2fc6:f802)  ──> 3.5 mm ──> speaker AUX
                header USB (EHCI root 2) ──> USB reader with the 512 GB microSD (ext4, label "hifi")
                onboard UWE5622 Wi-Fi ──> router   (power save off; watchdog timer)
                (DAC and disk sit on different USB root ports, so isochronous audio never competes with disk traffic)
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
| FLAC 24/192 (NetEase `jymaster` 超清母带, measured) | 157 MB/track | ≈ 2,880 tracks |
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

The Zero 3's Cortex-A53 @ 1.5 GHz is exactly the core class these figures were derived
for. For readers with other boards: a Cortex-A7 board (Orange Pi Zero LTS, Raspberry Pi 2)
is roughly 2–3× slower per core, so multiply the decode rows by ≈ 2.5 and treat the software
DSD conversion rows as not feasible; the JH7110's U74 is comparable to an A53 for integer
decode but has no SIMD unit. With a DAC doing native DSD none of the conversion rows apply.

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
- Measured on the **Zero 3 (2 GB)** after install: mpd 58 MB, smbd 22 MB, avahi 2 MB,
  1.49 GB free with a 16 MB MPD buffer. No pressure at all.
- On a **512 MB board** (for example an Orange Pi Zero LTS) the baseline fits with
  ≈ 150–300 MB left: no myMPD, no Node.js, `zram` swap as a safety net, MPD buffer 8 MB
  (the config generator picks 8 MB automatically below 1.2 GB of RAM).
- Node.js-based NetEase API sidecars would add 120–200 MB and are avoided (ADR-0002).

## 7. Network

| Flow | Rate | Comment |
|---|---|---|
| Local playback (the main mode) | 0 | files on the flash drive; Wi-Fi is not in the audio path |
| NetEase background download (lossless ≈ 30 MB/track) | 3–10 MB/s on Wi-Fi 5 | a few seconds per track; overnight syncs of whole playlists are easy |
| NetEase live streaming (measured 2026-09-24) | link sustains ≈ 560–610 kB/s (4.5–4.9 Mbps) | enough for `lossless` (124 kB/s) with a wide margin, **not** for a `jymaster` 24/192 master (690 kB/s), which starves the decoder and logs `Decoder is too slow`. Hence the separate streaming ladder in docs/08 §5; buffers delay a sustained deficit, they do not cure it |
| PWA control traffic | < 10 kbps | WebSocket state events; suffers only when the link drops entirely (watchdog) |
| Samba copy over the Zero 3's Wi-Fi 5 (5 GHz) | ≈ 5–15 MB/s | a 4 GB DSD album in 5–15 min; a 500 MB FLAC album in about a minute |
| Same on 2.4 GHz or with a weak driver (e.g. XR819) | 1–3 MB/s | plan bulk imports near the router |
| Ethernet (the Zero 3 has GbE, not cabled here) | ≈ 110 MB/s | limited by the card/reader at 20–40 MB/s anyway |

With separate 2.4 GHz and 5 GHz SSIDs, put the board on 5 GHz when its adapter supports it
(the Zero 3's does); phone and PC see it as long as both SSIDs are on the same LAN (same
router or bridged access points, no client isolation).

## 8. Power

| Item | Steady | Peak |
|---|---|---|
| Orange Pi Zero 3 (Wi-Fi active) | 1–2 W | 3 W (all cores) |
| Orange Pi RV board (Wi-Fi active) | 3–4 W | 6 W (all cores) |
| USB DAC dongle (ES9039Q2M or CS43131 with amp) | 0.5–1.0 W (100–200 mA) | 1.3 W |
| USB card reader with microSD / USB flash drive | 0.3–0.5 W | 1 W during writes |
| **Total (Zero 3)** | **≈ 2–3.5 W** | 5.5 W |
| **Total (RV)** | **≈ 4–5.5 W** | 8 W |

Use the board's rated supply (Zero 3: 5 V / 3 A USB-C; RV: 5 V / 4 A) with a short, thick
cable; low input voltage is the most common cause of USB device resets on these boards. No
powered hub is needed with a flash drive or card reader. The Zero 3 idled at 53 °C with the
dongle attached, so a small heatsink is advisable for a closed enclosure.

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

| Measurement | Zero 3 (2026-09-24) | RV | Script |
|---|---|---|---|
| `mpd` RSS (test library, 16 MB buffer) | 58 MB | | `ps -o rss -C mpd` |
| `smbd` / `avahi-daemon` RSS idle | 22 MB / 2 MB | | |
| Free RAM with everything running | 1.49 GB of 1.99 GB | | `free -m` |
| `hifid` RSS idle / during upload | (M2) | | |
| PCM bit-perfect check 44.1 / 96 / 192 k | S16_LE / S24_3LE / S24_3LE at file rate | | `deploy/scripts/test-audio.sh` |
| DSD64 / DSD128 delivery | native `DSD_U32_BE` (88 200 / 176 400 frames/s) | | `test-audio.sh --dsd` |
| CPU while playing DSD256 native (real music file) | MPD threads ≈ 4–5 % of one core in total (player 1 %, output 1.5 %, io 0.7 %, rtio 1 %); whole system 97 % idle | | `top -H -p $(pidof mpd)` |
| CPU while playing DSD128 native | ≈ 3 % of one core | | |
| `mpd` RSS while playing DSD256 / idle | 78 MB / 62 MB | | |
| dsd2pcm CPU %: DSD64, DSD128, DSD256 | not needed (native DSD) | | |
| USB disk sequential write MB/s | (pending) | | `dd` / `f3write` |
| Boot to MPD ready (s) | 25 s (3.3 s kernel + 21.5 s userspace) | | `systemd-analyze` |
| SoC temperature idle with dongle | 53 °C | | `/sys/class/thermal` |
| 24 h dropout count at 24/192 and DSD128 | (soak pending) | | MPD log grep |

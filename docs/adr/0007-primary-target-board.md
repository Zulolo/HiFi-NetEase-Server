# ADR-0007 · Target board: Orange Pi Zero 3 (2 GB) verified as the first deployment; any Debian SBC supported

- Status: accepted (revised 2026-09-24 after bench verification on the actual board)
- Date: 2026-09-13, revised 2026-09-24
- Requirements addressed: C-4, C-5, C-9, NFR-1, NFR-2, NFR-3, NFR-4

## Context

Earlier revisions of this record weighed an Orange Pi Zero LTS (H3, 512 MB) against an Orange
Pi RV (JH7110). The board actually put on the bench identifies itself as **Orange Pi Zero 3,
Allwinner H618, 2 GB RAM**, running the vendor Debian 12 image with kernel 6.1.31 and
Wi-Fi through a Unisoc UWE5622 module. That removes the two concerns that drove the previous
revision (512 MB and the XR819 Wi-Fi). The design goal for the public repository is that the
same scripts work on **any** Debian-based SBC; the Zero 3 is one tested configuration.

Bench results (2026-09-24, deploy/scripts on the Zero 3):

| Check | Result |
|---|---|
| Packages | Debian 12 + backports: mpd 0.24.2, ffmpeg 5.1, samba 4.22, avahi, alsa-utils |
| Music disk | 512 GB microSD in a USB reader, ext4 label `hifi`, mounted at `/srv/hifi` with bind mounts |
| DAC | Comtrue-bridge ES9039 dongle (`2fc6:f802`): PCM 44.1–768 kHz, 16/24/32 bit, hardware `PCM` mixer |
| Native DSD | Not offered by kernel 6.1 out of the box (`SPECIAL`); with `snd-usb-audio quirk_flags=0x8000` the card advertises `DSD_U32_BE`; persisted in `/etc/modprobe.d/hifi-dsd.conf` |
| Bit-perfect PCM | 44.1 k → `S16_LE 44100`, 96 k → `S24_3LE 96000`, 192 k → `S24_3LE 192000` in `hw_params` |
| Native DSD playback | DSD64 → `DSD_U32_BE 88200`, DSD128 → `DSD_U32_BE 176400` (no DoP, no conversion) |
| Persistence | Card name `ES9039`, quirk, mounts, mpd/smbd/avahi/watchdog all present after reboot |
| Resources | mpd 58 MB RSS, smbd 22 MB, 1.49 GB free of 1.99 GB; boot to ready 25 s; SoC 53 °C idle with the dongle |

## Options considered

| Option | Pros | Cons |
|---|---|---|
| **Orange Pi Zero 3 (2 GB) as the first deployment** | Verified above; ≈ 1–2 W; owner has it; plenty of RAM for MPD + hifid + Samba | USB 2.0 only, one Type-A port (disk and DAC on separate root ports here: fine); Wi-Fi driver is out-of-tree (watchdog enabled) |
| Orange Pi RV (JH7110) | More USB ports, mainline Broadcom Wi-Fi | Not needed now; stays a supported riscv64 target |
| Raspberry Pi 3/4/5 | Best-documented USB audio hosts | Not owned as the intended host; supported by the same scripts |

## Decision

Deploy on the Orange Pi Zero 3. Keep every script and unit architecture-neutral (arm64,
armhf, riscv64) and free of board-specific assumptions: disk by label, DACs by USB IDs,
Wi-Fi watchdog only when the default route is wireless, DSD quirk only when a DAC advertises
raw DSD that the kernel did not enable. The M0 acceptance thresholds (24 h stability, no
xruns, native DSD or DoP, RAM headroom) are met for the DAC and RAM criteria; the 24 h Wi-Fi
and playback soak runs in the background during M1.

## Consequences

- docs/05 and docs/12 keep the earlier board comparison as general guidance for readers with
  other boards; the "reported 512 MB" assumption is withdrawn.
- The vendor kernel 6.1 lacks the Comtrue native-DSD entry that mainline gained in 6.14; the
  installer's quirk handling covers this class of case on any older kernel.
- `hifid` (M2) can rely on 2 GB: the 16 MB MPD buffer and Samba stay enabled.

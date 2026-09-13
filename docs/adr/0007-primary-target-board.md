# ADR-0007 · Target board: Orange Pi RV (JH7110) for deployment; Orange Pi Zero LTS only with a Wi-Fi fix

- Status: accepted (revised 2026-09-13 after the owner confirmed the boards)
- Date: 2026-09-13
- Requirements addressed: C-4, C-5, C-9, NFR-1, NFR-4

## Context

The owner has an Orange Pi RV (StarFive JH7110, riscv64, 2–8 GB, 4× USB 3.0, Broadcom
AP6256 Wi-Fi) and an Orange Pi Zero LTS (Allwinner H3, 4× Cortex-A7, 512 MB, one USB
Type-A plus header ports, XR819 Wi-Fi: 2.4 GHz only, out-of-tree driver, widely reported
unstable). Larger boards including a Raspberry Pi 5 are available but the owner prefers a
low-power board, reasoning that the DAC adapter does most of the work. **Wi-Fi is the only
network link**, the disk is a USB flash drive, Samba is the primary import path, and the
ES9039Q2M dongle is the default output. Evaluation and scoring in docs/05 §1.

The owner's reasoning is right about the CPU: the DAC does the D/A conversion, native DSD is
pass-through, and FLAC decoding costs the H3 10–20 % of one core. It is not right about the
network: the server streams NetEase, serves Samba copies and talks to the phone only through
Wi-Fi, and the XR819 is the least reliable part of any board in the house.

## Options considered

| Option | Pros | Cons |
|---|---|---|
| **Orange Pi RV as deployment board** | Mainline Broadcom Wi-Fi (`brcmfmac`), 5 GHz; 2–8 GB RAM; 4× USB 3.0 (flash drive + two DACs, no hub); 5 V/4 A supply; Debian 13 riscv64 official; board DTB in mainline 6.19 | No Armbian; vendor image versions unverified; VL805 isochronous behaviour depends on firmware (Pi 4 history); ≈ 3–4 W idle; myMPD/upmpdcli from source; no SIMD for the (unneeded) software DSD path; power-button boot quirk to check |
| Orange Pi Zero LTS as-is | ≈ 1 W idle, owner's preference for a small board | XR819 Wi-Fi is a hard blocker for a Wi-Fi-only server (2.4 GHz only, ≈ 10–20 Mbps at best, disconnects); 512 MB leaves ≈ 150–300 MB free; micro-USB 2 A input marginal with three USB devices |
| Orange Pi Zero LTS + USB Wi-Fi adapter with a mainline driver (MediaTek MT7612U / MT7921AU, `mt76`) or Ethernet | Keeps the small, low-power board; mainline Wi-Fi on 5 GHz; CPU is sufficient | Needs the 13-pin expansion board (or a hub) for disk + DAC + Wi-Fi on one USB 2.0 root; 512 MB stays tight; armhf 32-bit build target; more cables than the RV |
| Raspberry Pi 5 | Most trouble-free USB audio host, mainline Wi-Fi, packages for everything | Over-specified for the task; ≈ 2.5–3 W idle (close to the RV anyway); owner prefers not to use it |

## Decision

Deploy on the Orange Pi RV. Build every artifact for riscv64 first and keep arm64 and armhf
builds (`GOARCH=arm GOARM=7`) so the Zero LTS remains usable. If the owner wants to try the
Zero LTS, it enters the M0 bake-off **only with a mainline-driver USB Wi-Fi adapter or
Ethernet**, under the same pass criteria:

1. 24 h Wi-Fi stability (no disconnects, ≤ 1 % packet loss, ≥ 20 Mbps sustained on 5 GHz)
   with power save off.
2. 24 h USB-DAC playback at 24/192 and DSD128/256 without xruns while Samba copies run.
3. Native DSD offered for the ES9039Q2M dongle (`DSD_U32_BE` in `stream0`), else DoP256.
4. RAM headroom ≥ 100 MB with MPD, `hifid`, Samba and Avahi running.

The RV is the default if it passes; the Zero LTS replaces it only if it also passes and the
owner prefers it. The Raspberry Pi 5 is the fallback if neither passes.

## Consequences

- Install script, units and binaries stay architecture-neutral; CI builds riscv64, arm64 and armhf.
- The RV needs a verified Debian 13 image path (vendor image, or Debian riscv64 with the
  board DTB) documented in docs/10 §1 during M0, plus the always-on/power-button check.
- Zero LTS extras: 13-pin expansion board, a `mt76` USB Wi-Fi adapter (≈ 15–20 USD), zram
  swap, no myMPD/upmpdcli, MPD buffer 8 MB.
- Idle power of the RV (≈ 3–4 W) is accepted for a device that plays music most of the day.

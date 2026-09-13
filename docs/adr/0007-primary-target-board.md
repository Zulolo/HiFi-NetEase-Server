# ADR-0007 · Orange Pi Zero 3 (Armbian Debian 13) is the primary target; Orange Pi RV is the second target

- Status: accepted (pending owner confirmation of the exact RV model, Q1)
- Date: 2026-09-13
- Requirements addressed: C-4, C-5, NFR-1, NFR-4

## Context

The owner has an Orange Pi Zero 3 (Allwinner H618, arm64) and an Orange Pi RV (StarFive
JH7110, riscv64; the RV2 is a different Ky X1 board). Evaluation and scoring in docs/05 §1.

## Options considered

| Option | Pros | Cons |
|---|---|---|
| **Zero 3 primary** | Plain EHCI host ports (no hub), Armbian Debian 13 with Linux 6.18 (fresh DSD quirk table), every needed package on arm64 including myMPD/upmpdcli, ≈ 1 W idle, owner has it | USB 2.0 only, one Type-A (expansion board or powered hub needed), undocumented VBUS budget, 1 GB variant is tight |
| RV (JH7110) primary | 4× USB 3.0, 5 V/4 A supply, more RAM, Debian riscv64 official, mainline DTB in 6.19 | No Armbian; kernel/DTB path needs care; VL805 firmware version unverified for isochronous fixes; ≈ 3–4 W idle; myMPD/upmpdcli from source |
| RV2 (Ky X1) | 8 cores, USB 3.0 | USB 2.0 mainline support still in review; vendor kernel bug; hub in front of USB 3.0 |
| Buy a Raspberry Pi 4 | Best-proven USB audio host | Not owned; price increases in 2026 |

## Decision

Develop and verify on the Zero 3 with Armbian's Debian 13 minimal image (6.18 kernel), 2 GB
or 4 GB variant preferred. Build every artifact for riscv64 as well and validate on the RV in
M5; choose the RV as the deployment chassis if the disk is a spinning HDD or if more than two
USB devices are needed. Nothing in the software is arm64-specific.

## Consequences

- Install script and units must be architecture-neutral; CI builds both binaries.
- Zero 3 setup needs a powered hub or self-powered SSD and a 5 V/3 A PSU (docs/05 §1.2, §8).
- If the owner's board is actually an RV2, the riscv64 secondary target shifts to Armbian's
  community image for it; the decision does not change.

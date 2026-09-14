# ADR-0007 · Target board: Orange Pi Zero LTS first, Orange Pi RV as the fallback

- Status: accepted (revised 2026-09-13 with the decision to try the Zero LTS first)
- Date: 2026-09-13
- Requirements addressed: C-4, C-5, C-9, NFR-1, NFR-4

## Context

The reference targets are an Orange Pi Zero LTS (Allwinner H3, 4× Cortex-A7, 512 MB, one USB
Type-A plus header ports, XR819 Wi-Fi: 2.4 GHz only, out-of-tree driver, widely reported
unstable) and an Orange Pi RV (StarFive JH7110, riscv64, 2–8 GB, 4× USB 3.0, Broadcom AP6256
Wi-Fi). **Wi-Fi is the only network link** at the speaker's location; separate 2.4 GHz and
5 GHz networks are available.

The usage profile changes the weight of the Wi-Fi risk:

- Music is played mostly from **local files** (NetEase downloads and Samba uploads), so the
  audio path does not depend on Wi-Fi during playback.
- The board is **always on**, so NetEase downloads can run in the background, where slow or
  briefly dropped Wi-Fi only delays a job.
- A spare low-power board is the preferred host; trying the Zero LTS costs nothing, and
  another board can replace it if Wi-Fi turns out to be a nuisance.

The CPU is not a constraint: the DAC does the D/A conversion, native DSD is pass-through,
and FLAC decoding costs the H3 10–20 % of one core. RAM (512 MB) is tight but sufficient for
MPD, `hifid`, Samba and Avahi (docs/05 §6).

## Options considered

| Option | Pros | Cons |
|---|---|---|
| **Orange Pi Zero LTS first, onboard XR819 on the 2.4 GHz network** | ≈ 1–1.5 W idle for an always-on box; a spare board; local playback needs no Wi-Fi; downloads tolerate slow links | XR819 throughput ≈ 10–25 Mbps and disconnects; phone control and Samba copies suffer when it drops; 512 MB; armhf; needs a hub or the 13-pin expansion board for disk + DAC; micro-USB 2 A input |
| Zero LTS + USB Wi-Fi adapter with a mainline driver (`mt76`) | Removes the XR819 problem, keeps the small board, 5 GHz | ≈ 15–20 USD and one more USB device on the single USB 2.0 root |
| Orange Pi RV | Mainline Broadcom Wi-Fi, 2–8 GB, 4× USB 3.0, 4 A supply, official Debian riscv64 | ≈ 3–4 W idle; vendor image / DTB path to verify; VL805 firmware unverified |
| Raspberry Pi 5 (reference) | Most trouble-free USB audio host | Over-specified for the task; ≈ 2.5–3 W idle |

## Decision

Deploy first on the **Orange Pi Zero LTS** using its onboard Wi-Fi on the 2.4 GHz network,
with these mitigations built into the design:

1. A Wi-Fi watchdog (`deploy/systemd/wifi-watchdog.timer`) that restarts the interface when
   the gateway stops answering; power save off; fixed channel on the 2.4 GHz access point.
2. Every network-dependent function is retry-tolerant: the download scheduler resumes
   interrupted files, the stream proxy re-resolves URLs, the PWA reconnects its WebSocket.
3. Local playback is the default mode: NetEase content is synced to disk (docs/08 §7.1)
   rather than streamed, so Wi-Fi never sits in the audio path once a track is downloaded.
4. Resource limits for 512 MB: MPD buffer 8 MB, zram swap, no myMPD/upmpdcli.

Acceptance thresholds during M0 (same as before, now pass/fail for staying on this board):
24 h without a Wi-Fi outage longer than 60 s and ≤ 2 % packet loss; 24 h of 24/192 and
DSD128/256 local playback without xruns while a Samba copy runs; native DSD or DoP256 for
the ES9039Q2M dongle; ≥ 100 MB RAM headroom.

If the Zero LTS fails the Wi-Fi threshold, the first escalation is a `mt76` USB Wi-Fi
adapter on the same board; the second is moving to the **Orange Pi RV**, which is why all
artifacts are built for armhf, riscv64 and arm64 alike.

## Consequences

- Build targets: armhf first (`GOARCH=arm GOARM=7`), riscv64 and arm64 kept; install script
  and units architecture-neutral.
- Zero LTS bill of materials: 13-pin expansion board or a small USB hub (flash drive + DAC),
  heatsink on the H3, good 5 V/2 A micro-USB supply and cable.
- Design emphasis shifts to offline-first: the NetEase sync job and a robust Samba import path
  matter more than streaming polish.
- The residual Wi-Fi risk (phone control latency, Samba speed of ≈ 1–3 MB/s on this chip's
  2.4 GHz link) is accepted for the first deployment.

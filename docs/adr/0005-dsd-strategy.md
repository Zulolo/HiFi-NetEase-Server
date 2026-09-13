# ADR-0005 · DSD strategy: native → DoP → PCM, configured per output; volume policy

- Status: accepted
- Date: 2026-09-13
- Requirements addressed: FR-4.2, FR-4.3, FR-4.5, FR-4.6, FR-2.2, NFR-1

## Context

The owner's dongles are ES9039Q2M (native to DSD512+, DoP to DoP512) and CS43131 (DoP to
DSD128 only, native to DSD256 via dedicated pins if the bridge exposes it) with unknown USB
bridges. Linux offers native DSD only for recognised devices. Software conversion of DSD256
costs 57–80 % of a Cortex-A53 core (docs/05 §5). Details in docs/04.

## Options considered

| Option | Pros | Cons |
|---|---|---|
| **Let MPD decide per output block with probed settings** (`dop`, `allowed_formats`, fallback rate pinned to 352.8 k) | Uses MPD's built-in native → DoP → PCM chain; per-DAC control; runtime `outputset` | Depends on correct probing; `allowed_formats` fallback semantics to verify |
| Always DoP | Works on every DSD dongle | Wastes native capability; DSD256+ impossible on 384 k-max bridges |
| Always convert to PCM | Simplest, no kernel quirks | Contradicts the "hardware first" requirement; CPU cost |
| Kernel patches for every dongle | Native everywhere | Maintenance across kernel updates; risky vendor-wide flags |

## Decision

1. Per output: `hifid` probes `/proc/asound/cardX/stream0` and the chip table, then sets
   `dop` and `allowed_formats` so that native is used when offered, DoP for rates the DAC
   accepts, and PCM fallback only above that, at 352.8 kHz (or 176.4 kHz) to keep integer
   resampling ratios.
2. Kernel side: use `snd-usb-audio quirk_flags=VID:PID:dsd_raw` (≥ 6.18) or `0x8000`
   (older) for dongles that advertise DSD but are not in the quirk table; submit upstream
   `DEVICE_FLG` patches; never vendor-wide flags for unknown bridges.
3. DSD256/DSD512 on a DAC that cannot take them natively or via DoP is served by an offline
   PCM twin generated once, not by real-time conversion.
4. Volume: `hardware` (USB feature unit) for PCM and for native DSD; `fixed` (100 %, speaker
   knob) for DoP until a dongle is verified not to scale DoP samples; `software` refused for
   DSD (MPD no-op) and never used on DoP.
5. The delivery mode is reported to the user (`format.delivery` badge).

## Consequences

- M0 must produce the DAC capability matrix (docs/04 §7) before M4 automation.
- `resampler` defaults to `soxr medium` with 2 threads on these boards.
- A "DSD fallback" output block with `format "352800:24:2"` is the backstop if
  `allowed_formats` does not select the intended fallback rate (R11).

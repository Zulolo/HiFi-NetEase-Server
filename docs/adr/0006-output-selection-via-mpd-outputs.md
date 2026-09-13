# ADR-0006 · Output selection = MPD output blocks + udev-stable card names + generated config

- Status: accepted
- Date: 2026-09-13
- Requirements addressed: FR-5.1 to FR-5.6, FR-2.5

## Context

Several USB DACs may be attached; the user selects one from the phone; identity must survive
reboots and re-plugging; hot-plug should work without manual steps. MPD cannot add outputs at
runtime but can enable/disable configured ones.

## Options considered

| Option | Pros | Cons |
|---|---|---|
| **One `audio_output` per known DAC, `enableoutput`/`disableoutput` to switch; udev `ATTR{id}` names; `hifid` generates blocks and restarts MPD only for new DACs** | Uses MPD's designed mechanism; works with M.A.L.P./myMPD too; simultaneous outputs possible; no restart for known DACs | New DAC = one MPD restart; disabled blocks for absent devices are visible in clients (harmless) |
| Single output block, rewrite `device` and restart MPD on every switch | Simplest config | Playback interruption of several seconds on every switch; no simultaneous outputs |
| ALSA `pcm.!default` indirection changed by `hifid` | MPD unaware | Requires MPD to reopen the device anyway; hides state from clients |
| PipeWire/PulseAudio routing | Dynamic | Not bit-perfect by default; extra RAM; against NFR-1 |
| One MPD instance per DAC | Isolation | Duplicated queues and libraries; confusing |

## Decision

Adopt the first option. Stable identity comes from a udev rule mapping `idVendor:idProduct`
(and serial where present) to an ALSA card id; MPD blocks reference `hw:CARD=<id>`. `hifid`
keeps per-output settings and aliases, renders `mpd.conf`, and handles udev hot-plug events:
known device → mark present (no restart); unknown device → add block and restart MPD when
playback is stopped or on user confirmation; removed active device → pause and notify.

## Consequences

- `deploy/udev/90-hifi-dac.rules` and the generated `mpd.conf` are part of the deliverable.
- MPD restart orchestration needs a narrow privilege path for `hifid` (docs/10 §2).
- Output capability (`caps`) is reported per block from the probe, enabling the format badge
  and the DSD policy of ADR-0005.

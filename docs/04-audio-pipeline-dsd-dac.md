# 04 · Audio Pipeline: PCM, DSD, USB DACs and Output Selection

Status: draft v0.1 · 2026-09-13 · addresses FR-4, FR-5, NFR-1; decisions in ADR-0005, ADR-0006

## 1. The chain

```
file / stream ──> MPD decoder plugin ──> (optional conversion) ──> MPD ALSA output ──> snd-usb-audio ──> USB dongle ──> DAC chip ──> 3.5 mm ──> Acton IV AUX
                 flac, ffmpeg, dsf,        Dsd2Pcm, soxr,          hw:CARD=<name>       UAC2 isochronous   Comtrue/XMOS/    ES9039Q2M
                 dsdiff, mad, opus …       DoP packing             S16/S24/S32/DSD_U32   endpoint           Savitech bridge  CS43131
```

Rules that make the chain bit-perfect (NFR-1):

1. MPD opens the device as `hw:CARD=<name>` (never `default`, `plughw`, dmix).
2. `auto_resample "no"`, `auto_format "no"`, `auto_channels "no"`: MPD only converts when the
   device refuses the format, and reports it.
3. No software volume, no ReplayGain, no `format` forcing in the output block.
4. No PulseAudio/PipeWire on the box.
5. Verification: `/proc/asound/card*/pcm0p/sub0/hw_params` while playing must show the
   file's rate and a lossless container format (`S24_3LE`, `S32_LE`, or `DSD_U32_BE`).

## 2. What each format needs from the DAC

| Source | Delivered as | DAC must support |
|---|---|---|
| PCM 16/44.1 … 24/192 | same rate, `S16_LE`/`S24_3LE`/`S32_LE` (MPD picks the container the device offers; 24-in-32 is still bit-perfect) | that rate |
| PCM 24/352.8, 24/384, 32/768 | same | rate (ES9039Q2M dongles usually to 768 k; CS43131 to 384 k) |
| DSD64 | native `DSD_U32_BE` at 352 800 "frames/s" (MPD counts bytes), or DoP in `S24_P32` at 176.4 k, or PCM 352.8 k after conversion | native: kernel quirk + bridge; DoP: 176.4 k PCM + DoP decoding |
| DSD128 | native, or DoP at 352.8 k, or PCM 705.6 k → resampled | CS43131: DoP OK (its max); ES9039Q2M: OK |
| DSD256 | native, or DoP at 705.6 k, or PCM 1411.2 k → resampled | CS43131: **native only via dedicated pins, DoP not possible** (chip limit); ES9039Q2M: OK if bridge does 705.6 k |
| DSD512 | native, or DoP at 1411.2 k (unrealistic on USB 2.0 dongles), or PCM | ES9039Q2M native only |

## 3. Decision chain (FR-4.3)

MPD already implements the order native → DoP → PCM inside its ALSA output plugin:

1. If the ALSA device offers `DSD_U32_BE` (native), MPD uses it automatically.
2. Else, if `dop "yes"` (globally or per format via `allowed_formats "dsd64:*=dop"`), MPD packs
   DoP frames into 24-bit PCM at rate/16 and tries to open the device at that PCM rate.
3. Else (or if that fails) MPD converts DSD to PCM with `Dsd2Pcm` (96-tap FIR, 8:1
   decimation, ≈160 dB stopband) producing PCM at DSD rate ÷ 8, then resamples to a rate the
   device accepts.

`hifid`'s job is to give each DAC the right settings so that step 1 or 2 is reached whenever
the hardware allows it, and to keep step 3 cheap when it is unavoidable:

```
probe(card):
  native  = "DSD_U32_BE" in /proc/asound/cardX/stream0
  rates   = playback rates listed in stream0 (per altsetting)
  dop_max = highest DSD rate whose DoP container rate ∈ rates, capped by chip table
  dsd_max = chip table (ES9039Q2M: DSD512+, CS43131: DSD128 via DoP / DSD256 native pins)

output block:
  dop            = "no"  if native else "yes"
  allowed_formats = explicit PCM rates the device supports
                    + "dsd64:*=dop dsd128:*=dop" (only those ≤ dop_max, only when not native)
                    + fallback PCM rate for higher DSD = highest supported multiple of 44.1 k (352.8 k or 176.4 k)
```

The user can pin `dsd_mode` per output (`auto | native | dop | pcm`, FR-4.3); `hifid`
translates it to `dop`/`allowed_formats` and applies it live with `outputset` where MPD allows,
or with a controlled restart otherwise.

Exact `allowed_formats` matching semantics ("first match wins; otherwise best fallback")
must be confirmed on hardware in M0 for the DSD-to-PCM fallback rate; if MPD's fallback choice
is not the intended one, `hifid` uses the coarser `format "352800:24:2"` only on a dedicated
"DSD fallback" output block (see §6).

## 4. Native DSD on Linux: making the kernel offer `DSD_U32_BE`

The kernel offers native DSD only for devices it recognises:

1. The UAC2 descriptor must advertise a `RAW_DATA` alt-setting (`dsd_raw`), which all
   DSD-capable dongles do.
2. And the device must be either in the explicit list in `sound/usb/quirks.c`, or its vendor
   ID must carry `QUIRK_FLAG_DSD_RAW`. Vendor-wide in mainline 2026: Comtrue 0x2fc6 (since
   6.14, also backported to 6.1-stable), XMOS 0x20b1, Thesycon 0x152a, FiiO 0x2972, iBasso
   0x18d1, Cayin, Gustard, HiBy, Khadas, Rotel, T+A and others (list in docs/02 §2.2).
   Savitech 0x262a (SA9312L, used by JCally JM20 Pro, FiiO KA11) is **not** vendor-wide.

Procedure per dongle (M0, `deploy/scripts/probe-dac.sh`):

```
lsusb                                   -> VID:PID
cat /proc/asound/cards                  -> card index and id
cat /proc/asound/cardX/stream0          -> "Format: DSD_U32_BE" (native available) or "SPECIAL" (not)
```

If `SPECIAL` and the dongle supports DSD per its datasheet:

| Kernel | Action |
|---|---|
| ≥ 6.18 | `/etc/modprobe.d/snd-usb-audio.conf`: `options snd-usb-audio quirk_flags=VVVV:PPPP:dsd_raw` (readable form, per device, sysfs-changeable) |
| 6.1 – 6.17 | `options snd-usb-audio quirk_flags=0x8000` (bit 15 = dsd_raw; applies by probe order, so index/vid/pid options may be needed to pin the order) |
| any | Send a one-line `DEVICE_FLG(0xVVVV, 0xPPPP, QUIRK_FLAG_DSD_RAW)` patch upstream; the fix arrives in later Debian/Armbian kernels |

Never set a vendor-wide flag for an unknown bridge: a wrong `DSD_RAW` flag has caused
mis-clocked playback on unrelated devices.

Preferred kernels: Armbian 6.18 (Zero 3) or Debian 6.12 (trixie) already contain the Comtrue
entry, which likely covers the Moondrop Dawn Pro (`2fc6:f06a`) and every other Comtrue-bridge
dongle.

## 5. Volume policy (FR-2.2, ADR-0005)

| Output volume mode | Mechanism | DSD safe? | When |
|---|---|---|---|
| `hardware` | MPD `mixer_type "hardware"` on the dongle's USB feature unit (`mixer_control "PCM"`, via `mixer_device "hw:CARD=<name>"`) | Native DSD: yes if the chip applies volume in its DSD processor (CS43131 and ES9039Q2M both do). DoP: only if the bridge forwards the control to the chip instead of scaling samples (per design; test in M0) | Default for PCM; default for DSD after the M0 test passes |
| `fixed` | `mixer_type "none"`, output at 100 %; volume on the Acton IV knob | Always | Default for DoP until proven; purist mode |
| `software` | MPD software mixer | **No**: MPD's software volume on DSD is a silent no-op; on DoP it destroys the markers | Only for PCM-only outputs |

`hifid` refuses `software` while a DSD stream is playing and shows the reason in the PWA.

## 6. Software fallback without dropouts (FR-4.5)

Measured on Cortex-A53 class boards (Raspberry Pi 3 B+): DSD256 → PCM ≈ 80 % of a core,
57 % when the output is forced to 352.8 kHz; DSD128 → 384 kHz reported at 100 % (dropouts).
Therefore:

1. Keep the fallback PCM rate at the DSD-family rate the device supports (352.8 k, else
   176.4 k) so the resampler works on integer ratios.
2. Use `resampler { plugin "soxr" quality "medium" threads "2" }` (or `libsamplerate` type 2)
   on these boards; "very high" is reserved for PCM-rate conversions that rarely happen.
3. DSD256/DSD512 on a DAC without native/DoP support is not played in real time. `hifid`
   offers **offline conversion**: a background job creates a PCM twin
   (`ffmpeg -i x.dsf -af "lowpass=f=30000" -ar 352800 -sample_fmt s32 x.dsd256.flac`, one-time,
   several minutes per album on the Zero 3) stored in `/srv/data/pcm-twins/` and mapped in the
   index; the PWA offers "play PCM twin" when the active output cannot do the DSD rate.
4. The PWA shows `pcm-converted` and `pcm-resampled` badges so the user knows (FR-4.6).

## 7. DAC capability matrix (fill in M0)

| Dongle (VID:PID) | Chip | Bridge | ALSA id (udev) | PCM rates | Native DSD (stream0) | DoP tested to | HW volume OK with DoP? | Kernel/quirk used |
|---|---|---|---|---|---|---|---|---|
| e.g. Moondrop Dawn Pro (2fc6:f06a) | 2× CS43131 | Comtrue | DawnPro | | | | | |
| ES9039Q2M dongle #1 ( : ) | ES9039Q2M | ? | | | | | | |
| CS43131 dongle #2 ( : ) | CS43131 | ? | | | | | | |

Expected outcomes from the datasheets:

| Chip | Best case on Linux | Typical fallback |
|---|---|---|
| ES9039Q2M | native DSD to DSD512 (bridge permitting), PCM to 768 k | DoP to DSD256 if bridge supports 705.6 k PCM, else DoP128 |
| CS43131 | native DSD to DSD256 via dedicated pins **only if the bridge exposes native DSD**; DoP64/128; PCM to 384 k | DoP128; DSD256 → software (352.8 k PCM twin or real-time at ≈ 60 % CPU) |

## 8. Output selection (FR-5, ADR-0006)

### 8.1 Identity

- udev rule (`deploy/udev/90-hifi-dac.rules`) sets `ATTR{id}` on `SUBSYSTEM=="sound"` cards
  by USB `idVendor`/`idProduct` (and serial when present) → ALSA names like `DawnPro`,
  `ES9039A`, addressable as `hw:CARD=DawnPro` regardless of plug order (FR-5.3).
- `hifid` keeps a table `vid:pid[:serial] → {id, alias, chip, settings}` in its config; unknown
  dongles get `usb-<vid>-<pid>` and a generic profile until the user names them.

### 8.2 MPD output blocks

One `audio_output` block per known dongle, `enabled "no"` for all but the active one. MPD
tolerates blocks whose device is absent as long as they are disabled. Switching = `disableoutput`
old + `enableoutput` new (FR-5.2); MPD re-opens the device and continues from its buffer.
Simultaneous playback (FR-5.6) = enable two blocks.

### 8.3 Hot-plug (FR-5.4)

```
udev add/remove (sound card) ──> systemd path/unit or udev RUN ──> hifid /outputs/rescan
   known dongle, block exists   -> mark present; if it is the configured "preferred" output, offer to switch
   unknown dongle               -> generate block, write mpd.conf, restart MPD when stopped (or on user confirmation)
   active dongle removed        -> MPD disables the output itself (error); hifid pauses, emits outputs event, PWA shows message
```

MPD cannot add outputs at runtime, so a new dongle needs one MPD restart; known dongles do not.

## 9. ALSA and system settings

| Setting | Value | Reason |
|---|---|---|
| `buffer_time` (MPD ALSA output) | 500 000 µs | USB dongles on SBCs tolerate scheduling jitter better with a large buffer; MPD caps at 2 s |
| `period_time` | 100 000 µs | fewer wakeups |
| MPD `audio_buffer_size` | 8192 kB | prebuffer for network streams and 24/192 |
| `LimitRTPRIO` / `LimitRTTIME` / `LimitMEMLOCK` | 40 / infinity / 64M | MPD's shipped unit values; output thread gets SCHED_FIFO |
| CPU governor | `performance` or `ondemand` with `io_is_busy` | avoid frequency dips during isochronous transfers |
| Wi-Fi power save | off (Ethernet preferred) | |
| `snd-usb-audio` module | no extra options unless a quirk flag is needed | |
| Sound servers | none installed | |

## 10. Verification checklist (M0 and M4)

1. `mpc play` a 24/96 FLAC → `hw_params` shows `rate: 96000`, format `S24_3LE`/`S32_LE`.
2. DSD64 DSF on a native-capable dongle → `hw_params` shows `DSD_U32_BE`, `rate: 352800`.
3. Same file with `dop "yes"` forced → `hw_params` shows `S24_3LE`/`S32_LE` at `176400`; dongle's DSD indicator lights.
4. Same file on a PCM-only dongle → MPD log "Falling back to PCM"; `hw_params` at 352 800 or the nearest supported rate; no xruns for 10 minutes; CPU % recorded in docs/05.
5. Volume change via USB mixer during native DSD and during DoP → no noise burst; note per dongle.
6. Switch outputs mid-track from M.A.L.P. → gap < 2 s, playback continues.
7. Unplug the active dongle → MPD pauses/errs without crashing; replug → same card name.
8. 24 h soak at 24/192 and DSD128 with `dmesg`/MPD log clean.

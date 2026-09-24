# deploy/scripts

Bash scripts for any Debian-based SBC (Debian 12/13, Armbian, Raspberry Pi OS, vendor
images; arm64, armhf, riscv64). Run them on the board as root. All were exercised on an
Orange Pi Zero 3 (Debian 12, kernel 6.1) with a Comtrue/ES9039 USB dongle on 2026-09-24.

| Script | Purpose |
|---|---|
| `install.sh` | One-shot setup: packages (mpd 0.24 from backports on Debian 12), disk, DAC detection, MPD config, Samba share, Avahi, Wi-Fi watchdog, services. `--help` lists the options. |
| `setup-disk.sh` | Format (only with `--format`) and mount the music disk at `/srv/hifi` with `/srv/music` and `/srv/data` bind mounts; `--local` keeps everything on the OS card. |
| `dac-setup.sh` | Enumerate USB DACs, write a udev rule for stable ALSA names, apply the name immediately, record capabilities in `/etc/hifi/dacs.conf`; `--try-native-dsd` adds and persists the `snd-usb-audio` DSD quirk when a DAC advertises raw DSD the kernel did not enable (legacy `0x8000` form below kernel 6.18, `VID:PID:dsd_raw` from 6.18). |
| `gen-mpd-conf.sh` | Render `/etc/mpd.conf` from `../mpd/mpd.conf.template`: one bit-perfect `audio_output` per DAC (`dop`, `allowed_formats`, hardware mixer when present), buffer sized by RAM, user region preserved, previous file backed up. |
| `test-audio.sh` | Generate 44.1/96/192 kHz tones, play them (and an optional DSD file) through MPD and print the format the DAC actually received. |
| `probe-dac.sh` | Read-only dump (kernel, `lsusb`, ALSA cards, `stream0`, mixers, live `hw_params`) for bug reports and the DAC matrix in docs/04 §7. |
| `wifi-watchdog.sh` | Ping the gateway; bounce the wireless interface on failure, restart the network manager after 3 misses. Installed by `install.sh` when the default route is wireless; run by `../systemd/wifi-watchdog.timer`. |
| `lib.sh` | Shared helpers (card enumeration, rate parsing, DSD state, logging). Sourced by the others. |

Planned: `backup.sh` (rsync of the music tree and `/etc/hifi`), `bench-dsd.sh` (CPU cost of the
software DSD fallback for boards whose DAC cannot do native DSD or DoP).

Windows note: if the files were edited on Windows, strip CR line endings once on the board:
`sed -i 's/\r$//' ~/hifi/deploy/scripts/* ~/hifi/deploy/mpd/* ~/hifi/deploy/systemd/*`.

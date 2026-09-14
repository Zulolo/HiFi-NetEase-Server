# deploy/scripts

| Script | Status | Purpose |
|---|---|---|
| `probe-dac.sh` | ready (read-only) | Dump kernel, `lsusb`, ALSA cards, `stream0` formats (native DSD?), mixer controls, live `hw_params`. Output fills docs/04 §7. |
| `wifi-watchdog.sh` | ready | Pings the gateway; bounces `wlan0` on failure, restarts the network manager after 3 consecutive failures. Run by `deploy/systemd/wifi-watchdog.timer`. XR819 mitigation on the Zero LTS (ADR-0007). |
| `install.sh` | planned (M1) | Fresh Debian/Armbian → packages, users, disk mounts, udev, MPD config, `hifid` unit; `--with-samba`, `--with-upmpdcli`, `--with-mympd`. Arch-neutral. |
| `bench-dsd.sh` | planned (M0) | Plays DSD64/128/256 test files through MPD in native, DoP and PCM-fallback modes and records CPU % of the MPD threads (`top -H`) and xruns; writes a table for docs/05 §11. |
| `backup.sh` | planned (M5) | rsync of `/srv/music`, `/srv/data/hifid`, `/etc/hifid`, `/etc/mpd.conf`, udev rule to a target path. |

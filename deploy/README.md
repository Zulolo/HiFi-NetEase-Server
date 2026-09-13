# deploy/ · Debian deployment artifacts

Templates and unit files used by the install procedure in docs/10. Everything
here is meant to be copied to the board as-is or rendered by `hifid` from
config.

```
deploy/
├── mpd/        mpd.conf template (bit-perfect ALSA outputs, DSD options, per-DAC blocks)
├── udev/       rules giving USB DACs stable ALSA card names and triggering hot-plug events
├── systemd/    hifid.service, mpd.service drop-in (RT priority, ordering after the USB disk mount),
│               mount unit for the 512 GB disk
├── nginx/      optional reverse proxy / TLS for PWA install on LAN (see docs/06)
└── scripts/    install.sh, probe-dac.sh (capability dump), bench-dsd.sh (CPU cost measurement)
```

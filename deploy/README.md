# deploy/ · Debian deployment

Everything needed to turn a Debian-based SBC into the playback layer of the server. Copy the
folder to the board and run `scripts/install.sh` (see [docs/USER-MANUAL.md](../docs/USER-MANUAL.md)).

```
deploy/
├── scripts/    install.sh and the helpers it calls (disk, DAC detection, MPD config, audio test, watchdog)
├── mpd/        mpd.conf.template (rendered by gen-mpd-conf.sh) and mpd.conf.example (as rendered on the verified board)
├── udev/       90-hifi-dac.rules example; the installer generates the real one from the attached DACs
├── systemd/    mpd.service drop-in (RT limits, mount ordering), wifi-watchdog timer/service, DAC hot-plug hook,
│               Avahi service file, hifid.service and mount units for reference (setup-disk.sh writes the mounts itself)
└── nginx/      optional reverse proxy notes (only for TLS in front of the future hifid)
```

Nothing here is specific to one board or one DAC: disks are addressed by label, DACs by their
USB vendor/product ids, the Wi-Fi watchdog is installed only when the default route is
wireless, and the DSD kernel quirk only when a DAC advertises raw DSD that the running kernel
did not enable.

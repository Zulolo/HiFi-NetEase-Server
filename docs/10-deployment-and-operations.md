# 10 · Deployment and Operations

Status: draft v0.1 · 2026-09-13 · addresses NFR-2, NFR-4, NFR-5, NFR-7, C-7

## 1. OS image

| Board | Image | Kernel | Why |
|---|---|---|---|
| Orange Pi Zero 3 (primary) | Armbian **Debian 13 (trixie) minimal**, community build | 6.18 mainline | USB, GbE, Wi-Fi supported; kernel already carries the Comtrue/XMOS/FiiO native-DSD quirks; `apt` gives mpd 0.24.4 |
| Orange Pi Zero 3 (alternative) | Vendor Debian 12 image | 6.1 legacy | Works, but older USB-audio quirk table and Debian 12's mpd 0.23 (use bookworm-backports 0.24) |
| Orange Pi RV (JH7110) | Debian 13 riscv64 (vendor rootfs or debian-installer) | 6.12 (Debian) with the board DTB, or self-built 6.19 | riscv64 is an official Debian 13 architecture; the RV device tree is in mainline 6.19 |

Both: 1.5 GB-RAM Zero 3 variants need the known DRAM-size fix for Armbian; buy the 2 GB or
4 GB variant if choosing new hardware.

## 2. Install procedure (rendered as `deploy/scripts/install.sh` in M1)

```
1. Flash image, boot, set hostname "hifi", static DHCP lease on the router, disable Wi-Fi power save (or use Ethernet).
2. apt install mpd mpc alsa-utils ffmpeg avahi-daemon udev inotify-tools                 # both arches
   optional: samba ; upmpdcli (arm64: vendor repo) ; mympd (arm64: OBS repo)
3. Disk: mkfs.ext4 -m 1 -L music /dev/sdX1 ; systemd mount unit deploy/systemd/srv-music.mount (noatime, nofail, x-systemd.device-timeout=30)
   mkdir -p /srv/music/{local,netease,playlists} /srv/data/{mpd,hifid,incoming} ; chown mpd:audio / hifid:audio
4. udev: copy deploy/udev/90-hifi-dac.rules ; udevadm control --reload ; replug dongles ; check /proc/asound/cards
5. MPD: copy deploy/mpd/mpd.conf.example -> /etc/mpd.conf (hifid regenerates it later) ; drop-in deploy/systemd/mpd.service.d/override.conf
6. hifid: copy binary to /usr/local/bin/hifid ; /etc/hifid/config.yaml ; /etc/hifid/env (token, secret) ; deploy/systemd/hifid.service ; systemctl enable --now hifid
7. Verify: mpc outputs ; curl http://hifi.local:8080/api/v1/system/status ; open http://hifi.local:8080 on the phone
```

Users and groups: `mpd` (Debian default) and `hifid` (system user) both in `audio`; `hifid`
owns `/srv/music/local`, `/srv/music/netease`, `/srv/data/hifid`, `/srv/data/incoming` and
can write `/etc/mpd.conf` through a small `sudoers` rule limited to `install -m 644` +
`systemctl restart mpd`.

## 3. systemd units

| Unit | Key settings |
|---|---|
| `srv-music.mount` | `Options=noatime,nofail`, `x-systemd.device-timeout=30s` |
| `mpd.service` (Debian) + `override.conf` | `After=srv-music.mount network-online.target`, `RequiresMountsFor=/srv/music /srv/data`, `LimitRTPRIO=40`, `LimitRTTIME=infinity`, `LimitMEMLOCK=64M`, `Restart=on-failure` |
| `hifid.service` | `After=mpd.service`, `Wants=mpd.service`, `User=hifid`, `EnvironmentFile=/etc/hifid/env`, `Restart=always`, `RestartSec=3`, `ProtectSystem=strict`, `ReadWritePaths=/srv/music /srv/data/hifid /srv/data/incoming /run/hifid`, `NoNewPrivileges` (except the sudo path for mpd.conf, handled by a separate one-shot unit `hifid-apply-mpd.service` triggered via a path unit) |
| `srv-hifi.mount` + `srv-music.mount` / `srv-data.mount` | the disk is mounted once at `/srv/hifi`; `/srv/music` and `/srv/data` are bind mounts of its subdirectories so both trees share one filesystem (atomic rename for uploads) |
| `hifid-dac-hotplug@.service` | started by the udev rule `RUN+="/bin/systemctl start hifid-dac-hotplug@%k.service"`; posts `/api/v1/outputs/rescan` over the local unix socket `/run/hifid/api.sock` (no token) |
| `avahi-daemon` | `/etc/avahi/services/hifid.service` publishes `_hifid._tcp` and `_http._tcp` |

MPD socket: MPD listens on `/run/mpd/socket` (for `hifid`, fast) and on the LAN IP port 6600
(for M.A.L.P.). `hifid` listens on `0.0.0.0:8080` by default but can be pinned to the LAN
interface; the stream proxy listens on `127.0.0.1` only.

## 4. Configuration files

### 4.1 `/etc/hifid/config.yaml` (shape)

```yaml
server_name: Living room
listen: 0.0.0.0:8080
auth: { mode: admin }                     # none | admin | all ; token in /etc/hifid/env
paths:
  music: /srv/music
  data: /srv/data/hifid
  incoming: /srv/data/incoming
mpd: { socket: /run/mpd/socket, host: 127.0.0.1, port: 6600 }
netease:
  level_preference: [hires, lossless, exhigh, higher, standard]
  stream_mode: redirect                   # redirect | pipe
  cache_while_playing: false
  export_playlists_every: 6h
outputs:
  - id: dawnpro
    match: { vid: "2fc6", pid: "f06a" }
    alias: Moondrop Dawn Pro
    alsa_id: DawnPro
    chip: cs43131
    dsd_mode: auto
    volume_mode: hardware
    max_rate: 384000
  - id: es9039a
    match: { vid: "20b1", pid: "xxxx", serial: "…" }
    alias: ES9039Q2M dongle
    alsa_id: ES9039A
    chip: es9039q2m
    dsd_mode: auto
    volume_mode: fixed
    max_rate: 768000
preferred_output: dawnpro
discovery: { mdns: true, udp_beacon: true }
```

### 4.2 `/etc/mpd.conf` (generated; template in `deploy/mpd/mpd.conf.example`)

Generated sections: one `audio_output` block per entry in `outputs`, `resampler`, and the
paths. Everything else is static template text. A header comment records the generation time
and config hash; manual edits are preserved only inside a marked "user" region.

## 5. Security (NFR-5)

- Bind to the LAN; no port forwarding on the router; no TLS required for LAN control
  (optional TLS for PWA install, docs/06 §4.4).
- API token in `/etc/hifid/env` (0600), generated at install, shown once and as a QR.
- NetEase cookie file encrypted with a key from `/etc/machine-id` + secret; never logged.
- Upload path sanitisation and extension allow-list; tus temp dir on the data disk.
- `hifid` has no shell access to the system beyond the `mpd.conf` apply path.
- Debian unattended-upgrades for security updates; MPD/kernel updates are manual (a kernel
  update may change DSD quirks; the probe script re-runs after each upgrade).

## 6. Observability (NFR-7)

- Logs: `journalctl -u hifid -u mpd`; `hifid` logs JSON lines with request ids; MPD
  `log_level "default"`.
- `/api/v1/system/status` includes: MPD connection, active output + its `hw_params`
  snapshot, NetEase session validity, disk free, last playlist export, queue length, uptime.
- Optional Prometheus `/metrics` (xruns from MPD log grep, stream errors, upload bytes).
- Health check for the phone: the PWA polls status every 30 s when in foreground.

## 7. Backup and restore

| What | Where | How |
|---|---|---|
| Music (`/srv/music`) | owner's data | `deploy/scripts/backup.sh` → rsync to the PC or a second disk; never automatic |
| `/srv/data/hifid` (index, session, covers, per-DAC settings) | small | included in `backup.sh` |
| `/etc/hifid`, `/etc/mpd.conf`, udev rule | tiny | included |
| MPD DB | rebuildable | not backed up |

Restore = reinstall from the procedure in §2 and copy the three items back.

## 8. Maintenance routine

| Frequency | Task |
|---|---|
| Weekly (automatic) | playlist export refresh; NetEase session refresh; tus expiry cleanup |
| Monthly | `apt upgrade` (security); check `/system/status` for disk ≥ 90 % |
| After a kernel upgrade | re-run `probe-dac.sh`; compare with docs/04 §7 matrix |
| When NetEase breaks | update the Go adapter dependency; run the recorded-fixture tests; redeploy one binary |

## 9. Building `hifid` on the PC

```
cd server
GOOS=linux GOARCH=arm64  CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/hifid-arm64  ./cmd/hifid
GOOS=linux GOARCH=riscv64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/hifid-riscv64 ./cmd/hifid
```

Pure-Go dependencies only (SQLite via a pure-Go driver, no cgo) so that cross-compilation
from Windows needs nothing but the Go toolchain. The PWA is built once with the web bundler
into `web/dist` and embedded.

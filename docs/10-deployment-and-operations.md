# 10 · Deployment and Operations

Status: draft v0.1 · 2026-09-13 · addresses NFR-2, NFR-4, NFR-5, NFR-7, C-7

## 1. OS image

| Board | Image | Kernel | Why |
|---|---|---|---|
| Orange Pi Zero 3, H618, 2 GB (verified 2026-09-24) | Vendor Debian 12 (bookworm) server image, or Armbian Debian 13 minimal | vendor 6.1.31 (bench) or Armbian 6.18 | mpd 0.24 from bookworm-backports; the vendor image pins backports samba-libs, so the installer takes samba from backports too; native DSD for Comtrue dongles needs the installer's quirk on 6.1 |
| Any other Debian-based SBC (Raspberry Pi OS, Armbian, stock Debian) | its standard server image | 6.x | `deploy/scripts/install.sh` detects codename, architecture, Wi-Fi and DACs; nothing is board-specific |
| Orange Pi RV, JH7110 (riscv64 secondary target) | Vendor Debian server image for the RV (check its version in M0), or Debian 13 riscv64 installed with debian-installer plus the board DTB | vendor 6.6 series, or Debian 6.12 with the RV DTB, or self-built 6.19 (board DT merged upstream) | riscv64 is an official Debian 13 architecture; Wi-Fi via Broadcom AP6256 (`brcmfmac`, needs the firmware package); mpd 0.24.4 from `apt`. Note: the RV may need the power button pressed to boot; check the wiki for the always-on jumper before deploying headless |

Wi-Fi is the only link at the speaker's location: after first boot set the network with
`nmtui` (or `netplan`), pin a DHCP lease on the router, disable power save persistently, and
confirm the router does not enable AP/client isolation (it blocks mDNS and Samba).

## 2. Install procedure (implemented in `deploy/scripts/install.sh`; user-facing steps in docs/USER-MANUAL.md)

```
1. Flash image, boot, set hostname "hifi", join Wi-Fi (5 GHz), pin a DHCP lease on the router, disable Wi-Fi power save persistently.
2. apt install mpd mpc alsa-utils ffmpeg avahi-daemon udev inotify-tools samba f3        # both arches
   optional: upmpdcli (arm64: vendor repo; riscv64: build) ; mympd (arm64: OBS repo; not on 512 MB boards)
3. Flash drive: f3probe /dev/sdX (genuine capacity) ; mkfs.ext4 -m 1 -L hifi /dev/sdX1 ; units deploy/systemd/srv-hifi.mount + bind mounts (noatime, commit=60, nofail)
   mkdir -p /srv/music/{local,netease,playlists} /srv/data/{mpd,hifid,incoming} ; chown mpd:audio / hifid:audio
4. udev: copy deploy/udev/90-hifi-dac.rules ; udevadm control --reload ; replug dongles ; check /proc/asound/cards
5. MPD: copy deploy/mpd/mpd.conf.example -> /etc/mpd.conf (hifid regenerates it later) ; drop-in deploy/systemd/mpd.service.d/override.conf
6. hifid: build it (`cd server && go build -o dist/hifid ./cmd/hifid`), then `sudo deploy/scripts/install-hifid.sh --binary server/dist/hifid` — it creates the `hifid` system user, `/etc/hifid/config.yaml`, a generated token in `/etc/hifid/env` (0640 root:hifid), installs the binary, the polkit rule that lets the `hifid` user power the board off from the phone (`deploy/polkit/50-hifid-power.rules`, power-off only, no reboot), and enables `hifid.service`. Re-running keeps an existing config and token.
7. Verify: mpc outputs ; curl http://hifi.local/api/v1/system/status ; open http://hifi.local on the phone
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
| `wifi-watchdog.timer` + `.service` | every 2 min: ping the gateway 3×; on failure bounce `wlan0` (and `wpa_supplicant`/NetworkManager) and log it; counters feed `/system/status` (XR819 mitigation, R16) |
| `hifid-dac-hotplug@.service` | started by the udev rule `RUN+="/bin/systemctl start hifid-dac-hotplug@%k.service"`; posts `/api/v1/outputs/rescan` over the local unix socket `/run/hifid/api.sock` (no token) |
| `avahi-daemon` | `/etc/avahi/services/hifid.service` publishes `_hifid._tcp` and `_http._tcp` |

MPD socket: MPD listens on `/run/mpd/socket` (for `hifid`, fast) and on the LAN IP port 6600
(for M.A.L.P.). `hifid` listens on `0.0.0.0:80` by default but can be pinned to the LAN
interface; the stream proxy listens on `127.0.0.1` only.

## 4. Configuration files

### 4.1 `/etc/hifid/config.yaml` (shape)

```yaml
server_name: Living room
listen: 0.0.0.0:80
auth: { mode: admin }                     # none | admin | all ; token in /etc/hifid/env
paths:
  music: /srv/music
  data: /srv/data/hifid
  incoming: /srv/data/incoming
mpd: { socket: /run/mpd/socket, host: 127.0.0.1, port: 6600 }
netease:
  level_preference: [jymaster, hires, lossless, exhigh, higher, standard]   # SVIP account
  stream_mode: pipe                       # pipe (default, ADR-0008) | redirect
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
    volume_mode: hardware        # in-chip volume covers PCM and DSD; set fixed if DoP shows noise in M0
    max_rate: 768000
preferred_output: es9039a        # owner preference; DSD library is DSD256 or lower
discovery: { mdns: true, udp_beacon: true }
```

### 4.2 `/etc/mpd.conf` (generated by `gen-mpd-conf.sh` from `deploy/mpd/mpd.conf.template`)

`dac-setup.sh` records every USB DAC in `/etc/hifi/dacs.conf` (ALSA id, USB ids, rates,
bit depth, DSD state, mixer control); `gen-mpd-conf.sh` renders one `audio_output` block per
DAC with `dop`, `allowed_formats` and the mixer derived from that record, sizes the buffer
from the board's RAM, and preserves the marked user region. `deploy/mpd/mpd.conf.example` is
the file rendered on the verified board. Until `hifid` exists, re-run the two scripts and
`systemctl restart mpd` after plugging a new DAC.

## 5. Security (NFR-5)

- Bind to the LAN; no port forwarding on the router; no TLS required for LAN control
  (optional TLS for PWA install, docs/06 §4.4).
- API token in `/etc/hifid/env` (0600), generated at install, shown once and as a QR.
- NetEase cookie file: **not encrypted** (decision 2026-09-25: the key would live on the same card); it is 0600 under the `hifid` user in `/srv/data/hifid/netease/`, never logged, excluded from git.
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
| `/srv/data/hifid` (download index + list, session cookie), `/srv/data/mpd`, `/srv/music/playlists`, `/etc/hifid`, `/etc/mpd.conf`, Samba config | small | `sudo deploy/scripts/backup-hifid.sh [DEST]` → one 0600 tarball (keeps the newest 10); `--restore FILE` stops mpd+hifid, extracts, restarts |
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
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/hifid-armhf ./cmd/hifid   # Zero LTS
```

Pure-Go dependencies only (SQLite via a pure-Go driver, no cgo) so that cross-compilation
from Windows needs nothing but the Go toolchain. The PWA is built once with the web bundler
into `web/dist` and embedded.

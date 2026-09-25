#!/bin/bash
# install-hifid.sh — install the hifid control service as a managed systemd unit.
#
#   sudo ./install-hifid.sh --binary /path/to/hifid [--listen 0.0.0.0:80]
#                           [--auth admin|none|all] [--unit /path/to/hifid.service]
#
# Creates the hifid system user, /etc/hifid/{config.yaml,env}, installs the binary to
# /usr/local/bin and enables deploy/systemd/hifid.service. Safe to re-run: an existing
# config and token are kept, only the binary and unit are refreshed.
set -euo pipefail

BIN=""; LISTEN="0.0.0.0:80"; AUTH="admin"; UNIT=""
here=$(cd "$(dirname "$0")" && pwd)

while [ $# -gt 0 ]; do
  case "$1" in
    --binary) BIN=${2:-}; shift 2 ;;
    --listen) LISTEN=${2:-}; shift 2 ;;
    --auth)   AUTH=${2:-}; shift 2 ;;
    --unit)   UNIT=${2:-}; shift 2 ;;
    -h|--help) sed -n '2,10p' "$0"; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

[ "$(id -u)" -eq 0 ] || { echo "run this as root" >&2; exit 1; }
[ -n "$BIN" ] && [ -x "$BIN" ] || { echo "--binary <path to hifid> is required" >&2; exit 1; }
[ -n "$UNIT" ] || UNIT="$here/../systemd/hifid.service"
[ -f "$UNIT" ] || { echo "unit file not found: $UNIT (pass --unit)" >&2; exit 1; }

# 1. unprivileged service account; 'audio' gives access to the ALSA/MPD side
if ! id hifid >/dev/null 2>&1; then
  useradd --system --no-create-home --home-dir /nonexistent \
          --shell /usr/sbin/nologin hifid
  echo "created system user hifid"
fi
usermod -aG audio hifid

# 2. binary
install -m 0755 -o root -g root "$BIN" /usr/local/bin/hifid
echo "installed: $(/usr/local/bin/hifid --version)"

# 3. configuration (kept if it already exists, so upgrades never clobber settings)
install -d -m 0755 /etc/hifid
if [ ! -f /etc/hifid/config.yaml ]; then
  cat > /etc/hifid/config.yaml <<YAML
# /etc/hifid/config.yaml — see docs/10 section 4.1
server_name: HiFi Server
listen: ${LISTEN}
auth:
  mode: ${AUTH}            # none | admin | all ; token lives in /etc/hifid/env
paths:
  music: /srv/music
  data: /srv/data/hifid
  incoming: /srv/data/incoming
mpd:
  socket: /run/mpd/socket  # preferred; TCP below is the fallback
  host: 127.0.0.1
  port: 6600
# netease: filled in at M2. Note ADR-0008: stream_mode must be 'pipe'.
YAML
  chmod 0644 /etc/hifid/config.yaml
  echo "wrote /etc/hifid/config.yaml"
else
  echo "kept existing /etc/hifid/config.yaml"
fi

# 4. API token (NFR-5): generated once, readable only by root and hifid
if [ ! -f /etc/hifid/env ]; then
  if command -v openssl >/dev/null 2>&1; then
    token=$(openssl rand -hex 24)
  else
    token=$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n')
  fi
  printf 'HIFID_TOKEN=%s\n' "$token" > /etc/hifid/env
  echo "generated /etc/hifid/env"
else
  echo "kept existing /etc/hifid/env"
fi
chown root:hifid /etc/hifid/env
chmod 0640 /etc/hifid/env

# 5. state directories hifid must be able to write (setgid keeps the audio group)
for d in /srv/data/hifid /srv/data/incoming; do
  if [ -d "$d" ]; then chgrp audio "$d"; chmod 2775 "$d"; fi
done

# 6. unit + polkit rule (power-off button in the PWA)
install -m 0644 "$UNIT" /etc/systemd/system/hifid.service
if [ -d /etc/polkit-1/rules.d ] && [ -f "$here/../polkit/50-hifid-power.rules" ]; then
  install -m 0644 -o root -g root "$here/../polkit/50-hifid-power.rules" /etc/polkit-1/rules.d/50-hifid-power.rules
fi
systemctl daemon-reload
systemctl enable --now hifid.service
sleep 2

systemctl --no-pager --lines=0 status hifid.service || true
echo
echo "hifid is installed. Token for API clients:"
echo "  sudo sed -n 's/^HIFID_TOKEN=//p' /etc/hifid/env"
port=${LISTEN##*:}; if [ "$port" = 80 ]; then echo "Open the phone UI at: http://$(hostname).local/"; else echo "Open the phone UI at: http://$(hostname).local:$port/"; fi

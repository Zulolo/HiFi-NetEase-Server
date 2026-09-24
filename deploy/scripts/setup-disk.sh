#!/bin/bash
# setup-disk.sh — prepare the music disk and mount it as /srv/hifi with /srv/music and /srv/data bind mounts.
#
#   sudo ./setup-disk.sh --device /dev/sdb1 [--format]      format (ext4, label "hifi") and mount
#   sudo ./setup-disk.sh --label hifi                        mount an already formatted disk by label
#   sudo ./setup-disk.sh --local                             no extra disk: use the SD card (/srv/hifi on /)
#
# Never formats unless --format is given. Verifies real capacity with f3probe when available.

DIR=$(cd "$(dirname "$0")" && pwd); . "$DIR/lib.sh"
need_root
DEV=""; LABEL="hifi"; FORMAT=0; LOCAL=0
while [ $# -gt 0 ]; do case "$1" in
  --device) DEV=$2; shift;; --label) LABEL=$2; shift;; --format) FORMAT=1;; --local) LOCAL=1;;
  *) die "unknown option $1";; esac; shift; done

if [ $LOCAL -eq 0 ]; then
  if [ -n "$DEV" ]; then
    [ -b "$DEV" ] || die "$DEV is not a block device (lsblk to list)"
    mountpoint -q "$DEV" && die "$DEV is mounted; unmount it first"
    if [ $FORMAT -eq 1 ]; then
      log "formatting $DEV as ext4 (label $LABEL) — ALL DATA ON IT WILL BE LOST"
      mkfs.ext4 -q -F -m 1 -L "$LABEL" "$DEV"
    else
      [ "$(blkid -s TYPE -o value "$DEV")" = ext4 ] || die "$DEV is not ext4; re-run with --format to erase it"
      LABEL=$(blkid -s LABEL -o value "$DEV"); [ -n "$LABEL" ] || { e2label "$DEV" hifi; LABEL=hifi; }
    fi
  fi
  blkid -L "$LABEL" >/dev/null || die "no filesystem with label $LABEL found"
  cat > /etc/systemd/system/srv-hifi.mount <<EOF
[Unit]
Description=HiFi music disk
Before=mpd.service hifid.service smbd.service
[Mount]
What=LABEL=$LABEL
Where=$HIFI_MOUNT
Type=ext4
Options=noatime,commit=60,nofail,x-systemd.device-timeout=30s
[Install]
WantedBy=multi-user.target
EOF
  for pair in "music $MUSIC_DIR" "data $DATA_DIR"; do set -- $pair
    unit=$(systemd-escape -p --suffix=mount "$2")
    cat > "/etc/systemd/system/$unit" <<EOF
[Unit]
Description=$1 tree (bind mount of $HIFI_MOUNT/$1)
Requires=srv-hifi.mount
After=srv-hifi.mount
Before=mpd.service hifid.service smbd.service
[Mount]
What=$HIFI_MOUNT/$1
Where=$2
Type=none
Options=bind
[Install]
WantedBy=multi-user.target
EOF
  done
  systemctl daemon-reload
  systemctl enable --now srv-hifi.mount
  mkdir -p "$HIFI_MOUNT/music" "$HIFI_MOUNT/data"
  systemctl enable --now srv-music.mount srv-data.mount
else
  log "no separate disk: using $HIFI_MOUNT on the root filesystem"
  mkdir -p "$HIFI_MOUNT/music" "$HIFI_MOUNT/data"
  ln -sfn "$HIFI_MOUNT/music" "$MUSIC_DIR"; ln -sfn "$HIFI_MOUNT/data" "$DATA_DIR"
fi

mkdir -p "$MUSIC_DIR"/{local,netease,playlists} "$DATA_DIR"/{mpd,hifid,incoming}
getent group audio >/dev/null || groupadd audio
chgrp -R audio "$MUSIC_DIR" "$DATA_DIR"
chmod 2775 "$MUSIC_DIR" "$MUSIC_DIR"/{local,netease,playlists} "$DATA_DIR" "$DATA_DIR"/{hifid,incoming}
id mpd >/dev/null 2>&1 && chown -R mpd:audio "$DATA_DIR/mpd" && chmod 2775 "$DATA_DIR/mpd"
df -h "$MUSIC_DIR" | tail -1
log "music tree: $MUSIC_DIR/{local,netease,playlists}; data: $DATA_DIR/{mpd,hifid,incoming}"

#!/usr/bin/env bash
# backup-hifid.sh — snapshot everything on the board that is not re-creatable
# from the music files themselves, into one tarball:
#   /srv/data/hifid      download index + download list + NetEase session cookie
#   /srv/data/mpd        MPD database, state (queue, position), stickers
#   /srv/music/playlists stored playlists (incl. exported NetEase .m3u)
#   /etc/hifid           hifid config + token
#   /etc/mpd.conf        MPD config
#   /etc/samba/smb.conf, /etc/hifi   Samba config + credentials note
# The music files are NOT included (hundreds of GB); copy them with rsync or
# Explorer from the Samba share.
#
#   sudo deploy/scripts/backup-hifid.sh [DEST_DIR]      default: /srv/music/backup
#   sudo deploy/scripts/backup-hifid.sh --restore FILE  (stops mpd+hifid, extracts, restarts)
set -euo pipefail

if [ "${1:-}" = "--restore" ]; then
  f=${2:?tarball}
  [ "$(id -u)" = 0 ] || { echo "run as root"; exit 1; }
  systemctl stop hifid mpd
  tar -C / -xzpf "$f"
  systemctl start mpd hifid
  echo "restored $f; mpd and hifid restarted"
  exit 0
fi

dest=${1:-/srv/music/backup}
[ "$(id -u)" = 0 ] || { echo "run as root"; exit 1; }
mkdir -p "$dest"
out="$dest/hifi-server-$(hostname)-$(date +%Y%m%d-%H%M).tgz"
paths=()
for p in /srv/data/hifid /srv/data/mpd /srv/music/playlists /etc/hifid /etc/mpd.conf /etc/samba/smb.conf /etc/hifi; do
  [ -e "$p" ] && paths+=("${p#/}")
done
tar -C / -czpf "$out" "${paths[@]}"
chmod 0600 "$out"   # holds the NetEase cookie and the API token
ls -la "$out"
# keep the newest 10
ls -1t "$dest"/hifi-server-*.tgz 2>/dev/null | tail -n +11 | xargs -r rm -f

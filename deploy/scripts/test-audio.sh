#!/bin/bash
# test-audio.sh — generate test signals, play them through MPD and prove the path is bit-perfect.
#
#   sudo ./test-audio.sh              16/44.1 and 24/96 and 24/192 FLAC tones (3 s each), then report hw_params
#   sudo ./test-audio.sh --dsd FILE   also play a DSF/DFF file you provide and report native / DoP / PCM
#
# Reads /proc/asound/card*/pcm0p/sub0/hw_params while playing: the rate and format printed there
# must equal the file's rate and a lossless container (S24_3LE, S32_LE, DSD_U32_BE).

DIR=$(cd "$(dirname "$0")" && pwd); . "$DIR/lib.sh"
need_root
DSD=""; [ "${1:-}" = "--dsd" ] && DSD=$2
command -v ffmpeg >/dev/null || die "ffmpeg missing"; command -v mpc >/dev/null || die "mpc missing"
T="$MUSIC_DIR/local/_hifi-test"; mkdir -p "$T"
for spec in "44100 s16" "96000 s32" "192000 s32"; do set -- $spec
  f="$T/tone-${1}-${2}.flac"
  [ -f "$f" ] || ffmpeg -v error -y -f lavfi -i "sine=frequency=440:sample_rate=$1:duration=3" -ac 2 -sample_fmt "$2" -c:a flac "$f"
done
if [ -n "$DSD" ]; then [ -f "$DSD" ] || die "$DSD not found"; cp -n "$DSD" "$T/"; fi
chown -R mpd:audio "$T" 2>/dev/null; chmod -R g+rw "$T"
systemctl is-active --quiet mpd || systemctl start mpd
mpc -q update --wait local/_hifi-test
mpc -q clear; mpc -q add local/_hifi-test; mpc -q volume 100 2>/dev/null
mpc -q repeat off; mpc -q random off

report() {   # print what the active USB card actually received
  local hp; for hp in /proc/asound/card*/pcm0p/sub0/hw_params; do
    grep -q closed "$hp" && continue
    echo "  card $(basename "$(dirname "$(dirname "$(dirname "$hp")")")") : $(awk '/format|rate/{printf "%s ", $0}' "$hp")"
  done
}
n=$(mpc playlist | wc -l)
for i in $(seq 1 "$n"); do
  mpc -q play "$i"; sleep 1.5
  printf '%-40s -> ' "$(mpc current -f '%file%')"; report
  echo "     mpd says: $(exec 3<>/dev/tcp/127.0.0.1/6600; printf 'status\nclose\n' >&3; sed -n 's/^audio: //p' <&3)   (file format rate:bits:channels as decoded)"
  sleep 1
done
mpc -q stop
log "if a DSD file played as 'dsd' with format DSD_U32_BE = native; with S24_3LE/S32_LE at rate/16 = DoP; at 352800/176400 PCM = software conversion"

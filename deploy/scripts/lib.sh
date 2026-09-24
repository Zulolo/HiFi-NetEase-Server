#!/bin/bash
# lib.sh — shared helpers for the HiFi-NetEase-Server deploy scripts. Sourced, not executed.
# Targets any Debian-based distribution (Debian, Armbian, Raspberry Pi OS, Orange Pi vendor images).

set -o pipefail

HIFI_ETC=/etc/hifi
HIFI_DACS_CONF="$HIFI_ETC/dacs.conf"
HIFI_MOUNT=/srv/hifi
MUSIC_DIR=/srv/music
DATA_DIR=/srv/data

log()  { printf '\033[1;32m[hifi]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[hifi] WARNING:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m[hifi] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

need_root() { [ "$(id -u)" -eq 0 ] || die "run as root (sudo $0 ...)"; }

# Debian codename (bookworm, trixie, ...) or empty
distro_codename() { . /etc/os-release 2>/dev/null; echo "${VERSION_CODENAME:-}"; }

# kernel major.minor as an integer (6.1 -> 601, 6.18 -> 618)
kernel_ver() { uname -r | awk -F. '{printf "%d%02d", $1, $2}'; }

# ---- USB audio card enumeration ----------------------------------------------------------
# Prints one line per USB sound card: index|alsa_id|vid|pid|name
usb_audio_cards() {
  local c idx id usbid name
  for c in /proc/asound/card[0-9]*; do
    [ -r "$c/usbid" ] || continue
    idx=${c##*card}; id=$(cat "$c/id"); usbid=$(cat "$c/usbid")
    name=$(sed -n 's/^\s*'"$idx"' \[[^]]*\]: USB-Audio - //p' /proc/asound/cards | head -1)
    printf '%s|%s|%s|%s|%s\n' "$idx" "$id" "${usbid%%:*}" "${usbid##*:}" "${name:-USB Audio}"
  done
}

# Playback rates of a card (union over alt-settings), space separated ascending
card_rates() {
  awk '/^Playback:/{p=1} /^Capture:/{p=0} p && /Rates:/{gsub(/Rates: /,""); gsub(/,/,""); print}' \
    "/proc/asound/card$1/stream0" 2>/dev/null | tr ' ' '\n' | grep -E '^[0-9]+$' | sort -un | tr '\n' ' '
}
# Max playback bit depth seen (16/24/32)
card_max_bits() {
  awk '/^Playback:/{p=1} /^Capture:/{p=0} p && /Bits:/{print $2}' "/proc/asound/card$1/stream0" 2>/dev/null | sort -n | tail -1
}
# "native" if the kernel offers DSD_U32_BE, "raw" if a raw-DSD alt-setting exists but is not enabled (SPECIAL), "none" otherwise
card_dsd_state() {
  local f="/proc/asound/card$1/stream0"
  if grep -q 'DSD_U32_BE\|DSD_U32_LE' "$f" 2>/dev/null; then echo native
  elif grep -q 'DSD raw' "$f" 2>/dev/null; then echo raw
  else echo none; fi
}
# First playback mixer control name (PCM, Master, Speaker, ...) or empty
card_mixer_control() {
  amixer -c "$1" scontrols 2>/dev/null | sed -n "s/^Simple mixer control '\([^']*\)',.*/\1/p" | grep -Ei '^(PCM|Master|Speaker|Headphone|Playback)' | head -1
}
# Sanitize a string into an ALSA card id (max 15 chars, [A-Za-z0-9])
alsa_id_from() { echo "$1" | tr -cd 'A-Za-z0-9' | cut -c1-15; }

# Highest 44.1k-family PCM rate the card supports, capped at 352.8 kHz (docs/04 §6: keeps the
# DSD->PCM fallback affordable on small CPUs), used as the DSD->PCM fallback rate
fallback_rate() { local r best=44100; for r in $1; do case $r in 44100|88200|176400|352800) best=$r;; esac; done; echo "$best"; }

# True when snd-usb-audio is currently loaded with a non-zero quirk_flags (e.g. from a manual test)
quirk_currently_set() {
  local p=/sys/module/snd_usb_audio/parameters/quirk_flags
  [ -r "$p" ] && tr ',' '\n' < "$p" | grep -qvE '^(0|)$'
}

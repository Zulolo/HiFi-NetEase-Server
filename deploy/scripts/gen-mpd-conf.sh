#!/bin/bash
# gen-mpd-conf.sh — render /etc/mpd.conf from deploy/mpd/mpd.conf.template and /etc/hifi/dacs.conf.
#
#   sudo ./gen-mpd-conf.sh [--buffer-kb N] [--dsd-mode auto|native|dop|pcm] [--volume hardware|none]
#
# One audio_output block per DAC recorded by dac-setup.sh; the first is enabled. Per DAC:
#   dop  = "yes" when the kernel does not offer native DSD but the DAC accepts >= 176.4 kHz PCM
#   allowed_formats = the PCM rates the DAC reports, DoP entries for the DSD rates it can carry,
#                     and a 44.1k-family fallback rate for DSD that must be converted to PCM
#   mixer = hardware (USB feature unit) when a playback control exists, else none (fixed 100 %)

DIR=$(cd "$(dirname "$0")" && pwd); . "$DIR/lib.sh"
need_root
TEMPLATE="$DIR/../mpd/mpd.conf.template"; [ -r "$TEMPLATE" ] || die "template not found: $TEMPLATE"
[ -r "$HIFI_DACS_CONF" ] || die "run dac-setup.sh first ($HIFI_DACS_CONF missing)"

BUFFER_KB=$(( $(awk '/MemTotal/{print $2}' /proc/meminfo) > 1200000 ? 16384 : 8192 ))
DSD_MODE=auto; VOLUME=auto
while [ $# -gt 0 ]; do case "$1" in
  --buffer-kb) BUFFER_KB=$2; shift;; --dsd-mode) DSD_MODE=$2; shift;; --volume) VOLUME=$2; shift;;
  *) die "unknown option $1";; esac; shift; done

render_output() {   # $1=section name in dacs.conf
  local sec=$1 k v alsa name rates max dsd mixer enabled dop="no" allowed="" fb
  while IFS='=' read -r k v; do
    case $k in alsa_id) alsa=$v;; name) name=$v;; rates) rates=$v;; max_rate) max=$v;; dsd) dsd=$v;; mixer_control) mixer=$v;; enabled) enabled=$v;; esac
  done < <(awk -v s="[$sec]" '$0==s{f=1;next} /^\[/{f=0} f' "$HIFI_DACS_CONF")
  for r in $rates; do allowed="$allowed $r:*:*"; done
  fb=$(fallback_rate "$rates")
  case $DSD_MODE in
    pcm) ;;
    *)
      if [ "$dsd" = native ] && [ "$DSD_MODE" != dop ]; then
        allowed="$allowed *:dsd:*"
      else
        # DoP needs a PCM container at DSD rate / 16: DSD64 -> 176.4k, DSD128 -> 352.8k, DSD256 -> 705.6k
        for pair in "dsd64 176400" "dsd128 352800" "dsd256 705600" "dsd512 1411200"; do
          set -- $pair; echo " $rates " | grep -q " $2 " && { allowed="$allowed $1:*=dop"; dop=yes; }
        done
      fi;;
  esac
  allowed="$allowed $fb:24:2"
  cat <<EOB

# $name  (usb $(awk -v s="[$sec]" '$0==s{f=1;next} /^\[/{f=0} f && /^usb=/{sub(/usb=/,"");print}' "$HIFI_DACS_CONF"))
audio_output {
    type               "alsa"
    name               "$name"
    device             "hw:CARD=$alsa,DEV=0"
    enabled            "$enabled"
    auto_resample      "no"
    auto_format        "no"
    auto_channels      "no"
    buffer_time        "500000"
    period_time        "100000"
    dop                "$dop"
    allowed_formats    "$(echo $allowed)"
EOB
  if { [ "$VOLUME" = auto ] && [ -n "$mixer" ]; } || [ "$VOLUME" = hardware ]; then
    cat <<EOB
    mixer_type         "hardware"
    mixer_device       "hw:CARD=$alsa"
    mixer_control      "${mixer:-PCM}"
EOB
  else
    echo '    mixer_type         "none"'
  fi
  echo "}"
}

OUTPUTS=""
for sec in $(sed -n 's/^\[\(dac[0-9]*\)\]$/\1/p' "$HIFI_DACS_CONF"); do OUTPUTS="$OUTPUTS$(render_output "$sec")"$'\n'; done

USER_REGION=""
if [ -r /etc/mpd.conf ]; then
  USER_REGION=$(awk '/^# ==== USER REGION/{f=1;next} /^# ==== END USER REGION/{f=0} f' /etc/mpd.conf)
fi
[ -r /etc/mpd.conf ] && cp /etc/mpd.conf "/etc/mpd.conf.bak.$(date +%Y%m%d%H%M%S)"

awk -v outputs="$OUTPUTS" -v buffer="$BUFFER_KB" -v userregion="$USER_REGION" -v stamp="$(date -Is)" '
  /@@GENERATED_OUTPUTS@@/ { print outputs; next }
  /@@BUFFER_KB@@/ { gsub(/@@BUFFER_KB@@/, buffer) }
  /@@STAMP@@/ { gsub(/@@STAMP@@/, stamp) }
  /@@USER_REGION@@/ { print userregion; next }
  { print }' "$TEMPLATE" > /etc/mpd.conf
log "wrote /etc/mpd.conf (buffer ${BUFFER_KB} kB, dsd-mode $DSD_MODE); previous config backed up"
mpd --no-daemon --stdout --verbose /etc/mpd.conf 2>&1 >/dev/null </dev/null & sleep 1; kill $! 2>/dev/null   # quick syntax check
grep -E '^\s*(name|device|dop|allowed_formats|mixer_type)' /etc/mpd.conf

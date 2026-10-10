#!/bin/bash
# wifi-prefer.sh — return to the preferred Wi-Fi network after a fallback.
#
# NetworkManager picks the profile with the highest autoconnect-priority when it
# connects, but once it has fallen back to a lower one (the 5 GHz AP blinked, so it
# joined the 2.4 GHz network) it stays there for as long as that link is up. This
# script, run from wifi-prefer.timer, moves the board back when the link is idle.
#
#   - does nothing when the active profile already has the highest priority
#   - does nothing while traffic flows (> 100 kB/s) or MPD plays a network stream
#   - tries the preferred profile, checks the gateway, falls back if that fails
#   - after a failed attempt waits 30 min before trying again
set -u
IFACE="${1:-wlan0}"
STATE=/run/wifi-prefer.last-fail
BUSY_KBPS=${BUSY_KBPS:-100}   # skip the switch while more than this flows (kB/s)
log(){ logger -t wifi-prefer -- "$*"; echo "wifi-prefer: $*"; }

active=$(nmcli -t -f NAME,DEVICE connection show --active 2>/dev/null | awk -F: -v i="$IFACE" '$NF==i{sub(":"i"$",""); print; exit}')
[ -n "$active" ] || exit 0
best=$(nmcli -t -f AUTOCONNECT-PRIORITY,AUTOCONNECT,TYPE,NAME connection show 2>/dev/null \
       | awk -F: '$3=="802-11-wireless" && $2=="yes"{p=$1; sub(/^[^:]*:[^:]*:[^:]*:/,""); print p"\t"$0}' | sort -t "$(printf '\t')" -k1,1nr | head -1)
best_prio=${best%%$'\t'*}; best_name=${best#*$'\t'}
act_prio=$(nmcli -g connection.autoconnect-priority connection show "$active" 2>/dev/null)
[ -n "$best_name" ] && [ "$best_name" != "$active" ] && [ "${best_prio:-0}" -gt "${act_prio:-0}" ] || exit 0

if [ -f "$STATE" ] && [ $(( $(date +%s) - $(cat "$STATE") )) -lt 1800 ]; then exit 0; fi

# idle check
r1=$(cat /sys/class/net/$IFACE/statistics/rx_bytes); t1=$(cat /sys/class/net/$IFACE/statistics/tx_bytes); sleep 3
r2=$(cat /sys/class/net/$IFACE/statistics/rx_bytes); t2=$(cat /sys/class/net/$IFACE/statistics/tx_bytes)
rate=$(( (r2 - r1 + t2 - t1) / 3 / 1024 ))
if [ "$rate" -gt "$BUSY_KBPS" ]; then log "on '$active', prefer '$best_name', but link is busy (${rate} kB/s); later"; exit 0; fi
if command -v mpc >/dev/null && mpc status 2>/dev/null | grep -q '^\[playing\]' && mpc -f '%file%' current 2>/dev/null | grep -q '^http'; then
  log "on '$active', prefer '$best_name', but a stream is playing; later"; exit 0
fi

gw_ok(){ local gw; for _ in 1 2 3 4 5 6 7 8; do gw=$(ip -4 route show default | awk -v i="$IFACE" '$0 ~ ("dev "i){print $3; exit}'); [ -n "$gw" ] && ping -c 2 -W 2 -I "$IFACE" "$gw" >/dev/null 2>&1 && return 0; sleep 3; done; return 1; }

log "on '$active' (priority ${act_prio:-0}); switching to preferred '$best_name' (priority $best_prio)"
if nmcli -w 45 connection up "$best_name" >/dev/null 2>&1 && gw_ok; then
  rm -f "$STATE"; log "now on '$best_name'"
else
  date +%s > "$STATE"
  log "'$best_name' not usable; back to '$active' (next attempt in 30 min)"
  nmcli -w 45 connection up "$active" >/dev/null 2>&1
fi

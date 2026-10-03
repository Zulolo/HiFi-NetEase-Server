#!/bin/bash
# usb-hs-watch.sh — USB speed watchdog for the music disk (runs from usb-hs-watch.timer).
#
# If the mass-storage link has dropped to full speed (12 Mbit/s), re-enumerate it at
# high speed by stopping the services that use the disk, unmounting it, running
# usb-hs-guard.sh (EHCI/OHCI re-bind), remounting and restarting. That is a short
# interruption, so it only happens when the board is idle: nothing playing, no
# download in flight, no file open on the share. Otherwise it logs and waits for the
# next run; hifid shows the degraded link in the web app header meanwhile.
#
# Each run also asks the kernel to compact memory: the Wi-Fi driver on some boards
# (Unisoc UWE5622) needs large contiguous atomic buffers and fails them on a
# fragmented page cache ("page allocation failure: order:6" in dmesg).
set -u
GUARD=/usr/local/sbin/usb-hs-guard.sh
SERVICES="hifid mympd smbd nmbd mpd"
MOUNTS="srv-music.mount srv-data.mount srv-hifi.mount"
log(){ echo "usb-hs-watch: $*"; }

storage_speed(){
  local d
  for d in /sys/bus/usb/devices/[0-9]*-[0-9]*; do
    [ -f "$d/bDeviceClass" ] || continue
    grep -qs '^08$' "$d"/*/bInterfaceClass 2>/dev/null && { cat "$d/speed"; return; }
  done
  echo 0
}

echo 1 > /proc/sys/vm/compact_memory 2>/dev/null || true

speed=$(storage_speed)
if [ "$speed" -eq 0 ] || [ "$speed" -ge 480 ]; then exit 0; fi

busy=""
mpc -q status >/dev/null 2>&1 && mpc status 2>/dev/null | grep -q '^\[playing\]' && busy="playing"
curl -s --max-time 3 http://127.0.0.1/api/v1/netease/downloads | grep -q '"current":"[^"]' && busy="${busy:+$busy, }download in flight"
[ "$(fuser -m /srv/music 2>/dev/null | wc -w)" -gt 0 ] && busy="${busy:+$busy, }files open on the share"
if [ -n "$busy" ]; then log "music disk at ${speed} Mbit/s but the board is busy ($busy); deferring"; exit 0; fi

log "music disk at ${speed} Mbit/s and the board is idle: re-enumerating"
systemctl stop $SERVICES 2>/dev/null; sync
systemctl stop $MOUNTS
"$GUARD"
systemctl start $MOUNTS
systemctl start $SERVICES 2>/dev/null
log "done: music disk now at $(storage_speed) Mbit/s; services $(systemctl is-active mpd hifid smbd | tr '\n' ' ')"

#!/bin/sh
# probe-dac.sh — read-only dump of every USB audio device for docs/04 §7 (milestone M0).
# Usage: sudo sh deploy/scripts/probe-dac.sh > probe-$(date +%F).txt
# Prints: kernel, snd-usb-audio quirk flags, lsusb, ALSA cards, per-card stream0 (formats, rates,
# DSD_U32_BE = native DSD offered), mixer controls, and the live hw_params if something is playing.

set -u
echo "== kernel";            uname -r
echo "== snd-usb-audio quirk_flags parameter"
cat /sys/module/snd_usb_audio/parameters/quirk_flags 2>/dev/null || echo "(module not loaded or parameter absent)"
echo "== lsusb";             lsusb 2>/dev/null || echo "(install usbutils)"
echo "== /proc/asound/cards"; cat /proc/asound/cards

for card in /proc/asound/card[0-9]*; do
  n=${card##*card}
  id=$(cat "$card/id" 2>/dev/null)
  usbid=$(cat "$card/usbid" 2>/dev/null || echo "not-usb")
  [ "$usbid" = "not-usb" ] && continue
  echo
  echo "==== card $n  id=$id  usb=$usbid"
  echo "-- stream0 (look for 'Format: DSD_U32_BE' = native DSD; 'SPECIAL' = raw alt-setting without quirk)"
  cat "$card/stream0" 2>/dev/null || echo "(no stream0)"
  echo "-- mixer controls (hardware volume candidates)"
  amixer -c "$n" scontrols 2>/dev/null || echo "(alsa-utils missing)"
  echo "-- current hw_params (only while playing)"
  for hp in "$card"/pcm*p/sub*/hw_params; do
    [ -r "$hp" ] && { echo "$hp:"; cat "$hp"; }
  done
done

echo
echo "== hint: native DSD missing but the datasheet says DSD?  kernel >= 6.18:"
echo "   echo 'options snd-usb-audio quirk_flags=VVVV:PPPP:dsd_raw' | sudo tee /etc/modprobe.d/hifi-dsd.conf"
echo "   older kernels: quirk_flags=0x8000 (bit 15, probe order).  See docs/04 §4."

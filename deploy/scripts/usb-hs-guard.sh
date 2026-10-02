#!/bin/bash
# usb-hs-guard.sh — make sure the USB music disk runs at high speed before it is mounted.
#
# Some card readers fail the USB 2.0 high-speed handshake when the board powers up
# (seen with a Genesys 05e3:0764 reader on an Orange Pi Zero 3: it enumerates on the
# OHCI companion controller at 12 Mbit/s, "not running at top speed" in dmesg, and
# every write to the disk crawls at ~1 MB/s). Power-cycling the port does not help;
# unbinding and re-binding the EHCI/OHCI pair of that port does: the device then
# re-enumerates with EHCI owning the port and negotiates 480 Mbit/s.
#
# Runs as a oneshot service before local-fs.target (see deploy/systemd/usb-hs-guard.service).
# Exit status is always 0: a disk at full speed must not block the boot.
set -u
WAIT=${WAIT:-20}          # seconds to wait for a mass-storage device to appear
TRIES=${TRIES:-3}

log(){ echo "usb-hs-guard: $*"; }

# first mass-storage device: its sysfs node and negotiated speed
find_storage(){
  local d
  for d in /sys/bus/usb/devices/[0-9]*-[0-9]*; do
    [ -f "$d/bDeviceClass" ] || continue
    if grep -qs '^08$' "$d"/*/bInterfaceClass 2>/dev/null; then echo "$d"; return 0; fi
  done
  return 1
}

for _ in $(seq 1 "$WAIT"); do dev=$(find_storage) && break; sleep 1; done
if [ -z "${dev:-}" ]; then log "no USB mass-storage device found within ${WAIT}s"; exit 0; fi

speed=$(cat "$dev/speed")
if [ "$speed" -ge 480 ]; then log "$(cat "$dev/product" 2>/dev/null) at ${speed} Mbit/s, nothing to do"; exit 0; fi

# controller that owns the device now (OHCI/UHCI companion), and its EHCI partner.
# Allwinner: ehci at X000.usb, ohci at X400.usb. Other SoCs: fall back to any ehci.
ctrl=$(basename "$(readlink -f "$dev/../..")")
ehci=${ctrl/400.usb/000.usb}
[ -e "/sys/bus/platform/drivers/ehci-platform/$ehci" ] || ehci=$(ls /sys/bus/platform/drivers/ehci-platform/ 2>/dev/null | grep '\.usb$' | head -1)
cdrv=$(basename "$(readlink -f "/sys/bus/platform/devices/$ctrl/driver")")
if [ -z "$ehci" ] || [ -z "$cdrv" ]; then log "cannot map $ctrl to a driver pair; leaving it at ${speed} Mbit/s"; exit 0; fi
log "$(cat "$dev/product" 2>/dev/null) is at ${speed} Mbit/s on $ctrl ($cdrv); re-binding $ehci + $ctrl"

for try in $(seq 1 "$TRIES"); do
  echo "$ctrl" > "/sys/bus/platform/drivers/$cdrv/unbind" 2>/dev/null; sleep 1
  echo "$ehci" > /sys/bus/platform/drivers/ehci-platform/unbind 2>/dev/null; sleep 3
  echo "$ehci" > /sys/bus/platform/drivers/ehci-platform/bind 2>/dev/null; sleep 2
  echo "$ctrl" > "/sys/bus/platform/drivers/$cdrv/bind" 2>/dev/null
  for _ in $(seq 1 15); do sleep 1; dev=$(find_storage) && break; done
  speed=$(cat "${dev:-/dev/null}/speed" 2>/dev/null || echo 0)
  log "try $try: ${speed} Mbit/s"
  [ "$speed" -ge 480 ] && break
done
# give udev time to create the block device and settle before the mount units run
udevadm settle --timeout=15 2>/dev/null || true
exit 0

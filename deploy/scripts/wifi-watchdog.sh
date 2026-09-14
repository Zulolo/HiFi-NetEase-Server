#!/bin/sh
# wifi-watchdog.sh [iface] — bounce the Wi-Fi interface when the default gateway is unreachable.
# Installed to /usr/local/bin and run by wifi-watchdog.timer (deploy/systemd). Logs to journald.
# Escalation: 1st failure -> restart the interface; 3 consecutive failures -> restart the
# network manager service; nothing else (no reboot) so playback of local files is never interrupted.

IFACE=${1:-wlan0}
STATE=/run/wifi-watchdog.fails
GW=$(ip -4 route show default dev "$IFACE" 2>/dev/null | awk '{print $3; exit}')

if [ -z "$GW" ]; then
  logger -t wifi-watchdog "no default route on $IFACE"
  fails=1
elif ping -c 3 -W 2 -I "$IFACE" "$GW" >/dev/null 2>&1; then
  echo 0 > "$STATE"
  exit 0
else
  fails=$(( $(cat "$STATE" 2>/dev/null || echo 0) + 1 ))
fi

echo "$fails" > "$STATE"
logger -t wifi-watchdog "gateway ${GW:-unknown} unreachable on $IFACE (consecutive failures: $fails)"

# keep power save off after every bounce (the driver may re-enable it)
ip link set "$IFACE" down
sleep 2
ip link set "$IFACE" up
iw dev "$IFACE" set power_save off 2>/dev/null || true

if [ "$fails" -ge 3 ]; then
  if systemctl is-active --quiet NetworkManager; then
    systemctl restart NetworkManager
  elif systemctl is-active --quiet "wpa_supplicant@$IFACE"; then
    systemctl restart "wpa_supplicant@$IFACE"
  elif systemctl is-active --quiet wpa_supplicant; then
    systemctl restart wpa_supplicant
  fi
  logger -t wifi-watchdog "restarted the network manager after $fails failures"
fi

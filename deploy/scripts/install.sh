#!/bin/bash
# install.sh — one-shot setup of the HiFi-NetEase-Server playback layer on any Debian-based SBC.
#
#   sudo ./install.sh [--disk /dev/sdX1 [--format] | --disk-label hifi | --no-disk]
#                     [--no-samba] [--samba-user NAME] [--no-wifi-watchdog] [--no-native-dsd]
#                     [--dac-name VID:PID=ALSAID]...
#
# What it does (each step is a separate script you can re-run alone):
#   1. packages: mpd (0.24 from backports on Debian 12), mpc, alsa-utils, ffmpeg, avahi, samba, inotify-tools, f3
#   2. setup-disk.sh   music disk at /srv/hifi (+ /srv/music, /srv/data bind mounts) or local folders
#   3. dac-setup.sh    stable ALSA names, native-DSD quirk when possible, capability record
#   4. gen-mpd-conf.sh /etc/mpd.conf with one output per DAC, bit-perfect settings
#   5. samba share on /srv/music/local, avahi advertisement, optional Wi-Fi watchdog
#   6. enable services and print how to connect from the phone
#
# Tested: Orange Pi Zero 3 (Debian 12, kernel 6.1) with a Comtrue/ES9039 dongle. Should work on
# Raspberry Pi OS, Armbian and stock Debian 12/13 on arm64, armhf and riscv64.

DIR=$(cd "$(dirname "$0")" && pwd); . "$DIR/lib.sh"
need_root
DISK=""; DISK_LABEL=""; FORMAT=""; NO_DISK=0; SAMBA=1; SAMBA_USER=hifi; WATCHDOG=auto; NATIVE_DSD="--try-native-dsd"; DAC_NAMES=()
while [ $# -gt 0 ]; do case "$1" in
  --disk) DISK=$2; shift;; --disk-label) DISK_LABEL=$2; shift;; --format) FORMAT=--format;; --no-disk) NO_DISK=1;;
  --no-samba) SAMBA=0;; --samba-user) SAMBA_USER=$2; shift;; --no-wifi-watchdog) WATCHDOG=0;;
  --no-native-dsd) NATIVE_DSD="";; --dac-name) DAC_NAMES+=(--name "$2"); shift;;
  -h|--help) sed -n '2,20p' "$0"; exit 0;; *) die "unknown option $1 (try --help)";; esac; shift; done
[ -n "$DISK$DISK_LABEL" ] || [ $NO_DISK -eq 1 ] || die "choose --disk /dev/sdX1 [--format], --disk-label LABEL, or --no-disk"

# ---- 1. packages ---------------------------------------------------------------------------------
export DEBIAN_FRONTEND=noninteractive
APT="apt-get -y -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold"
CODENAME=$(distro_codename)
log "Debian codename: ${CODENAME:-unknown}, arch: $(dpkg --print-architecture), kernel: $(uname -r)"
apt-get update -q
if [ "$CODENAME" = bookworm ]; then
  grep -rqs "bookworm-backports" /etc/apt/sources.list /etc/apt/sources.list.d/ || \
    echo "deb http://deb.debian.org/debian bookworm-backports main" > /etc/apt/sources.list.d/backports.list && apt-get update -q
  $APT install -t bookworm-backports mpd mpc
else
  $APT install mpd mpc
fi
$APT install alsa-utils ffmpeg avahi-daemon inotify-tools f3 usbutils curl
if [ $SAMBA -eq 1 ]; then
  # some vendor images already carry backports samba-libs; then plain samba conflicts -> take all of samba from backports
  $APT install samba || { [ "$CODENAME" = bookworm ] && $APT install -t bookworm-backports samba; } || die "samba installation failed"
fi
systemctl stop mpd 2>/dev/null; systemctl disable mpd.socket 2>/dev/null
log "mpd $(mpd --version | head -1 | awk '{print $NF}') installed"

# ---- 2. disk ----------------------------------------------------------------------------------------
if [ $NO_DISK -eq 1 ]; then "$DIR/setup-disk.sh" --local
elif [ -n "$DISK" ]; then "$DIR/setup-disk.sh" --device "$DISK" $FORMAT
else "$DIR/setup-disk.sh" --label "$DISK_LABEL"; fi

# ---- 3 + 4. DACs and mpd.conf ----------------------------------------------------------------------
"$DIR/dac-setup.sh" $NATIVE_DSD "${DAC_NAMES[@]}"
"$DIR/gen-mpd-conf.sh"
mkdir -p /etc/systemd/system/mpd.service.d
cp "$DIR/../systemd/mpd.service.d-override.conf" /etc/systemd/system/mpd.service.d/override.conf
cp "$DIR/../systemd/hifid-dac-hotplug@.service" /etc/systemd/system/ 2>/dev/null || true

# ---- 5. samba, avahi, watchdog -----------------------------------------------------------------------
if [ $SAMBA -eq 1 ]; then
  id "$SAMBA_USER" >/dev/null 2>&1 || useradd -r -M -s /usr/sbin/nologin -G audio "$SAMBA_USER"
  usermod -aG audio "$SAMBA_USER"
  if ! grep -q '^\[music\]' /etc/samba/smb.conf; then cat >> /etc/samba/smb.conf <<EOF

[music]
   comment = HiFi music library (drop files here)
   path = $MUSIC_DIR/local
   valid users = $SAMBA_USER
   writable = yes
   create mask = 0664
   directory mask = 2775
   force group = audio
EOF
  fi
  if ! pdbedit -L 2>/dev/null | grep -q "^$SAMBA_USER:"; then
    SMBPW=$(tr -dc 'A-Za-z0-9' </dev/urandom | head -c 12)
    printf '%s\n%s\n' "$SMBPW" "$SMBPW" | smbpasswd -s -a "$SAMBA_USER" >/dev/null
    mkdir -p "$HIFI_ETC"; echo "samba_user=$SAMBA_USER" > "$HIFI_ETC/samba.txt"; echo "samba_password=$SMBPW" >> "$HIFI_ETC/samba.txt"; chmod 600 "$HIFI_ETC/samba.txt"
  fi
  systemctl enable --now smbd >/dev/null 2>&1
fi
cp "$DIR/../systemd/avahi-hifid.service.xml" /etc/avahi/services/hifid.service 2>/dev/null || true
systemctl enable --now avahi-daemon >/dev/null 2>&1

WIFI_IF=$(ip -4 route show default 2>/dev/null | awk '/dev wl/{print $5; exit}')
if [ "$WATCHDOG" != 0 ] && [ -n "$WIFI_IF" ]; then
  install -m 755 "$DIR/wifi-watchdog.sh" /usr/local/bin/wifi-watchdog.sh
  sed "s/wlan0/$WIFI_IF/" "$DIR/../systemd/wifi-watchdog.service" > /etc/systemd/system/wifi-watchdog.service
  cp "$DIR/../systemd/wifi-watchdog.timer" /etc/systemd/system/
  iw dev "$WIFI_IF" set power_save off 2>/dev/null || true
  systemctl daemon-reload; systemctl enable --now wifi-watchdog.timer >/dev/null
  log "Wi-Fi watchdog enabled on $WIFI_IF (default route is wireless)"
fi

# ---- 6. services + summary --------------------------------------------------------------------------
systemctl daemon-reload
systemctl enable --now mpd >/dev/null 2>&1; sleep 2
IP=$(hostname -I | awk '{print $1}')
echo; log "DONE. Summary:"
echo "  MPD $(mpd --version | head -1 | awk '{print $NF}'): $(systemctl is-active mpd), outputs:"; mpc outputs | sed 's/^/     /'
[ $SAMBA -eq 1 ] && echo "  Samba share: \\\\$IP\\music  (user/password in $HIFI_ETC/samba.txt)"
echo "  Phone: install M.A.L.P. (F-Droid/Play), add server $IP port 6600, or use hostname $(hostname).local"
echo "  Test the audio path: sudo $DIR/test-audio.sh [--dsd /path/to/file.dsf]"
echo "  DAC details: $HIFI_DACS_CONF ; regenerate config after plugging a new DAC: dac-setup.sh && gen-mpd-conf.sh && systemctl restart mpd"

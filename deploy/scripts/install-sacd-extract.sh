#!/usr/bin/env bash
# install-sacd-extract.sh — build and install sacd_extract, the tool hifid uses to turn
# SACD disc images (.iso) into DSF files (Library tab → "Extract to DSF").
# MPD cannot read inside an SACD ISO; DSF plays as native DSD / DoP like any other file.
#   sudo deploy/scripts/install-sacd-extract.sh
set -euo pipefail
[ "$(id -u)" = 0 ] || { echo "run as root"; exit 1; }
apt-get install -y -q git cmake build-essential libxml2-dev zlib1g-dev
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
git clone -q --depth 1 https://github.com/EuFlo/sacd-ripper.git "$tmp/sacd-ripper"
cd "$tmp/sacd-ripper/tools/sacd_extract"
cmake . >/dev/null
make -j"$(nproc)" >/dev/null
install -m 0755 sacd_extract /usr/local/bin/sacd_extract
/usr/local/bin/sacd_extract --version 2>&1 | head -1 || true
echo "installed /usr/local/bin/sacd_extract"

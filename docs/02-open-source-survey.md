# 02 · Open-Source Resource Survey

Status: v0.1 · surveyed 2026-09-13 · web sources verified on that date unless marked UNVERIFIED

Legend: A64 = linux/arm64, RV64 = linux/riscv64. "Verdict" states the role in this project.

## 1. NetEase Cloud Music access

### 1.1 Official client

| Item | Finding | Verdict |
|---|---|---|
| Linux client | Last release 1.2.1 (2019-04), x86_64 only (Qt5 + libvlc, GUI). Download link removed from the official site in 2019. No 2.x/3.x Linux build found; a UOS/deepin arm64 build could not be confirmed (UNVERIFIED). arm64 users try box64 emulation and fail on avahi. | Not usable: closed, x86_64, GUI-only, no remote API. Sources: [AUR](https://aur.archlinux.org/cgit/aur.git/plain/PKGBUILD?h=netease-cloud-music), [box64 #839](https://github.com/ptitSeb/box64/issues/839) |
| Android app DLNA | "投射到设备"/DLNA moved to the player-page cast button after 8.6.65; 2024 report of the app pushing a 96 kHz stream to MPD + upmpdcli. | Optional side path (docs/06 §6). Sources: [MPD #2037](https://github.com/MusicPlayerDaemon/MPD/issues/2037), [Zhihu](https://www.zhihu.com/question/525032938) |

### 1.2 API implementations

| Project | Lang / Licence | Status (2026-09) | A64 / RV64 | Login | Notes | Verdict |
|---|---|---|---|---|---|---|
| [chaunsin/netease-cloud-music](https://github.com/chaunsin/netease-cloud-music) (`ncmctl`) | Go / MIT | v0.8.0, 2026-09-05, active | release binaries for both, plus loong64/mips | QR, cookie, CookieCloud (SMS/password flagged risky) | weapi (most complete), eapi, xeapi; all levels incl. jymaster; cloud-disk upload; NCM decrypt; scrobble | **Primary base for the `hifid` adapter** |
| [go-musicfox/netease-music](https://github.com/go-musicfox/netease-music) | Go / MIT | pushed 2026-04 | pure Go | QR, phone, email, cookie | 160+ endpoints, cookiejar sessions; used by go-musicfox | Fallback library |
| [NeteaseCloudMusicApiEnhanced/api-enhanced](https://github.com/NeteaseCloudMusicApiEnhanced/api-enhanced) | Node ≥ 22 / MIT | v4.40.1 2026-08, active | Docker amd64+arm64 | QR, SMS, email, cookie | Successor of Binaryify's project (taken down after a 2024 legal notice); docs at docs-neteasecloudmusicapi.focalors.ltd | **Reference specification** of endpoints; not a runtime (RAM) |
| [Binaryify/NeteaseCloudMusicApi](https://github.com/Binaryify/NeteaseCloudMusicApi) | Node / MIT | archived 2024-04, npm frozen at 4.32.0 | | | | Historical |
| [XiaoMengXinX/Music163Api-Go](https://github.com/XiaoMengXinX/Music163Api-Go) | Go / GPL-3 | pushed 2025-07 | pure Go | QR, cookie | song URL incl. FLAC; no cloud disk | No (GPL, less active) |
| [mos9527/pyncm](https://libraries.io/pypi/pyncm) | Python / Apache-2 | **gone** (GitHub and PyPI 404 since 2026-05) | | | | No |
| [darknessomi/musicbox](https://github.com/darknessomi/musicbox) | Python / MIT | 0.5.3, 2026-08, active | pure Python | QR only | TUI, mpg123/mpv, daemon control, lossless + cloud disk | No (Python + external players) |
| [gmg137/netease-cloud-music-api](https://github.com/gmg137/netease-cloud-music-api) | Rust | pushed 2026-09 | | | powers netease-cloud-music-gtk | No (Rust toolchain not otherwise needed) |
| [SPlayer-Dev/ncm-api-rs](https://github.com/SPlayer-Dev/ncm-api-rs) | Rust / WTFPL | new (2026-03) | x64/A64 | QR, phone, email | lib + Axum server, ~5 MB RSS | Watch |

### 1.3 Clients and bridges

| Project | Lang / Licence | Status | Notes | Verdict |
|---|---|---|---|---|
| [go-musicfox](https://github.com/go-musicfox/go-musicfox) | Go / GPL-3 | v5.1.0 2026-08, active | TUI; engines beep / **mpd** / mpv; QR/cookie login; downloads, lyrics, MPRIS; A64 release assets, RV64 via source build | **Interim tool for M1** (validates account + API from the board); not embeddable (GPL, TUI) |
| [4fuu/net-mpd](https://github.com/4fuu/net-mpd) | Go / MIT | created 2026-07, very young | MPD-protocol *server* backed by NetEase (playlists, liked, daily, cloud disk read-only) | Watch; interesting precedent for exposing NetEase to MPD clients |
| [YesPlayMusic](https://github.com/qier222/YesPlayMusic) | Electron / MIT | v0.4.10 2025-10 | arm64 deb exists; desktop GUI | No (headless) |
| [SPlayer](https://github.com/SPlayer-Dev/SPlayer) / SPlayer-Next | Electron / AGPL | archived → successor | GUI | No |
| [netease-cloud-music-gtk](https://github.com/gmg137/netease-cloud-music-gtk) | Rust GTK4 / GPL-3 | 2.5.4 2026-08; Flathub x86_64 + aarch64 | GUI | No |
| [Listen1](https://github.com/listen1/listen1_chrome_extension), [netease-music-tui](https://github.com/betta-cyber/netease-music-tui) | | 2025 / dormant 2022 | | No |
| Mopidy / Volumio / moOde / Lyrion NetEase backends | | none found (0 repos) | | Confirms the need for a custom bridge |
| [shaonianzhentan/ha_cloud_music](https://github.com/shaonianzhentan/ha_cloud_music) | Python / MIT | pushed 2026-05 | Home Assistant integration; outputs to DLNA/MPD/Xiaomi; needs api-enhanced | Reference for HA users |
| [neqq3/ha_ncloud_music](https://github.com/neqq3/ha_ncloud_music), [ma_ncloud_music](https://github.com/neqq3/ma_ncloud_music) | Python / MIT | 2026 | NetEase as Jellyfin/OpenSubsonic for Music Assistant | Reference |
| [UnblockNeteaseMusic/server](https://github.com/UnblockNeteaseMusic/server) | Node / LGPL-3 | active | substitutes other sources for unlicensed tracks | **Excluded** (licence circumvention, C-3) |

### 1.4 API facts used in the design (from api-enhanced docs and ncmctl)

- `song/url/v1` levels: standard, higher, exhigh, lossless, hires, jyeffect, dolby, vivid, jymaster, sky. lossless/hires need 黑胶 VIP, sky/jymaster need SVIP. Response `expi` = 1200 s. `privilege.plLevel / dlLevel / maxBrLevel` give the entitlement ceiling.
- QR login: `login/qr/key` → `login/qr/create` → poll `login/qr/check` (800 expired, 801 waiting, 802 scanned, 803 confirmed). Cookie login = `MUSIC_U`; keep `NMTID`.
- Risk control: frequent login calls, password login (captcha), cloud/overseas IPs (`-460 cheating`) and task automation are the documented ban triggers.
- Cloud disk: `user/cloud`, `user/cloud/detail`, `cloud` (upload, multipart).

## 2. Playback engine and DSD

### 2.1 Engines

| Engine | Licence | DSD | A64 / RV64 (Debian 13) | Verdict |
|---|---|---|---|---|
| [MPD](https://www.musicpd.org/) 0.24.x (latest 0.24.15, 2026-08) | GPL-2 | native DSD automatic if ALSA offers it; DoP with `dop "yes"` (also per format via `allowed_formats "… dsd64:*=dop"`); automatic PCM fallback (Dsd2Pcm, 96-tap FIR, 8:1) then resampling (soxr/libsamplerate) | `mpd` 0.24.4 in trixie for both arches | **Chosen engine (ADR-0001)** |
| GStreamer ≥ 1.24 (trixie 1.26.2) | LGPL | `audio/x-dsd`, `dsdconvert`, alsasink native DSD; DoP not documented | both arches | No: DoP missing, no player/queue layer |
| Mopidy | Apache-2 | GStreamer-based, DSD not exposed, noise reports | Python | No |
| mpv | GPL/LGPL | FFmpeg decodes DSD to PCM 352.8 k only | both | No for HiFi DSD; fine as a helper |
| squeezelite 2.0 | GPL-3 | `-D` DoP or native u32be | trixie both arches | Needs a Lyrion server; no |
| Snapcast | GPL-3 | PCM only | | No (multi-room out of scope) |
| Roon Bridge | proprietary | native DSD | armv8 only | No riscv64, closed |
| Volumio / moOde / RoPieee | mixed | good | Raspberry Pi images only | No |

Key MPD facts (docs/04 relies on them): [user manual](https://mpd.readthedocs.io/en/stable/user.html), [plugins](https://mpd.readthedocs.io/en/stable/plugins.html), [protocol](https://mpd.readthedocs.io/en/stable/protocol.html), [AlsaOutputPlugin.cxx](https://github.com/MusicPlayerDaemon/MPD/blob/master/src/output/plugins/AlsaOutputPlugin.cxx), [Dsd2Pcm.cxx](https://github.com/MusicPlayerDaemon/MPD/blob/master/src/pcm/Dsd2Pcm.cxx):

- `SetupOrDop()`: try DoP (when enabled) → on failure plain setup (native DSD, else PCM).
- Software volume on DSD is a silent no-op in `pcm/Volume.cxx`; hardware mixer via `mixer_type "hardware"`, `mixer_control "PCM"`.
- `outputs / enableoutput / disableoutput / toggleoutput / outputset` and the `idle output` event; `dop` and `allowed_formats` are changeable at runtime with `outputset`.
- `extm3u` playlist plugin reads `#EXTINF:sec,name` (name is opaque text).
- `curl` input seeks with HTTP Range when the server advertises `Accept-Ranges`.
- MPD counts DSD rates in bytes (DSD64 = 352800); shorthand `dsd64:2` in formats.
- Shipped unit: `LimitRTPRIO=40`, `LimitRTTIME=infinity`, `LimitMEMLOCK=64M`.
- Benchmarks on Cortex-A53 class: DSD256→PCM ≈ 80 % of a core, 57 % when forced to 352.8 kHz output ([discussion #2372](https://github.com/MusicPlayerDaemon/MPD/discussions/2372)); soxr "very high" 44.1→192 ≈ 30–35 % on Pi 2 ([RuneAudio](https://www.runeaudio.com/forum/mpd-soxr-resampling-t996.html)).

### 2.2 Linux kernel native DSD (snd-usb-audio)

Sources: [format.c](https://github.com/torvalds/linux/blob/master/sound/usb/format.c), [quirks.c](https://github.com/torvalds/linux/blob/master/sound/usb/quirks.c), [alsa-configuration.rst v6.18](https://github.com/torvalds/linux/blob/v6.18/Documentation/sound/alsa-configuration.rst).

- A UAC2 alt-setting advertising `RAW_DATA` sets `dsd_raw`; the device is then offered `DSD_U32_BE` if it is in the explicit device list, or its vendor/device carries `QUIRK_FLAG_DSD_RAW`.
- Vendor-wide `DSD_RAW` (mainline, 2026): AURALiC 0x1511, Thesycon 0x152a, iBasso 0x18d1, XMOS 0x20b1, Accuphase 0x21ed, Oppo 0x22d9, Mytek 0x25ce, IAG 0x2622, Musical Fidelity 0x2772, Rotel 0x278b, Gustard 0x292b, FiiO 0x2972, T+A 0x2ab6, McIntosh 0x2afd, Cayin 0x2d87, **Comtrue 0x2fc6 (since 6.14, backported to 6.1-stable)**, HEM 0x3336, Khadas 0x3353, MSB 0x35f4, EVGA 0x3842, HiBy 0xc502. Not vendor-wide: Savitech 0x262a (only one device), C-Media 0x0d8c, Cirrus, Conexant, Realtek.
- Check: `/proc/asound/cardX/stream0` shows `Format: DSD_U32_BE` when the quirk applies, `SPECIAL` when not.
- Enable without a kernel rebuild: module parameter `quirk_flags`, bit 15 = `dsd_raw` (0x8000). Kernel ≥ 6.18 accepts the readable form `quirk_flags=VID:PID:dsd_raw` (sysfs-changeable); older kernels take an integer array in probe order (`quirk_flags=0x8000`). Example on a Savitech JM45 (262a:0001) then played raw DSD with MPD 0.24. Permanent fix = a `DEVICE_FLG` patch upstream. Risk: a wrong vendor-wide flag can break a device (Musical Fidelity M6s regression, 2026-07).

### 2.3 DAC chips and dongles

| Chip | PCM | Native DSD | DoP | Volume | Source |
|---|---|---|---|---|---|
| ES9039Q2M | 32/768 k | to DSD1024 | to DoP512 | in-chip, applies to PCM, DoP and DSD | [datasheet](https://www.esstech.com/wp-content/uploads/2026/05/ES9039Q2M_Datasheet_v0.2.3.pdf) |
| CS43131 | 32/384 k | dedicated DSD pins to 256×Fs | **DSD64 and DSD128 only** (176.4/352.8 k containers) | DSD processor applies volume in the DSD domain; 2 Vrms line, 30 mW/32 Ω | [datasheet](https://statics.cirrus.com/pubs/proDatasheet/CS43131_DS1155F2.pdf) |

| Dongle | DAC | USB bridge | Linux native-DSD prospect |
|---|---|---|---|
| Moondrop Dawn Pro | 2× CS43131 | Comtrue, `2fc6:f06a` | vendor-wide DSD_RAW since 6.14 → likely native; UNVERIFIED on hardware |
| iBasso DC-Elite | ROHM BD34301 | Comtrue `2fc6:f0b5` (explicit quirk entry) | native |
| Hidizs S9 Pro Plus | ES9038Q2M | XMOS XUF208 | native if it enumerates as XMOS 0x20b1 (UNVERIFIED) |
| FiiO KA11 / KA13 / KA5 | CS43131 / 2× CS43131 / 2× CS43198 | SA9312L / Comtrue CT7601 | FiiO VID 0x2972 vendor-wide; UNVERIFIED |
| JCally JM20 Pro | CS43131 | Savitech SA9312L (0x262a) | needs `quirk_flags`; DoP works |
| JCally JM6 / JM6 Pro | Conexant CX31993 | integrated | PCM only, no DSD |
| Tanchjim Space, Fosi DS2, Kiwi Ears Allegro, Truthear Shio | CS43131 / CS43131 / ES9028Q2M / CS43198 | UNVERIFIED | probe in M0 |

No public `stream0` dumps exist for these dongle classes: **M0 must probe each one**.

### 2.4 DoP facts

DoP 1.1: 24-bit PCM frames, marker bytes 0x05/0xFA, 16 DSD bits per channel per frame; DSD64 at 176.4 k, DSD128 at 352.8 k, DSD256 at 705.6 k, DSD512 at 1.4112 M ([DoP spec](https://www.dsd-guide.com/sites/default/files/white-papers/DoP_openStandard_1v1.pdf)). Any sample scaling destroys the marker → noise; keep volume at 100 % during DoP unless the dongle's USB volume is applied inside the DAC chip after DoP decoding (per design, UNVERIFIED per dongle).

### 2.5 Bit-perfect and stability practices

- Do not install PipeWire/PulseAudio on the server; if present, udev `ACP_IGNORE` (PipeWire ≥ 0.3.83) / `PULSE_IGNORE`.
- Stable card names: udev `SUBSYSTEM=="sound", ATTR{id}="Name"` then `hw:CARD=Name` ([ALSA wiki](https://www.alsa-project.org/wiki/Changing_card_IDs_with_udev)); or `snd-usb-audio vid= pid= index=`.
- Dropout mitigations: generous `buffer_time`, RT priority, `performance` governor, Wi-Fi power save off, IRQ affinity. No verified dropout reports specific to H618 or JH7110 host ports (UNVERIFIED either way).
- USB 2.0 isochronous: ≤ 1024 B × 3 per 125 µs microframe, ≤ 80 % periodic; PCM 768 k/32 = 768 B per microframe, DSD512 native = 706 B; DoP512 exceeds one packet per microframe and is unrealistic on dongles.

## 3. Boards and OS

Full comparison in docs/05 §1. Sources: [CNX Zero 3](https://www.cnx-software.com/2023/07/03/orange-pi-zero-3-allwinner-h618-sbc-ships-with-up-to-4gb-ram/), [Armbian Zero 3](https://www.armbian.com/orange-pi-zero-3/), [CNX Orange Pi RV](https://www.cnx-software.com/2025/04/01/orange-pi-rv-low-cost-risc-v-sbc-with-starfive-jh7110-soc/), [Orange Pi RV NAS blog (Linux 6.19)](https://michal.hrusecky.net/2026/01/orange-pi-rv/), [CNX RV2](https://www.cnx-software.com/2025/03/08/orange-pi-rv2-low-cost-risc-v-sbc-ky-x1-octa-core-soc-2-tops-ai-accelerator/), [Armbian RV2](https://armbian.com/boards/orangepirv2), [Debian RISC-V](https://wiki.debian.org/RISC-V), [Zero 3 power](https://en.neonhero.dev/2026/02/orange-pi-zero-3-power-usage-benchmarks.html).

Package check on packages.debian.org (trixie): mpd 0.24.4, mpc 0.35, ffmpeg 7.1, samba 4.22, nginx 1.26, avahi 0.8, nodejs 20.19, golang-go 1.24, shairport-sync 4.3, gmediarender 0.3 all present on **both** arm64 and riscv64. Not in Debian: myMPD (OBS repo: amd64/arm64/armhf), upmpdcli (vendor repo: amd64/arm64) → source builds on riscv64.

Go: official `linux-riscv64` binaries since 1.21 ([go.dev/dl](https://go.dev/dl/)); cross-compile from Windows with `GOOS=linux GOARCH=riscv64 CGO_ENABLED=0`. Node: no official riscv64 binaries ([unofficial-builds](https://github.com/nodejs/unofficial-builds)).

## 4. Remote control

| Option | Licence / status | Outputs switch | Verdict |
|---|---|---|---|
| [M.A.L.P.](https://gitlab.com/gateship-one/malp) ([F-Droid](https://f-droid.org/packages/org.gateshipone.malp/)) | GPL-3, 1.3.2 (2024-07) | yes | **Day-1 Android client** |
| [MAFA](https://mafa.indi.software/) | closed, 3.2.3 (2026-06), sideload | yes | Alternative |
| MPDroid, MPD Remote, Mupeace | dead | | No |
| [myMPD](https://github.com/jcorporation/myMPD) | GPL-3, v26.0.0 (2026-08), C, PWA | yes | Optional companion (OBS deb arm64; source on riscv64) |
| [upmpdcli](https://www.lesbonscomptes.com/upmpdcli/) | GPL-2, 1.9.18 (2026-09) | shares MPD outputs | Optional DLNA renderer for the NetEase app |
| [gmrender-resurrect](https://github.com/hzeller/gmrender-resurrect) | GPL-2, dormant | bypasses MPD | No |
| Subsonic jukebox (DSub, Ultrasonic; gonic, Navidrome ≥ 0.50, Ampache Localplay) | mixed | no output selection in the API; Symfonium/Tempo do not implement jukebox; [mpdsub](https://github.com/mdlayher/mpdsub)/[mpdsonic](https://github.com/pborzenkov/mpdsonic) archived | Rejected |
| [Home Assistant MPD](https://www.home-assistant.io/integrations/mpd/) | | no (`outputs` unused) | Bonus |
| PWA tooling: Chrome 142 [Local Network Access](https://developer.chrome.com/blog/local-network-access), Android 16 [local network permission](https://developer.android.com/privacy-and-security/local-network-permission), [NSD](https://developer.android.com/develop/connectivity/wifi/use-nsd), [network_security_config](https://developer.android.com/privacy-and-security/security-config) | | | Constraints in docs/06 |

## 5. Upload and library

| Option | Licence / status | Verdict |
|---|---|---|
| [tus](https://tus.io/) protocol: [tusd](https://github.com/tus/tusd) v2.10 (Go, MIT, embeddable), [tus-js-client](https://github.com/tus/tus-js-client) 4.3, [Uppy](https://uppy.io/) 6.0 (MIT) | active | **Chosen (ADR-0004)** |
| Resumable.js | dead (2018) | No |
| Plain multipart streamed with Go `MultipartReader` | | Kept for scripts |
| Samba 4.22 (trixie, both arches) | | **Bulk alternative** |
| SFTP / rsync / [Syncthing](https://github.com/syncthing/syncthing/releases) (riscv64 builds) | | Documented |
| WebDAV | Windows client size cap | No |
| [Navidrome](https://github.com/navidrome/navidrome) 0.63 (arm64 + riscv64), [gonic](https://github.com/sentriz/gonic), [beets](https://github.com/beetbox/beets) 2.14, [Picard](https://picard.musicbrainz.org/) | | Optional library/tagging helpers |

## 6. Summary of choices

| Need | Choice | Backup |
|---|---|---|
| Playback engine | MPD 0.24 (Debian) | — |
| NetEase API | chaunsin/netease-cloud-music (Go, MIT) | go-musicfox/netease-music; api-enhanced sidecar |
| Interim NetEase player | go-musicfox with MPD engine | musicbox |
| Phone control day 1 | M.A.L.P. | MAFA, myMPD |
| Phone control final | `hifid` PWA | Capacitor/Kotlin app |
| Upload | tusd + Uppy in `hifid` | Samba |
| DLNA side path | upmpdcli | — |
| Primary board | Orange Pi Zero 3, Armbian Debian 13 (6.18) | Orange Pi RV, Debian 13 riscv64 |

## 7. Source index

Sources are linked inline above. The four raw survey reports (NetEase, MPD/DSD/DAC, boards,
control/upload) were produced on 2026-09-13; claims marked UNVERIFIED could not be confirmed
from a primary source and are re-checked in milestone M0.

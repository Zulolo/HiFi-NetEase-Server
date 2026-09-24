# User Manual · HiFi-NetEase-Server playback layer

This manual covers what exists today: a bit-perfect MPD-based music server on a small Debian
board with USB DAC dongles, phone control through any MPD client, and music import over a
Samba share. The NetEase Cloud Music service (`hifid`) and the phone web app are the next
milestones (docs/11) and will get their own chapters.

Everything here is generic. The hardware named in examples is one tested configuration; any
Debian-based SBC and any USB Audio Class 2 DAC should behave the same way.

## 1. What you need

| Item | Requirement | Notes |
|---|---|---|
| Board | Debian-based Linux (Debian 12/13, Armbian, Raspberry Pi OS, vendor images), ≥ 512 MB RAM, one free USB host port | Tested: Orange Pi Zero 3 (2 GB, Debian 12 vendor image, kernel 6.1). Raspberry Pi 3/4/5, Orange Pi 3/5, Radxa boards work the same |
| Network | Ethernet, or Wi-Fi with a stable driver | On Wi-Fi the installer enables a link watchdog and disables power saving |
| Music storage | USB flash drive or SSD, or a second microSD in a USB reader; will be formatted ext4 | The OS card is not used for music |
| DAC | Any USB Audio Class 2 dongle or DAC | Native DSD depends on the USB bridge chip; DoP works on every DoP-capable DAC; PCM always works |
| Amplifier / speaker | Any analog line input (3.5 mm or RCA) | The DAC's headphone output drives line inputs fine at 100 % volume |
| Phone | Any phone with a browser (myMPD web client on the board), or an MPD client app | M.A.L.P. for Android via F-Droid (the Play Store hides it on Android 14+), or a desktop client like Cantata |
| PC | Windows/macOS/Linux with SMB support | For dropping music onto the share |

## 2. Prepare the board

1. Flash the distribution image, boot, log in over SSH (or the serial console), and make sure
   the board has network and the date is correct.
2. Plug in the USB storage and the DAC. Check that both are seen:

   ```
   lsblk                      # your disk appears as /dev/sda1 or /dev/sdb1
   lsusb                      # your DAC appears with its VID:PID
   cat /proc/asound/cards     # the DAC appears as a "USB-Audio" card
   ```

3. Copy this repository's `deploy/` folder to the board, for example from your PC:

   ```
   scp -r deploy user@board:~/hifi/
   ssh user@board 'chmod +x ~/hifi/deploy/scripts/*.sh'
   ```

   (If the files were edited on Windows, run `sed -i 's/\r$//' ~/hifi/deploy/scripts/* ~/hifi/deploy/mpd/*` once.)

## 3. Install

One command does everything (packages, disk, DAC detection, MPD config, Samba, Avahi, Wi-Fi watchdog):

```
cd ~/hifi/deploy/scripts
sudo ./install.sh --disk /dev/sdb1 --format          # erases /dev/sdb1 and formats it ext4 (label "hifi")
```

Variants:

| Situation | Command |
|---|---|
| Disk already formatted ext4 with label `hifi` | `sudo ./install.sh --disk-label hifi` |
| No separate disk, keep music on the OS card | `sudo ./install.sh --no-disk` |
| No Samba (you will use SFTP/rsync instead) | add `--no-samba` |
| Give a DAC a friendly ALSA name | add `--dac-name 2fc6:f802=ES9039` (VID:PID from `lsusb`, name ≤ 15 letters/digits) |
| Do not touch the kernel's DSD quirk | add `--no-native-dsd` |

The installer prints a summary at the end: MPD version and outputs, the Samba path and where
its password is stored (`/etc/hifi/samba.txt`), and how to connect from the phone. Every step
is a separate script you can re-run alone (`setup-disk.sh`, `dac-setup.sh`, `gen-mpd-conf.sh`).

## 4. Verify the audio path

```
sudo ./test-audio.sh                                   # plays 44.1/96/192 kHz test tones
sudo ./test-audio.sh --dsd /path/to/some.dsf           # also plays a DSD file you provide
```

For each file the script prints what the DAC actually received, read from the kernel while playing:

| Printed format | Meaning |
|---|---|
| `S16_LE` / `S24_3LE` / `S32_LE` at the file's rate | bit-perfect PCM |
| `DSD_U32_BE` | **native DSD** (the DAC decodes DSD itself) |
| `S24_3LE`/`S32_LE` at DSD-rate ÷ 16 (176.4 k for DSD64) | DoP, still bit-perfect |
| PCM at 352.8 k or 176.4 k while playing DSD | software conversion (DAC cannot take this DSD rate) |

No DSD file at hand? `python3 tools/gen-dsf.py test.dsf --rate 64` (from this repository, runs
on the PC) synthesizes a 3-second DSD64 test tone; `--rate 128` or `256` for higher rates.

Result on the tested dongle (Comtrue bridge, ES9039, `2fc6:f802`, kernel 6.1): PCM to
768 kHz and native DSD after the installer added `quirk_flags=0x8000` for `snd-usb-audio`.

## 5. Control from the phone

Two ways; both talk to the same MPD, so you can mix them.

### 5.1 Browser, no app (myMPD)

Install the web client on the board with `sudo ./install.sh ... --with-mympd` (or later:
the same steps are in `install.sh` §6; Debian 12/13 on arm64/amd64, other architectures
build it from source). Then open `http://<board-ip>/` on the phone. Library, queue,
transport, volume, playlists, **Outputs**, web radio and smart playlists are all there. To
pin it as an app icon with full-screen behaviour, open `https://<board-ip>/` once, accept
the self-signed certificate, and use the browser's "Add to Home screen".

**Queue vs. library.** MPD keeps two lists: the *library* (everything on the disk, indexed
automatically) and the *queue* (what will play now). New files never appear in the queue by
themselves. In myMPD open the database icon in the left bar, choose *Filesystem* or *Albums*,
find the folder or album, and use its menu to *Append to queue* or *Play*. The same applies
in M.A.L.P. (Library tab → long-press → add to queue).

### 5.2 Native app (M.A.L.P. or any MPD client)

- **Play Store note:** on Android 14 and newer the Play Store shows "not available for your
  device" for M.A.L.P. because the app targets an older Android SDK level. It still installs
  and runs fine; get it from **F-Droid** (install the F-Droid client from f-droid.org, search
  "M.A.L.P."), or download the APK from
  https://f-droid.org/packages/org.gateshipone.malp/ and allow "install unknown apps" for
  your browser once.
- Alternatives: MAFA (closed source, APK from its website), or any desktop client such as
  Cantata on the PC.
- Add a server: the board's IP address (shown by `hostname -I`) or `<hostname>.local`,
  port `6600`.
- You get: library browsing, queue, play/pause/next, volume (through the DAC's own USB
  volume control when it has one), stored playlists, and **Outputs** to switch between DACs
  when several are attached.

Volume note: the DAC's USB volume control works for PCM and for native DSD on the chips
tested. When playing DoP through a DAC that scales samples in its USB bridge, changing the
volume makes noise; in that case keep the DAC at 100 % and use the amplifier's knob
(`gen-mpd-conf.sh --volume none` makes this permanent).

## 6. Add music

### 6.1 Samba share (Windows Explorer)

- Path: `\\<board-ip>\music` (or `\\<hostname>\music`), user and password from
  `/etc/hifi/samba.txt` on the board (`sudo cat /etc/hifi/samba.txt`).
- Windows must connect with that user, not with your Windows account. Either map a drive
  ("This PC → Map network drive → Connect using different credentials", user `hifi`), or
  once from a command prompt:

  ```
  net use \\<board-ip>\music /user:hifi <password> /persistent:yes
  ```

  If Explorer says "Windows can't find \\board\music" it usually tried your Windows account
  first; the installer's `map to guest = Never` setting makes it ask for credentials instead.
- Drop files or whole folders. Any folder structure is fine; `Artist/Album/NN - Title.ext` is
  recommended for tidy browsing.
- MPD notices new files by itself (`auto_update`); large drops can take a minute to appear.
  To force it: `mpc update`.
- Speed depends on your link: ≈ 10–25 MB/s on 5 GHz Wi-Fi or Ethernet with a flash drive,
  1–3 MB/s on weak 2.4 GHz Wi-Fi.

### 6.2 Other ways

`scp`/SFTP to `/srv/music/local/`, `rsync`, or Syncthing all work; files must be readable
by group `audio` (the share does this automatically; for SFTP use `chmod -R g+rw`).

### 6.3 Supported formats

FLAC, ALAC, WAV, AIFF, APE, WavPack, MP3, AAC, OGG Vorbis, Opus, DSF, DFF (uncompressed),
CUE sheets. SACD ISO and DST-compressed DFF must be converted on the PC first.

## 7. Several DACs

Plug them all in, then:

```
sudo ./dac-setup.sh --try-native-dsd [--dac-name VID:PID=Name ...]
sudo ./gen-mpd-conf.sh
sudo systemctl restart mpd
```

Each DAC becomes an MPD output (`mpc outputs`); enable the one you want from the phone
(`Outputs` in M.A.L.P.) or with `mpc enable 2` / `mpc disable 1`. Names stay stable across
reboots because the installer writes a udev rule keyed on the DAC's USB IDs.

## 8. DSD: what to expect per DAC class

| DAC class | Native DSD on Linux | DoP | Notes |
|---|---|---|---|
| ES9039Q2M / ES9038 dongles with XMOS or Comtrue bridge | yes (Comtrue: kernels ≥ 6.14 out of the box, older kernels via the installer's quirk) | to DSD256 (needs 705.6 kHz PCM) | tested |
| CS43131 dongles with Comtrue bridge | likely (same rule) | to DSD128 (chip limit) | DSD256 falls back to software |
| CS43131 dongles with Savitech bridge (JCally, FiiO KA11) | via the installer's quirk on any kernel | to DSD128 | |
| Conexant CX31993 dongles | no DSD at all | no | PCM only, up to 384 kHz |
| Older or unknown bridges | run `dac-setup.sh --try-native-dsd`; if `stream0` still shows `SPECIAL`, DoP is used automatically | | |

If a DAC misbehaves with the quirk (mis-clocked playback), remove
`/etc/modprobe.d/hifi-dsd.conf` and reboot; MPD will use DoP instead.

## 9. Troubleshooting

| Symptom | Check |
|---|---|
| No sound, `mpc outputs` shows the DAC | `sudo journalctl -u mpd -n 30`; if it says "Failed to open ALSA device", the card name changed: `cat /proc/asound/cards`, then `sudo ./dac-setup.sh && sudo ./gen-mpd-conf.sh && sudo systemctl restart mpd` |
| MPD not running after boot | `systemctl status mpd srv-music.mount`; the music disk must be mounted first (label `hifi`) |
| Clicks or dropouts | `dmesg | grep -i xrun`; on Wi-Fi check `iw dev wlan0 get power_save` (must be off); keep the disk and DAC on different USB ports if the board has them |
| Phone cannot find the server | use the IP instead of `.local`; make sure the phone is on the same network and the router does not isolate clients |
| "Windows can't find \\board\music" | Port 445 is open but Windows connected as your own account and got no access. Connect as `hifi`: `net use \\board-ip\music /user:hifi <password>` (password in `/etc/hifi/samba.txt`); verify from the board with `testparm -s` that `[music]` exists |
| Samba asks for a password again and again | `sudo smbpasswd -a hifi` to set a new one, or use `\\ip\music` with the user `hifi` |
| DSD plays as PCM | `grep -o DSD_U32_BE /proc/asound/card*/stream0` empty → run `sudo ./dac-setup.sh --try-native-dsd`; DoP needs the DAC to accept the container rate (see §8) |
| New files do not appear | `mpc update`; check permissions (`ls -l /srv/music/local`), files must be group `audio` readable |

## 10. Uninstall

```
sudo systemctl disable --now mpd smbd wifi-watchdog.timer srv-music.mount srv-data.mount srv-hifi.mount
sudo rm -f /etc/udev/rules.d/90-hifi-dac.rules /etc/modprobe.d/hifi-dsd.conf /etc/systemd/system/srv-*.mount /etc/systemd/system/wifi-watchdog.*
sudo apt remove mpd mpc samba      # optional
```

Your music on the disk is untouched.

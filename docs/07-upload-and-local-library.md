# 07 · Upload and Local Library

Status: draft v0.1 · 2026-09-13 · addresses FR-3, FR-4.1, NFR-8, C-7

## 1. Import paths

| Path | For | Resumable | Multi-GB | Needs our code |
|---|---|---|---|---|
| **SMB share (samba)** — primary | Bulk drag-and-drop from Windows Explorer, robocopy; the preferred path | OS-level | Yes | Watcher only |
| **Browser upload (tus)** — secondary | "Web page, many files at once" from any browser, progress in the PWA | Yes | Yes | Yes |
| SFTP (OpenSSH + WinSCP) | Scripted or occasional | Client-level | Yes | No |
| rsync (cwRsync/WSL) | Mirroring a PC folder | Yes | Yes | No |
| Syncthing | Continuous two-way sync | Yes | Yes | No |
| WebDAV | Not recommended: Windows' built-in client has a file-size cap (default ≈ 50 MB, max 4 GB) | | | |

Decision (ADR-0004, revised): Samba is installed by default and is the primary import path,
with an inotify watcher in `hifid` that triggers incremental MPD updates; the tus browser
upload is the secondary path, built after Samba is live. Both write into the same tree and
trigger the same scan. Over a Wi-Fi link both run at the Wi-Fi rate (docs/05 §7).

## 2. Browser upload design (FR-3.1, FR-3.2)

### 2.1 Protocol: tus 1.0.0

- Server: the `tusd` Go library (`pkg/handler` + `filestore`) embedded in `hifid` at
  `/api/v1/upload/tus/`. MIT licence, actively maintained, no separate process.
- Client: Uppy (Dashboard + Tus plugin) in the PWA upload screen, or `tus-js-client` with a
  custom UI if Uppy's bundle size is an issue.
- Chunk size 8 MB; 3 files in parallel; retries with back-off; uploads survive page reloads
  (Uppy stores upload URLs in the browser; `GET /upload/sessions` lists server-side state).
- Folder input: `webkitdirectory` for the file picker; drag-and-drop uses
  `webkitGetAsEntry` + `readEntries` in a loop (Chromium returns at most 100 entries per
  call). The relative path is sent as tus metadata `relativePath`.
- Integrity: the browser computes SHA-256 incrementally while reading the file (Web Crypto,
  streaming through the chunks it already reads) and sends it as metadata; the server verifies
  before finalizing. Optional per file; on by default for files above 100 MB.

### 2.2 Server pipeline

```
tus create  -> /srv/data/incoming/<upload id>            (same filesystem as /srv/music: rename is atomic)
tus patch   -> append; offset persisted by tusd's filestore
complete    -> hifid hook:
   1. extension allow-list: flac wav aiff aif alac m4a mp3 aac ogg opus ape wv dsf dff dsdiff
                            + covers: jpg jpeg png + cue/log/txt (kept next to audio)
   2. sha256 check if provided (mismatch -> keep in incoming/quarantine, report)
   3. target = /srv/music/local/<relativePath>/<filename>, sanitized (no '..', no control chars)
   4. if target exists: compare size+hash; identical -> drop new copy, report "duplicate";
      different -> save as "<name> (2).<ext>", emit library.duplicate
   5. rename() into place; fsync directory
   6. mpd: update "local/<relativePath>"   (incremental, non-blocking)
   7. ws: upload.done, library.update
```

- Constant memory: tusd streams to disk; no whole-file buffering (1 GB board).
- `incoming/` is cleaned of abandoned uploads after 7 days (tus expiration extension).
- Simple multipart endpoint (`POST /api/v1/upload/simple`) kept for `curl` scripts, capped
  at 512 MB per file.

### 2.3 Throughput expectations

See docs/05 §7: ≈ 35 MB/s on the Zero 3 (USB 2.0 disk bound), ≈ 100 MB/s on the RV with an
SSD. SHA-256 on the server costs ≈ 10–20 % of one A53 core at 35 MB/s.

## 3. SMB share (FR-3.1, primary path)

`deploy/scripts/install.sh` installs `samba` (Debian arm64 and riscv64) by default with one
share; `--without-samba` skips it:

```
[music]
   path = /srv/music/local
   valid users = hifi
   writable = yes
   create mask = 0664
   directory mask = 0775
   force group = audio
```

Change detection: MPD does not watch the filesystem by itself. `hifid` runs an inotify
watcher on `/srv/music/local` (recursive, debounced 10 s) and issues `mpd update <dir>` for
the changed directories. This makes SMB drops appear on the phone as quickly as web uploads.

## 4. Library layout and naming

```
/srv/music/
├── local/                 # uploads; the uploader's folder structure is preserved as sent
│   └── <Artist>/<Album>/<NN - Title>.<ext>       (recommended, not enforced)
├── netease/               # downloads (docs/08 §7), always <AlbumArtist>/<Album>/<NN - Title>.<ext>
└── playlists/             # MPD stored playlists (.m3u); NetEase/*.m3u generated
```

- MPD's database indexes both trees; the `hifid` SQLite index stores per-file: path, size,
  sha256, added time, source (`upload`, `smb`, `netease`), and NetEase song id when known.
- "Recently added" (PWA Library tab) is served from the SQLite index (MPD has `added` sorting
  only in newer protocol versions; the index is authoritative).

## 5. Formats and what MPD does with them (FR-4.1)

| Format | MPD decoder | Notes |
|---|---|---|
| FLAC | `flac` | gapless, tags, embedded art |
| ALAC (m4a) | `ffmpeg` | |
| WAV / AIFF | `sndfile` or `ffmpeg` | tags in RIFF/ID3 chunks may be sparse |
| APE (Monkey's Audio) | `ffmpeg` | CPU heavier than FLAC (high compression levels) |
| WavPack | `wavpack` | incl. DSD-in-WavPack (decoded to PCM by the plugin) |
| MP3 | `mad` or `ffmpeg` | |
| AAC (m4a) | `ffmpeg` | |
| OGG Vorbis / Opus | `vorbis` / `opus` | |
| DSF / DFF | `dsf` / `dsdiff` | native DSD, DoP or dsd2pcm chosen at the output (docs/04). DST-compressed DFF is not supported by MPD's decoder; convert on the PC. |
| SACD ISO | not supported | extract to DSF on the PC (`sacd_extract`) before upload |
| CUE + single file | `cue` playlist plugin | one big FLAC + CUE shows as tracks |

Uploads are never transcoded (NFR-8, HiFi). Optional offline "PCM twin" generation for DSD
files that a chosen DAC cannot play natively is described in docs/04 §6.

## 6. Duplicates and hygiene (FR-3.5)

- Exact duplicates (same sha256) are rejected at finalize with a warning event.
- Same title/artist/album with different hash (for example 16/44 and 24/96 versions) are both
  kept; the PWA shows the format so the user can choose.
- Tagging is done on the PC (MusicBrainz Picard) before upload; on the server, `beets` is an
  optional add-on for auto-tagging, triggered manually, never automatically (NFR-8).

## 7. Backup

The music tree is the owner's data; `deploy/scripts/backup.sh` documents an `rsync` of
`/srv/music` and `/srv/data/hifid` to the PC or a second disk. MPD's database is rebuildable
and not backed up.

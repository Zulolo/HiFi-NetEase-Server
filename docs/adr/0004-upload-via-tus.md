# ADR-0004 · Samba share is the primary import path; browser uploads (tus) are the secondary path

- Status: accepted (revised 2026-09-13: owner chose Samba first, web upload second)
- Date: 2026-09-13
- Requirements addressed: FR-3, NFR-8

## Context

Multi-file imports of multi-GB DSD albums from a Windows PC over **Wi-Fi** onto a 512 GB USB
flash drive (sustained write 10–30 MB/s). Windows Explorer drag-and-drop is preferred over a
web page, with a browser upload kept as a second path. Survey in docs/07.

## Options considered

| Option | Pros | Cons |
|---|---|---|
| **Samba share (Debian `samba`, both arches) + inotify watcher → MPD update** | Native Explorer/robocopy workflow, resumable at OS level, no browser limits, throughput limited only by Wi-Fi and the flash drive; zero custom upload code | Needs a share credential; change detection must be built (inotify); no progress in the phone UI |
| tus 1.0.0 (tusd Go library embedded + Uppy) | Resumable, chunked, works from any browser, progress and hash check in the PWA | More client code; second path only |
| Plain multipart streamed to disk | Trivial | Not resumable; a failed 5 GB upload restarts |
| WebDAV | Explorer integration | Windows client file-size cap (default ≈ 50 MB, max 4 GB) |
| SFTP / rsync / Syncthing | Robust | Not what the owner asked for; documented only |

## Decision

1. Install Samba by default with one share on `/srv/music/local` (user `hifi`, group `audio`).
   `hifid` runs a recursive inotify watcher (debounced 10 s) on that tree and issues
   incremental `mpd update` calls, so files dropped from Explorer appear on the phone within
   a minute. A "recently added" view comes from the `hifid` index.
2. Build the tus browser upload in the PWA as the secondary path in M3, after Samba is live:
   same target tree, same finalize rules (extension allow-list, duplicate check, atomic
   rename, incremental update), Uppy Dashboard with folder drag-and-drop and SHA-256.
3. Keep a simple multipart endpoint for scripts.

## Consequences

- Samba over Wi-Fi is bounded by the weaker of Wi-Fi (≈ 10–25 MB/s on 5 GHz) and the flash
  drive (10–30 MB/s); a 4 GB DSD album takes 3–7 minutes either way.
- Samba adds ≈ 20–40 MB RSS, which matters on a 512 MB board (docs/05 §6).
- `incoming/` for tus stays on the same filesystem as the music tree (atomic rename).
- Uploads are never transcoded or deleted by the service.

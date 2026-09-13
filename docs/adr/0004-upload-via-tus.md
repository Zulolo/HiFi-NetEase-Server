# ADR-0004 · Browser uploads use the tus resumable protocol; Samba is the bulk alternative

- Status: accepted
- Date: 2026-09-13
- Requirements addressed: FR-3, NFR-8

## Context

Multi-file uploads of multi-GB DSD albums from a Windows PC over the LAN, with automatic
library import, on a board with 1 GB RAM and a USB 2.0 disk (≈ 35 MB/s). Survey in docs/07.

## Options considered

| Option | Pros | Cons |
|---|---|---|
| **tus 1.0.0 (tusd Go library embedded + Uppy/tus-js-client)** | Resumable, chunked, parallel, constant memory, folder drag-and-drop, checksum extension, active projects (MIT) | Slightly more client code than a form |
| Plain multipart streamed to disk | Trivial | Not resumable; a failed 5 GB upload restarts |
| WebDAV (nginx/rclone) | Explorer integration | Windows client file-size cap (default ≈ 50 MB, max 4 GB) |
| SMB share only | Best raw throughput, no browser | Not the requested "web page"; needs Samba credentials |
| SFTP / rsync / Syncthing | Robust | Not browser-based |

## Decision

Embed tusd in `hifid` at `/api/v1/upload/tus/`, with Uppy in the PWA. Finalize by atomic
rename into `/srv/music/local/<relativePath>/`, verify SHA-256 when supplied, trigger an
incremental MPD update. Install Samba optionally as the bulk path; an inotify watcher makes
SMB drops appear like uploads. Keep a simple multipart endpoint for scripts.

## Consequences

- `incoming/` must be on the same filesystem as the music tree (atomic rename).
- Uploads are never transcoded or deleted by the service.
- Uppy adds ≈ 100 kB gzipped to the PWA; acceptable for the upload screen (lazy-loaded).

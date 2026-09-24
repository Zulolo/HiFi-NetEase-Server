# 08 · NetEase Cloud Music Integration Design

Status: draft v0.1 · 2026-09-13 · addresses FR-1, C-1, C-2, C-3, NFR-5, NFR-6

## 1. Position in the system

The NetEase adapter is one module inside `hifid` (see docs/03). It has three jobs:

1. **Account/session**: log in with the owner's account, keep the session alive, report health.
2. **Catalogue**: expose the owner's library to the API layer (playlists, liked songs,
   recommendations, search, cloud disk, song details, available qualities).
3. **Audio delivery**: turn a NetEase song ID into bytes that MPD can play, either by
   redirecting MPD to a fresh CDN URL (streaming) or by serving a locally downloaded copy.

It never touches audio samples and never talks to ALSA. If NetEase breaks, only NetEase
features degrade (NFR-6).

## 2. Adapter interface

The rest of `hifid` only depends on this Go interface (names indicative):

```go
type Client interface {
    // session
    QRLoginStart(ctx) (key string, qrURL string, err error)
    QRLoginPoll(ctx, key) (QRStatus, error)            // waiting | scanned | confirmed | expired
    LoginWithCookie(ctx, musicU string) error
    Session(ctx) (Session, error)                        // user id, nickname, vip type, valid
    Refresh(ctx) error

    // catalogue
    UserPlaylists(ctx, uid) ([]Playlist, error)
    PlaylistTracks(ctx, pid, offset, limit) ([]Song, error)
    LikedSongIDs(ctx, uid) ([]int64, error)
    DailyRecommend(ctx) ([]Song, error)
    Search(ctx, q, kind, offset, limit) (SearchResult, error)
    Album(ctx, id) (AlbumDetail, error)
    ArtistSongs(ctx, id, offset, limit) ([]Song, error)
    CloudList(ctx, offset, limit) ([]CloudSong, error)
    SongDetail(ctx, ids []int64) ([]Song, error)          // batched, up to ~1000 ids

    // audio
    SongURL(ctx, ids []int64, level Level) ([]SongURL, error)
    Lyric(ctx, id) (Lyric, error)                         // optional, phase 3
}
```

Implementation choice (ADR-0002, docs/02 §1): the adapter is built on
**chaunsin/netease-cloud-music** (MIT, pure Go, weapi + eapi + xeapi, QR/cookie login, all
quality levels, cloud-disk upload, release binaries for linux/arm64 and linux/riscv64,
v0.8.0 of 2026-09). Fallback library with the same shape: **go-musicfox/netease-music**
(MIT, 160+ endpoints). The Node.js **api-enhanced** project is used as the reference
specification of endpoints and response fields, not as a runtime. The interface is
deliberately small so the implementation can be swapped (another Go library, or a sidecar
process speaking the api-enhanced HTTP contract) without touching the rest of the service.

## 3. Login and session

| Path | How | Notes |
|------|-----|-------|
| QR (default) | Server requests a unikey (`login/qr/key`), renders the QR (`login/qr/create`) in the PWA; the owner scans it with the NetEase phone app; server polls `login/qr/check` every 2 s: 801 waiting, 802 scanned, 803 confirmed (cookie returned), 800 expired. | No password or captcha on the server; works headless (C-8). |
| Cookie import | Owner pastes `MUSIC_U` from a browser session. | Fallback if QR endpoints break. |
| Phone + SMS / password | Supported by the libraries but disabled by default. | Password login triggers a captcha; both are flagged as risk-control (风控) sensitive. |

Persistence: cookie jar (including `MUSIC_U` and the `NMTID` device cookie, which must be
kept stable between sessions) stored at `<data>/netease/session.json`, encrypted with a key
derived from `/etc/machine-id` plus a config secret; file mode 0600, owned by the service
user. Requests carry the `os=pc` cookie so that quality levels behave consistently.
Login endpoints are called only on explicit user action, never in a retry loop: frequent
login calls are the documented way to get an account flagged. A background task calls
refresh once a day and marks the session `invalid` on a 301/302 "need login" answer, emitting
a `netease.session` event so the PWA shows "re-login needed". Requests originate from the
home IP; cloud/overseas IPs receive `-460 cheating` and are out of scope.

## 4. Quality ladder and entitlement

NetEase `song/url/v1` takes a `level`. Values and what they mean for a HiFi server:

| level | Typical payload | Use |
|-------|-----------------|-----|
| standard | MP3 128 kbps | last resort |
| higher | MP3 192 kbps | |
| exhigh | MP3 320 kbps | best lossy |
| lossless | FLAC 16/44.1 | default target |
| hires | FLAC 24-bit (44.1 to 192 kHz) | preferred when entitled (黑胶 VIP) |
| jymaster | "超清母带" (master) FLAC | optional, SVIP-gated, often 24/192 |
| jyeffect / sky / dolby / vivid | DSP-processed "surround", Dolby Atmos and "vivid" variants | excluded: not bit-faithful to the stereo release |

Rules:

1. Preference order is configurable. The default assumes an SVIP-tier account:
   `jymaster > hires > lossless > exhigh > higher > standard`; lower tiers simply never get
   the top levels granted and fall through the ladder. `jymaster` (超清母带) is a
   stereo master-quality FLAC rather than a DSP effect, so it belongs in the ladder; the
   effect variants (`jyeffect`, `sky`, `dolby`, `vivid`) stay excluded.
2. The response tells the level actually granted; the adapter records it and the UI shows it
   (FR-4.6 badge "NetEase FLAC 24/96").
3. Entitlement is known before asking for a URL: the song's `privilege` object carries
   `plLevel` (max play level), `dlLevel` (max download level), `maxBrLevel`, and the
   free/trial flags. The adapter requests exactly the highest permitted level, so trial
   clips are avoided rather than detected after the fact.
4. If a response nevertheless carries a **free-trial marker** (30 s clip), the song is flagged
   `trial` in the UI and skipped in "play all" unless the user opts in.
5. `fee`/`privilege` fields are cached per song for 6 h to avoid re-asking.

Measured on 2026-09-24 with the owner's SVIP account (`vipType 110`) against track 22605222,
via `ncmctl curl --kind weapi SongPlayerV1`:

| Requested level | Granted level | Bitrate | Format | Size |
|---|---|---|---|---|
| standard | standard | 128 kbps | mp3 | 3.6 MB |
| exhigh | exhigh | 320 kbps | mp3 | 9.1 MB |
| lossless | lossless | 990 kbps | flac | 28.2 MB |
| hires | **lossless** (downgraded) | 990 kbps | flac | 28.2 MB |
| jymaster | jymaster | 5.52 Mbps | flac 24/192 | 157 MB |

Two things are confirmed on real data. First, SVIP entitlement is genuine: `jymaster` resolves
to a 24-bit/192 kHz master. Second, `hires` silently degraded to `lossless` on a track with no
Hi-Res master while `jymaster` still succeeded, so the granted level is **not** a monotonic
function of the requested one. Rule 2 above (always read back the granted `level`) is therefore
mandatory rather than defensive, and the ladder must keep trying lower rungs on its own terms.

## 5. Audio delivery: the local stream proxy

MPD's queue must never hold raw CDN URLs (C-2: they expire in minutes and are bound to the
session). Instead every NetEase track is enqueued as

```
http://127.0.0.1:8080/stream/ncm/<songID>
```

Behaviour of `GET /stream/ncm/{id}`:

```
if local copy exists (downloaded earlier)      -> serve file, Range supported, correct Content-Type
else
   url = cache.get(id) or SongURL(level=permitted) ; cache until (expi - 60 s), expi is 1200 s today
   if config.stream.mode == "pipe"               -> pipe bytes, rewrite Content-Type (default, ADR-0008)
   else                                          -> 302 Location: <cdn url>        (redirect, lossy-only)
```

- **Pipe mode is the default (ADR-0008), because redirect mode cannot play lossless.**
  Measured on the target board on 2026-09-24: the NetEase CDN serves lossless payloads with
  `Content-Type: audio/mpeg; charset=UTF-8` even when the object really is FLAC (a `.flac`
  URL, 164 723 846 bytes, confirmed by `file` and `ffprobe` as FLAC 24-bit/192 kHz). MPD picks
  its decoder from the MIME type, selects `mad`, and aborts with
  `mad: input does not appear to be a mp3 bit stream`, then
  `avformat_open_input() failed: Invalid data found when processing input`. A 302 inherits the
  same wrong header because MPD follows it to the CDN, so redirect mode fails on every
  `lossless`, `hires` and `jymaster` track.
- The proxy therefore reads the bytes itself and re-sends `Content-Type: audio/flac` (or
  `audio/mpeg` for the genuinely lossy levels), passes `Content-Length` and `Content-Range`
  through, and honours `Range` — the CDN answers `HTTP/1.1 206 Partial Content`, so seeking
  and MPD's read-ahead still work. Verified end to end on the bench: MPD played a `jymaster`
  stream and ALSA delivered `S24_3LE @ 192000 Hz, 2 ch` bit-perfect to the ES9039 dongle at
  0.6 % CPU and 80 MB RSS.
- Redirect mode stays available as a config switch for lossy-only setups and for debugging.
  It is cheaper, but it must not be the default.
- **Live streaming uses a lower ladder than downloading.** Measured on the deployed board on
  2026-09-24: the link sustains ≈ 560–610 kB/s to the CDN, while a `jymaster` 24/192 master
  needs ≈ 690 kB/s continuously. Streaming one therefore starves the decoder, and MPD logs
  `alsa_output: Decoder is too slow; playing silence to avoid xrun` every few seconds. No
  buffer size fixes a sustained deficit — it only delays the first dropout. So `hifid` keeps
  two ladders: `level_preference` (downloads, `jymaster` first) and `stream_level_preference`
  (live, `lossless` first at ≈ 124 kB/s, a 4–5× margin on the same link).
- The proxy also reads ahead: a goroutine pulls from the CDN into a bounded 8 MB queue that the
  HTTP handler drains, so ordinary Wi-Fi jitter never reaches MPD. This absorbs stalls; it does
  not create bandwidth.
- A downloaded copy always wins. `POST /queue` with an `ncm:` ref checks the download index
  first and enqueues the plain `local:` path, so MPD reads the file from disk at full quality,
  seekable, with no network in the audio path at all.
- Tags: MPD reads tags from the FLAC/MP3 stream, but NetEase files often carry sparse tags.
  After `addid`, `hifid` issues MPD `addtagid` for Title, Artist, Album, Track, Date and stores
  the cover URL in its own queue-metadata map keyed by MPD song id. This is the officially
  supported MPD mechanism for tagging remote songs.
- Gapless: MPD opens the next queue item ahead of time; the proxy resolves a fresh URL at
  that moment, so expiry never hits mid-album.

## 6. Exporting NetEase playlists to plain MPD clients

To satisfy FR-2.8 (off-the-shelf MPD clients keep working), `hifid` regenerates MPD stored
playlists on login, on demand, and every 6 hours:

```
<music>/playlists/NetEase/我喜欢的音乐.m3u
<music>/playlists/NetEase/<playlist name>.m3u
#EXTM3U
#EXTINF:243,Artist - Title
http://127.0.0.1:8080/stream/ncm/123456
```

MPD's `extm3u` playlist plugin reads `#EXTINF` (duration and display title), so M.A.L.P.,
myMPD or `mpc` can load and play a NetEase playlist without the PWA. Search and download
still need the PWA/API.

## 7. Download pipeline (FR-1.5)

```
job = {songs[], level=best}
for each song (concurrency 2, polite):
   1. SongURL(level ladder)            -> url, size, md5, type
   2. GET -> <data>/incoming/<id>.<ext>  (resumable via Range if interrupted)
   3. verify size and md5
   4. write tags + cover:  Title, Artist(s), Album, AlbumArtist, Track, Disc, Date, Genre,
      NetEaseID (custom tag), embedded cover JPEG   (ffmpeg -c copy, or Go tag libraries)
   5. move to <music>/netease/<AlbumArtist>/<Album>/<NN - Title>.<flac|mp3>
   6. record in index (SQLite): id -> path, level, md5, downloaded_at
   7. mpd update netease/<AlbumArtist>/<Album>
   8. emit download.progress / download.done
```

- Files are never deleted by the service (NFR-8).
- The index makes the stream proxy prefer the local copy, so a downloaded playlist plays
  fully offline and bit-identical to the download.
- Users who already have `.ncm` files from the official client can convert them on the PC
  with an open-source `ncmdump` tool before uploading; `hifid` does not decrypt `.ncm`.

### 7.1 Offline sync (FR-1.7): keeping playlists on disk automatically

The main mode of use is playback from local files on an always-on box, so the download
pipeline is driven by a scheduler rather than only by manual taps:

```
sync config (PWA Settings → NetEase → Offline):
   subscriptions: [ liked, playlist:456, playlist:789 ]     # what to keep on disk
   level: best                                               # per subscription optional
   window: 01:00–07:00 local, plus "when idle" (no playback for 10 min)
   pace: 1 song per 15 s, max 600 songs per day, 2 concurrent transfers
   keep_removed: true                                        # never delete files when a song leaves a playlist (NFR-8)

scheduler loop (every 30 min, and on demand):
   for each subscription: fetch track ids (cached 10 min) → diff against the index
   enqueue missing ids into the download job "sync-<date>" (dedup against running jobs)
   run the job under the pace limits, only inside the window or when idle
   on network error: exponential back-off (1 min … 1 h), resume partial files by Range
   emit download.progress / download.done; PWA shows "offline: 1,203 / 1,240 tracks"
```

- The pace limits exist because of NetEase risk control (R18): bulk fetching at full speed
  from one account is the pattern that gets flagged. 600 lossless tracks per night is
  ≈ 18 GB, well inside what the XR819 link (1–3 MB/s) can move in six hours.
- Storage guard: the sync stops at 90 % disk usage and shows a warning (docs/05 §4).
- Stream proxy preference: once a track is on disk, `/stream/ncm/{id}` serves the local
  file, and queue items added from NetEase screens play from disk without any Wi-Fi.
- The exported `.m3u` playlists (§6) are regenerated after each sync so that M.A.L.P. also
  plays the local copies.

## 8. Rate limiting, caching, and politeness

| Data | Cache | Why |
|------|-------|-----|
| Playlist list | 10 min | UI opens it constantly |
| Playlist tracks | 10 min per playlist | pagination |
| Song detail | 6 h | metadata rarely changes |
| Song URL | until `expi - 60 s` | expiry is server-provided |
| Cover images | on disk, forever (small) | avoid re-fetch on every UI open |

- Song detail and URL requests are batched (the endpoints accept lists).
- Global limiter: at most 4 concurrent NetEase HTTP requests, 10 req/s burst.
- All caches live in memory plus a small SQLite file on the USB disk (C-7).
- No scrobbling, "listen count" automation, or other task-style traffic: NetEase's risk
  control is documented as strict in 2025–2026 and such automation is a known ban trigger.

## 9. Failure handling

| Failure | Behaviour |
|---------|-----------|
| Session invalid | `netease.session` event; catalogue endpoints return 401 with `code=ncm_login_required`; local library unaffected. |
| URL resolution fails for a queued song | Proxy returns 502; MPD skips to next song (its normal behaviour on stream errors) and `hifid` logs the song id. |
| API schema change | Adapter returns typed errors; UI shows "NetEase temporarily unavailable"; a regression test suite with recorded fixtures is part of the server tests. |
| CDN rejects redirect, or mislabels FLAC as `audio/mpeg` (observed 2026-09-24) | Pipe mode is the default (ADR-0008): it rewrites `Content-Type` and sends a browser-like User-Agent. |

## 10. Legal and etiquette notes (C-3)

- Single account, personal use, LAN only. No public exposure of the proxy.
- No "unblock" or license-circumvention modules in the default build.
- Downloads use exactly the files NetEase serves to the logged-in account; VIP-only content
  stays VIP-only.

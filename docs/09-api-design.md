# 09 · API Design (`hifid` REST + WebSocket)

Status: draft v0.1 · 2026-09-13 · addresses FR-2, FR-3, FR-5, NFR-5, NFR-7

One HTTP server on port 8080 (configurable) serves:

| Prefix | Purpose |
|--------|---------|
| `/` | PWA static assets (embedded) |
| `/api/v1/…` | JSON REST API described below |
| `/api/v1/ws` | WebSocket event stream |
| `/api/v1/upload/tus/` | tus 1.0.0 resumable upload endpoint |
| `/stream/ncm/{id}` | NetEase stream proxy used by MPD (loopback only by default) |

MPD's own protocol stays reachable on port 6600 for third-party clients. The same HTTP API
is additionally served on the unix socket `/run/hifid/api.sock` (no token required) for local
systemd units such as the DAC hot-plug hook.

## 1. Conventions

- JSON, UTF-8. Errors: `{"error":{"code":"ncm_login_required","message":"…","details":{}}}`.
- Auth: `Authorization: Bearer <token>`. `auth.mode` in config:
  `none` (trusted LAN), `admin` (token only for upload, settings, login, downloads), `all`.
  Default `admin`.
- IDs: MPD queue entries use MPD's numeric song id (`qid`). NetEase songs use `ncm:<id>`.
  Local files use `local:<path relative to music dir>`.
- Pagination: `?offset=&limit=`; responses carry `total` when known.
- All mutating transport calls return the new `/player` state to avoid a round-trip.

## 2. System

| Method | Path | Returns |
|--------|------|---------|
| GET | `/system/status` | version, arch, uptime, mpd {version, connected}, disk {music_total, music_free}, netease {logged_in, nickname, vip}, active_output, event counters |
| GET | `/system/log?lines=200` | recent structured log lines (admin) |
| POST | `/system/restart-mpd` | admin; used after output config changes that MPD cannot apply live |

## 3. Player (transport)

```
GET  /player
{
  "state": "play|pause|stop",
  "qid": 17, "pos": 3, "elapsed": 61.2, "duration": 243.0,
  "volume": 62, "volume_mode": "hardware|software|fixed",
  "repeat": false, "random": false, "single": false, "consume": false,
  "song": { ...same object as a queue item... },
  "format": {                       // what MPD is actually sending to the DAC (FR-4.6)
     "source": "dsd|pcm", "sample_rate": 2822400, "bits": 1, "channels": 2,
     "delivery": "native-dsd|dop|pcm|pcm-converted|pcm-resampled",
     "output_rate": 176400, "output_bits": 24
  },
  "output": { "id": "es9039-moondrop", "alias": "Moondrop Dawn Pro" }
}
POST /player/play        {"qid":17} | {"pos":3} | {}     -> state
POST /player/pause       {}                              -> state (toggle when playing)
POST /player/stop
POST /player/next  ·  POST /player/prev
POST /player/seek        {"seconds": 120.0} | {"delta": -10}
PUT  /player/options     {"repeat":true,"random":false,"single":false,"consume":false}
PUT  /player/volume      {"volume": 40} | {"delta": +5} | {"mute": true}
```

Volume rules (FR-2.2, ADR-0005): if the active output is in `fixed` mode the call returns
`409 volume_fixed` with a hint ("use the speaker knob"); if `hardware`, the value maps to the
DAC's USB mixer; `software` is refused while DSD is playing (`409 dsd_software_volume`).

## 4. Queue

```
GET    /queue                       -> { "version": 88, "items":[ QueueItem ] }
POST   /queue                       add
       { "items":[ {"ref":"ncm:123"}, {"ref":"ncm:playlist:456"}, {"ref":"local:Albums/x.flac"},
                   {"ref":"local:album", "artist":"…", "album":"…"} ],
         "mode":"append|next|replace", "play": true }
DELETE /queue                       clear
DELETE /queue/{qid}
POST   /queue/move                  {"qid":17,"to":0}
POST   /queue/shuffle
POST   /queue/save                  {"name":"Sunday"}          (MPD stored playlist)

QueueItem = { "qid":17, "pos":3, "ref":"ncm:123", "source":"ncm|local",
              "title":"…","artist":"…","album":"…","duration":243,
              "cover":"/api/v1/cover?ref=ncm:123", "quality_hint":"hires|lossless|…|null",
              "file_format": {"codec":"flac","sample_rate":96000,"bits":24} | null }
```

`ncm:playlist:<id>` expands server-side (batched detail fetch, `addid` + `addtagid`).
Large playlists are added in chunks of 100 with progress events.

## 5. Outputs (FR-5)

```
GET /outputs
[ { "id":"es9039-moondrop",            // stable id derived from USB vid:pid:serial (or user alias)
    "mpd_output_id": 1, "alias":"Moondrop Dawn Pro", "present": true, "enabled": true,
    "alsa": {"card":"DawnPro","device":"hw:CARD=DawnPro,DEV=0","vid":"0x2fc6","pid":"0xf06a"},
    "caps": { "pcm_rates":[44100,48000,88200,96000,176400,192000,352800,384000,705600,768000],
              "pcm_formats":["S16_LE","S24_3LE","S32_LE"],
              "native_dsd": true, "native_dsd_max":"DSD512", "dop": true, "dop_max":"DSD256",
              "hw_volume": true },
    "settings": { "dsd_mode":"auto|native|dop|pcm", "volume_mode":"hardware|software|fixed",
                  "max_rate": 768000 } } ]
PUT   /outputs/{id}/active            {}                // exclusive: enable this, disable others
PUT   /outputs/{id}/enabled           {"enabled":true}  // for simultaneous outputs (FR-5.6)
PATCH /outputs/{id}                   {"alias":"…","dsd_mode":"dop","volume_mode":"fixed","max_rate":192000}
POST  /outputs/rescan                 {}                // re-probe ALSA cards, regenerate MPD config if needed
```

`caps` come from `/proc/asound/card*/stream0` plus an ALSA `hw_params` probe (docs/04).
`settings` changes that need an MPD config change (dsd_mode, max_rate) are written to the
generated `mpd.conf` output block and applied with a controlled MPD restart when playback is
stopped, or on user confirmation.

## 6. NetEase

```
GET  /netease/session                -> {"logged_in":true,"user":{"id":1,"nickname":"…","avatar":"…","vip_type":11},"valid_since":"…"}
POST /netease/login/qr               -> {"key":"…","qr_url":"…","qr_svg":"<svg…>"}
GET  /netease/login/qr/{key}         -> {"status":"waiting|scanned|confirmed|expired"}
POST /netease/login/cookie           {"music_u":"…"}
POST /netease/logout
GET  /netease/me/playlists           -> [ {"id":456,"name":"…","count":120,"cover":"…","special":"liked|null"} ]
GET  /netease/me/liked?offset&limit  -> Song[]
GET  /netease/recommend/daily        -> Song[]
GET  /netease/playlist/{id}?offset&limit
GET  /netease/album/{id}
GET  /netease/artist/{id}/songs?offset&limit
GET  /netease/search?q=&type=song|album|artist|playlist&offset&limit
GET  /netease/cloud?offset&limit     -> CloudSong[]
GET  /netease/song/{id}              -> Song + {"levels_available":["standard","exhigh","lossless","hires"],"trial":false,"local_copy":"netease/…/x.flac"|null}
POST /netease/playlists/export       {}                 // regenerate MPD stored playlists (docs/08 §6)

Song = { "ref":"ncm:123","id":123,"title":"…","artists":[{"id":1,"name":"…"}],"album":{"id":9,"name":"…","cover":"…"},
         "duration":243,"fee":8,"max_level_hint":"hires" }
```

## 7. Downloads (FR-1.5, FR-2.4)

```
POST   /downloads                    {"items":[{"ref":"ncm:123"},{"ref":"ncm:playlist:456"}],"level":"best|hires|lossless|exhigh"}
                                     -> {"job":"d_20260913_0001","count":120}
GET    /downloads                    -> [ {"job":"…","state":"queued|running|done|failed|cancelled","done":17,"failed":1,"total":120,"bytes":…} ]
GET    /downloads/{job}              -> job + per-song rows
DELETE /downloads/{job}              cancel (files already completed stay)
```

## 8. Local library (FR-3.3)

```
GET  /library/browse?path=           -> { "dirs":[…], "files":[ LocalSong ] }     (MPD lsinfo)
GET  /library/artists                -> [ {"name":"…","albums":12} ]
GET  /library/albums?artist=         -> [ {"artist":"…","album":"…","date":"…","tracks":10,"cover":"…"} ]
GET  /library/album?artist=&album=   -> LocalSong[]
GET  /library/search?q=&limit=       -> LocalSong[]
GET  /library/recent?limit=50        -> albums ordered by added time (uploads show up here)
POST /library/update                 {"path":"local/…"|null}  -> {"job":123}
GET  /library/status                 -> {"updating":false,"songs":18342,"albums":1502,"artists":610,"db_updated":"…"}
GET  /cover?ref=local:…|ncm:…        -> image (MPD albumart/readpicture, or NetEase cover cache)
GET  /library/playlists              -> stored playlists (includes "NetEase/…")
POST /library/playlists/{name}/load  {"mode":"append|replace","play":true}
LocalSong = { "ref":"local:Albums/…/01.dsf","title":"…","artist":"…","album":"…","track":1,"duration":…,
              "format":{"codec":"dsf","sample_rate":2822400,"bits":1,"channels":2} }
```

## 9. Upload (FR-3)

The tus 1.0.0 protocol at `/api/v1/upload/tus/` (create with `POST`, chunks with `PATCH`,
offset with `HEAD`, cancel with `DELETE`; extensions: creation, termination, checksum,
expiration). Required metadata keys: `filename`, `relativePath` (optional folder prefix from
`webkitdirectory`), `sha256` (optional; if present the server verifies before finalizing).

Finalization (server side, automatic on last byte):

1. Reject if the extension is not an allowed audio/cover type.
2. Move from `<data>/incoming/` to `<music>/local/<relativePath>/<filename>`; if the target
   exists and differs, keep both (`name (2).ext`) and emit a `library.duplicate` warning.
3. Trigger `mpd update local/<relativePath>`.
4. Emit `upload.done`.

Plain multipart `POST /api/v1/upload/simple` is kept for scripts (`curl -F`), limited to
files below 512 MB.

`GET /api/v1/upload/sessions` lists in-progress uploads for the PWA to resume after a page
reload.

## 10. Settings

```
GET /settings
PUT /settings   { "server_name":"Living room", "auth_mode":"admin",
                  "netease": {"level_preference":["hires","lossless","exhigh"], "stream_mode":"redirect|pipe", "cache_while_playing":false},
                  "library": {"music_dir":"/srv/music", "auto_update": true},
                  "discovery": {"mdns": true} }
```

## 11. WebSocket events (`/api/v1/ws`)

Server pushes `{"type":"…","ts":…,"data":{…}}`; client may send `{"type":"ping"}` and
`{"type":"subscribe","topics":[…]}` (default all).

| type | data |
|------|------|
| `player` | full `/player` object (on any MPD `player`/`mixer`/`options` idle event, and every 1 s while playing for `elapsed`) |
| `queue` | `{"version":89}` (client re-fetches) or full list if < 200 items |
| `outputs` | full outputs list (hot-plug, enable/disable, settings) |
| `netease.session` | `{"logged_in":…}` |
| `download.progress` | `{"job":"…","done":…,"total":…,"current":{"ref":"…","pct":…}}` |
| `download.done` | job summary |
| `upload.progress` / `upload.done` | `{"id":"…","filename":"…","offset":…,"size":…}` |
| `library.update` | `{"updating":true|false,"job":…}` |
| `library.duplicate` | `{"path":"…","existing":"…"}` |
| `error` | `{"code":"…","message":"…"}` for background failures |

## 12. Discovery (FR-2.6)

- DNS-SD service `_hifid._tcp.local.` port 8080, TXT: `v=1`, `api=/api/v1`, `name=<server_name>`,
  `id=<machine id prefix>`. Also `_http._tcp` so generic browsers list it.
- UDP fallback: broadcast `HIFID-DISCOVER` to port 8081 → reply `{"name":…,"url":"http://<ip>:8080"}`.
- The PWA cannot browse mDNS; it relies on the user opening `http://<hostname>.local:8080` once,
  then remembers the URL. A native app uses Android NSD.

## 13. Versioning and compatibility

- Breaking changes bump `/api/v2`; `/api/v1` kept for one release.
- `GET /api/v1/openapi.json` serves the OpenAPI 3 document generated from the handlers
  (source of truth for the PWA and any native app).

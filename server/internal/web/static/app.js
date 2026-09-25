// Minimal PWA controller for hifid (M1). State always comes from the server;
// the WebSocket keeps it live (docs/06, docs/09 §11).
const $ = (id) => document.getElementById(id);
const API = "/api/v1";

const mmss = (s) => {
  s = Math.max(0, Math.floor(s || 0));
  return Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0");
};

let dragging = false;

async function call(path, method = "POST", body) {
  const r = await fetch(API + path, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!r.ok) return null;
  return r.json();
}

function fmtLabel(f) {
  if (!f) return "—";
  if (f.delivery === "native-dsd") {
    const dsd = Math.round(f.sample_rate / 44100);
    return `DSD${dsd} native`;
  }
  const khz = (f.sample_rate / 1000).toFixed(1).replace(/\.0$/, "");
  let s = `PCM ${f.bits || "?"}/${khz}k`;
  if (f.output_format) s += ` · ${f.output_format}`;
  return s;
}

function render(st) {
  if (!st) return;
  $("dot").className = "dot" + (st.connected ? " ok" : "");
  const playing = st.state === "play";
  const pp = $("playpause");
  pp.textContent = playing ? "⏸" : "▶";
  pp.setAttribute("aria-label", playing ? "Pause" : "Play");
  const song = st.song || {};
  $("title").textContent = song.title || "Nothing playing";
  $("artist").textContent = [song.artist, song.album].filter(Boolean).join(" — ");
  $("fmt").textContent = fmtLabel(st.format);
  const dur = st.duration || song.duration || 0;
  $("elapsed").textContent = mmss(st.elapsed);
  $("duration").textContent = mmss(dur);
  $("qpos").textContent = st.queue_length > 1 ? `${(st.pos || 0) + 1} / ${st.queue_length}` : "";
  $("prog").style.width = dur ? Math.min(100, (st.elapsed / dur) * 100) + "%" : "0";
  if (!dragging && st.volume >= 0) {
    $("vol").value = st.volume;
    $("volval").textContent = st.volume + "%";
  }
  $("repeat").classList.toggle("on", !!st.repeat);
  $("repeat").setAttribute("aria-pressed", String(!!st.repeat));
  $("random").classList.toggle("on", !!st.random);
  $("random").setAttribute("aria-pressed", String(!!st.random));
  lastDur = dur;
  markPlaying(st);
}
let lastDur = 0;

async function refresh() {
  render(await call("/player", "GET"));
}

async function loadOutputs() {
  const outs = await call("/outputs", "GET");
  if (!outs) return;
  const sel = $("outputs");
  sel.innerHTML = "";
  outs.forEach((o) => {
    const opt = document.createElement("option");
    opt.value = o.mpd_output_id;
    opt.textContent = o.alias;
    opt.selected = o.enabled;
    sel.appendChild(opt);
  });
}

$("prev").onclick = async () => render(await call("/player/prev"));
$("next").onclick = async () => render(await call("/player/next"));
$("stop").onclick = async () => render(await call("/player/stop"));
$("playpause").onclick = async () => {
  const st = await call("/player", "GET");
  render(st && st.state === "stop" ? await call("/player/play", "POST", {}) : await call("/player/pause"));
};

$("repeat").onclick = async () => render(await call("/player/options", "PUT", { repeat: !$("repeat").classList.contains("on") }));
$("random").onclick = async () => render(await call("/player/options", "PUT", { random: !$("random").classList.contains("on") }));

// tap anywhere on the progress bar to seek within the current track
$("seek").onclick = async (e) => {
  if (!lastDur) return;
  const r = e.currentTarget.getBoundingClientRect();
  const frac = Math.min(1, Math.max(0, (e.clientX - r.left) / r.width));
  $("prog").style.width = frac * 100 + "%";
  render(await call("/player/seek", "POST", { seconds: Math.floor(frac * lastDur) }));
};

$("poweroff").onclick = async () => {
  if (!confirm("Power off HiFi Server? Playback stops and the board halts; unplug and replug power to start it again.")) return;
  const res = await call("/system/poweroff", "POST", {});
  if (res && res.ok) {
    $("title").textContent = "Shutting down…";
    $("artist").textContent = "Wait for the board LED to go off before unplugging.";
    $("dot").className = "dot";
  }
};

$("vol").oninput = (e) => {
  dragging = true;
  $("volval").textContent = e.target.value + "%";
};
$("vol").onchange = async (e) => {
  dragging = false;
  render(await call("/player/volume", "PUT", { volume: Number(e.target.value) }));
};

$("outputs").onchange = async (e) => {
  await call(`/outputs/${e.target.value}/active`, "PUT", {});
  await loadOutputs();
  refresh();
};

function connect() {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  const ws = new WebSocket(`${proto}//${location.host}${API}/ws`);
  ws.onmessage = (ev) => {
    try {
      const msg = JSON.parse(ev.data);
      if (msg.state) render(msg.state);
      if (msg.type === "output") loadOutputs();
    } catch (_) {}
  };
  ws.onclose = () => setTimeout(connect, 2000); // survive Wi-Fi blips
}

fetch(API + "/system/status")
  .then((r) => r.json())
  .then((s) => { if (s.server_name) $("server").textContent = s.server_name; })
  .catch(() => {});

refresh();
loadOutputs();
connect();
setInterval(refresh, 1000); // keeps the elapsed counter moving between events

// ---- NetEase browsing (M2) -------------------------------------------------
// Two levels: playlists, then the tracks of one playlist. Tapping a track
// replaces the queue and plays it; the server resolves the quality ladder and
// enqueues its own /stream/ncm/<id> proxy URL, never a raw CDN URL.
const bList = $("browse-list");
const bTitle = $("browse-title");
const bStatus = $("browse-status");
const bBack = $("browse-back");

const img = (src) =>
  src ? `<img src="${src.replace(/^http:/, "https:")}?param=80y80" alt="" loading="lazy">` : `<img alt="">`;

function setStatus(text) {
  bStatus.textContent = text || "";
  bStatus.hidden = !text;
}

async function showPlaylists() {
  viewingPlaylist = null;
  reloadTracks = null;
  setPlayAll(null);
  refreshDownloads();
  bTitle.textContent = "NetEase";
  bBack.hidden = true;
  setStatus("loading playlists…");
  bList.innerHTML = "";
  const r = await call("/netease/playlists?limit=60", "GET");
  if (!r || !r.items) {
    setStatus("NetEase unavailable — check the session on the server.");
    return;
  }
  hideLoginPanel();
  setStatus(`${r.items.length} playlists`);
  {
    const li = document.createElement("li");
    li.innerHTML = `<div class="ico">📅</div><div class="txt"><div class="n">Daily picks</div><div class="s">每日推荐 · today's 30 for you</div></div>`;
    li.onclick = () => showDaily();
    const play = document.createElement("button");
    play.className = "act";
    play.textContent = "▶";
    play.title = "play today's picks";
    play.onclick = async (e) => {
      e.stopPropagation();
      play.textContent = "…";
      const d = await call("/netease/daily", "GET");
      const st = d && d.items && d.items.length ? await call("/queue", "POST", { items: d.items.map(trackItem), mode: "replace", play: true }) : null;
      play.textContent = "▶";
      if (st) render(st);
    };
    li.appendChild(play);
    bList.appendChild(li);
  }
  r.items.forEach((p) => {
    const li = document.createElement("li");
    li.innerHTML = `${img(p.cover)}<div class="txt"><div class="n">${p.liked ? "★ " : ""}${p.name}</div><div class="s">${p.track_count} tracks</div></div>`;
    li.onclick = () => showTracks(p.id, p.name);
    const play = document.createElement("button");
    play.className = "act";
    play.textContent = "▶";
    play.title = `play all ${p.track_count} tracks`;
    play.onclick = async (e) => {
      e.stopPropagation();
      play.textContent = "…";
      const st = await call("/queue", "POST", { items: [{ ref: "ncm:playlist:" + p.id }], mode: "replace", play: true });
      play.textContent = "▶";
      if (st) render(st);
    };
    li.appendChild(play);
    bList.appendChild(li);
  });
}

let reloadTracks = null;

async function showTracks(id, name) {
  viewingPlaylist = id;
  bBack.onclick = () => showPlaylists();
  reloadTracks = () => showTracks(id, name);
  refreshDownloads();
  bTitle.textContent = name;
  bBack.hidden = false;
  setStatus("loading tracks…");
  bList.innerHTML = "";
  const r = await call(`/netease/playlists/${id}/tracks?limit=100`, "GET");
  if (!r || !r.items) {
    setStatus("could not load tracks");
    return;
  }
  const count = () => setStatus(`${r.items.length} of ${r.total} tracks`);
  count();
  setPlayAll(async () => {
    setStatus(`queueing all ${r.total} tracks…`);
    const st = await call("/queue", "POST", { items: [{ ref: "ncm:playlist:" + id }], mode: "replace", play: true });
    count();
    return st;
  });
  r.items.forEach((t) => {
    const li = document.createElement("li");
    const txt = document.createElement("div");
    txt.className = "txt";
    txt.innerHTML = `<div class="n">${t.title}</div><div class="s">${t.artist}</div>`;
    li.innerHTML = img(t.cover);
    li.appendChild(txt);

    const dl = document.createElement("button");
    dl.className = "act dlbtn";
    dl.dataset.ncm = t.id;
    if (t.on_disk) dl.dataset.state = "done";
    paintDlButton(dl);
    dl.onclick = async (e) => {
      e.stopPropagation();
      if (dl.dataset.state === "done") return;
      dl.textContent = "…";
      const r = await call("/netease/download", "POST", { ref: t.ref });
      if (r && r.on_disk) dl.dataset.state = "done";
      await refreshDownloads(); // repaints this button from the server's view
    };
    li.appendChild(dl);
    li.dataset.ncm = t.id;

    txt.onclick = async () => {
      // Already in the queue (e.g. after Play all)? Jump there: instant, and
      // the rest of the collection stays. Otherwise rebuild from this track.
      const from = r.items.indexOf(t);
      const items = r.items.slice(from).map((x) => ({ ref: x.ref, title: x.title, artist: x.artist, album: x.album }));
      await playHere(li, t.ref, items, t.title, count);
    };
    bList.appendChild(li);
  });
  refresh(); // mark the playing row straight away
}


fetch(API + "/netease/status")
  .then((r) => r.json())
  .then((s) => {
    if (s && s.logged_in) showPlaylists();
    else setStatus("NetEase not logged in on the server.");
  })
  .catch(() => setStatus("NetEase unavailable."));


// ---- download list ---------------------------------------------------------
// Nothing downloads by itself. Playing streams at the lower quality ladder;
// only what is added here is fetched, at the download ladder. Same split as
// the desktop client's 音质播放设置 / 音质下载设置.
const dlLine = $("dl-status");
const dlBtn = $("dl-playlist");
let viewingPlaylist = null;
let queuedIDs = new Set();

let currentDlID = 0, lastDoneID = 0, failedIDs = new Set();
function renderDownloads(s) {
  if (!s) { dlLine.textContent = ""; return; }
  queuedIDs = new Set((s.pending || []).map((i) => i.id));
  failedIDs = new Set((s.failed || []).map((i) => i.id));
  currentDlID = s.current_item ? s.current_item.id : 0;
  lastDoneID = s.last_done ? s.last_done.id : 0;
  updateRowButtons();
  const bits = [`${s.total_on_disk} on disk` + (s.on_disk_bytes ? ` (${fmtBytes(s.on_disk_bytes)})` : "")];
  if (s.current) bits.push(`downloading: ${s.current}`);
  if (s.pending && s.pending.length) bits.push(`${s.pending.length} queued`);
  if (s.failed && s.failed.length) bits.push(`${s.failed.length} failed`);
  dlLine.textContent = bits.join(" · ");
  dlBtn.hidden = viewingPlaylist === null;
}

async function refreshDownloads() {
  renderDownloads(await call("/netease/downloads", "GET"));
}

dlBtn.onclick = async () => {
  if (viewingPlaylist === null) return;
  const label = dlBtn.textContent;
  dlBtn.textContent = "…";
  const r = await call("/netease/download/playlist", "POST", { playlist_id: viewingPlaylist });
  dlBtn.textContent = label;
  if (r) await refreshDownloads();
};

refreshDownloads();
setInterval(refreshDownloads, 5000);

// ---- local library (Samba uploads + NetEase downloads as files) -------------
// MPD indexes both trees, so one browser covers everything already on disk.
const tabNcm = $("tab-ncm");
const tabLib = $("tab-lib");
const libSearch = $("lib-search");
let mode = "ncm";
let libPath = "";

function setMode(m) {
  mode = m;
  tabNcm.classList.toggle("on", m === "ncm");
  tabLib.classList.toggle("on", m === "lib");
  tabDl.classList.toggle("on", m === "dl");
  libSearch.hidden = m !== "lib";
  ncmSearch.hidden = m !== "ncm";
  if (m !== "ncm") ncmSearch.value = "";
  if (m !== "lib") libSummary.hidden = true;
  dlPanel.hidden = m !== "dl";
  if (m !== "dl") clearInterval(dlTimer);
  dlBtn.hidden = true;
  setPlayAll(null);
  if (m === "ncm") { libSearch.value = ""; showPlaylists(); }
  else if (m === "lib") { libPath = ""; showLibrary(""); }
  else showDownloads();
}

// The two roots are both "on disk"; what differs is where the music came from.
// Directory names on disk stay local/ and netease/ (the Samba share, installer,
// download index and docs all refer to them); only the display changes.
const ROOTS = {
  local:   { name: "Uploads",           sub: "copied from your PC over Samba",  icon: "💻" },
  netease: { name: "NetEase downloads", sub: "fetched from your download list", icon: "☁️" },
};
const fileIcon = (p) => (/\.(dsf|dff)$/i.test(p) ? "◉" : "🎵");

function libRow(e) {
  const li = document.createElement("li");
  const isDir = e.type === "directory";
  const root = isDir ? ROOTS[e.path] : null;
  const name = isDir ? (root ? root.name : e.name) : e.title;
  const sub = isDir ? (root ? root.sub : "folder")
    : [e.artist, e.album].filter(Boolean).join(" — ") || e.path.replace(/\/[^/]*$/, "");
  const icon = isDir ? (root ? root.icon : "📁") : fileIcon(e.path);
  li.innerHTML = `<div class="ico">${icon}</div>`;
  const txt = document.createElement("div");
  txt.className = "txt";
  txt.innerHTML = `<div class="n">${name}</div><div class="s">${sub}</div>`;
  li.appendChild(txt);
  if (!isDir) li.dataset.path = e.path;
  if (!isDir) {
    const m = document.createElement("div");
    m.className = "meta";
    const codec = (e.path.split(".").pop() || "").toUpperCase();
    const line2 = [codec, fmtMpdFormat(e.format), e.duration ? mmss(e.duration) : "", kbps(e.size, e.duration)]
      .filter(Boolean).join(" · ");
    m.innerHTML = "<b>" + fmtBytes(e.size) + "</b>" + (line2 ? "<br>" + line2 : "");
    li.appendChild(m);
  }
  li.onclick = async () => {
    if (isDir) { showLibrary(e.path); return; }
    const siblings = currentEntries.filter((x) => x.type === "file");
    const from = Math.max(0, siblings.indexOf(e));
    const items = siblings.slice(from).map((x) => ({ ref: "local:" + x.path }));
    await playHere(li, "local:" + e.path, items, e.title, () => setStatus(""));
  };
  return li;
}

async function showLibrary(p) {
  libPath = p;
  bTitle.textContent = p
    ? p.split("/").map((seg, i) => (i === 0 && ROOTS[seg] ? ROOTS[seg].name : seg)).join(" / ")
    : "All music on disk";
  setStatus("loading…");
  bList.innerHTML = "";
  const r = await call("/library?path=" + encodeURIComponent(p), "GET");
  if (!r) { setStatus("library unavailable"); return; }
  bBack.hidden = r.at_root;
  bBack.onclick = () => (r.at_root ? setMode("lib") : showLibrary(r.parent));
  const files = r.items.filter((e) => e.type === "file");
  const bytes = files.reduce((a, e) => a + (e.size || 0), 0);
  const secs = files.reduce((a, e) => a + (e.duration || 0), 0);
  setStatus(`${r.items.length} item(s)` + (files.length ? ` · ${files.length} file(s) · ${fmtBytes(bytes)} · ${mmss(secs)}` : ""));
  refreshLibSummary();
  currentEntries = r.items;
  setPlayAll(files.length ? async () => call("/queue", "POST", {
    items: files.map((x) => ({ ref: "local:" + x.path })), mode: "replace", play: true,
  }) : null);
  r.items.forEach((e) => bList.appendChild(libRow(e)));
}

async function runLibSearch(q) {
  bTitle.textContent = `Search: ${q}`;
  setStatus("searching…");
  bList.innerHTML = "";
  const r = await call("/library/search?q=" + encodeURIComponent(q), "GET");
  if (!r) { setStatus("search failed"); return; }
  bBack.hidden = false;
  bBack.onclick = () => showLibrary(libPath);
  setStatus(`${r.items.length} result(s) · ${fmtBytes(r.items.reduce((a, e) => a + (e.size || 0), 0))}`);
  currentEntries = r.items;
  setPlayAll(null);
  r.items.forEach((e) => bList.appendChild(libRow(e)));
  refresh();
}

let searchTimer = null;
libSearch.oninput = (e) => {
  const q = e.target.value.trim();
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => (q ? runLibSearch(q) : showLibrary(libPath)), 350);
};

tabNcm.onclick = () => setMode("ncm");
tabLib.onclick = () => setMode("lib");

// ---- QR login -----------------------------------------------------------------
// The phone app scans a code rendered by the server; nothing but the session
// cookie ever reaches hifid (FR-1.1).
const loginPanel = $("login-panel");
const loginMsg = $("login-msg");
const loginBtn = $("login-btn");
const loginQr = $("login-qr");
let loginPoll = null;

function showLoginPanel(msg) {
  loginPanel.hidden = false;
  loginMsg.textContent = msg || "Not logged in to NetEase.";
  loginBtn.hidden = false;
  loginQr.hidden = true;
  bList.innerHTML = "";
  setStatus("");
}

function hideLoginPanel() {
  loginPanel.hidden = true;
  clearInterval(loginPoll);
  loginPoll = null;
}

loginBtn.onclick = async () => {
  loginBtn.hidden = true;
  loginMsg.textContent = "starting…";
  const r = await call("/netease/login/qr", "POST", {});
  if (!r || !r.key) { showLoginPanel("Could not start login. Is the server online?"); return; }
  loginQr.src = r.image + "?t=" + Date.now();
  loginQr.hidden = false;
  loginMsg.textContent = "Scan with the NetEase Cloud Music app, then confirm on the phone.";
  clearInterval(loginPoll);
  loginPoll = setInterval(async () => {
    const st = await call(`/netease/login/qr/${r.key}`, "GET");
    if (!st) return;
    if (st.status === "scanned") loginMsg.textContent = "Scanned — confirm on your phone.";
    else if (st.status === "expired") { showLoginPanel("Code expired. Try again."); }
    else if (st.status === "ok") {
      hideLoginPanel();
      setStatus("logged in");
      showPlaylists();
    }
  }, 2000);
};

// Replace the boot-time status check: show the login panel instead of a dead end.
fetch(API + "/netease/status")
  .then((r) => r.json())
  .then((s) => { if (!(s && s.logged_in)) showLoginPanel(); })
  .catch(() => showLoginPanel("NetEase unavailable."));

// ---- sizes and storage ---------------------------------------------------------
const fmtBytes = (n) => {
  if (!n && n !== 0) return "";
  const u = ["B", "kB", "MB", "GB", "TB"]; let i = 0; let v = n;
  while (v >= 1000 && i < u.length - 1) { v /= 1000; i++; }
  return (i === 0 ? v : v.toFixed(v >= 100 ? 0 : 1)) + " " + u[i];
};
// "192000:24:2" -> "24/192k"; "dsd256:2" -> "DSD256"
const fmtMpdFormat = (f) => {
  if (!f) return "";
  const p = f.split(":");
  if (p[0].startsWith("dsd")) return p[0].toUpperCase();
  const khz = (+p[0] / 1000).toFixed(1).replace(/\.0$/, "");
  return (p[1] && p[1] !== "f" ? p[1] + "/" : "") + khz + "k";
};
const kbps = (size, dur) => (size && dur ? Math.round((size * 8) / dur / 1000) + " kbps" : "");

const libSummary = $("lib-summary");
async function refreshLibSummary() {
  if (mode !== "lib") { libSummary.hidden = true; return; }
  const s = await call("/library/stats", "GET");
  if (!s) { libSummary.hidden = true; return; }
  const t = s.trees || {};
  const parts = [`${s.songs || 0} tracks`];
  const onDisk = (t.netease?.bytes || 0) + (t.local?.bytes || 0);
  parts.push(`${fmtBytes(onDisk)} on disk (downloads ${fmtBytes(t.netease?.bytes || 0)}, uploads ${fmtBytes(t.local?.bytes || 0)})`);
  if (s.disk && s.disk.total) parts.push(`${fmtBytes(s.disk.free)} free of ${fmtBytes(s.disk.total)}`);
  libSummary.textContent = parts.join(" · ");
  libSummary.hidden = false;
}
setInterval(refreshLibSummary, 30000);

// ---- volume fine-tuning: ±1 % steps via the API's delta form -------------------
const nudge = async (d) => render(await call("/player/volume", "PUT", { delta: d }));
$("vol-down").onclick = () => nudge(-1);
$("vol-up").onclick = () => nudge(+1);

// ---- play all / play from here --------------------------------------------------
let currentEntries = [];   // library rows currently shown (for play-from-here)
const playAllBtn = $("play-all");
let playAllAction = null;
function setPlayAll(fn) {
  playAllAction = fn;
  playAllBtn.hidden = !fn;
}
playAllBtn.onclick = async () => {
  if (!playAllAction) return;
  const label = playAllBtn.textContent;
  playAllBtn.textContent = "…";
  const st = await playAllAction();
  playAllBtn.textContent = label;
  if (st) render(st);
};

// ---- Downloads tab ---------------------------------------------------------------
const tabDl = $("tab-dl");
const dlPanel = $("dl-panel");
const dlNow = $("dl-now");
const dlProg = $("dl-prog");
const dlPct = $("dl-pct");
const dlPause = $("dl-pause");
let dlTimer = null;

function renderDlPanel(s) {
  if (!s) return;
  const cur = s.current_item;
  if (cur) {
    const pct = s.current_size ? Math.min(100, (100 * s.current_bytes) / s.current_size) : 0;
    dlNow.textContent = `Downloading: ${cur.artist} — ${cur.title}`;
    dlProg.style.width = pct + "%";
    dlPct.textContent = s.current_size
      ? `${fmtBytes(s.current_bytes)} of ${fmtBytes(s.current_size)} (${pct.toFixed(0)}%)`
      : fmtBytes(s.current_bytes || 0);
  } else {
    dlNow.textContent = s.paused ? "Paused." : "Nothing downloading.";
    dlProg.style.width = "0";
    dlPct.textContent = "";
  }
  dlPause.textContent = s.paused ? "▶ Resume" : "⏸ Pause";
  dlPause.classList.toggle("busy", !!s.paused);
  bList.innerHTML = "";
  const add = (label, it, cls) => {
    const li = document.createElement("li");
    li.innerHTML = `<img alt=""><div class="txt"><div class="n">${it.title || it.id}</div><div class="s">${it.artist || ""}</div></div><div class="meta ${cls || ""}">${label}</div>`;
    bList.appendChild(li);
  };
  (s.pending || []).forEach((it) => add("queued", it));
  (s.failed || []).forEach((it) => add("failed" + (it.tries ? " x" + it.tries : ""), it, "err"));
  const nPend = (s.pending || []).length, nFail = (s.failed || []).length;
  setStatus(`${s.paused ? "PAUSED · " : ""}${s.done} finished this session · ${nPend} queued · ${nFail} failed · ${s.total_on_disk} on disk (${fmtBytes(s.on_disk_bytes || 0)})`);
}

async function showDownloads() {
  bTitle.textContent = "Download list";
  bBack.hidden = true;
  dlPanel.hidden = false;
  renderDlPanel(await call("/netease/downloads", "GET"));
  clearInterval(dlTimer);
  dlTimer = setInterval(async () => {
    if (mode === "dl") renderDlPanel(await call("/netease/downloads", "GET"));
  }, 2000);
}
$("dl-clear").onclick = async () => renderDlPanel(await call("/netease/downloads", "DELETE"));
dlPause.onclick = async () => {
  const paused = dlPause.classList.contains("busy");
  renderDlPanel(await call(paused ? "/netease/downloads/resume" : "/netease/downloads/pause", "POST"));
};
tabDl.onclick = () => setMode("dl");

// ---- now-playing marker in lists ------------------------------------------------
// A NetEase row matches by song id (streams carry it in the URL, downloads are
// reverse-mapped by the server); a Library row matches by path.
let lastPlayingKey = "";
function markPlaying(st) {
  const song = st && st.state !== "stop" ? st.song : null;
  const id = song && song.ncm_id ? String(song.ncm_id) : "";
  const path = song && song.ref && song.ref.startsWith("local:") ? song.ref.slice(6) : "";
  let hit = null;
  bList.querySelectorAll("li").forEach((li) => {
    const on = (id && li.dataset.ncm === id) || (path && li.dataset.path === path);
    li.classList.toggle("playing", !!on);
    if (on) hit = li;
  });
  const key = hit ? (id || path) : "";
  if (hit && key !== lastPlayingKey) hit.scrollIntoView({ block: "nearest" });
  lastPlayingKey = key;
}

// ---- download button states, painted from the server's list --------------------
// done: on disk · downloading: in flight · queued: waiting · failed: gave up · idle
function dlStateFor(id, btn) {
  if (btn.dataset.state === "done" || id === lastDoneID) return "done";
  if (id === currentDlID) return "downloading";
  if (queuedIDs.has(id)) return "queued";
  if (failedIDs.has(id)) return "failed";
  // it was in the list a moment ago and is in no list now: it finished
  if (btn.dataset.state === "downloading" || btn.dataset.state === "queued") return "done";
  return "idle";
}
const DL_GLYPH = { done: "✓", downloading: "⬇", queued: "⋯", failed: "!", idle: "↓" };
const DL_TITLE = {
  done: "on disk", downloading: "downloading now", queued: "in the download list",
  failed: "download failed", idle: "add to the download list",
};
function paintDlButton(btn) {
  const id = Number(btn.dataset.ncm);
  const st = dlStateFor(id, btn);
  btn.dataset.state = st;
  btn.textContent = DL_GLYPH[st];
  btn.title = DL_TITLE[st];
  btn.classList.toggle("done", st === "done");
  btn.classList.toggle("busy", st === "downloading");
}
function updateRowButtons() {
  document.querySelectorAll("button.dlbtn").forEach(paintDlButton);
}

// ---- tap-to-play: jump if queued, else rebuild from here -------------------------
// One request at a time: a second tap while the first is running used to start
// a second rebuild and double the queue.
let queueBusy = false;
async function playHere(row, ref, items, title, done) {
  if (queueBusy) return;
  queueBusy = true;
  // immediate feedback on the row itself, before the server answers
  bList.querySelectorAll("li.playing").forEach((x) => x.classList.remove("playing"));
  row.classList.add("playing");
  try {
    let st = await call("/player/jump", "POST", { ref });
    if (!st) {
      setStatus(`queueing ${items.length} track(s) from "${title}"…`);
      st = await call("/queue", "POST", { items, mode: "replace", play: true });
    }
    if (st) render(st); else setStatus(`could not play "${title}"`);
  } finally {
    queueBusy = false;
    if (done) done();
  }
}


// ---- board status tiles --------------------------------------------------------
// Fixed-width tiles (values live in fixed character boxes) so nothing shifts as
// numbers change; sparklines keep the last two minutes at a 3 s cadence.
const sysEl = $("sys");
const HIST_N = 40;
const hist = { cpu: [], temp: [], rx: [] };
const push = (k, v) => { hist[k].push(v); if (hist[k].length > HIST_N) hist[k].shift(); };
const padL = (s, n) => String(s).padStart(n, " ");

function spark(vals, lo, hi, w = 64, h = 22) {
  if (vals.length < 2) return `<svg class="spark" viewBox="0 0 ${w} ${h}"></svg>`;
  const step = w / (HIST_N - 1), off = HIST_N - vals.length;
  const y = (v) => (h - 1 - ((Math.min(Math.max(v, lo), hi) - lo) / (hi - lo)) * (h - 3)).toFixed(1);
  const pts = vals.map((v, i) => `${((off + i) * step).toFixed(1)},${y(v)}`);
  const x0 = (off * step).toFixed(1), x1 = ((off + vals.length - 1) * step).toFixed(1);
  return `<svg class="spark" viewBox="0 0 ${w} ${h}"><polygon points="${x0},${h} ${pts.join(" ")} ${x1},${h}"/><polyline points="${pts.join(" ")}"/></svg>`;
}
const rateFixed = (bps) => { // always 8 chars: "  1.3 MB" / " 12  kB" style, unit /s implied
  if (bps >= 1e6) return padL((bps / 1e6).toFixed(1) + " MB", 8);
  if (bps >= 1e3) return padL(Math.round(bps / 1e3) + " kB", 8);
  return padL(Math.round(bps) + " B", 8);
};

const tile = (cls, label, value, sub, ind) =>
  `<div class="tile ${cls}"><div class="l">${label}</div><div class="v">${value}</div><div class="s">${sub}</div><div class="ind">${ind}</div></div>`;

async function refreshSys() {
  const s = await call("/system/stats", "GET");
  if (!s || s.mem_total === undefined) { sysEl.innerHTML = ""; return; }
  push("cpu", s.cpu_percent); push("temp", s.temp_c || 0); push("rx", s.net_rx_bps);
  const tempCls = s.temp_c >= 80 ? "hot" : s.temp_c >= 70 ? "warn" : "";
  const cpuCls = s.cpu_percent >= 85 ? "warn" : "";
  const rxMax = Math.max(50e3, ...hist.rx);
  const bars = s.wifi_dbm >= -55 ? 4 : s.wifi_dbm >= -65 ? 3 : s.wifi_dbm >= -75 ? 2 : 1;
  const sig = `<div class="sig">${[1, 2, 3, 4].map((i) => `<i class="${i <= bars ? "on" : ""}"></i>`).join("")}</div>`;
  const ramPct = (100 * s.mem_used) / s.mem_total;
  sysEl.innerHTML = [
    tile(cpuCls, "cpu", padL(s.cpu_percent.toFixed(0), 3) + " %",
      `${padL((s.cpu_mhz / 1000).toFixed(1), 4)} GHz  ld ${s.load1.toFixed(2)}`, spark(hist.cpu, 0, 100)),
    tile("", "ram", padL(fmtBytes(s.mem_used), 7),
      `of ${fmtBytes(s.mem_total)}  ${padL(ramPct.toFixed(0), 3)} %`, `<div class="bar2"><i style="width:${ramPct.toFixed(0)}%"></i></div>`),
    tile(tempCls, "soc", padL((s.temp_c || 0).toFixed(0), 3) + " °C",
      (s.temp_zone || "").replace("-thermal", "").padEnd(12), spark(hist.temp, 30, 90)),
    tile("", "wi-fi ↓", rateFixed(s.net_rx_bps) + "/s",
      `↑${rateFixed(s.net_tx_bps)}/s  ${padL(s.wifi_dbm || 0, 4)} dBm`,
      `<div class="indcol">${sig}${spark(hist.rx, 0, rxMax)}</div>`),
  ].join("");
}
refreshSys();
setInterval(refreshSys, 3000);

// ---- NetEase search --------------------------------------------------------------
// Results render with the same rows as a playlist: tap to play (live, at the
// streaming ladder, or from disk if downloaded), ↓ to add to the download list.
const ncmSearch = $("ncm-search");
let ncmSearchTimer = null;

function renderTrackRows(items, statusText) {
  // shared renderer for playlist pages and search results
  bList.innerHTML = "";
  const r = { items, total: items.length };
  const count = () => setStatus(statusText || `${items.length} track(s)`);
  count();
  items.forEach((t) => {
    const li = document.createElement("li");
    const txt = document.createElement("div");
    txt.className = "txt";
    txt.innerHTML = `<div class="n">${t.title}</div><div class="s">${t.artist}${t.album ? " — " + t.album : ""}</div>`;
    li.innerHTML = img(t.cover);
    li.appendChild(txt);
    const dl = document.createElement("button");
    dl.className = "act dlbtn";
    dl.dataset.ncm = t.id;
    if (t.on_disk) dl.dataset.state = "done";
    paintDlButton(dl);
    dl.onclick = async (e) => {
      e.stopPropagation();
      if (dl.dataset.state === "done") return;
      dl.textContent = "…";
      const res = await call("/netease/download", "POST", { ref: t.ref });
      if (res && res.on_disk) dl.dataset.state = "done";
      await refreshDownloads();
    };
    li.appendChild(dl);
    li.dataset.ncm = t.id;
    txt.onclick = async () => {
      const from = r.items.indexOf(t);
      const rest = r.items.slice(from).map((x) => ({ ref: x.ref, title: x.title, artist: x.artist, album: x.album }));
      await playHere(li, t.ref, rest, t.title, count);
    };
    bList.appendChild(li);
  });
  refresh();
}

const trackItem = (x) => ({ ref: x.ref, title: x.title, artist: x.artist, album: x.album });

async function showDaily() {
  viewingPlaylist = null;
  reloadTracks = () => showDaily();
  refreshDownloads();
  bTitle.textContent = "Daily picks";
  bBack.hidden = false;
  bBack.onclick = () => showPlaylists();
  setStatus("loading today's picks…");
  bList.innerHTML = "";
  const res = await call("/netease/daily", "GET");
  if (!res || !res.items) { setStatus("could not load daily picks"); return; }
  renderTrackRows(res.items, `${res.items.length} picks for today`);
  setPlayAll(async () => call("/queue", "POST", { items: res.items.map(trackItem), mode: "replace", play: true }));
}

async function runNcmSearch(q) {
  viewingPlaylist = null;
  setPlayAll(null);
  bTitle.textContent = `Search: ${q}`;
  bBack.hidden = false;
  bBack.onclick = () => { ncmSearch.value = ""; showPlaylists(); };
  setStatus("searching NetEase…");
  bList.innerHTML = "";
  const res = await call("/netease/search?q=" + encodeURIComponent(q) + "&limit=50", "GET");
  if (!res || !res.items) { setStatus("search failed"); return; }
  renderTrackRows(res.items, `${res.items.length} of ${res.total} result(s)`);
}

ncmSearch.oninput = (e) => {
  const q = e.target.value.trim();
  clearTimeout(ncmSearchTimer);
  ncmSearchTimer = setTimeout(() => (q ? runNcmSearch(q) : showPlaylists()), 400);
};

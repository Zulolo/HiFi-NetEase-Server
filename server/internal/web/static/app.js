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
  $("prog").style.width = dur ? Math.min(100, (st.elapsed / dur) * 100) + "%" : "0";
  if (!dragging && st.volume >= 0) {
    $("vol").value = st.volume;
    $("volval").textContent = st.volume + "%";
  }
}

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

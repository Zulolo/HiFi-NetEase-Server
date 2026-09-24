// Package api serves hifid's REST + WebSocket contract (docs/09).
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Zulolo/HiFi-NetEase-Server/server/internal/config"
	"github.com/Zulolo/HiFi-NetEase-Server/server/internal/netease"
	"github.com/Zulolo/HiFi-NetEase-Server/server/internal/player"
)

type Server struct {
	cfg     *config.Config
	pl      *player.Player
	log     *slog.Logger
	started time.Time
	version string
	ncm     *netease.Client
	sync    *netease.Syncer

	hub *hub
}

func New(cfg *config.Config, pl *player.Player, log *slog.Logger, version string) *Server {
	return &Server{cfg: cfg, pl: pl, log: log, started: time.Now(), version: version, hub: newHub()}
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, id, msg string) {
	writeJSON(w, code, map[string]apiError{"error": {Code: id, Message: msg}})
}

func decode(r *http.Request, v any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	return json.NewDecoder(r.Body).Decode(v)
}

// requireToken guards mutating calls when auth.mode is admin or all.
func (s *Server) requireToken(r *http.Request) bool {
	if s.cfg.Auth.Mode == "none" || s.cfg.Auth.Token == "" {
		return true
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	return got == s.cfg.Auth.Token
}

func (s *Server) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.Auth.Mode == "all" && !s.requireToken(r) {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "bearer token required")
			return
		}
		next(w, r)
	}
}

func (s *Server) Routes(ui http.Handler) http.Handler {
	m := http.NewServeMux()
	g := s.guard

	m.HandleFunc("GET /api/v1/system/status", g(s.status))
	m.HandleFunc("GET /api/v1/player", g(s.getPlayer))
	m.HandleFunc("POST /api/v1/player/play", g(s.play))
	m.HandleFunc("POST /api/v1/player/pause", g(s.simple(func() error { return s.pl.Pause() })))
	m.HandleFunc("POST /api/v1/player/stop", g(s.simple(s.pl.Stop)))
	m.HandleFunc("POST /api/v1/player/next", g(s.simple(s.pl.Next)))
	m.HandleFunc("POST /api/v1/player/prev", g(s.simple(s.pl.Prev)))
	m.HandleFunc("POST /api/v1/player/seek", g(s.seek))
	m.HandleFunc("PUT /api/v1/player/volume", g(s.volume))
	m.HandleFunc("PUT /api/v1/player/options", g(s.options))

	m.HandleFunc("GET /api/v1/queue", g(s.getQueue))
	m.HandleFunc("POST /api/v1/queue", g(s.addQueue))
	m.HandleFunc("DELETE /api/v1/queue", g(s.simple(s.pl.Clear)))
	m.HandleFunc("DELETE /api/v1/queue/{qid}", g(s.delQueueItem))

	m.HandleFunc("GET /api/v1/outputs", g(s.getOutputs))
	m.HandleFunc("PUT /api/v1/outputs/{id}/active", g(s.setActiveOutput))

	m.HandleFunc("GET /api/v1/netease/status", g(s.ncmStatus))
	m.HandleFunc("GET /api/v1/netease/playlists", g(s.ncmPlaylists))
	m.HandleFunc("GET /api/v1/netease/playlists/{id}/tracks", g(s.ncmPlaylistTracks))
	m.HandleFunc("POST /api/v1/netease/download", g(s.ncmDownload))
	m.HandleFunc("GET /api/v1/netease/sync", g(s.syncStatus))
	m.HandleFunc("POST /api/v1/netease/sync/subscribe", g(s.syncSubscribe))
	m.HandleFunc("POST /api/v1/netease/sync/run", g(s.syncRun))

	// MPD fetches this; it is loopback-only and carries no token (docs/08 §5).
	m.HandleFunc("GET /stream/ncm/{id}", s.ncmStream)
	m.HandleFunc("HEAD /stream/ncm/{id}", s.ncmStream)

	m.HandleFunc("GET /api/v1/ws", s.hub.serveWS)
	m.Handle("/", ui)
	return m
}

// simple wraps a no-argument player action and answers with the new state.
func (s *Server) simple(fn func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(); err != nil {
			writeErr(w, http.StatusBadGateway, "mpd_error", err.Error())
			return
		}
		s.respondState(w)
	}
}

func (s *Server) respondState(w http.ResponseWriter) {
	st, err := s.pl.Status()
	if err != nil {
		writeErr(w, http.StatusBadGateway, "mpd_unavailable", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) getPlayer(w http.ResponseWriter, r *http.Request) { s.respondState(w) }

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	st, mpdErr := s.pl.Status()
	outs, _ := s.pl.Outputs()
	active := ""
	for _, o := range outs {
		if o.Enabled {
			active = o.Alias
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":       s.version,
		"server_name":   s.cfg.ServerName,
		"arch":          runtime.GOARCH,
		"go":            runtime.Version(),
		"uptime":        int(time.Since(s.started).Seconds()),
		"mpd":           map[string]any{"connected": mpdErr == nil && st.Connected, "state": st.State},
		"active_output": active,
		"format":        st.Format,
		"ws_clients":    s.hub.count(),
	})
}

func (s *Server) play(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QID *int `json:"qid"`
		Pos *int `json:"pos"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	var err error
	switch {
	case req.QID != nil:
		err = s.pl.PlayID(*req.QID)
	case req.Pos != nil:
		err = s.pl.Play(*req.Pos)
	default:
		err = s.pl.Play(-1)
	}
	if err != nil {
		writeErr(w, http.StatusBadGateway, "mpd_error", err.Error())
		return
	}
	s.respondState(w)
}

func (s *Server) seek(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Seconds *float64 `json:"seconds"`
		Delta   *float64 `json:"delta"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	var err error
	switch {
	case req.Seconds != nil:
		err = s.pl.Seek(*req.Seconds, false)
	case req.Delta != nil:
		err = s.pl.Seek(*req.Delta, true)
	default:
		writeErr(w, http.StatusBadRequest, "bad_request", "seconds or delta required")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadGateway, "mpd_error", err.Error())
		return
	}
	s.respondState(w)
}

func (s *Server) volume(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Volume *int `json:"volume"`
		Delta  *int `json:"delta"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	st, err := s.pl.Status()
	if err != nil {
		writeErr(w, http.StatusBadGateway, "mpd_unavailable", err.Error())
		return
	}
	target := st.Volume
	switch {
	case req.Volume != nil:
		target = *req.Volume
	case req.Delta != nil:
		target += *req.Delta
	default:
		writeErr(w, http.StatusBadRequest, "bad_request", "volume or delta required")
		return
	}
	// MPD reports -1 when the active output has no mixer: that is the
	// fixed-volume case of ADR-0005, not a transient failure.
	if st.Volume < 0 {
		writeErr(w, http.StatusConflict, "volume_fixed", "active output has no mixer; use the amplifier volume")
		return
	}
	if err := s.pl.SetVolume(target); err != nil {
		writeErr(w, http.StatusBadGateway, "mpd_error", err.Error())
		return
	}
	s.respondState(w)
}

func (s *Server) options(w http.ResponseWriter, r *http.Request) {
	var req map[string]bool
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	for _, name := range []string{"repeat", "random", "single", "consume"} {
		if v, ok := req[name]; ok {
			if err := s.pl.SetOption(name, v); err != nil {
				writeErr(w, http.StatusBadGateway, "mpd_error", err.Error())
				return
			}
		}
	}
	s.respondState(w)
}

func (s *Server) getQueue(w http.ResponseWriter, r *http.Request) {
	items, err := s.pl.Queue()
	if err != nil {
		writeErr(w, http.StatusBadGateway, "mpd_unavailable", err.Error())
		return
	}
	if items == nil {
		items = []player.Song{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) delQueueItem(w http.ResponseWriter, r *http.Request) {
	qid, err := strconv.Atoi(r.PathValue("qid"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "qid must be numeric")
		return
	}
	if err := s.pl.DeleteID(qid); err != nil {
		writeErr(w, http.StatusBadGateway, "mpd_error", err.Error())
		return
	}
	s.respondState(w)
}

func (s *Server) getOutputs(w http.ResponseWriter, r *http.Request) {
	outs, err := s.pl.Outputs()
	if err != nil {
		writeErr(w, http.StatusBadGateway, "mpd_unavailable", err.Error())
		return
	}
	if outs == nil {
		outs = []player.Output{}
	}
	writeJSON(w, http.StatusOK, outs)
}

func (s *Server) setActiveOutput(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "output id must be numeric (MPD output id)")
		return
	}
	if err := s.pl.SetActiveOutput(id); err != nil {
		writeErr(w, http.StatusBadGateway, "mpd_error", err.Error())
		return
	}
	s.getOutputs(w, r)
}

// Broadcast relays an MPD idle subsystem to every WebSocket client.
func (s *Server) Broadcast(subsystem string) {
	st, err := s.pl.Status()
	payload := map[string]any{"type": subsystem}
	if err == nil {
		payload["state"] = st
	}
	b, _ := json.Marshal(payload)
	s.hub.broadcast(b)
}

package api

import (
	"net/http"
	"strconv"

	"github.com/Zulolo/HiFi-NetEase-Server/server/internal/netease"
)

// SetNetEase attaches the NetEase adapter. Nil keeps the endpoints present but
// answering 503, so a broken NetEase side never blocks local playback (NFR-6).
func (s *Server) SetNetEase(c *netease.Client) { s.ncm = c }

func (s *Server) ncmReady(w http.ResponseWriter) bool {
	if s.ncm == nil {
		writeErr(w, http.StatusServiceUnavailable, "ncm_disabled", "NetEase support is disabled")
		return false
	}
	return true
}

func (s *Server) ncmStatus(w http.ResponseWriter, r *http.Request) {
	if !s.ncmReady(w) {
		return
	}
	p, err := s.ncm.Profile(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"logged_in": false, "reason": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) ncmPlaylists(w http.ResponseWriter, r *http.Request) {
	if !s.ncmReady(w) {
		return
	}
	offset := queryInt(r, "offset", 0)
	limit := queryInt(r, "limit", 50)
	pls, err := s.ncm.Playlists(r.Context(), offset, limit)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "ncm_error", err.Error())
		return
	}
	if pls == nil {
		pls = []netease.Playlist{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": pls, "offset": offset})
}

func (s *Server) ncmPlaylistTracks(w http.ResponseWriter, r *http.Request) {
	if !s.ncmReady(w) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "playlist id must be numeric")
		return
	}
	offset := queryInt(r, "offset", 0)
	limit := queryInt(r, "limit", 100)
	tracks, total, err := s.ncm.PlaylistTracks(r.Context(), id, offset, limit)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "ncm_error", err.Error())
		return
	}
	if tracks == nil {
		tracks = []netease.Track{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": tracks, "total": total, "offset": offset})
}

// stream serves GET /stream/ncm/{id} in pipe mode (ADR-0008).
func (s *Server) ncmStream(w http.ResponseWriter, r *http.Request) {
	if s.ncm == nil {
		http.Error(w, "NetEase support is disabled", http.StatusServiceUnavailable)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad song id", http.StatusBadRequest)
		return
	}
	s.ncm.ServeStream(w, r, id, s.log)
}

func queryInt(r *http.Request, name string, def int) int {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}

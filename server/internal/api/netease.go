package api

import (
	"context"
	"net/http"
	"path"
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

// SetSyncer attaches the offline sync scheduler (docs/08 §7.1).
func (s *Server) SetSyncer(sy *netease.Syncer) { s.sync = sy }

// ncmDownload fetches one song to disk at the best granted level (FR-1.5).
func (s *Server) ncmDownload(w http.ResponseWriter, r *http.Request) {
	if !s.ncmReady(w) {
		return
	}
	var req struct {
		Ref string `json:"ref"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	id, ok := netease.ParseRef(req.Ref)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad_request", "ref must look like ncm:<id>")
		return
	}
	rel, err := s.ncm.Download(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "ncm_download_failed", err.Error())
		return
	}
	// let MPD see the new file straight away
	if err := s.pl.Update(path.Dir(rel)); err != nil && s.log != nil {
		s.log.Warn("mpd update after download", "path", rel, "err", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ref": req.Ref, "path": rel, "on_disk": true})
}

func (s *Server) syncReady(w http.ResponseWriter) bool {
	if s.sync == nil {
		writeErr(w, http.StatusServiceUnavailable, "sync_disabled", "offline sync is not enabled")
		return false
	}
	return true
}

func (s *Server) syncStatus(w http.ResponseWriter, r *http.Request) {
	if !s.syncReady(w) {
		return
	}
	writeJSON(w, http.StatusOK, s.sync.Status())
}

// syncSubscribe adds or removes a playlist from the offline set.
func (s *Server) syncSubscribe(w http.ResponseWriter, r *http.Request) {
	if !s.syncReady(w) {
		return
	}
	var req struct {
		PlaylistID int64 `json:"playlist_id"`
		Remove     bool  `json:"remove"`
	}
	if err := decode(r, &req); err != nil || req.PlaylistID == 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "playlist_id is required")
		return
	}
	var err error
	if req.Remove {
		err = s.sync.Unsubscribe(req.PlaylistID)
	} else {
		err = s.sync.Subscribe(req.PlaylistID)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "sync_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.sync.Status())
}

// syncRun kicks a pass immediately instead of waiting for the timer.
func (s *Server) syncRun(w http.ResponseWriter, r *http.Request) {
	if !s.syncReady(w) {
		return
	}
	go func() {
		if err := s.sync.RunOnce(context.Background()); err != nil && s.log != nil {
			s.log.Warn("manual sync run", "err", err)
		}
	}()
	writeJSON(w, http.StatusAccepted, s.sync.Status())
}

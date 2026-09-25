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

// SetQueue attaches the explicit download list.
func (s *Server) SetQueue(q *netease.Queue) { s.dl = q }

func (s *Server) dlReady(w http.ResponseWriter) bool {
	if s.dl == nil {
		writeErr(w, http.StatusServiceUnavailable, "download_disabled", "downloads are not enabled")
		return false
	}
	return true
}

// ncmDownload adds one song to the download list and returns immediately.
// A jymaster track is ~157 MB, so the fetch must never block the request.
func (s *Server) ncmDownload(w http.ResponseWriter, r *http.Request) {
	if !s.ncmReady(w) || !s.dlReady(w) {
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
	if rel, on := s.ncm.LocalPath(id); on {
		writeJSON(w, http.StatusOK, map[string]any{"ref": req.Ref, "queued": false, "on_disk": true, "path": rel})
		return
	}
	item := netease.Item{ID: id}
	if t, err := s.ncm.TrackInfo(r.Context(), id); err == nil {
		item.Title, item.Artist = t.Title, t.Artist
	}
	added := s.dl.Add(item)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"ref": req.Ref, "queued": added > 0, "on_disk": false, "pending": s.dl.Pending(),
	})
}

// ncmDownloadPlaylist queues a whole playlist in one explicit action.
func (s *Server) ncmDownloadPlaylist(w http.ResponseWriter, r *http.Request) {
	if !s.ncmReady(w) || !s.dlReady(w) {
		return
	}
	var req struct {
		PlaylistID int64 `json:"playlist_id"`
	}
	if err := decode(r, &req); err != nil || req.PlaylistID == 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "playlist_id is required")
		return
	}
	added, err := s.dl.AddPlaylist(r.Context(), req.PlaylistID)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "ncm_error", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"added": added, "pending": s.dl.Pending()})
}

func (s *Server) ncmDownloads(w http.ResponseWriter, r *http.Request) {
	if !s.dlReady(w) {
		return
	}
	writeJSON(w, http.StatusOK, s.dl.Status())
}

func (s *Server) ncmDownloadsClear(w http.ResponseWriter, r *http.Request) {
	if !s.dlReady(w) {
		return
	}
	s.dl.Clear()
	writeJSON(w, http.StatusOK, s.dl.Status())
}

// ---- QR login (FR-1.1) ------------------------------------------------------

func (s *Server) ncmLoginStart(w http.ResponseWriter, r *http.Request) {
	if !s.ncmReady(w) {
		return
	}
	key, _, err := s.ncm.StartQRLogin(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, "ncm_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"key":        key,
		"image":      "/api/v1/netease/login/qr/" + key + "/image",
		"expires_in": 300,
	})
}

func (s *Server) ncmLoginImage(w http.ResponseWriter, r *http.Request) {
	if s.ncm == nil {
		http.Error(w, "NetEase support is disabled", http.StatusServiceUnavailable)
		return
	}
	png, ok := s.ncm.QRImage(r.PathValue("key"))
	if !ok {
		http.Error(w, "unknown or expired login code", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(png)
}

func (s *Server) ncmLoginStatus(w http.ResponseWriter, r *http.Request) {
	if !s.ncmReady(w) {
		return
	}
	st, err := s.ncm.CheckQRLogin(r.Context(), r.PathValue("key"))
	if err != nil {
		writeErr(w, http.StatusBadGateway, "ncm_error", err.Error())
		return
	}
	if st.Status == "ok" && s.log != nil {
		if p, perr := s.ncm.Profile(r.Context()); perr == nil {
			s.log.Info("netease login via QR", "nickname", p.Nickname, "vip_type", p.VipType)
		}
	}
	writeJSON(w, http.StatusOK, st)
}

// ncmSearch answers GET /api/v1/netease/search?q=&offset=&limit=.
func (s *Server) ncmSearch(w http.ResponseWriter, r *http.Request) {
	if !s.ncmReady(w) {
		return
	}
	q := r.URL.Query().Get("q")
	tracks, total, err := s.ncm.Search(r.Context(), q, queryInt(r, "offset", 0), queryInt(r, "limit", 50))
	if err != nil {
		writeErr(w, http.StatusBadGateway, "ncm_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"query": q, "items": tracks, "total": total})
}

// ncmDownloadsPause / ncmDownloadsResume toggle the download worker. Pause
// aborts the transfer in flight; it is retried first on resume.
func (s *Server) ncmDownloadsPause(w http.ResponseWriter, r *http.Request) {
	if !s.dlReady(w) {
		return
	}
	s.dl.SetPaused(true)
	writeJSON(w, http.StatusOK, s.dl.Status())
}

func (s *Server) ncmDownloadsResume(w http.ResponseWriter, r *http.Request) {
	if !s.dlReady(w) {
		return
	}
	s.dl.SetPaused(false)
	writeJSON(w, http.StatusOK, s.dl.Status())
}

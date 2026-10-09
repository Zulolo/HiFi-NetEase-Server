package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/Zulolo/HiFi-NetEase-Server/server/internal/remote"
)

// SetRemote attaches the Bluetooth remote manager.
func (s *Server) SetRemote(m *remote.Manager) { s.remote = m }

// RemoteAction maps a key press to the player. Play on an empty queue fills
// it with everything under local/ (the Samba uploads), the owner's choice of
// default for a one-button remote.
func (s *Server) RemoteAction(a remote.Action) error {
	st, err := s.pl.Status()
	if err != nil {
		return err
	}
	switch a {
	case remote.ActPlayPause, remote.ActPlay:
		switch {
		case st.State == "play" && a == remote.ActPlayPause:
			return s.pl.Pause()
		case st.State == "pause":
			return s.pl.Pause() // toggles back to play
		case st.QueueLen == 0:
			if err := s.pl.Add("local"); err != nil {
				return err
			}
			return s.pl.Play(0)
		case st.State == "stop":
			return s.pl.Play(-1)
		}
		return nil
	case remote.ActPause:
		if st.State == "play" {
			return s.pl.Pause()
		}
		return nil
	case remote.ActStop:
		return s.pl.Stop()
	case remote.ActNext:
		return s.pl.Next()
	case remote.ActPrev:
		return s.pl.Prev()
	case remote.ActVolUp:
		return s.pl.SetVolume(min(100, st.Volume+3))
	case remote.ActVolDown:
		return s.pl.SetVolume(max(0, st.Volume-3))
	case remote.ActMute:
		return s.pl.SetVolume(0)
	case remote.ActSeekFwd:
		return s.pl.Seek(10, true)
	case remote.ActSeekBack:
		return s.pl.Seek(-10, true)
	}
	return errors.New("unknown action")
}

func (s *Server) remoteReady(w http.ResponseWriter) bool {
	if s.remote == nil {
		writeErr(w, http.StatusServiceUnavailable, "remote_disabled", "Bluetooth remote support is disabled")
		return false
	}
	return true
}

// GET /api/v1/remotes
func (s *Server) remotes(w http.ResponseWriter, r *http.Request) {
	if !s.remoteReady(w) {
		return
	}
	writeJSON(w, http.StatusOK, s.remote.Status(r.Context()))
}

// POST /api/v1/remotes/scan
func (s *Server) remoteScan(w http.ResponseWriter, r *http.Request) {
	if !s.remoteReady(w) {
		return
	}
	_ = s.remote.Scan(12 * time.Second)
	writeJSON(w, http.StatusAccepted, s.remote.Status(r.Context()))
}

// POST /api/v1/remotes/pair {"mac":"AA:BB:…"}
func (s *Server) remotePair(w http.ResponseWriter, r *http.Request) {
	if !s.remoteReady(w) {
		return
	}
	var req struct {
		MAC string `json:"mac"`
	}
	if err := decode(r, &req); err != nil || req.MAC == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "mac is required")
		return
	}
	if err := s.remote.Pair(r.Context(), req.MAC); err != nil {
		writeErr(w, http.StatusBadGateway, "pair_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.remote.Status(r.Context()))
}

// DELETE /api/v1/remotes/{mac}
func (s *Server) remoteRemove(w http.ResponseWriter, r *http.Request) {
	if !s.remoteReady(w) {
		return
	}
	if err := s.remote.Remove(r.Context(), r.PathValue("mac")); err != nil {
		writeErr(w, http.StatusBadGateway, "remove_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.remote.Status(r.Context()))
}

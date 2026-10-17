package api

import (
	"net/http"
	"time"

	"github.com/Zulolo/HiFi-NetEase-Server/server/internal/player"
)

// Sleep timer: pause playback after a set time. It lives in the server, not
// the page, so it keeps counting after the phone screen goes off.

const (
	sleepMinMinutes = 5
	sleepMaxMinutes = 120
)

// decorate adds server-side state (the sleep timer) to a player status.
func (s *Server) decorate(st *player.Status) {
	s.enrichSong(st)
	s.sleepMu.Lock()
	if !s.sleepEnd.IsZero() && time.Now().Before(s.sleepEnd) {
		st.SleepEndsAt = s.sleepEnd.UTC().Format(time.RFC3339)
	}
	s.sleepMu.Unlock()
}

// PUT /api/v1/player/sleep {"minutes": 30}   (0 = off)
func (s *Server) sleep(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Minutes int `json:"minutes"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.Minutes != 0 && (req.Minutes < sleepMinMinutes || req.Minutes > sleepMaxMinutes) {
		writeErr(w, http.StatusBadRequest, "bad_request", "minutes must be 0 (off) or between 5 and 120")
		return
	}
	s.setSleep(time.Duration(req.Minutes) * time.Minute)
	s.respondState(w)
}

func (s *Server) setSleep(d time.Duration) {
	s.sleepMu.Lock()
	defer s.sleepMu.Unlock()
	if s.sleepTimer != nil {
		s.sleepTimer.Stop()
		s.sleepTimer = nil
	}
	s.sleepEnd = time.Time{}
	if d <= 0 {
		s.log.Info("sleep timer off")
		return
	}
	s.sleepEnd = time.Now().Add(d)
	s.sleepTimer = time.AfterFunc(d, s.sleepFire)
	s.log.Info("sleep timer set", "minutes", int(d.Minutes()))
}

func (s *Server) sleepFire() {
	s.sleepMu.Lock()
	s.sleepEnd, s.sleepTimer = time.Time{}, nil
	s.sleepMu.Unlock()
	st, err := s.pl.Status()
	if err == nil && st.State == "play" {
		if err := s.pl.Pause(); err != nil {
			s.log.Warn("sleep timer: pause failed", "err", err)
		} else {
			s.log.Info("sleep timer: paused playback")
		}
	}
	s.Broadcast("player") // phones drop the countdown
}

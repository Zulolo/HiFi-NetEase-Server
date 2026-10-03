package api

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Zulolo/HiFi-NetEase-Server/server/internal/player"
	"github.com/Zulolo/HiFi-NetEase-Server/server/internal/sacd"
)

// isoEntries lists disc images in one library folder. MPD ignores .iso
// files, so they are read from the disk and shown with an Extract button.
func (s *Server) isoEntries(dir string) []player.Entry {
	des, err := os.ReadDir(filepath.Join(s.cfg.Paths.Music, filepath.FromSlash(dir)))
	if err != nil {
		return nil
	}
	var out []player.Entry
	for _, de := range des {
		if de.IsDir() || !strings.EqualFold(filepath.Ext(de.Name()), ".iso") {
			continue
		}
		rel := de.Name()
		if dir != "" {
			rel = dir + "/" + de.Name()
		}
		e := player.Entry{Type: "iso", Path: rel, Name: de.Name(), Title: de.Name()}
		if fi, err := de.Info(); err == nil {
			e.Size = fi.Size()
		}
		e.Extracted = s.sacd != nil && s.sacd.Extracted(rel)
		out = append(out, e)
	}
	return out
}

func (s *Server) sacdErr(w http.ResponseWriter, err error) {
	code := http.StatusBadRequest
	switch {
	case errors.Is(err, sacd.ErrNoTool):
		code = http.StatusNotImplemented
	case errors.Is(err, sacd.ErrBusy):
		code = http.StatusConflict
	case errors.Is(err, sacd.ErrNoSpace):
		code = http.StatusInsufficientStorage
	}
	writeErr(w, code, "sacd_error", err.Error())
}

// extractStart answers POST /api/v1/library/extract {"path":"local/…/disc.iso"}.
func (s *Server) extractStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := s.sacd.Start(req.Path); err != nil {
		s.sacdErr(w, err)
		return
	}
	s.log.Info("sacd extraction started", "iso", req.Path)
	writeJSON(w, http.StatusAccepted, s.sacd.Status())
}

// extractStatus answers GET /api/v1/library/extract.
func (s *Server) extractStatus(w http.ResponseWriter, r *http.Request) {
	st := s.sacd.Status()
	writeJSON(w, http.StatusOK, map[string]any{"available": sacd.Available(), "job": st})
}

// isoDelete answers DELETE /api/v1/library/iso?path=… (only after extraction).
func (s *Server) isoDelete(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	if err := s.sacd.Delete(p); err != nil {
		s.sacdErr(w, err)
		return
	}
	s.log.Warn("iso deleted on request", "iso", p, "from", r.RemoteAddr)
	writeJSON(w, http.StatusOK, map[string]any{"deleted": p})
}

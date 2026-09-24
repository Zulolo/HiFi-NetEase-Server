package api

import (
	"net/http"
	"strings"

	"github.com/Zulolo/HiFi-NetEase-Server/server/internal/player"
)

// getLibrary browses the local library: everything MPD has indexed, which is
// both the Samba uploads under local/ and the NetEase downloads under netease/.
func (s *Server) getLibrary(w http.ResponseWriter, r *http.Request) {
	dir := strings.Trim(r.URL.Query().Get("path"), "/")
	entries, err := s.pl.Browse(dir)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "mpd_error", err.Error())
		return
	}
	if entries == nil {
		entries = []player.Entry{}
	}
	parent := ""
	if dir != "" {
		if i := strings.LastIndexByte(dir, '/'); i > 0 {
			parent = dir[:i]
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path": dir, "parent": parent, "at_root": dir == "", "items": entries,
	})
}

func (s *Server) searchLibrary(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "q is required")
		return
	}
	entries, err := s.pl.SearchLibrary(q, queryInt(r, "limit", 100))
	if err != nil {
		writeErr(w, http.StatusBadGateway, "mpd_error", err.Error())
		return
	}
	if entries == nil {
		entries = []player.Entry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"query": q, "items": entries})
}

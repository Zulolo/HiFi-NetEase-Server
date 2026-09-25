package api

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

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
	s.enrich(entries)
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
	s.enrich(entries)
	if entries == nil {
		entries = []player.Entry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"query": q, "items": entries})
}

// enrich adds what MPD does not know: the file's size on disk. One stat per
// row of a directory listing is cheap; the tree-wide totals live in libraryStats.
func (s *Server) enrich(entries []player.Entry) {
	for i := range entries {
		if entries[i].Type != "file" {
			continue
		}
		if fi, err := os.Stat(filepath.Join(s.cfg.Paths.Music, entries[i].Path)); err == nil {
			entries[i].Size = fi.Size()
		}
	}
}

type treeStat struct {
	Files int   `json:"files"`
	Bytes int64 `json:"bytes"`
}

var (
	treeMu    sync.Mutex
	treeCache map[string]treeStat
	treeAt    time.Time
)

// treeSizes walks local/ and netease/ and caches the result for a minute: a
// walk over a few thousand files is fine on demand but not on every poll.
func (s *Server) treeSizes() map[string]treeStat {
	treeMu.Lock()
	defer treeMu.Unlock()
	if treeCache != nil && time.Since(treeAt) < time.Minute {
		return treeCache
	}
	out := map[string]treeStat{}
	for _, name := range []string{"local", "netease"} {
		var st treeStat
		root := filepath.Join(s.cfg.Paths.Music, name)
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if strings.HasPrefix(d.Name(), ".") { // .part-* downloads in flight
				return nil
			}
			if fi, e := d.Info(); e == nil {
				st.Files++
				st.Bytes += fi.Size()
			}
			return nil
		})
		out[name] = st
	}
	treeCache, treeAt = out, time.Now()
	return out
}

// libraryStats answers GET /api/v1/library/stats: MPD's counters, bytes per
// tree, and free space on the music disk (docs/09 §2 promised the disk part
// in /system/status; it is served in both places).
func (s *Server) libraryStats(w http.ResponseWriter, r *http.Request) {
	stats, _ := s.pl.Stats()
	total, free, _ := diskUsage(s.cfg.Paths.Music)
	toInt := func(k string) int { n, _ := strconv.Atoi(stats[k]); return n }
	writeJSON(w, http.StatusOK, map[string]any{
		"songs":       toInt("songs"),
		"albums":      toInt("albums"),
		"artists":     toInt("artists"),
		"db_playtime": toInt("db_playtime"),
		"trees":       s.treeSizes(),
		"disk": map[string]any{
			"path": s.cfg.Paths.Music, "total": total, "free": free, "used": total - free,
		},
	})
}

// libraryTags answers GET /api/v1/library/tags?tag=artist|album with the
// distinct values, for the Artists / Albums views.
func (s *Server) libraryTags(w http.ResponseWriter, r *http.Request) {
	tag := r.URL.Query().Get("tag")
	if tag != "artist" && tag != "album" {
		writeErr(w, http.StatusBadRequest, "bad_request", "tag must be artist or album")
		return
	}
	vals, err := s.pl.ListTag(tag)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "mpd_error", err.Error())
		return
	}
	if vals == nil {
		vals = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tag": tag, "items": vals})
}

// libraryFind answers GET /api/v1/library/find?tag=&value= with the songs
// carrying that exact tag value.
func (s *Server) libraryFind(w http.ResponseWriter, r *http.Request) {
	tag, value := r.URL.Query().Get("tag"), r.URL.Query().Get("value")
	if (tag != "artist" && tag != "album") || value == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "tag (artist|album) and value are required")
		return
	}
	entries, err := s.pl.FindTag(tag, value)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "mpd_error", err.Error())
		return
	}
	s.enrich(entries)
	if entries == nil {
		entries = []player.Entry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tag": tag, "value": value, "items": entries})
}

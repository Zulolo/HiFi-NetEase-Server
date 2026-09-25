package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Zulolo/HiFi-NetEase-Server/server/internal/netease"
)

// addQueue implements POST /api/v1/queue (docs/09 §4). A "ncm:<id>" ref is
// enqueued as a local proxy URL, never as a raw CDN URL (C-2, ADR-0008), and is
// tagged with addtagid so plain MPD clients show the right title.
func (s *Server) addQueue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Items []struct {
			Ref string `json:"ref"`
			// Optional metadata the client already has (a rendered track list).
			// With it, no per-track NetEase lookup is needed to tag the entry.
			Title  string `json:"title"`
			Artist string `json:"artist"`
			Album  string `json:"album"`
		} `json:"items"`
		Mode string `json:"mode"` // append (default) | replace
		Play bool   `json:"play"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if len(req.Items) == 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "items is empty")
		return
	}
	if req.Mode == "replace" {
		if err := s.pl.Clear(); err != nil {
			writeErr(w, http.StatusBadGateway, "mpd_error", err.Error())
			return
		}
	}

	var firstQID = -1
	var added int
	var lastErr error
	for _, it := range req.Items {
		ref := strings.TrimSpace(it.Ref)
		var (
			qid int
			err error
		)
		switch {
		case strings.HasPrefix(ref, "ncm:playlist:"):
			qid, err = s.addNCMPlaylist(r, strings.TrimPrefix(ref, "ncm:playlist:"))
		case strings.HasPrefix(ref, "ncm:"):
			qid, err = s.addNCM(r, ref, netease.Track{Title: it.Title, Artist: it.Artist, Album: it.Album})
		case strings.HasPrefix(ref, "local:"):
			uri := strings.TrimPrefix(ref, "local:")
			qid, err = s.pl.AddTagged(uri, nil)
		default:
			err = fmt.Errorf("unsupported ref %q", ref)
		}
		if err != nil {
			// one unplayable track must not abort the rest of the list
			lastErr = err
			continue
		}
		added++
		if firstQID < 0 {
			firstQID = qid
			// Start playing as soon as the first track is in, not after the
			// last: a long list then plays within a fraction of a second
			// while the rest is still being added.
			if req.Play {
				if perr := s.pl.PlayID(firstQID); perr != nil {
					writeErr(w, http.StatusBadGateway, "mpd_error", perr.Error())
					return
				}
			}
		}
	}
	if added == 0 {
		msg := "nothing could be queued"
		if lastErr != nil {
			msg = lastErr.Error()
		}
		writeErr(w, http.StatusBadGateway, "queue_add_failed", msg)
		return
	}
	s.respondState(w)
}

// addNCM enqueues one NetEase track. meta may carry the title, artist and
// album the client already knows; when it is empty they are looked up.
func (s *Server) addNCM(r *http.Request, ref string, meta netease.Track) (int, error) {
	if s.ncm == nil {
		return 0, fmt.Errorf("NetEase support is disabled")
	}
	id, ok := netease.ParseRef(ref)
	if !ok {
		return 0, fmt.Errorf("bad ref %q", ref)
	}
	// A downloaded copy always wins: MPD reads it straight off the disk, at
	// full quality, seekable, with no network in the audio path.
	if rel, ok := s.ncm.LocalPath(id); ok {
		qid, err := s.pl.AddTagged(rel, nil)
		if err == nil {
			return qid, nil
		}
		// The file is on disk but MPD has not indexed it (a scan still
		// running, or the file moved and the database went stale). Ask for a
		// rescan and stream this time rather than failing the request.
		if uerr := s.pl.Update(topSegment(rel)); uerr != nil && s.log != nil {
			s.log.Warn("mpd rescan after local add failed", "path", rel, "err", uerr)
		}
		if s.log != nil {
			s.log.Warn("local copy not playable yet, streaming instead", "path", rel, "err", err)
		}
	}
	// Metadata is best-effort: a tagging failure must not stop playback. Use
	// what the client sent; only ask NetEase when it sent nothing, because a
	// lookup per track is what made queueing a hundred tracks take tens of
	// seconds.
	if meta.Title == "" {
		if t, err := s.ncm.TrackInfo(r.Context(), id); err == nil {
			meta = t
		}
	}
	return s.pl.AddTagged(s.streamURL(id), map[string]string{
		"Title": meta.Title, "Artist": meta.Artist, "Album": meta.Album,
	})
}

// streamURL is the loopback address MPD fetches from. MPD runs on the same
// host, so 127.0.0.1 keeps the stream off the network entirely.
func (s *Server) streamURL(id int64) string {
	_, port, err := net_SplitHostPort(s.cfg.Listen)
	if err != nil || port == "" {
		port = "80"
	}
	return "http://127.0.0.1:" + port + "/stream/ncm/" + strconv.FormatInt(id, 10)
}

func net_SplitHostPort(addr string) (string, string, error) {
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return addr, "", fmt.Errorf("no port in %q", addr)
	}
	return addr[:i], addr[i+1:], nil
}

// topSegment returns the first path element ("netease"), which is ASCII by
// construction. MPD 0.23 fails to update a sub-path containing non-ASCII
// characters, so rescans are always aimed at an ASCII ancestor.
func topSegment(rel string) string {
	if i := strings.IndexByte(rel, '/'); i > 0 {
		return rel[:i]
	}
	return rel
}

// addNCMPlaylist enqueues every track of a playlist in its own order and
// returns the queue id of the first one. Metadata comes from the batched track
// listing, so a 1,000-track playlist costs a handful of API calls rather than
// one lookup per song. Adding is local MPD work; URLs are only resolved when a
// track actually starts playing.
func (s *Server) addNCMPlaylist(r *http.Request, idText string) (int, error) {
	if s.ncm == nil {
		return 0, fmt.Errorf("NetEase support is disabled")
	}
	plID, err := strconv.ParseInt(idText, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("bad playlist id %q", idText)
	}
	// A previous expansion still running would race this one for the queue.
	s.expandMu.Lock()
	if s.expandCancel != nil {
		s.expandCancel()
	}
	bg, cancel := context.WithCancel(context.Background())
	s.expandCancel = cancel
	s.expandMu.Unlock()

	// First page synchronously, so playback starts within a couple of
	// seconds; the rest continues detached from the request, because a
	// browser that gives up on a long request must not leave half a playlist.
	const page = 100
	first := -1
	tracks, total, err := s.ncm.PlaylistTracks(r.Context(), plID, 0, page)
	if err != nil {
		return 0, err
	}
	for _, t := range tracks {
		if qid, err := s.addTrack(t); err == nil && first < 0 {
			first = qid
		}
	}
	if first < 0 {
		return 0, fmt.Errorf("playlist %d: nothing could be queued", plID)
	}
	if total > len(tracks) {
		go s.expandRest(bg, plID, len(tracks), total, page)
	}
	return first, nil
}

// expandRest appends the remaining pages of a playlist to the queue.
func (s *Server) expandRest(ctx context.Context, plID int64, offset, total, page int) {
	added := 0
	for offset < total && ctx.Err() == nil {
		tracks, _, err := s.ncm.PlaylistTracks(ctx, plID, offset, page)
		if err != nil || len(tracks) == 0 {
			if err != nil && s.log != nil {
				s.log.Warn("playlist expansion stopped", "playlist", plID, "offset", offset, "err", err)
			}
			return
		}
		for _, t := range tracks {
			if ctx.Err() != nil {
				return
			}
			if _, err := s.addTrack(t); err == nil {
				added++
			}
		}
		offset += len(tracks)
	}
	if s.log != nil {
		s.log.Info("playlist expanded", "playlist", plID, "added_in_background", added, "total", total)
	}
}

// addTrack applies the local-first rule using metadata already in hand.
func (s *Server) addTrack(t netease.Track) (int, error) {
	if rel, ok := s.ncm.LocalPath(t.ID); ok {
		if qid, err := s.pl.AddTagged(rel, nil); err == nil {
			return qid, nil
		}
		_ = s.pl.Update(topSegment(rel))
	}
	return s.pl.AddTagged(s.streamURL(t.ID), map[string]string{
		"Title": t.Title, "Artist": t.Artist, "Album": t.Album,
	})
}

// ncmIDOf recovers the NetEase id behind a queue entry's URI: parsed from a
// proxy stream URL, or reverse-mapped from a downloaded file's path.
func (s *Server) ncmIDOf(uri string) int64 {
	if i := strings.Index(uri, "/stream/ncm/"); i >= 0 {
		id, _ := strconv.ParseInt(strings.TrimRight(uri[i+len("/stream/ncm/"):], "/"), 10, 64)
		return id
	}
	if s.ncm != nil {
		if id, ok := s.ncm.IDForPath(uri); ok {
			return id
		}
	}
	return 0
}

// jump implements POST /player/jump {"ref": "ncm:<id>" | "local:<path>"}: if
// the track is already in the queue, play that entry in place — no clearing,
// no re-adding, the rest of the collection stays. 404 means "not queued", and
// the client then falls back to rebuilding the queue.
func (s *Server) jump(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ref string `json:"ref"`
	}
	if err := decode(r, &req); err != nil || req.Ref == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "ref is required")
		return
	}
	items, err := s.pl.Queue()
	if err != nil {
		writeErr(w, http.StatusBadGateway, "mpd_unavailable", err.Error())
		return
	}
	wantID, isNCM := netease.ParseRef(req.Ref)
	wantPath := strings.TrimPrefix(req.Ref, "local:")
	for _, it := range items {
		uri := it.Ref
		if strings.HasPrefix(uri, "local:") {
			uri = uri[len("local:"):]
		}
		match := false
		if isNCM {
			match = s.ncmIDOf(uri) == wantID
		} else {
			match = uri == wantPath
		}
		if match {
			if err := s.pl.PlayID(it.QID); err != nil {
				writeErr(w, http.StatusBadGateway, "mpd_error", err.Error())
				return
			}
			s.respondState(w)
			return
		}
	}
	writeErr(w, http.StatusNotFound, "not_queued", "track is not in the current queue")
}

package api

import (
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
			qid, err = s.addNCM(r, ref)
		case strings.HasPrefix(ref, "local:"):
			uri := strings.TrimPrefix(ref, "local:")
			qid, err = s.pl.AddTagged(uri, nil)
		default:
			err = fmt.Errorf("unsupported ref %q", ref)
		}
		if err != nil {
			writeErr(w, http.StatusBadGateway, "queue_add_failed", err.Error())
			return
		}
		if firstQID < 0 {
			firstQID = qid
		}
		added++
	}

	if req.Play && firstQID >= 0 {
		if err := s.pl.PlayID(firstQID); err != nil {
			writeErr(w, http.StatusBadGateway, "mpd_error", err.Error())
			return
		}
	}
	s.respondState(w)
}

func (s *Server) addNCM(r *http.Request, ref string) (int, error) {
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
	// Metadata is best-effort: a tagging failure must not stop playback.
	tags := map[string]string{}
	if t, err := s.ncm.TrackInfo(r.Context(), id); err == nil {
		tags["Title"] = t.Title
		tags["Artist"] = t.Artist
		tags["Album"] = t.Album
	}
	return s.pl.AddTagged(s.streamURL(id), tags)
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
	first := -1
	for offset := 0; ; {
		tracks, total, err := s.ncm.PlaylistTracks(r.Context(), plID, offset, 200)
		if err != nil {
			return first, err
		}
		if len(tracks) == 0 {
			break
		}
		for _, t := range tracks {
			qid, err := s.addTrack(t)
			if err != nil {
				continue // one unplayable track must not abort the album
			}
			if first < 0 {
				first = qid
			}
		}
		offset += len(tracks)
		if offset >= total {
			break
		}
	}
	if first < 0 {
		return 0, fmt.Errorf("playlist %d: nothing could be queued", plID)
	}
	return first, nil
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

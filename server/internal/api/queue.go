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
	// Metadata is best-effort: a tagging failure must not stop playback.
	tags := map[string]string{}
	if t, err := s.ncm.TrackInfo(r.Context(), id); err == nil {
		tags["Title"] = t.Title
		tags["Artist"] = t.Artist
		tags["Album"] = t.Album
	}
	uri := s.streamURL(id)
	return s.pl.AddTagged(uri, tags)
}

// streamURL is the loopback address MPD fetches from. MPD runs on the same
// host, so 127.0.0.1 keeps the stream off the network entirely.
func (s *Server) streamURL(id int64) string {
	_, port, err := net_SplitHostPort(s.cfg.Listen)
	if err != nil || port == "" {
		port = "8080"
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

package netease

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExportPlaylist writes one NetEase playlist as an extended M3U into MPD's
// playlist directory so plain MPD clients (M.A.L.P., myMPD) can load it. Tracks
// on disk are written as library-relative paths; the rest as the loopback
// stream URL that MPD already uses for live playback. Returns the file name
// and the number of entries.
func (c *Client) ExportPlaylist(ctx context.Context, playlistID int64, name, playlistDir, streamBase string) (string, int, error) {
	if playlistDir == "" {
		return "", 0, fmt.Errorf("netease: export: playlist directory not configured")
	}
	var all []Track
	for offset := 0; ; {
		page, total, err := c.PlaylistTracks(ctx, playlistID, offset, 200)
		if err != nil {
			return "", 0, err
		}
		all = append(all, page...)
		offset += len(page)
		if len(page) == 0 || offset >= total {
			break
		}
	}
	base := strings.TrimSpace(safeName(name))
	if base == "" {
		base = fmt.Sprintf("playlist-%d", playlistID)
	}
	file := "NetEase - " + base + ".m3u"
	if err := os.MkdirAll(playlistDir, 0o775); err != nil {
		return "", 0, err
	}
	tmp := filepath.Join(playlistDir, "."+file+".tmp")
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o664)
	if err != nil {
		return "", 0, err
	}
	w := bufio.NewWriter(f)
	fmt.Fprintln(w, "#EXTM3U")
	for _, t := range all {
		fmt.Fprintf(w, "#EXTINF:%d,%s - %s\n", int(t.Duration), t.Artist, t.Title)
		if rel, ok := c.LocalPath(t.ID); ok {
			fmt.Fprintln(w, filepath.ToSlash(rel))
		} else {
			fmt.Fprintf(w, "%s/stream/ncm/%d\n", strings.TrimRight(streamBase, "/"), t.ID)
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", 0, err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return "", 0, err
	}
	if err := os.Rename(tmp, filepath.Join(playlistDir, file)); err != nil {
		os.Remove(tmp)
		return "", 0, err
	}
	return file, len(all), nil
}

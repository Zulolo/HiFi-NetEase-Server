package netease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// index remembers which NetEase songs are already on disk, so playback can use
// the local file (full quality, seekable, no network) instead of the proxy.
type index struct {
	path string
	mu   sync.RWMutex
	// Songs maps the song id to a path relative to the music directory.
	Songs map[string]string `json:"songs"`
}

func openIndex(path string) (*index, error) {
	ix := &index{path: path, Songs: map[string]string{}}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ix, nil
		}
		return nil, fmt.Errorf("netease: index: %w", err)
	}
	if err := json.Unmarshal(b, ix); err != nil {
		// a corrupt index is not fatal: it is a cache of what is on disk
		ix.Songs = map[string]string{}
	}
	if ix.Songs == nil {
		ix.Songs = map[string]string{}
	}
	return ix, nil
}

func (ix *index) get(id int64) (string, bool) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	p, ok := ix.Songs[strconv.FormatInt(id, 10)]
	return p, ok
}

func (ix *index) put(id int64, rel string) error {
	ix.mu.Lock()
	ix.Songs[strconv.FormatInt(id, 10)] = rel
	b, err := json.MarshalIndent(ix, "", " ")
	ix.mu.Unlock()
	if err != nil {
		return err
	}
	tmp := ix.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, ix.path)
}

func (ix *index) count() int {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return len(ix.Songs)
}

// LocalPath returns the music-dir-relative path of a downloaded song, if the
// file is still present. MPD plays this directly, so it never touches the proxy.
func (c *Client) LocalPath(id int64) (string, bool) {
	if c.idx == nil || c.musicDir == "" {
		return "", false
	}
	rel, ok := c.idx.get(id)
	if !ok {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(c.musicDir, rel)); err != nil {
		return "", false
	}
	return rel, true
}

func (c *Client) Downloaded() int {
	if c.idx == nil {
		return 0
	}
	return c.idx.count()
}

// safeName keeps file names portable across ext4 and SMB clients.
var unsafeChars = strings.NewReplacer(
	"/", "-", "\\", "-", ":", "-", "*", "-", "?", "", "\"", "'",
	"<", "(", ">", ")", "|", "-", "\n", " ", "\r", " ",
)

func safeName(s string) string {
	s = unsafeChars.Replace(strings.TrimSpace(s))
	s = strings.Trim(s, ". ")
	if s == "" {
		s = "Unknown"
	}
	if len(s) > 120 {
		s = strings.TrimSpace(s[:120])
	}
	return s
}

// Download fetches one song at the best level the account is granted and files
// it under <music>/netease/<Artist>/<Album>/<Title>.<ext> with tags embedded
// (FR-1.5). It is idempotent: an already-indexed, present file is a no-op.
func (c *Client) Download(ctx context.Context, id int64) (string, error) {
	if c.musicDir == "" {
		return "", errors.New("netease: music directory not configured")
	}
	if rel, ok := c.LocalPath(id); ok {
		return rel, nil
	}
	meta, err := c.TrackInfo(ctx, id)
	if err != nil {
		return "", err
	}
	res, err := c.ResolveBest(ctx, id)
	if err != nil {
		return "", err
	}

	ext := strings.ToLower(res.Type)
	if ext == "" {
		ext = "mp3"
	}
	artist := safeName(firstArtist(meta.Artist))
	album := safeName(meta.Album)
	title := safeName(meta.Title)
	relDir := filepath.Join("netease", artist, album)
	rel := filepath.Join(relDir, title+"."+ext)
	absDir := filepath.Join(c.musicDir, relDir)
	if err := os.MkdirAll(absDir, 0o775); err != nil {
		return "", fmt.Errorf("netease: mkdir: %w", err)
	}

	// Download to a temp file in the same directory so the move is atomic and
	// a half-finished file is never seen by MPD's scanner.
	tmp, err := os.CreateTemp(absDir, ".part-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName)
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, res.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", browserUA)
	resp, err := streamHTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("netease: download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("netease: download status %s", resp.Status)
	}
	n, err := io.Copy(tmp, resp.Body)
	if err != nil {
		return "", fmt.Errorf("netease: download body: %w", err)
	}
	if res.Size > 0 && n != res.Size {
		return "", fmt.Errorf("netease: short download: %d of %d bytes", n, res.Size)
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}

	abs := filepath.Join(c.musicDir, rel)
	if err := tagInto(ctx, tmpName, abs, meta, res.Level); err != nil {
		// tagging is a nicety; keep the audio even when ffmpeg is unhappy
		if err2 := os.Rename(tmpName, abs); err2 != nil {
			return "", fmt.Errorf("netease: finalize: %w", err2)
		}
	}
	_ = os.Chmod(abs, 0o664)
	if err := c.idx.put(id, rel); err != nil {
		return rel, fmt.Errorf("netease: index write: %w", err)
	}
	return rel, nil
}

func firstArtist(s string) string {
	if i := strings.Index(s, ","); i > 0 {
		return s[:i]
	}
	return s
}

// tagInto copies the stream without re-encoding and writes the tags, so the
// audio stays bit-identical to what NetEase served (docs/08 §7).
func tagInto(ctx context.Context, src, dst string, t Track, level string) error {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", src,
		"-c", "copy",
		"-metadata", "title=" + t.Title,
		"-metadata", "artist=" + t.Artist,
		"-metadata", "album=" + t.Album,
		"-metadata", "comment=NetEase " + level + " (ncm:" + strconv.FormatInt(t.ID, 10) + ")",
		dst,
	}
	cmd := exec.CommandContext(ctx, ff, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		os.Remove(dst)
		return fmt.Errorf("ffmpeg: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

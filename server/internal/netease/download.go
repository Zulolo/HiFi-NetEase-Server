package netease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
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

	bytes   int64     // cached total of the files above
	bytesAt time.Time // when bytes was computed

	gen        uint64           // bumped on every put, so caches can notice
	reverse    map[string]int64 // path -> id, rebuilt when gen moves
	reverseGen uint64
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
	ix.gen++
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

// Progress is called during a download with bytes copied so far and the total
// (0 when unknown), so a UI can show a percentage.
type Progress func(done, total int64)

// Download fetches one song at the best level the account is granted and files
// it under <music>/netease/<Artist>/<Album>/<Title>.<ext> with tags embedded
// (FR-1.5). It is idempotent: an already-indexed, present file is a no-op.
func (c *Client) Download(ctx context.Context, id int64) (string, error) {
	return c.DownloadWithProgress(ctx, id, nil)
}

// DownloadWithProgress is Download with a progress callback.
// minFreeBytes is the headroom kept on the music disk: a download is refused
// below it so a full disk never corrupts MPD's database or the state files.
const minFreeBytes = 2 << 30

// A CDN connection can survive a Wi-Fi hiccup in a crippled state (seconds of
// RTT, kB/s throughput) while a fresh one runs at full speed. The watchdog
// drops a transfer that moves fewer than stallMinBytes in stallWindow and the
// queue retries it at once on a new connection.
const (
	stallWindow   = 20 * time.Second
	stallMinBytes = 256 << 10 // under ~13 kB/s a 100 MB master would take hours
	// prepTimeout bounds the metadata + URL lookups before the transfer. A
	// wedged TCP connection to the API (seen after a reboot: send queue
	// growing, no acks) otherwise holds the worker for many minutes.
	prepTimeout = 60 * time.Second
)

// ErrStalled marks a download aborted by the stall watchdog; the queue keeps
// the item at the head and backs off before retrying (a bad CDN route is a
// network condition, not a bad track).
var ErrStalled = errors.New("netease: download stalled")

func (c *Client) DownloadWithProgress(ctx context.Context, id int64, report Progress) (string, error) {
	if c.musicDir == "" {
		return "", errors.New("netease: music directory not configured")
	}
	if rel, ok := c.LocalPath(id); ok {
		return rel, nil
	}
	prepCtx, cancelPrep := context.WithTimeout(ctx, prepTimeout)
	meta, err := c.TrackInfo(prepCtx, id)
	if err != nil {
		cancelPrep()
		return "", prepError(ctx, prepCtx, err)
	}
	res, err := c.ResolveBest(prepCtx, id)
	cancelPrep()
	if err != nil {
		return "", prepError(ctx, prepCtx, err)
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
	if free, ok := freeBytes(c.musicDir); ok && free < minFreeBytes {
		return "", fmt.Errorf("netease: music disk nearly full (%d MB free, keeps %d MB); download refused", free>>20, minFreeBytes>>20)
	}
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

	dlCtx, cancelDl := context.WithCancel(ctx)
	defer cancelDl()
	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, res.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", browserUA)
	resp, err := streamHTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("netease: download from %s: %w", req.URL.Host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("netease: download status %s", resp.Status)
	}
	var got atomic.Int64
	var stalled atomic.Bool
	go func() {
		t := time.NewTicker(stallWindow)
		defer t.Stop()
		last := int64(0)
		for {
			select {
			case <-dlCtx.Done():
				return
			case <-t.C:
				cur := got.Load()
				if cur-last < stallMinBytes {
					stalled.Store(true)
					cancelDl()
					return
				}
				last = cur
			}
		}
	}()
	var body io.Reader = &countReader{r: resp.Body, n: &got}
	var n int64
	if report == nil {
		n, err = io.Copy(tmp, body)
	} else {
		report(0, res.Size)
		n, err = io.Copy(tmp, &progressReader{r: body, total: res.Size, report: report})
	}
	if err != nil {
		if stalled.Load() {
			return "", fmt.Errorf("%w after %d bytes from %s", ErrStalled, n, req.URL.Host)
		}
		return "", fmt.Errorf("netease: download body from %s: %w", req.URL.Host, err)
	}
	if res.Size > 0 && n != res.Size {
		return "", fmt.Errorf("netease: short download: %d of %d bytes", n, res.Size)
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}

	abs := filepath.Join(c.musicDir, rel)
	if err := tagInto(ctx, tmpName, abs, meta, res.Level); err != nil {
		// tagging is a nicety; keep the audio even when the tagger is unhappy
		slog.Warn("download: tagging failed, keeping untagged file", "song", id, "path", rel, "err", err)
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
	comment := "NetEase " + level + " (ncm:" + strconv.FormatInt(t.ID, 10) + ")"
	// FLAC: write the Vorbis comment in place with metaflac. NetEase's files
	// carry a padding block, so this touches a few kB instead of rewriting
	// 100–200 MB on the USB card (which used to overrun the ffmpeg limit right
	// after the download had just flushed the same amount to that card).
	if strings.EqualFold(filepath.Ext(src), ".flac") {
		if mf, err := exec.LookPath("metaflac"); err == nil {
			if err := metaflacTag(ctx, mf, src, t.Title, t.Artist, t.Album, comment); err != nil {
				return err
			}
			return os.Rename(src, dst)
		}
	}
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", src,
		"-c", "copy",
		"-metadata", "title=" + t.Title,
		"-metadata", "artist=" + t.Artist,
		"-metadata", "album=" + t.Album,
		"-metadata", "comment=" + comment,
		dst,
	}
	cmd := exec.CommandContext(ctx, ff, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		os.Remove(dst)
		return fmt.Errorf("ffmpeg: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// metaflacTag sets the four tags in place (padding permitting; metaflac
// rewrites the file itself only when it has to).
func metaflacTag(ctx context.Context, mf, path, title, artist, album, comment string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	args := []string{
		"--preserve-modtime",
		"--remove-tag=TITLE", "--remove-tag=ARTIST", "--remove-tag=ALBUM", "--remove-tag=COMMENT",
		"--set-tag=TITLE=" + title, "--set-tag=ARTIST=" + artist, "--set-tag=ALBUM=" + album,
		"--set-tag=COMMENT=" + comment,
		path,
	}
	if out, err := exec.CommandContext(ctx, mf, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("metaflac: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// RetagMissing walks the download index and tags every FLAC that has no
// TITLE (the files saved while ffmpeg tagging was timing out). It asks
// NetEase for each track's names, paced, and reports how many it fixed.
// done is called at the end so the caller can trigger an MPD rescan.
func (c *Client) RetagMissing(ctx context.Context, log *slog.Logger, done func(fixed int)) {
	mf, err := exec.LookPath("metaflac")
	if err != nil || c.idx == nil {
		done(0)
		return
	}
	c.idx.mu.RLock()
	songs := make(map[int64]string, len(c.idx.Songs))
	for k, v := range c.idx.Songs {
		if id, err := strconv.ParseInt(k, 10, 64); err == nil {
			songs[id] = v
		}
	}
	c.idx.mu.RUnlock()
	fixed := 0
	for id, rel := range songs {
		if ctx.Err() != nil {
			break
		}
		if !strings.EqualFold(filepath.Ext(rel), ".flac") {
			continue
		}
		abs := filepath.Join(c.musicDir, rel)
		out, err := exec.CommandContext(ctx, mf, "--show-tag=TITLE", abs).Output()
		if err != nil || strings.TrimSpace(string(out)) != "" {
			continue // missing file, or already tagged
		}
		meta, err := c.TrackInfo(ctx, id)
		if err != nil {
			log.Warn("retag: track info", "song", id, "err", err)
			continue
		}
		level := "download"
		if err := metaflacTag(ctx, mf, abs, meta.Title, meta.Artist, meta.Album, "NetEase "+level+" (ncm:"+strconv.FormatInt(id, 10)+")"); err != nil {
			log.Warn("retag failed", "song", id, "path", rel, "err", err)
			continue
		}
		fixed++
		time.Sleep(300 * time.Millisecond) // pace the SongDetail calls
	}
	log.Info("retag done", "fixed", fixed)
	done(fixed)
}

// DownloadedBytes sums the size of every indexed file still present. Cached
// for a minute; the index is small but this is polled by the UI.
func (c *Client) DownloadedBytes() int64 {
	if c.idx == nil || c.musicDir == "" {
		return 0
	}
	c.idx.mu.Lock()
	defer c.idx.mu.Unlock()
	if time.Since(c.idx.bytesAt) < time.Minute {
		return c.idx.bytes
	}
	var total int64
	for _, rel := range c.idx.Songs {
		if fi, err := os.Stat(filepath.Join(c.musicDir, rel)); err == nil {
			total += fi.Size()
		}
	}
	c.idx.bytes, c.idx.bytesAt = total, time.Now()
	return total
}

// progressReader reports every ~512 kB so the UI moves without flooding.
// prepError classifies a failure of the pre-transfer lookups: when the prep
// deadline expired (and the caller did not cancel) it is a network stall, so
// the queue keeps the track and backs off instead of counting a try.
func prepError(ctx, prepCtx context.Context, err error) error {
	timedOut := errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(err.Error(), "deadline exceeded") || strings.Contains(err.Error(), "timeout")
	if prepCtx.Err() != nil && ctx.Err() == nil && timedOut {
		return fmt.Errorf("%w: NetEase API did not answer within %s (%v)", ErrStalled, prepTimeout, err)
	}
	return err
}

// countReader adds every byte read to n, for the stall watchdog.
type countReader struct {
	r io.Reader
	n *atomic.Int64
}

func (c *countReader) Read(b []byte) (int, error) {
	k, err := c.r.Read(b)
	c.n.Add(int64(k))
	return k, err
}

type progressReader struct {
	r      io.Reader
	done   int64
	total  int64
	last   int64
	report Progress
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if p.done-p.last >= 512<<10 || err != nil {
		p.last = p.done
		p.report(p.done, p.total)
	}
	return n, err
}

// IDForPath answers "which NetEase song is this downloaded file?" for a path
// relative to the music directory. The reverse map is rebuilt lazily whenever
// the index has changed since it was last built.
func (c *Client) IDForPath(rel string) (int64, bool) {
	if c.idx == nil {
		return 0, false
	}
	c.idx.mu.Lock()
	defer c.idx.mu.Unlock()
	if c.idx.reverse == nil || c.idx.reverseGen != c.idx.gen {
		c.idx.reverse = make(map[string]int64, len(c.idx.Songs))
		for id, p := range c.idx.Songs {
			if n, err := strconv.ParseInt(id, 10, 64); err == nil {
				c.idx.reverse[p] = n
			}
		}
		c.idx.reverseGen = c.idx.gen
	}
	id, ok := c.idx.reverse[rel]
	return id, ok
}

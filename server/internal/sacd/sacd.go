// Package sacd turns an SACD disc image (.iso) in the music tree into DSF
// files MPD can play, by running the external sacd_extract tool. MPD cannot
// read inside an SACD ISO; DSF plays as native DSD / DoP like any other file.
package sacd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Tool is the extractor binary; deploy/scripts/install-sacd-extract.sh builds it.
// A variable so tests can point it at a stand-in.
var Tool = "sacd_extract"

var (
	ErrNoTool   = errors.New("sacd_extract is not installed on the board (run deploy/scripts/install-sacd-extract.sh)")
	ErrBusy     = errors.New("another extraction is already running")
	ErrNotSACD  = errors.New("not an SACD image (no SACD table of contents); DVD and CD images are not supported")
	ErrBadPath  = errors.New("path must be an .iso file inside the music directory")
	ErrNoSpace  = errors.New("not enough free space on the music disk for the extracted tracks")
	ErrNotReady = errors.New("extract the image first: no intact extracted tracks are recorded for it")
)

// Status is the state of the one extraction slot.
type Status struct {
	State    string    `json:"state"` // idle | running | done | failed
	ISO      string    `json:"iso,omitempty"`
	Percent  int       `json:"percent"`
	Tracks   int       `json:"tracks,omitempty"`
	Bytes    int64     `json:"bytes,omitempty"`
	OutDir   string    `json:"out_dir,omitempty"`
	Error    string    `json:"error,omitempty"`
	Started  time.Time `json:"started,omitempty"`
	Finished time.Time `json:"finished,omitempty"`
}

// Extractor runs at most one extraction at a time.
type Extractor struct {
	musicDir string
	free     func(path string) (uint64, bool)
	// OnDone receives the music-relative directory holding the new files.
	OnDone func(relDir string)

	mu sync.Mutex
	st Status
}

func New(musicDir string, free func(string) (uint64, bool)) *Extractor {
	return &Extractor{musicDir: musicDir, free: free, st: Status{State: "idle"}}
}

func (e *Extractor) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.st
}

// Available reports whether the extractor binary can be found.
func Available() bool { _, err := exec.LookPath(Tool); return err == nil }

// resolve maps a music-relative .iso path to an absolute one, refusing
// anything outside the music directory.
func (e *Extractor) resolve(rel string) (string, error) {
	rel = filepath.Clean(filepath.FromSlash(strings.Trim(rel, "/")))
	if rel == "." || strings.HasPrefix(rel, "..") || !strings.EqualFold(filepath.Ext(rel), ".iso") {
		return "", ErrBadPath
	}
	abs := filepath.Join(e.musicDir, rel)
	fi, err := os.Stat(abs)
	if err != nil || fi.IsDir() {
		return "", ErrBadPath
	}
	return abs, nil
}

// IsSACD checks for the "SACDMTOC" signature at sector 510.
func IsSACD(abs string) bool {
	f, err := os.Open(abs)
	if err != nil {
		return false
	}
	defer f.Close()
	b := make([]byte, 8)
	if _, err := f.ReadAt(b, 510*2048); err != nil {
		return false
	}
	return string(b) == "SACDMTOC"
}

// dsfUnder lists .dsf files below dir with a quick header sanity check.
func dsfUnder(dir string) map[string]int64 {
	out := map[string]int64{}
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".dsf") {
			return nil
		}
		if fi, err := d.Info(); err == nil {
			out[p] = fi.Size()
		}
		return nil
	})
	return out
}

// marker is the record written next to an image after a successful
// extraction: exactly which tracks came out of it. "Already extracted" and
// the delete guard rely on it, not on whatever DSF files share the folder.
type marker struct {
	ISO    string   `json:"iso"`
	Tracks []string `json:"tracks"` // relative to the image's folder
	At     string   `json:"at"`
}

func markerPath(isoAbs string) string {
	return filepath.Join(filepath.Dir(isoAbs), "."+filepath.Base(isoAbs)+".extracted.json")
}

// extractedOK is true when the marker exists and every track it lists is
// still present with a consistent DSF header.
func extractedOK(isoAbs string) bool {
	b, err := os.ReadFile(markerPath(isoAbs))
	if err != nil {
		return false
	}
	var m marker
	if json.Unmarshal(b, &m) != nil || len(m.Tracks) == 0 {
		return false
	}
	for _, t := range m.Tracks {
		p := filepath.Join(filepath.Dir(isoAbs), filepath.FromSlash(t))
		fi, err := os.Stat(p)
		if err != nil || !dsfHeaderOK(p, fi.Size()) {
			return false
		}
	}
	return true
}

// dsfHeaderOK verifies the DSD chunk and that the size recorded in the
// header equals the file size (a truncated extraction fails this).
func dsfHeaderOK(p string, size int64) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	h := make([]byte, 32)
	if _, err := io.ReadFull(f, h); err != nil {
		return false
	}
	return string(h[:4]) == "DSD " && string(h[28:32]) == "fmt " && int64(binary.LittleEndian.Uint64(h[12:20])) == size
}

// Extracted reports whether this image was extracted and its tracks are intact.
func (e *Extractor) Extracted(rel string) bool {
	abs, err := e.resolve(rel)
	if err != nil {
		return false
	}
	return extractedOK(abs)
}

var totalRe = regexp.MustCompile(`Total:\s*(\d+)%`)

// Start launches the extraction of one image in the background. Stereo
// tracks are written as DSF (DST decompressed) into a sub-folder named
// after the album, next to the image.
func (e *Extractor) Start(rel string) error {
	tool, err := exec.LookPath(Tool)
	if err != nil {
		return ErrNoTool
	}
	abs, err := e.resolve(rel)
	if err != nil {
		return err
	}
	if !IsSACD(abs) {
		return ErrNotSACD
	}
	fi, _ := os.Stat(abs)
	// DST is roughly 2:1; the stereo area of a 4.7 GB disc expands to at most ~4 GB
	if free, ok := e.free(e.musicDir); ok && int64(free) < fi.Size()+(2<<30) {
		return ErrNoSpace
	}
	e.mu.Lock()
	if e.st.State == "running" {
		e.mu.Unlock()
		return ErrBusy
	}
	e.st = Status{State: "running", ISO: filepath.ToSlash(strings.Trim(rel, "/")), Started: time.Now()}
	e.mu.Unlock()
	go e.run(tool, abs)
	return nil
}

func (e *Extractor) run(tool, abs string) {
	dir := filepath.Dir(abs)
	before := dsfUnder(dir)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Hour)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, "-2", "-s", "-c", "-i", abs, "-y", dir)
	cmd.Dir = dir
	out, _ := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout
	var tail []string
	fail := func(msg string) {
		e.mu.Lock()
		e.st.State, e.st.Error, e.st.Finished = "failed", msg, time.Now()
		e.mu.Unlock()
	}
	if err := cmd.Start(); err != nil {
		fail(err.Error())
		return
	}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	sc.Split(func(data []byte, atEOF bool) (int, []byte, error) { // progress lines end in \r
		if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
			return i + 1, data[:i], nil
		}
		if atEOF && len(data) > 0 {
			return len(data), data, nil
		}
		return 0, nil, nil
	})
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if m := totalRe.FindStringSubmatch(line); m != nil {
			p, _ := strconv.Atoi(m[1])
			e.mu.Lock()
			e.st.Percent = p
			e.mu.Unlock()
			continue
		}
		if tail = append(tail, line); len(tail) > 6 {
			tail = tail[1:]
		}
	}
	err := cmd.Wait()
	var tracks, bad []string
	var total int64
	for p, size := range dsfUnder(dir) {
		if old, ok := before[p]; ok && old == size {
			continue // was there before this run
		}
		rel, _ := filepath.Rel(dir, p)
		tracks = append(tracks, filepath.ToSlash(rel))
		total += size
		if !dsfHeaderOK(p, size) {
			bad = append(bad, filepath.Base(p))
		}
	}
	sort.Strings(tracks)
	n := len(tracks)
	switch {
	case err != nil:
		fail(fmt.Sprintf("sacd_extract: %v: %s", err, strings.Join(tail, " | ")))
		return
	case n == 0:
		fail("sacd_extract finished but wrote no DSF files: " + strings.Join(tail, " | "))
		return
	case len(bad) > 0:
		fail("incomplete DSF files: " + strings.Join(bad, ", "))
		return
	}
	if b, err := json.MarshalIndent(marker{ISO: filepath.Base(abs), Tracks: tracks, At: time.Now().Format(time.RFC3339)}, "", " "); err == nil {
		_ = os.WriteFile(markerPath(abs), b, 0o664)
	}
	// the tool creates world-writable folders; match the rest of the share
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == dir {
			return nil
		}
		if d.IsDir() {
			_ = os.Chmod(p, 0o2775)
		} else if strings.EqualFold(filepath.Ext(p), ".dsf") {
			_ = os.Chmod(p, 0o664)
		}
		return nil
	})
	relDir, _ := filepath.Rel(e.musicDir, dir)
	e.mu.Lock()
	e.st.State, e.st.Percent, e.st.Tracks, e.st.Bytes = "done", 100, n, total
	e.st.OutDir, e.st.Finished = filepath.ToSlash(relDir), time.Now()
	e.mu.Unlock()
	if e.OnDone != nil {
		e.OnDone(filepath.ToSlash(relDir))
	}
}

// Delete removes an image, only once DSF files exist next to it and no
// extraction is running. The server never deletes music on its own (NFR-8);
// this is the explicit, user-triggered exception for a now-redundant image.
func (e *Extractor) Delete(rel string) error {
	abs, err := e.resolve(rel)
	if err != nil {
		return err
	}
	e.mu.Lock()
	running := e.st.State == "running"
	e.mu.Unlock()
	if running {
		return ErrBusy
	}
	if !extractedOK(abs) {
		return ErrNotReady
	}
	if err := os.Remove(abs); err != nil {
		return err
	}
	_ = os.Remove(markerPath(abs))
	return nil
}

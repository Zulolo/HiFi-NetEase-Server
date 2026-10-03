//go:build linux

package sacd

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// dsf builds a minimal file whose header passes dsfHeaderOK.
func dsf(size int) []byte {
	b := make([]byte, size)
	copy(b[0:], "DSD ")
	binary.LittleEndian.PutUint64(b[4:], 28)
	binary.LittleEndian.PutUint64(b[12:], uint64(size))
	copy(b[28:], "fmt ")
	return b
}

func fakeISO(t *testing.T, p string, sacd bool) {
	t.Helper()
	b := make([]byte, 510*2048+2048)
	if sacd {
		copy(b[510*2048:], "SACDMTOC")
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func wait(t *testing.T, e *Extractor) Status {
	t.Helper()
	for i := 0; i < 100; i++ {
		if st := e.Status(); st.State != "running" {
			return st
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("extraction did not finish")
	return Status{}
}

func TestExtractFlow(t *testing.T) {
	music := t.TempDir()
	album := filepath.Join(music, "local", "album")
	if err := os.MkdirAll(album, 0o775); err != nil {
		t.Fatal(err)
	}
	tpl := filepath.Join(t.TempDir(), "tpl.dsf")
	if err := os.WriteFile(tpl, dsf(4096), 0o644); err != nil {
		t.Fatal(err)
	}
	// stand-in for sacd_extract: same arguments, same \r-terminated progress lines
	tool := filepath.Join(t.TempDir(), "fake_sacd_extract")
	script := "#!/bin/sh\nwhile [ $# -gt 0 ]; do case \"$1\" in -y) out=\"$2\"; shift;; esac; shift; done\n" +
		"mkdir -p \"$out/Album/Stereo\"\n" +
		"printf 'Completed: 50%% (x), Total: 40%% (y)\\r'\n" +
		"cp '" + tpl + "' \"$out/Album/Stereo/01 - One.dsf\"\n" +
		"printf 'Completed: 99%% (x), Total: 90%% (y)\\r'\n" +
		"cp '" + tpl + "' \"$out/Album/Stereo/02 - Two.dsf\"\n" +
		"echo 'We are done exporting DSF...'\n"
	if err := os.WriteFile(tool, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	Tool = tool
	defer func() { Tool = "sacd_extract" }()

	// a DSF that was already in the folder must not count as "extracted"
	if err := os.WriteFile(filepath.Join(album, "unrelated.dsf"), dsf(2048), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeISO(t, filepath.Join(album, "disc.iso"), true)
	fakeISO(t, filepath.Join(album, "other.iso"), true)
	fakeISO(t, filepath.Join(album, "dvd.iso"), false)

	var doneDir string
	e := New(music, func(string) (uint64, bool) { return 100 << 30, true })
	e.OnDone = func(rel string) { doneDir = rel }

	if e.Extracted("local/album/disc.iso") {
		t.Fatal("unextracted image reported as extracted because of an unrelated DSF")
	}
	if err := e.Delete("local/album/disc.iso"); !errors.Is(err, ErrNotReady) {
		t.Fatalf("delete before extraction: %v", err)
	}
	if err := e.Start("local/album/dvd.iso"); !errors.Is(err, ErrNotSACD) {
		t.Fatalf("non-SACD image: %v", err)
	}
	if err := e.Start("../outside.iso"); !errors.Is(err, ErrBadPath) {
		t.Fatalf("path escape: %v", err)
	}

	if err := e.Start("local/album/disc.iso"); err != nil {
		t.Fatal(err)
	}
	st := wait(t, e)
	if st.State != "done" || st.Tracks != 2 || st.Percent != 100 || st.Bytes != 8192 {
		t.Fatalf("status: %+v", st)
	}
	if doneDir != "local/album" {
		t.Fatalf("OnDone dir = %q", doneDir)
	}
	if !e.Extracted("local/album/disc.iso") {
		t.Fatal("extracted image not reported as extracted")
	}
	if e.Extracted("local/album/other.iso") {
		t.Fatal("a different image in the same folder must not be reported as extracted")
	}
	if err := e.Delete("local/album/other.iso"); !errors.Is(err, ErrNotReady) {
		t.Fatalf("delete of an unextracted sibling: %v", err)
	}

	// a truncated track withdraws the permission to delete
	tr := filepath.Join(album, "Album", "Stereo", "02 - Two.dsf")
	if err := os.Truncate(tr, 1000); err != nil {
		t.Fatal(err)
	}
	if err := e.Delete("local/album/disc.iso"); !errors.Is(err, ErrNotReady) {
		t.Fatalf("delete with a damaged track: %v", err)
	}
	if err := os.WriteFile(tr, dsf(4096), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := e.Delete("local/album/disc.iso"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(album, "disc.iso")); !os.IsNotExist(err) {
		t.Fatal("image still present after delete")
	}
	if _, err := os.Stat(filepath.Join(album, "Album", "Stereo", "01 - One.dsf")); err != nil {
		t.Fatal("extracted track removed by delete")
	}
}

//go:build linux

package remote

import (
	"context"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// evdev input_event on 64-bit Linux: struct timeval (16 bytes), type, code, value.
const eventSize = 24

const (
	evKey      = 1
	keyRelease = 0
	keyPress   = 1
	keyRepeat  = 2
)

// runInputs polls /sys/class/input every 3 s and opens every event device
// that belongs to a Bluetooth device (uniq = its MAC address) or whose name
// looks like a remote or keyboard, reading keys until it disappears.
func (m *Manager) runInputs(ctx context.Context) {
	open := map[string]context.CancelFunc{} // event path → stop
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		devs, _ := filepath.Glob("/sys/class/input/event*")
		seen := map[string]bool{}
		for _, d := range devs {
			ev := "/dev/input/" + filepath.Base(d)
			seen[ev] = true
			if _, ok := open[ev]; ok {
				continue
			}
			name := strings.TrimSpace(readFile(filepath.Join(d, "device", "name")))
			uniq := strings.ToLower(strings.TrimSpace(readFile(filepath.Join(d, "device", "uniq"))))
			if !isRemote(name, uniq) {
				continue
			}
			f, err := os.Open(ev)
			if err != nil {
				continue // not readable: hifid is not in the input group
			}
			cctx, cancel := context.WithCancel(ctx)
			open[ev] = cancel
			m.log.Info("remote: input device opened", "dev", ev, "name", name, "mac", uniq)
			m.mu.Lock()
			if uniq != "" {
				m.inputs[uniq] = true
			}
			m.mu.Unlock()
			go func(ev, uniq string, f *os.File) {
				m.readKeys(cctx, f)
				f.Close()
				m.mu.Lock()
				delete(m.inputs, uniq)
				m.mu.Unlock()
				m.log.Info("remote: input device closed", "dev", ev)
			}(ev, uniq, f)
		}
		for ev, cancel := range open {
			if !seen[ev] {
				cancel()
				delete(open, ev)
			}
		}
		select {
		case <-ctx.Done():
			for _, c := range open {
				c()
			}
			return
		case <-t.C:
		}
	}
}

// isRemote: a Bluetooth HID (uniq is a MAC address) or anything that calls
// itself a keyboard, remote or consumer-control device. The DAC's own
// HID interface (volume buttons on some dongles) is excluded on purpose.
func isRemote(name, uniq string) bool {
	if len(uniq) == 17 && strings.Count(uniq, ":") == 5 {
		return true
	}
	n := strings.ToLower(name)
	for _, k := range []string{"keyboard", "remote", "consumer control", "media"} {
		if strings.Contains(n, k) {
			return true
		}
	}
	return false
}

func (m *Manager) readKeys(ctx context.Context, f *os.File) {
	go func() { <-ctx.Done(); f.SetReadDeadline(time.Now()) }()
	buf := make([]byte, eventSize*16)
	for {
		n, err := f.Read(buf)
		if err != nil {
			if ctx.Err() != nil || err == io.EOF {
				return
			}
			if os.IsTimeout(err) {
				return
			}
			return // device gone
		}
		for i := 0; i+eventSize <= n; i += eventSize {
			e := buf[i : i+eventSize]
			typ := binary.LittleEndian.Uint16(e[16:18])
			code := binary.LittleEndian.Uint16(e[18:20])
			val := int32(binary.LittleEndian.Uint32(e[20:24]))
			if typ != evKey || val == keyRelease {
				continue
			}
			m.dispatch(code, val == keyRepeat)
		}
	}
}

func readFile(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}

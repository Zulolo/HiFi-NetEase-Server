// Package remote turns Bluetooth (BLE) keyboards and media remotes into
// player commands. Pairing goes through BlueZ (bluetoothctl); once a device
// is paired and trusted it reconnects by itself whenever it wakes, the kernel
// exposes it as /dev/input/eventN, and this package reads the key events.
package remote

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Action is what a key asks the player to do.
type Action string

const (
	ActPlayPause Action = "play_pause"
	ActPlay      Action = "play"
	ActPause     Action = "pause"
	ActStop      Action = "stop"
	ActNext      Action = "next"
	ActPrev      Action = "prev"
	ActVolUp     Action = "volume_up"
	ActVolDown   Action = "volume_down"
	ActMute      Action = "mute"
	ActSeekFwd   Action = "seek_forward"
	ActSeekBack  Action = "seek_back"
)

// Linux input key codes (linux/input-event-codes.h) → actions.
var keyActions = map[uint16]Action{
	164: ActPlayPause, // KEY_PLAYPAUSE
	207: ActPlay,      // KEY_PLAY
	119: ActPause,     // KEY_PAUSE
	166: ActStop,      // KEY_STOPCD
	163: ActNext,      // KEY_NEXTSONG
	165: ActPrev,      // KEY_PREVIOUSSONG
	115: ActVolUp,     // KEY_VOLUMEUP
	114: ActVolDown,   // KEY_VOLUMEDOWN
	113: ActMute,      // KEY_MUTE
	106: ActSeekFwd,   // KEY_RIGHT
	105: ActSeekBack,  // KEY_LEFT
	208: ActSeekFwd,   // KEY_FASTFORWARD
	168: ActSeekBack,  // KEY_REWIND
	28:  ActPlayPause, // KEY_ENTER (some remotes send Enter for the centre key)
	352: ActPlayPause, // KEY_OK
}

// Device is a Bluetooth device as BlueZ knows it.
type Device struct {
	MAC       string `json:"mac"`
	Name      string `json:"name"`
	Paired    bool   `json:"paired"`
	Trusted   bool   `json:"trusted"`
	Connected bool   `json:"connected"`
	Input     bool   `json:"input"` // an input device is open for it
	Icon      string `json:"icon,omitempty"`
}

// Status is what the web page shows.
type Status struct {
	Available bool     `json:"available"` // bluetoothctl present and the adapter answers
	Adapter   string   `json:"adapter,omitempty"`
	Scanning  bool     `json:"scanning"`
	Paired    []Device `json:"paired"`
	Found     []Device `json:"found"`
	Pairing   string   `json:"pairing,omitempty"` // MAC being paired right now
	LastKey   string   `json:"last_key,omitempty"`
	LastKeyAt string   `json:"last_key_at,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// Manager pairs devices and dispatches their keys.
type Manager struct {
	log     *slog.Logger
	handler func(Action) error

	mu       sync.Mutex
	scanning bool
	found    map[string]Device
	pairing  string
	lastErr  string
	lastKey  string
	lastAt   time.Time
	inputs   map[string]bool // MAC → input device open
}

func New(log *slog.Logger, handler func(Action) error) *Manager {
	return &Manager{log: log, handler: handler, found: map[string]Device{}, inputs: map[string]bool{}}
}

// Run keeps input devices of paired remotes open until ctx ends.
func (m *Manager) Run(ctx context.Context) { m.runInputs(ctx) }

func (m *Manager) dispatch(code uint16, repeat bool) {
	act, ok := keyActions[code]
	if !ok {
		m.log.Debug("remote: unmapped key", "code", code)
		return
	}
	// held keys repeat only where it makes sense
	if repeat {
		switch act {
		case ActVolUp, ActVolDown, ActSeekFwd, ActSeekBack:
		default:
			return
		}
	}
	m.mu.Lock()
	m.lastKey, m.lastAt = string(act), time.Now()
	m.mu.Unlock()
	if m.handler != nil {
		if err := m.handler(act); err != nil {
			m.log.Warn("remote: action failed", "action", act, "err", err)
		}
	}
}

// ---- BlueZ via bluetoothctl -------------------------------------------------

func btctl(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "bluetoothctl", args...).CombinedOutput()
	return stripANSI(string(out)), err
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]|\r`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

var devLine = regexp.MustCompile(`Device ([0-9A-Fa-f:]{17}) ?(.*)`)

// Available reports whether BlueZ answers.
func Available(ctx context.Context) (string, bool) {
	out, err := btctl(ctx, 5*time.Second, "show")
	if err != nil || !strings.Contains(out, "Powered: yes") {
		return "", false
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "Name:") {
			return strings.TrimSpace(strings.SplitN(l, ":", 2)[1]), true
		}
	}
	return "", true
}

// Paired lists the devices BlueZ has paired, with connection state.
func (m *Manager) Paired(ctx context.Context) []Device {
	out, _ := btctl(ctx, 8*time.Second, "devices", "Paired")
	var devs []Device
	for _, l := range strings.Split(out, "\n") {
		if mm := devLine.FindStringSubmatch(l); mm != nil {
			d := Device{MAC: strings.ToUpper(mm[1]), Name: strings.TrimSpace(mm[2]), Paired: true}
			info, _ := btctl(ctx, 5*time.Second, "info", d.MAC)
			d.Connected = strings.Contains(info, "Connected: yes")
			d.Trusted = strings.Contains(info, "Trusted: yes")
			for _, il := range strings.Split(info, "\n") {
				if strings.Contains(il, "Icon:") {
					d.Icon = strings.TrimSpace(strings.SplitN(il, ":", 2)[1])
				}
			}
			m.mu.Lock()
			d.Input = m.inputs[strings.ToLower(d.MAC)]
			m.mu.Unlock()
			devs = append(devs, d)
		}
	}
	return devs
}

// Scan discovers nearby devices for d seconds in the background.
func (m *Manager) Scan(d time.Duration) error {
	m.mu.Lock()
	if m.scanning {
		m.mu.Unlock()
		return nil
	}
	m.scanning, m.found, m.lastErr = true, map[string]Device{}, ""
	m.mu.Unlock()
	go func() {
		defer func() { m.mu.Lock(); m.scanning = false; m.mu.Unlock() }()
		ctx, cancel := context.WithTimeout(context.Background(), d+10*time.Second)
		defer cancel()
		// stdbuf: bluetoothctl block-buffers its output on a pipe and nothing
		// would arrive before the scan ends; line-buffered it reports at once
		cmd := exec.CommandContext(ctx, "stdbuf", "-oL", "-eL", "bluetoothctl", "--timeout", fmt.Sprint(int(d.Seconds())), "scan", "on")
		out, err := cmd.StdoutPipe()
		if err != nil {
			return
		}
		cmd.Stderr = cmd.Stdout
		if err := cmd.Start(); err != nil {
			m.mu.Lock()
			m.lastErr = "scan: " + err.Error()
			m.mu.Unlock()
			return
		}
		paired := map[string]bool{}
		for _, p := range m.Paired(ctx) {
			paired[p.MAC] = true
		}
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			l := stripANSI(sc.Text())
			mm := devLine.FindStringSubmatch(l)
			if mm == nil || !(strings.Contains(l, "NEW") || strings.Contains(l, "CHG")) {
				continue
			}
			mac := strings.ToUpper(mm[1])
			if paired[mac] {
				continue
			}
			name := strings.TrimSpace(mm[2])
			m.mu.Lock()
			cur := m.found[mac]
			cur.MAC = mac
			// "CHG ... Name: X" / "CHG ... Alias: X" carry the name later than NEW
			if i := strings.Index(name, "Name: "); i >= 0 {
				cur.Name = strings.TrimSpace(name[i+6:])
			} else if i := strings.Index(name, "Alias: "); i >= 0 && cur.Name == "" {
				cur.Name = strings.TrimSpace(name[i+7:])
			} else if strings.Contains(l, "NEW") && name != "" && name != strings.ReplaceAll(mac, ":", "-") {
				cur.Name = name
			} else if i := strings.Index(name, "Icon: "); i >= 0 {
				cur.Icon = strings.TrimSpace(name[i+6:])
			}
			m.found[mac] = cur
			m.mu.Unlock()
		}
		_ = cmd.Wait()
	}()
	return nil
}

// Pair pairs, trusts and connects one device through one interactive
// bluetoothctl session: scanning stays on so BlueZ keeps the device object,
// commands are sent only after the prompt is up, and the agent's yes/no
// questions (passkey confirmation, service authorisation) are answered yes.
// A remote that insists on a typed passkey cannot be paired this way.
func (m *Manager) Pair(ctx context.Context, mac string) error {
	mac = strings.ToUpper(strings.TrimSpace(mac))
	if !regexp.MustCompile(`^([0-9A-F]{2}:){5}[0-9A-F]{2}$`).MatchString(mac) {
		return fmt.Errorf("bad address %q", mac)
	}
	m.mu.Lock()
	if m.pairing != "" {
		m.mu.Unlock()
		return fmt.Errorf("already pairing %s", m.pairing)
	}
	m.pairing, m.lastErr = mac, ""
	m.mu.Unlock()
	fail := func(msg string) error {
		m.mu.Lock()
		m.lastErr, m.pairing = msg, ""
		m.mu.Unlock()
		m.log.Warn("remote: pairing failed", "mac", mac, "msg", msg)
		return fmt.Errorf("%s", msg)
	}

	ctx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "stdbuf", "-oL", "-eL", "bluetoothctl")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fail(err.Error())
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fail(err.Error())
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return fail("bluetoothctl: " + err.Error())
	}
	defer func() { _, _ = io.WriteString(stdin, "quit\n"); _ = cmd.Wait() }()

	lines := make(chan string, 256)
	go func() {
		rd := bufio.NewReader(stdout)
		for {
			chunk, err := rd.ReadString('\n')
			for _, l := range strings.Split(stripANSI(chunk), "\n") {
				if t := strings.TrimSpace(l); t != "" {
					lines <- t
				}
			}
			if err != nil {
				close(lines)
				return
			}
		}
	}()
	send := func(c string) { m.log.Debug("remote: btctl >", "cmd", c); _, _ = io.WriteString(stdin, c+"\n") }
	// waitFor reads until one of the markers appears (or the deadline), answering prompts on the way
	waitFor := func(d time.Duration, markers ...string) (string, bool) {
		deadline := time.After(d)
		for {
			select {
			case l, ok := <-lines:
				if !ok {
					return "", false
				}
				m.log.Debug("remote: btctl <", "line", l)
				switch {
				case strings.Contains(l, "(yes/no)"):
					send("yes")
				case strings.Contains(l, "Enter passkey") || strings.Contains(l, "Enter PIN"):
					send("")
					return l, false
				}
				for _, k := range markers {
					if strings.Contains(l, k) {
						return l, true
					}
				}
			case <-deadline:
				return "", false
			case <-ctx.Done():
				return "", false
			}
		}
	}
	// bluetoothctl registers its own agent on start; wait for that before anything else
	if _, ok := waitFor(10*time.Second, "Agent registered"); !ok {
		return fail("bluetoothctl did not come up (is the hifid user in the bluetooth group?)")
	}
	// The device was listed by a scan moments ago, so BlueZ usually still has
	// it: pair straight away (the keyboard sleeps ~10 s after advertising).
	// Only when BlueZ answers "not available" scan for it first.
	send("pair " + mac)
	l, ok := waitFor(6*time.Second, "Pairing successful", "Failed to pair", "AlreadyExists", "Authentication", "ConnectionAttemptFailed", "not available", "Attempting to pair")
	if !ok || strings.Contains(l, "not available") {
		send("scan on")
		if _, seen := waitFor(15*time.Second, mac); !seen {
			send("scan off")
			return fail("device " + mac + " not seen: switch it on and put it in pairing mode, then scan again")
		}
		time.Sleep(800 * time.Millisecond)
		send("pair " + mac)
		l, ok = waitFor(6*time.Second, "Pairing successful", "Failed to pair", "AlreadyExists", "Authentication", "ConnectionAttemptFailed", "not available", "Attempting to pair")
	}
	if ok && strings.Contains(l, "Attempting to pair") {
		l, ok = waitFor(40*time.Second, "Pairing successful", "Failed to pair", "AlreadyExists", "AuthenticationFailed", "AuthenticationCanceled", "AuthenticationRejected", "ConnectionAttemptFailed", "not available")
	}
	switch {
	case ok && (strings.Contains(l, "Pairing successful") || strings.Contains(l, "AlreadyExists")):
	case strings.Contains(l, "Enter passkey") || strings.Contains(l, "Enter PIN"):
		return fail("this device wants a passkey typed on it, which cannot be done from here")
	case ok:
		return fail("pairing failed: " + l)
	default:
		return fail("pairing gave no answer within 40 s (keep the keyboard awake and in pairing mode)")
	}
	send("trust " + mac)
	waitFor(5*time.Second, "trust succeeded", "Changing")
	send("connect " + mac)
	cl, cok := waitFor(20*time.Second, "Connection successful", "Failed to connect")
	send("scan off")
	m.mu.Lock()
	m.pairing = ""
	m.mu.Unlock()
	if !cok || strings.Contains(cl, "Failed") {
		m.log.Warn("remote: paired, connect not confirmed (it will connect when a key is pressed)", "mac", mac, "line", cl)
	}
	m.log.Info("remote: paired", "mac", mac)
	return nil
}

// Remove forgets a device.
func (m *Manager) Remove(ctx context.Context, mac string) error {
	out, err := btctl(ctx, 10*time.Second, "remove", strings.ToUpper(mac))
	if err != nil && !strings.Contains(out, "removed") {
		return fmt.Errorf("remove: %s", lastLine(out, "Failed", "not available"))
	}
	return nil
}

func lastLine(out string, keys ...string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		for _, k := range keys {
			if strings.Contains(lines[i], k) {
				return strings.TrimSpace(lines[i])
			}
		}
	}
	if len(lines) > 0 {
		return strings.TrimSpace(lines[len(lines)-1])
	}
	return ""
}

// Status assembles the page view.
func (m *Manager) Status(ctx context.Context) Status {
	name, ok := Available(ctx)
	st := Status{Available: ok, Adapter: name}
	if ok {
		st.Paired = m.Paired(ctx)
	}
	m.mu.Lock()
	st.Scanning, st.Pairing, st.Error, st.LastKey = m.scanning, m.pairing, m.lastErr, m.lastKey
	if !m.lastAt.IsZero() {
		st.LastKeyAt = m.lastAt.Format(time.RFC3339)
	}
	for _, d := range m.found {
		st.Found = append(st.Found, d)
	}
	m.mu.Unlock()
	sort.Slice(st.Found, func(i, j int) bool { // named devices first
		if (st.Found[i].Name != "") != (st.Found[j].Name != "") {
			return st.Found[i].Name != ""
		}
		return st.Found[i].MAC < st.Found[j].MAC
	})
	if st.Paired == nil {
		st.Paired = []Device{}
	}
	if st.Found == nil {
		st.Found = []Device{}
	}
	return st
}

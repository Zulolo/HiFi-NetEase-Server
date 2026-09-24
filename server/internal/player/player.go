// Package player is hifid's MPD adapter (ADR-0001: MPD owns playback,
// hifid never touches samples).
package player

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fhs/gompd/v2/mpd"
)

type Player struct {
	network, addr string

	mu sync.Mutex
	cl *mpd.Client
}

func New(network, addr string) *Player {
	return &Player{network: network, addr: addr}
}

// conn returns a live client, redialling when the socket has gone away.
func (p *Player) conn() (*mpd.Client, error) {
	if p.cl != nil {
		if err := p.cl.Ping(); err == nil {
			return p.cl, nil
		}
		_ = p.cl.Close()
		p.cl = nil
	}
	cl, err := mpd.Dial(p.network, p.addr)
	if err != nil {
		return nil, err
	}
	p.cl = cl
	return cl, nil
}

func (p *Player) with(fn func(*mpd.Client) error) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	cl, err := p.conn()
	if err != nil {
		return err
	}
	if err := fn(cl); err != nil {
		// A failed command may mean a dropped connection: discard it so the
		// next call redials instead of reusing a broken socket.
		if p.cl != nil {
			_ = p.cl.Close()
			p.cl = nil
		}
		return err
	}
	return nil
}

func (p *Player) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cl != nil {
		_ = p.cl.Close()
		p.cl = nil
	}
}

type Format struct {
	Delivery   string `json:"delivery"` // native-dsd | pcm
	SampleRate int    `json:"sample_rate"`
	Bits       int    `json:"bits"`
	Channels   int    `json:"channels"`
	OutputRate int    `json:"output_rate,omitempty"`
	OutputFmt  string `json:"output_format,omitempty"`
}

type Song struct {
	QID      int     `json:"qid"`
	Pos      int     `json:"pos"`
	Ref      string  `json:"ref"`
	Source   string  `json:"source"`
	Title    string  `json:"title"`
	Artist   string  `json:"artist"`
	Album    string  `json:"album"`
	Duration float64 `json:"duration"`
}

type Status struct {
	Connected bool    `json:"connected"`
	State     string  `json:"state"`
	QID       int     `json:"qid"`
	Pos       int     `json:"pos"`
	Elapsed   float64 `json:"elapsed"`
	Duration  float64 `json:"duration"`
	Volume    int     `json:"volume"`
	Repeat    bool    `json:"repeat"`
	Random    bool    `json:"random"`
	Single    bool    `json:"single"`
	Consume   bool    `json:"consume"`
	Song      *Song   `json:"song"`
	Format    *Format `json:"format"`
}

func atoi(s string) int     { n, _ := strconv.Atoi(s); return n }
func atof(s string) float64 { f, _ := strconv.ParseFloat(s, 64); return f }
func isOn(s string) bool    { return s == "1" }

func songFrom(a mpd.Attrs) *Song {
	if len(a) == 0 {
		return nil
	}
	file := a["file"]
	s := &Song{
		QID:      atoi(a["Id"]),
		Pos:      atoi(a["Pos"]),
		Ref:      "local:" + file,
		Source:   "local",
		Title:    a["Title"],
		Artist:   a["Artist"],
		Album:    a["Album"],
		Duration: atof(a["duration"]),
	}
	if strings.HasPrefix(file, "http://") || strings.HasPrefix(file, "https://") {
		s.Source, s.Ref = "stream", file
	}
	if s.Title == "" {
		s.Title = filepath.Base(file)
	}
	return s
}

// parseAudio turns MPD's "44100:24:2" or "dsd64:2" into a Format.
func parseAudio(audio string) *Format {
	if audio == "" {
		return nil
	}
	f := &Format{Delivery: "pcm"}
	parts := strings.Split(audio, ":")
	if strings.HasPrefix(parts[0], "dsd") {
		f.Delivery, f.Bits = "native-dsd", 1
		f.SampleRate = atoi(strings.TrimPrefix(parts[0], "dsd")) * 44100
	} else {
		f.SampleRate = atoi(parts[0])
	}
	switch len(parts) {
	case 2:
		f.Channels = atoi(parts[1])
	case 3:
		f.Bits, f.Channels = atoi(parts[1]), atoi(parts[2])
	}
	return f
}

// alsaHWParams reports what the DAC is actually being fed (FR-4.6), read from
// the first open playback substream under /proc/asound.
func alsaHWParams() (format string, rate int) {
	paths, _ := filepath.Glob("/proc/asound/card*/pcm*p/sub*/hw_params")
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		text := strings.TrimSpace(string(b))
		if text == "" || text == "closed" {
			continue
		}
		var f string
		var r int
		for _, line := range strings.Split(text, "\n") {
			k, v, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			k, v = strings.TrimSpace(k), strings.TrimSpace(v)
			if k == "format" {
				f = v
			} else if k == "rate" {
				if fields := strings.Fields(v); len(fields) > 0 {
					r = atoi(fields[0])
				}
			}
		}
		if f != "" {
			return f, r
		}
	}
	return "", 0
}

func (p *Player) Status() (Status, error) {
	var st Status
	err := p.with(func(cl *mpd.Client) error {
		a, err := cl.Status()
		if err != nil {
			return err
		}
		st.Connected = true
		st.State = a["state"]
		st.Elapsed = atof(a["elapsed"])
		st.Duration = atof(a["duration"])
		st.Volume = atoi(a["volume"])
		st.Repeat, st.Random = isOn(a["repeat"]), isOn(a["random"])
		st.Single, st.Consume = isOn(a["single"]), isOn(a["consume"])
		st.QID, st.Pos = atoi(a["songid"]), atoi(a["song"])
		st.Format = parseAudio(a["audio"])
		cur, err := cl.CurrentSong()
		if err != nil {
			return err
		}
		st.Song = songFrom(cur)
		return nil
	})
	if err != nil {
		return Status{Connected: false}, err
	}
	if st.Format != nil {
		if f, r := alsaHWParams(); f != "" {
			st.Format.OutputFmt, st.Format.OutputRate = f, r
		}
	}
	return st, nil
}

func (p *Player) Queue() ([]Song, error) {
	var out []Song
	err := p.with(func(cl *mpd.Client) error {
		items, err := cl.PlaylistInfo(-1, -1)
		if err != nil {
			return err
		}
		out = out[:0]
		for _, it := range items {
			if s := songFrom(it); s != nil {
				out = append(out, *s)
			}
		}
		return nil
	})
	return out, err
}

func (p *Player) Play(pos int) error { return p.with(func(c *mpd.Client) error { return c.Play(pos) }) }
func (p *Player) PlayID(id int) error {
	return p.with(func(c *mpd.Client) error { return c.PlayID(id) })
}
func (p *Player) Stop() error  { return p.with(func(c *mpd.Client) error { return c.Stop() }) }
func (p *Player) Next() error  { return p.with(func(c *mpd.Client) error { return c.Next() }) }
func (p *Player) Prev() error  { return p.with(func(c *mpd.Client) error { return c.Previous() }) }
func (p *Player) Clear() error { return p.with(func(c *mpd.Client) error { return c.Clear() }) }

func (p *Player) DeleteID(id int) error {
	return p.with(func(c *mpd.Client) error { return c.DeleteID(id) })
}

func (p *Player) Add(uri string) error {
	return p.with(func(c *mpd.Client) error { return c.Add(uri) })
}

// Pause toggles while playing, matching POST /player/pause.
func (p *Player) Pause() error {
	return p.with(func(c *mpd.Client) error {
		a, err := c.Status()
		if err != nil {
			return err
		}
		return c.Pause(a["state"] == "play")
	})
}

func (p *Player) SetVolume(v int) error {
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	return p.with(func(c *mpd.Client) error { return c.SetVolume(v) })
}

func (p *Player) Seek(seconds float64, relative bool) error {
	d := time.Duration(seconds * float64(time.Second))
	return p.with(func(c *mpd.Client) error { return c.SeekCur(d, relative) })
}

func (p *Player) SetOption(name string, on bool) error {
	return p.with(func(c *mpd.Client) error {
		switch name {
		case "repeat":
			return c.Repeat(on)
		case "random":
			return c.Random(on)
		case "single":
			return c.Single(on)
		case "consume":
			return c.Consume(on)
		}
		return errors.New("unknown option " + name)
	})
}

type Output struct {
	MPDID   int    `json:"mpd_output_id"`
	Alias   string `json:"alias"`
	Enabled bool   `json:"enabled"`
	Plugin  string `json:"plugin,omitempty"`
}

func (p *Player) Outputs() ([]Output, error) {
	var out []Output
	err := p.with(func(c *mpd.Client) error {
		list, err := c.ListOutputs()
		if err != nil {
			return err
		}
		out = out[:0]
		for _, o := range list {
			out = append(out, Output{
				MPDID:   atoi(o["outputid"]),
				Alias:   o["outputname"],
				Enabled: isOn(o["outputenabled"]),
				Plugin:  o["plugin"],
			})
		}
		return nil
	})
	return out, err
}

// SetActiveOutput enables one output and disables the others (FR-5.2).
func (p *Player) SetActiveOutput(id int) error {
	return p.with(func(c *mpd.Client) error {
		list, err := c.ListOutputs()
		if err != nil {
			return err
		}
		for _, o := range list {
			oid := atoi(o["outputid"])
			if oid == id {
				if err := c.EnableOutput(oid); err != nil {
					return err
				}
			} else if isOn(o["outputenabled"]) {
				if err := c.DisableOutput(oid); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Watch relays MPD idle subsystem names to onEvent until stop is called.
func (p *Player) Watch(onEvent func(subsystem string)) (stop func(), err error) {
	w, err := mpd.NewWatcher(p.network, p.addr, "",
		"player", "mixer", "options", "playlist", "output", "database", "update")
	if err != nil {
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case ev, ok := <-w.Event:
				if !ok {
					return
				}
				onEvent(ev)
			case _, ok := <-w.Error:
				if !ok {
					return
				}
				// the watcher redials on its own; transient errors are ignored
			}
		}
	}()
	return func() { close(done); w.Close() }, nil
}

// Package config loads hifid's YAML configuration (docs/10 §4.1).
package config

import (
	"fmt"
	"net"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ServerName string  `yaml:"server_name"`
	Listen     string  `yaml:"listen"`
	Auth       Auth    `yaml:"auth"`
	Paths      Paths   `yaml:"paths"`
	MPD        MPD     `yaml:"mpd"`
	NetEase    NetEase `yaml:"netease"`
}

// Auth mode: none (trusted LAN) | admin (token for mutating admin calls) | all.
type Auth struct {
	Mode  string `yaml:"mode"`
	Token string `yaml:"-"` // from HIFID_TOKEN, never stored in the YAML
}

type Paths struct {
	Music    string `yaml:"music"`
	Data     string `yaml:"data"`
	Incoming string `yaml:"incoming"`
}

type NetEase struct {
	Enabled bool `yaml:"enabled"`
	// LevelPreference is the quality ladder, best first (docs/08 §4).
	LevelPreference []string `yaml:"level_preference"`
	// StreamMode must stay "pipe": the CDN mislabels FLAC (ADR-0008).
	StreamMode string `yaml:"stream_mode"`
}

type MPD struct {
	Socket string `yaml:"socket"`
	Host   string `yaml:"host"`
	Port   int    `yaml:"port"`
}

func Default() *Config {
	return &Config{
		ServerName: "HiFi Server",
		Listen:     "0.0.0.0:8080",
		Auth:       Auth{Mode: "admin"},
		Paths: Paths{
			Music:    "/srv/music",
			Data:     "/srv/data/hifid",
			Incoming: "/srv/data/incoming",
		},
		MPD: MPD{Socket: "/run/mpd/socket", Host: "127.0.0.1", Port: 6600},
		NetEase: NetEase{
			Enabled:         true,
			LevelPreference: []string{"jymaster", "hires", "lossless", "exhigh", "higher", "standard"},
			StreamMode:      "pipe",
		},
	}
}

// Load reads path over the defaults. A missing file is not an error: the
// defaults match the layout install.sh creates.
func Load(path string) (*Config, error) {
	c := Default()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("read %s: %w", path, err)
			}
		} else if err := yaml.Unmarshal(b, c); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	c.Auth.Token = os.Getenv("HIFID_TOKEN")
	switch c.Auth.Mode {
	case "", "none", "admin", "all":
	default:
		return nil, fmt.Errorf("auth.mode %q: want none|admin|all", c.Auth.Mode)
	}
	if c.Auth.Mode == "" {
		c.Auth.Mode = "admin"
	}
	return c, nil
}

// Dial returns the network and address for the MPD connection. The unix
// socket is preferred when present; TCP is the fallback (docs/10 §3).
func (m MPD) Dial() (network, addr string) {
	if m.Socket != "" {
		if fi, err := os.Stat(m.Socket); err == nil && fi.Mode()&os.ModeSocket != 0 {
			return "unix", m.Socket
		}
	}
	host := m.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := m.Port
	if port == 0 {
		port = 6600
	}
	return "tcp", net.JoinHostPort(host, strconv.Itoa(port))
}

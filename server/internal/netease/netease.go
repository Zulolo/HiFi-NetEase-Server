// Package netease adapts NetEase Cloud Music to hifid (ADR-0002). It wraps the
// chaunsin/netease-cloud-music weapi client, keeps the session on disk, and
// resolves playable URLs by walking the configured quality ladder (docs/08 §4).
package netease

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chaunsin/netease-cloud-music/api"
	"github.com/chaunsin/netease-cloud-music/api/types"
	"github.com/chaunsin/netease-cloud-music/api/weapi"
	"github.com/chaunsin/netease-cloud-music/pkg/cookie"
)

// ErrNotLoggedIn is returned when the stored session is missing or expired.
var ErrNotLoggedIn = errors.New("netease: not logged in")

type Config struct {
	// StateDir holds the cookie jar and the library's own state files.
	StateDir string
	// MusicDir is the root of the music tree; downloads land in <MusicDir>/netease.
	MusicDir string
	// Levels is the download ladder, best first (docs/08 §4).
	Levels []string
	// StreamLevels is the ladder for live playback. It is deliberately lower
	// than Levels: a 24/192 master needs ~690 kB/s sustained, more than a
	// 2.4 GHz link reliably carries, and a starved decoder stutters.
	StreamLevels []string
	Timeout      time.Duration
}

type Client struct {
	we           *weapi.Api
	raw          *api.Client
	levels       []types.Level
	streamLevels []types.Level
	musicDir     string
	idx          *index

	mu      sync.RWMutex
	urls    map[int64]cachedURL
	profile *Profile
	qr      map[string]qrSession
}

type cachedURL struct {
	res     Resolved
	expires time.Time
}

type Profile struct {
	LoggedIn bool   `json:"logged_in"`
	UserID   int64  `json:"user_id,omitempty"`
	Nickname string `json:"nickname,omitempty"`
	VipType  int64  `json:"vip_type,omitempty"`
}

// Resolved is one playable stream, with the level the account was actually
// granted rather than the one requested (docs/08 §4 rule 2).
type Resolved struct {
	ID         int64  `json:"id"`
	URL        string `json:"-"` // signed CDN URL: never sent to clients
	Level      string `json:"level"`
	Type       string `json:"type"`
	Size       int64  `json:"size"`
	Bitrate    int64  `json:"bitrate"`
	DurationMS int64  `json:"duration_ms"`
}

// ContentType is what the proxy must send to MPD. The CDN mislabels FLAC as
// audio/mpeg, which breaks MPD's decoder choice — see ADR-0008.
func (r Resolved) ContentType() string {
	switch strings.ToLower(r.Type) {
	case "flac":
		return "audio/flac"
	case "mp3":
		return "audio/mpeg"
	case "aac", "m4a":
		return "audio/mp4"
	case "wav":
		return "audio/wav"
	}
	return "application/octet-stream"
}

func DefaultLevels() []string {
	return []string{"jymaster", "hires", "lossless", "exhigh", "higher", "standard"}
}

// DefaultStreamLevels starts at lossless on purpose. Live playback is limited
// by the link, not by entitlement: lossless needs ~124 kB/s where a jymaster
// master needs ~690 kB/s. Downloads still use the full ladder.
func DefaultStreamLevels() []string {
	return []string{"lossless", "exhigh", "higher", "standard"}
}

func New(cfg Config) (*Client, error) {
	if cfg.StateDir == "" {
		return nil, errors.New("netease: StateDir is required")
	}
	if err := os.MkdirAll(cfg.StateDir, 0o700); err != nil {
		return nil, fmt.Errorf("netease: state dir: %w", err)
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	toLadder := func(in []string, def []string) []types.Level {
		if len(in) == 0 {
			in = def
		}
		out := make([]types.Level, 0, len(in))
		for _, l := range in {
			out = append(out, types.Level(l))
		}
		return out
	}
	ladder := toLadder(cfg.Levels, DefaultLevels())
	streamLadder := toLadder(cfg.StreamLevels, DefaultStreamLevels())

	raw, err := api.NewClient(&api.Config{
		Timeout: timeout,
		Retry:   2,
		HomeDir: cfg.StateDir,
		Cookie: cookie.Config{
			Filepath: filepath.Join(cfg.StateDir, "cookie.json"),
			// 3 s (the library default) keeps a fresh QR login durable almost
			// immediately; a longer interval risks losing it on a quick restart.
			Interval: 3 * time.Second,
		},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("netease: client: %w", err)
	}
	idx, err := openIndex(filepath.Join(cfg.StateDir, "downloads.json"))
	if err != nil {
		return nil, err
	}
	return &Client{
		we:           weapi.New(raw),
		raw:          raw,
		levels:       ladder,
		streamLevels: streamLadder,
		musicDir:     cfg.MusicDir,
		idx:          idx,
		urls:         make(map[int64]cachedURL),
	}, nil
}

func (c *Client) Close(ctx context.Context) error { return c.raw.Close(ctx) }

// Profile reports the logged-in account. The result is cached until a call fails.
func (c *Client) Profile(ctx context.Context) (Profile, error) {
	c.mu.RLock()
	p := c.profile
	c.mu.RUnlock()
	if p != nil {
		return *p, nil
	}
	resp, err := c.we.GetUserInfo(ctx, &weapi.GetUserInfoReq{})
	if err != nil {
		return Profile{}, fmt.Errorf("netease: user info: %w", err)
	}
	out := Profile{}
	if resp.Profile != nil && resp.Profile.UserId != 0 {
		out = Profile{
			LoggedIn: true,
			UserID:   resp.Profile.UserId,
			Nickname: resp.Profile.Nickname,
			VipType:  resp.Profile.VipType,
		}
	}
	if !out.LoggedIn {
		return out, ErrNotLoggedIn
	}
	c.mu.Lock()
	c.profile = &out
	c.mu.Unlock()
	return out, nil
}

type Playlist struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	TrackCount int64  `json:"track_count"`
	Cover      string `json:"cover,omitempty"`
	// Liked marks 我喜欢的音乐 (specialType 5).
	Liked bool `json:"liked,omitempty"`
}

type Track struct {
	ID       int64   `json:"id"`
	Ref      string  `json:"ref"` // ncm:<id>
	Title    string  `json:"title"`
	Artist   string  `json:"artist"`
	Album    string  `json:"album"`
	Cover    string  `json:"cover,omitempty"`
	Duration float64 `json:"duration"`
	// OnDisk means a downloaded copy exists, so playback needs no network.
	OnDisk bool `json:"on_disk"`
}

// Playlists lists the account's own and subscribed playlists (FR-1.3).
func (c *Client) Playlists(ctx context.Context, offset, limit int) ([]Playlist, error) {
	p, err := c.Profile(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := c.we.Playlist(ctx, &weapi.PlaylistReq{
		Uid:    strconv.FormatInt(p.UserID, 10),
		Offset: strconv.Itoa(offset),
		Limit:  strconv.Itoa(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("netease: playlists: %w", err)
	}
	out := make([]Playlist, 0, len(resp.Playlist))
	for _, pl := range resp.Playlist {
		out = append(out, Playlist{
			ID:         pl.Id,
			Name:       pl.Name,
			TrackCount: pl.TrackCount,
			Cover:      pl.CoverImgUrl,
			Liked:      pl.SpecialType == 5,
		})
	}
	return out, nil
}

// PlaylistTracks returns up to limit tracks. PlaylistDetail only carries ten
// full track objects, so the ids come from trackIds and the details from a
// batched SongDetail call.
func (c *Client) PlaylistTracks(ctx context.Context, id int64, offset, limit int) ([]Track, int, error) {
	detail, err := c.we.PlaylistDetail(ctx, &weapi.PlaylistDetailReq{
		Id: strconv.FormatInt(id, 10),
		N:  "0",
		S:  "0",
	})
	if err != nil {
		return nil, 0, fmt.Errorf("netease: playlist detail: %w", err)
	}
	all := detail.Playlist.TrackIds
	total := len(all)
	if offset >= total {
		return []Track{}, total, nil
	}
	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}
	page := all[offset:end]

	ids := make([]weapi.SongDetailReqList, 0, len(page))
	for _, t := range page {
		ids = append(ids, weapi.SongDetailReqList{Id: strconv.FormatInt(t.Id, 10), V: 0})
	}
	songs, err := c.we.SongDetail(ctx, &weapi.SongDetailReq{C: ids})
	if err != nil {
		return nil, total, fmt.Errorf("netease: song detail: %w", err)
	}
	byID := make(map[int64]Track, len(songs.Songs))
	for _, s := range songs.Songs {
		names := make([]string, 0, len(s.Ar))
		for _, a := range s.Ar {
			if a.Name != "" {
				names = append(names, a.Name)
			}
		}
		byID[s.Id] = Track{
			ID:       s.Id,
			Ref:      "ncm:" + strconv.FormatInt(s.Id, 10),
			Title:    s.Name,
			Artist:   strings.Join(names, ", "),
			Album:    s.Al.Name,
			Cover:    s.Al.PicUrl,
			Duration: float64(s.Dt) / 1000.0,
		}
	}
	// keep the playlist's own order
	out := make([]Track, 0, len(page))
	for _, t := range page {
		if tr, ok := byID[t.Id]; ok {
			_, tr.OnDisk = c.LocalPath(tr.ID)
			out = append(out, tr)
		}
	}
	return out, total, nil
}

// Resolve walks the quality ladder and returns the first level the account is
// actually granted. The granted level is read back from the response because it
// is not a monotonic function of the request (docs/08 §4).
// Resolve picks a URL for live streaming (the lower ladder).
func (c *Client) Resolve(ctx context.Context, id int64) (Resolved, error) {
	return c.resolve(ctx, id, c.streamLevels, true)
}

// ResolveBest picks the highest level the account is granted, for downloading.
func (c *Client) ResolveBest(ctx context.Context, id int64) (Resolved, error) {
	return c.resolve(ctx, id, c.levels, false)
}

func (c *Client) resolve(ctx context.Context, id int64, ladder []types.Level, useCache bool) (Resolved, error) {
	if useCache {
		c.mu.RLock()
		hit, ok := c.urls[id]
		c.mu.RUnlock()
		if ok && time.Now().Before(hit.expires) {
			return hit.res, nil
		}
	}

	var lastErr error
	for _, level := range ladder {
		resp, err := c.we.SongPlayerV1(ctx, &weapi.SongPlayerV1Req{
			Ids:        types.IntsString{id},
			Level:      level,
			EncodeType: "flac",
		})
		if err != nil {
			lastErr = err
			continue
		}
		if len(resp.Data) == 0 {
			lastErr = fmt.Errorf("no data for level %s", level)
			continue
		}
		d := resp.Data[0]
		if d.Url == "" || d.Code != 200 {
			// not entitled or unavailable at this level: try the next rung
			lastErr = fmt.Errorf("level %s unavailable (code %d)", level, d.Code)
			continue
		}
		// NetEase may answer a request for one level with a *different* one.
		// A lower stereo level is the documented graceful downgrade and is
		// accepted. An effect variant (sky, jyeffect, dolby, vivid) is not:
		// it is a DSP-processed surround mix, excluded by docs/08 §4, and
		// showed up as a 16/44 "sky" FLAC where the real stereo release was a
		// 320 kbps MP3. Skip it and let the next rung ask for that release.
		if !stereoLevel(d.Level) {
			lastErr = fmt.Errorf("level %s answered with excluded variant %q", level, d.Level)
			continue
		}
		res := Resolved{
			ID:         d.Id,
			URL:        d.Url,
			Level:      d.Level,
			Type:       d.Type,
			Size:       d.Size,
			Bitrate:    d.Br,
			DurationMS: d.Time,
		}
		ttl := time.Duration(d.Expi) * time.Second
		if ttl <= 0 {
			ttl = 20 * time.Minute
		}
		// renew a minute early so a track never starts on a dying URL
		if ttl > time.Minute {
			ttl -= time.Minute
		}
		if useCache {
			c.mu.Lock()
			c.urls[id] = cachedURL{res: res, expires: time.Now().Add(ttl)}
			c.mu.Unlock()
		}
		return res, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no level granted")
	}
	return Resolved{}, fmt.Errorf("netease: resolve %d: %w", id, lastErr)
}

// Forget drops a cached URL so the next play resolves a fresh one.
func (c *Client) Forget(id int64) {
	c.mu.Lock()
	delete(c.urls, id)
	c.mu.Unlock()
}

// TrackInfo returns one track's metadata, used to tag the MPD queue entry.
func (c *Client) TrackInfo(ctx context.Context, id int64) (Track, error) {
	songs, err := c.we.SongDetail(ctx, &weapi.SongDetailReq{
		C: []weapi.SongDetailReqList{{Id: strconv.FormatInt(id, 10)}},
	})
	if err != nil {
		return Track{}, fmt.Errorf("netease: song detail: %w", err)
	}
	if len(songs.Songs) == 0 {
		return Track{}, fmt.Errorf("netease: song %d not found", id)
	}
	s := songs.Songs[0]
	names := make([]string, 0, len(s.Ar))
	for _, a := range s.Ar {
		if a.Name != "" {
			names = append(names, a.Name)
		}
	}
	return Track{
		ID:       s.Id,
		Ref:      "ncm:" + strconv.FormatInt(s.Id, 10),
		Title:    s.Name,
		Artist:   strings.Join(names, ", "),
		Album:    s.Al.Name,
		Cover:    s.Al.PicUrl,
		Duration: float64(s.Dt) / 1000.0,
	}, nil
}

// ParseRef turns "ncm:123" into its numeric song id.
func ParseRef(ref string) (int64, bool) {
	rest, ok := strings.CutPrefix(ref, "ncm:")
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// stereoLevel reports whether a granted level is a plain stereo release rather
// than one of NetEase's processed effect variants.
func stereoLevel(level string) bool {
	switch types.Level(level) {
	case types.LevelStandard, types.LevelHigher, types.LevelExhigh,
		types.LevelLossless, types.LevelHires, types.LevelJymaster:
		return true
	}
	return false
}

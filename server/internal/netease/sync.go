package netease

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"
)

// SyncState is the persisted list of playlists kept on disk (docs/08 §7.1).
type SyncState struct {
	Playlists []int64 `json:"playlists"`
}

// SyncStatus is what the UI shows.
type SyncStatus struct {
	Running     bool      `json:"running"`
	Playlists   []int64   `json:"playlists"`
	Wanted      int       `json:"wanted"`
	OnDisk      int       `json:"on_disk"`
	Downloaded  int       `json:"downloaded_this_run"`
	Failed      int       `json:"failed_this_run"`
	Current     string    `json:"current,omitempty"`
	LastRun     time.Time `json:"last_run,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
	TotalOnDisk int       `json:"total_on_disk"`
}

// Syncer downloads subscribed playlists in the background, paced so the
// account is never hammered (docs/12 R18: bulk downloading risks 风控).
type Syncer struct {
	c     *Client
	// OnDownloaded is called with the music-relative path of each new file so
	// the caller can trigger an incremental MPD update.
	OnDownloaded func(rel string)
	log   *slog.Logger
	path  string
	pace  time.Duration
	mu    sync.Mutex
	state SyncState
	st    SyncStatus
	// running guards against two concurrent runs.
	running bool
}

func NewSyncer(c *Client, statePath string, pace time.Duration, log *slog.Logger) *Syncer {
	if pace <= 0 {
		pace = 15 * time.Second
	}
	s := &Syncer{c: c, log: log, path: statePath, pace: pace}
	if b, err := os.ReadFile(statePath); err == nil {
		_ = json.Unmarshal(b, &s.state)
	}
	return s
}

func (s *Syncer) save() error {
	b, err := json.MarshalIndent(s.state, "", " ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Syncer) Subscribe(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.state.Playlists {
		if existing == id {
			return nil
		}
	}
	s.state.Playlists = append(s.state.Playlists, id)
	return s.save()
}

func (s *Syncer) Unsubscribe(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.state.Playlists[:0]
	for _, existing := range s.state.Playlists {
		if existing != id {
			out = append(out, existing)
		}
	}
	s.state.Playlists = out
	return s.save()
}

func (s *Syncer) Status() SyncStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.st
	out.Running = s.running
	out.Playlists = append([]int64(nil), s.state.Playlists...)
	out.TotalOnDisk = s.c.Downloaded()
	return out
}

// ErrSyncBusy means a run is already in progress.
var ErrSyncBusy = errors.New("sync already running")

// RunOnce walks every subscribed playlist and downloads what is missing. It is
// paced and resumable: an interrupted run simply picks up the gaps next time,
// because presence on disk is the only state that matters.
func (s *Syncer) RunOnce(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return ErrSyncBusy
	}
	s.running = true
	lists := append([]int64(nil), s.state.Playlists...)
	s.st = SyncStatus{}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.st.LastRun = time.Now()
		s.st.Current = ""
		s.mu.Unlock()
	}()

	for _, plID := range lists {
		offset := 0
		for {
			tracks, total, err := s.c.PlaylistTracks(ctx, plID, offset, 200)
			if err != nil {
				s.setErr(err)
				break
			}
			if len(tracks) == 0 {
				break
			}
			for _, t := range tracks {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				s.mu.Lock()
				s.st.Wanted++
				s.mu.Unlock()

				if _, ok := s.c.LocalPath(t.ID); ok {
					s.mu.Lock()
					s.st.OnDisk++
					s.mu.Unlock()
					continue
				}
				s.mu.Lock()
				s.st.Current = t.Artist + " — " + t.Title
				s.mu.Unlock()

				rel, err := s.c.Download(ctx, t.ID)
				if err != nil {
					s.setErr(err)
					s.mu.Lock()
					s.st.Failed++
					s.mu.Unlock()
					if s.log != nil {
						s.log.Warn("sync download failed", "song", t.ID, "title", t.Title, "err", err)
					}
				} else {
					s.mu.Lock()
					s.st.Downloaded++
					s.mu.Unlock()
					if s.OnDownloaded != nil {
						s.OnDownloaded(rel)
					}
					if s.log != nil {
						s.log.Info("sync downloaded", "song", t.ID, "title", t.Title)
					}
				}
				// pace between fetches, but stay responsive to shutdown
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(s.pace):
				}
			}
			offset += len(tracks)
			if offset >= total {
				break
			}
		}
	}
	return nil
}

func (s *Syncer) setErr(err error) {
	s.mu.Lock()
	s.st.LastError = err.Error()
	s.mu.Unlock()
}

// Loop runs a sync pass every interval until ctx is cancelled.
func (s *Syncer) Loop(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = 6 * time.Hour
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.RunOnce(ctx); err != nil && !errors.Is(err, ErrSyncBusy) && s.log != nil {
				s.log.Warn("sync run failed", "err", err)
			}
		}
	}
}

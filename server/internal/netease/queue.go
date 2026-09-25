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

// Queue is the explicit download list, mirroring the desktop client: playing a
// track never downloads it, only what the user adds here is fetched, and it is
// fetched at the download ladder (best granted level) rather than the lower
// ladder used for live streaming.
type Queue struct {
	c    *Client
	log  *slog.Logger
	path string
	pace time.Duration

	// OnDownloaded receives the music-relative path of each finished file so
	// the caller can trigger an incremental MPD update.
	OnDownloaded func(rel string)

	mu      sync.Mutex
	pending []Item
	failed  []Item
	current string
	curItem *Item
	curDone int64
	curSize int64
	done    int
	lastErr string
	wake    chan struct{}
}

// Item is one queued download.
type Item struct {
	ID     int64  `json:"id"`
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Tries  int    `json:"tries,omitempty"`
}

type QueueStatus struct {
	Pending     []Item `json:"pending"`
	Failed      []Item `json:"failed"`
	Current     string `json:"current,omitempty"`
	CurrentItem *Item  `json:"current_item,omitempty"`
	CurrentDone int64  `json:"current_bytes"`
	CurrentSize int64  `json:"current_size"`
	Done        int    `json:"done"`
	TotalOnDisk int    `json:"total_on_disk"`
	OnDiskBytes int64  `json:"on_disk_bytes"`
	LastError   string `json:"last_error,omitempty"`
}

type queueFile struct {
	Pending []Item `json:"pending"`
	Failed  []Item `json:"failed"`
}

const maxTries = 3

func NewQueue(c *Client, path string, pace time.Duration, log *slog.Logger) *Queue {
	if pace <= 0 {
		pace = 5 * time.Second
	}
	q := &Queue{c: c, log: log, path: path, pace: pace, wake: make(chan struct{}, 1)}
	if b, err := os.ReadFile(path); err == nil {
		var f queueFile
		if json.Unmarshal(b, &f) == nil {
			q.pending, q.failed = f.Pending, f.Failed
		}
	}
	return q
}

// save must be called with the lock held.
func (q *Queue) save() {
	b, err := json.MarshalIndent(queueFile{Pending: q.pending, Failed: q.failed}, "", " ")
	if err != nil {
		return
	}
	tmp := q.path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, q.path)
	}
}

func (q *Queue) nudge() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// Add enqueues one song. Songs already on disk or already queued are ignored,
// so tapping download twice is harmless.
func (q *Queue) Add(items ...Item) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	var added int
	for _, it := range items {
		if _, ok := q.c.LocalPath(it.ID); ok {
			continue
		}
		dup := false
		for _, p := range q.pending {
			if p.ID == it.ID {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		it.Tries = 0
		q.pending = append(q.pending, it)
		added++
	}
	if added > 0 {
		q.save()
		q.nudge()
	}
	return added
}

func (q *Queue) Clear() {
	q.mu.Lock()
	q.pending = nil
	q.failed = nil
	q.save()
	q.mu.Unlock()
}

func (q *Queue) Status() QueueStatus {
	q.mu.Lock()
	defer q.mu.Unlock()
	return QueueStatus{
		Pending:     append([]Item(nil), q.pending...),
		Failed:      append([]Item(nil), q.failed...),
		Current:     q.current,
		CurrentItem: q.curItem,
		CurrentDone: q.curDone,
		CurrentSize: q.curSize,
		Done:        q.done,
		TotalOnDisk: q.c.Downloaded(),
		OnDiskBytes: q.c.DownloadedBytes(),
		LastError:   q.lastErr,
	}
}

// Pending reports how many downloads are waiting, for callers that only need
// the count.
func (q *Queue) Pending() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

// Run processes the list one song at a time until ctx is cancelled. It idles on
// a channel rather than polling, so an empty list costs nothing.
func (q *Queue) Run(ctx context.Context) {
	for {
		it, ok := q.next()
		if !ok {
			select {
			case <-ctx.Done():
				return
			case <-q.wake:
				continue
			case <-time.After(5 * time.Minute):
				continue // periodic retry sweep for anything re-queued
			}
		}

		q.mu.Lock()
		q.current = it.Artist + " — " + it.Title
		cur := it
		q.curItem, q.curDone, q.curSize = &cur, 0, 0
		q.mu.Unlock()

		rel, err := q.c.DownloadWithProgress(ctx, it.ID, func(done, total int64) {
			q.mu.Lock()
			q.curDone, q.curSize = done, total
			q.mu.Unlock()
		})

		q.mu.Lock()
		q.current = ""
		q.curItem, q.curDone, q.curSize = nil, 0, 0
		if err != nil {
			if ctx.Err() != nil {
				// shutting down: put it back untouched
				q.pending = append([]Item{it}, q.pending...)
				q.save()
				q.mu.Unlock()
				return
			}
			it.Tries++
			q.lastErr = err.Error()
			if it.Tries >= maxTries {
				q.failed = append(q.failed, it)
				if q.log != nil {
					q.log.Warn("download gave up", "song", it.ID, "title", it.Title, "err", err)
				}
			} else {
				q.pending = append(q.pending, it) // back of the line
			}
		} else {
			q.done++
			if q.log != nil {
				q.log.Info("downloaded", "song", it.ID, "title", it.Title, "path", rel)
			}
		}
		q.save()
		q.mu.Unlock()

		if err == nil && q.OnDownloaded != nil {
			q.OnDownloaded(rel)
		}

		// pace between fetches so bulk adds never hammer the account (R18)
		select {
		case <-ctx.Done():
			return
		case <-time.After(q.pace):
		}
	}
}

func (q *Queue) next() (Item, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.pending) > 0 {
		it := q.pending[0]
		q.pending = q.pending[1:]
		if _, ok := q.c.LocalPath(it.ID); ok {
			continue // downloaded by another path in the meantime
		}
		q.save()
		return it, true
	}
	return Item{}, false
}

// AddPlaylist queues every track of a playlist that is not already on disk.
// This is an explicit one-off action, not a standing subscription.
func (q *Queue) AddPlaylist(ctx context.Context, playlistID int64) (int, error) {
	var added, offset int
	for {
		tracks, total, err := q.c.PlaylistTracks(ctx, playlistID, offset, 200)
		if err != nil {
			return added, err
		}
		if len(tracks) == 0 {
			return added, nil
		}
		items := make([]Item, 0, len(tracks))
		for _, t := range tracks {
			items = append(items, Item{ID: t.ID, Title: t.Title, Artist: t.Artist})
		}
		added += q.Add(items...)
		offset += len(tracks)
		if offset >= total {
			return added, nil
		}
	}
}

var ErrQueueDisabled = errors.New("download queue is not enabled")

package netease

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// browserUA keeps the CDN happy; it rejects some default client agents.
const browserUA = "Mozilla/5.0 (X11; Linux aarch64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"

// ServeStream implements GET /stream/ncm/{id} in pipe mode (ADR-0008).
//
// The CDN labels FLAC objects "Content-Type: audio/mpeg", which makes MPD pick
// the mp3 decoder and fail. A 302 inherits the same wrong header, so the bytes
// are relayed here and the type is corrected from the granted level. Range is
// passed through because the CDN answers 206, keeping seeking and read-ahead.
func (c *Client) ServeStream(w http.ResponseWriter, r *http.Request, id int64, log *slog.Logger) {
	res, err := c.Resolve(r.Context(), id)
	if err != nil {
		http.Error(w, "resolve failed", http.StatusBadGateway)
		if log != nil {
			log.Warn("ncm stream resolve failed", "song", id, "err", err)
		}
		return
	}

	upstream, err := c.fetch(r, res)
	if err != nil {
		// A cached URL may have expired early; drop it and retry once.
		c.Forget(id)
		if res, err = c.Resolve(r.Context(), id); err == nil {
			upstream, err = c.fetch(r, res)
		}
		if err != nil {
			http.Error(w, "upstream failed", http.StatusBadGateway)
			if log != nil {
				log.Warn("ncm stream upstream failed", "song", id, "err", err)
			}
			return
		}
	}
	defer upstream.Body.Close()

	h := w.Header()
	h.Set("Content-Type", res.ContentType()) // the whole point of ADR-0008
	h.Set("Accept-Ranges", "bytes")
	for _, k := range []string{"Content-Length", "Content-Range"} {
		if v := upstream.Header.Get(k); v != "" {
			h.Set(k, v)
		}
	}
	code := http.StatusOK
	if upstream.StatusCode == http.StatusPartialContent {
		code = http.StatusPartialContent
	}
	w.WriteHeader(code)

	if r.Method == http.MethodHead {
		return
	}
	// 8 MB of read-ahead (64 x 128 kB) rides out Wi-Fi stalls.
	pf := newPrefetch(r.Context(), upstream.Body, 128<<10, 64)
	if _, err := pf.WriteTo(w); err != nil && log != nil {
		// a client that seeks or skips closes the socket: that is normal
		log.Debug("ncm stream ended", "song", id, "err", err)
	}
}

func (c *Client) fetch(r *http.Request, res Resolved) (*http.Response, error) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, res.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", browserUA)
	if rng := r.Header.Get("Range"); rng != "" {
		req.Header.Set("Range", rng)
	}
	resp, err := streamHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return nil, fmt.Errorf("upstream status %s", resp.Status)
	}
	return resp, nil
}

// streamHTTP has no overall timeout: a 157 MB jymaster track is a long read.
// The dial and header timeouts still bound a dead CDN node (seen 2026-09-25:
// one node stopped answering SYNs while others were fine; without a dial
// timeout each attempt sat in SYN-SENT for the kernel's two minutes).
var streamHTTP = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:          8,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	},
}

// prefetch decouples MPD's reads from network jitter: a goroutine pulls from
// the CDN as fast as the link allows into a bounded in-memory queue, and the
// HTTP handler drains that queue. It cannot cure a sustained bandwidth deficit
// (for that, stream a lower level or play a downloaded copy), but it absorbs
// the stalls that otherwise show up as "Decoder is too slow" in the MPD log.
type prefetch struct {
	chunks chan []byte
	err    chan error
	cancel func()
}

func newPrefetch(ctx context.Context, src io.Reader, chunk, depth int) *prefetch {
	if chunk <= 0 {
		chunk = 128 << 10
	}
	if depth <= 0 {
		depth = 64
	}
	ctx, cancel := context.WithCancel(ctx)
	p := &prefetch{
		chunks: make(chan []byte, depth),
		err:    make(chan error, 1),
		cancel: cancel,
	}
	go func() {
		defer close(p.chunks)
		for {
			buf := make([]byte, chunk)
			n, err := io.ReadFull(src, buf)
			if n > 0 {
				select {
				case p.chunks <- buf[:n]:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
					p.err <- err
				}
				return
			}
		}
	}()
	return p
}

// WriteTo drains the queue into w.
func (p *prefetch) WriteTo(w io.Writer) (int64, error) {
	defer p.cancel()
	var total int64
	for b := range p.chunks {
		n, err := w.Write(b)
		total += int64(n)
		if err != nil {
			return total, err
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	select {
	case err := <-p.err:
		return total, err
	default:
		return total, nil
	}
}

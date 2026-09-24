package netease

import (
	"fmt"
	"io"
	"log/slog"
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
	if _, err := io.Copy(w, upstream.Body); err != nil && log != nil {
		// a client that seeks or skips closes the socket: that is normal
		log.Debug("ncm stream copy ended", "song", id, "err", err)
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
// The dial and header timeouts still bound a dead CDN.
var streamHTTP = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          8,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	},
}

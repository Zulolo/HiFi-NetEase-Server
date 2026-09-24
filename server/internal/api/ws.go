package api

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// hub fans MPD idle events out to the connected PWA clients (docs/09 §11).
type hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]chan []byte
	up      websocket.Upgrader
}

func newHub() *hub {
	return &hub{
		clients: make(map[*websocket.Conn]chan []byte),
		up: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 4096,
			// LAN-only service (NFR-5); the PWA is served from this same origin.
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (h *hub) count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *hub) add(c *websocket.Conn) chan []byte {
	ch := make(chan []byte, 16)
	h.mu.Lock()
	h.clients[c] = ch
	h.mu.Unlock()
	return ch
}

func (h *hub) remove(c *websocket.Conn) {
	h.mu.Lock()
	if ch, ok := h.clients[c]; ok {
		close(ch)
		delete(h.clients, c)
	}
	h.mu.Unlock()
	_ = c.Close()
}

func (h *hub) broadcast(msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, ch := range h.clients {
		select {
		case ch <- msg:
		default: // slow client: drop rather than block the MPD watcher
		}
	}
}

func (h *hub) serveWS(w http.ResponseWriter, r *http.Request) {
	c, err := h.up.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	ch := h.add(c)
	defer h.remove(c)

	// Reader: we expect no client messages, but the read loop is what
	// detects a closed socket and keeps pongs flowing.
	go func() {
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				h.remove(c)
				return
			}
		}
	}()

	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			_ = c.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ping.C:
			_ = c.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package api

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

const maxWebSocketClients = 10

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		allowed := allowedWSOrigins()
		if len(allowed) == 0 {
			return true // no restriction configured — allow all (local dev default)
		}
		for _, a := range allowed {
			if strings.EqualFold(origin, a) {
				return true
			}
		}
		return false
	},
}

// allowedWSOrigins returns the configured allowed origins for WebSocket
// connections. If ALLOWED_WS_ORIGINS is unset, returns nil (allow all).
func allowedWSOrigins() []string {
	v := os.Getenv("ALLOWED_WS_ORIGINS")
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// wsClient wraps a websocket.Conn with a per-connection write mutex.
type wsClient struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

// wsHub manages WebSocket client connections and broadcasts.
type wsHub struct {
	mu      sync.RWMutex
	clients map[*wsClient]bool
}

func newWSHub() *wsHub {
	return &wsHub{clients: make(map[*wsClient]bool)}
}

// tryAdd atomically checks the client limit and adds the connection.
// Returns the wsClient on success, or nil if the limit is reached.
func (h *wsHub) tryAdd(conn *websocket.Conn) *wsClient {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.clients) >= maxWebSocketClients {
		return nil
	}
	c := &wsClient{conn: conn}
	h.clients[c] = true
	return c
}

func (h *wsHub) removeClient(c *wsClient) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	_ = c.conn.Close()
}

func (h *wsHub) broadcast(msg []byte) {
	// Snapshot clients under read lock.
	h.mu.RLock()
	snapshot := make([]*wsClient, 0, len(h.clients))
	for c := range h.clients {
		snapshot = append(snapshot, c)
	}
	h.mu.RUnlock()

	// Write to each client outside the hub lock, using per-client mutex.
	var failed []*wsClient
	for _, c := range snapshot {
		c.writeMu.Lock()
		err := c.conn.WriteMessage(websocket.TextMessage, msg)
		c.writeMu.Unlock()
		if err != nil {
			slog.Warn("WebSocket write failed", "error", err)
			failed = append(failed, c)
		}
	}

	// Remove failed clients.
	for _, c := range failed {
		h.removeClient(c)
	}
}

// closeAll sends close frames and removes all clients.
func (h *wsHub) closeAll() {
	h.mu.Lock()
	clients := make([]*wsClient, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.clients = make(map[*wsClient]bool)
	h.mu.Unlock()
	for _, c := range clients {
		_ = c.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutdown"))
		_ = c.conn.Close()
	}
}

package main

// 管理端：独立端口的 REST API + WebSocket 实时推送 + 内嵌前端。

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed all:web/dist
var webDist embed.FS

// ---------------- WebSocket Hub ----------------

type wsClient struct {
	hub  *wsHub
	conn *websocket.Conn
	send chan []byte
	done chan struct{}
	once sync.Once
}

type wsHub struct {
	mu      sync.Mutex
	clients map[*wsClient]struct{}
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func (h *wsHub) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &wsClient{hub: h, conn: conn, send: make(chan []byte, 4), done: make(chan struct{})}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	go c.writeLoop()
	go c.readLoop()
}

// dropLocked 关闭并移除客户端，调用方需持有 h.mu。
func (h *wsHub) dropLocked(c *wsClient) {
	if _, ok := h.clients[c]; !ok {
		return
	}
	delete(h.clients, c)
	c.once.Do(func() {
		close(c.done)
		c.conn.Close()
	})
}

func (c *wsClient) drop() {
	c.hub.mu.Lock()
	c.hub.dropLocked(c)
	c.hub.mu.Unlock()
}

func (c *wsClient) writeLoop() {
	for {
		select {
		case msg := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				c.drop()
				return
			}
		case <-c.done:
			return
		}
	}
}

func (c *wsClient) readLoop() {
	defer c.drop()
	c.conn.SetReadLimit(1024)
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (h *wsHub) loop() {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for range tick.C {
		b, err := json.Marshal(stats.Snapshot())
		if err != nil {
			continue
		}
		h.mu.Lock()
		for c := range h.clients {
			select {
			case c.send <- b:
			default: // 慢客户端直接踢掉
				h.dropLocked(c)
			}
		}
		h.mu.Unlock()
	}
}

// ---------------- HTTP 服务 ----------------

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func handleCloseConn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[3] != "close" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	id, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]bool{"ok": stats.closeConn(id)})
}

func staticHandler() http.Handler {
	sub, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		log.Fatal("monitor: embed web/dist:", err)
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(sub, p); err != nil {
			r.URL.Path = "/" // SPA 回退到首页
		}
		fileServer.ServeHTTP(w, r)
	})
}

func startAdmin(addr string, hdb *HistoryDB) {
	hub := &wsHub{clients: make(map[*wsClient]struct{})}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.handleWS)
	mux.HandleFunc("/api/overview", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, stats.Overview())
	})
	mux.HandleFunc("/api/connections", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"active": stats.Active(), "closed": stats.RecentClosed(100)})
	})
	mux.HandleFunc("/api/connections/", handleCloseConn)
	mux.HandleFunc("/api/tops", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"domains": stats.TopDomains(20), "rules": stats.TopRules(15)})
	})
	mux.HandleFunc("/api/history", func(w http.ResponseWriter, r *http.Request) {
		h := hdb
		if h == nil {
			writeJSON(w, HistoryResult{Range: r.URL.Query().Get("range"), Points: []HistoryPoint{}, Domains: []HistoryDomain{}})
			return
		}
		writeJSON(w, h.queryHistory(r.URL.Query().Get("range")))
	})
	mux.Handle("/", staticHandler())

	go hub.loop()
	srv := &http.Server{Addr: addr, Handler: mux}
	log.Printf("monitor: dashboard on http://%s", addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Println("monitor server:", err)
	}
}

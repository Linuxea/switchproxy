package main

// 管理端：独立端口的 REST API + WebSocket 实时推送 + 内嵌前端。

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net"
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

// ---------------- 设置 API ----------------

// handleConfig GET 返回当前配置摘要；PUT 应用设置页改动（点状写回，保留注释与规则）。
func handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg := current.Load().(*Config)
		rules := cfg.Rules
		if rules == nil {
			rules = []string{}
		}
		writeJSON(w, map[string]any{
			"listen":        cfg.Listen,
			"monitorListen": cfg.Monitor.Listen,
			"default":       cfg.Default,
			"rules":         rules,
			"upstream": map[string]string{
				"type":     cfg.Upstream.Type,
				"addr":     cfg.Upstream.Addr,
				"username": cfg.Upstream.Username,
				"password": cfg.Upstream.Password,
			},
			"ruleCount":  len(cfg.rules),
			"configPath": configPath,
			"version":    version,
		})
	case http.MethodPut:
		var p settingsPayload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, "请求体不是合法 JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := p.validate(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		closed, err := applySettings(&p)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "closed": closed})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleConfigImport 用请求体里的整份 YAML 替换当前配置（先整体校验，坏行报错不落盘）。
func handleConfigImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "读取请求体失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	body = bytes.TrimPrefix(body, []byte("\xef\xbb\xbf")) // 容忍 Windows 编辑器留下的 BOM
	if len(bytes.TrimSpace(body)) == 0 {
		http.Error(w, "导入内容为空", http.StatusBadRequest)
		return
	}
	closeConns := r.URL.Query().Get("closeConns") == "1"
	closed, err := applyConfigBytes(body, closeConns)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// 带回实际生效的管理端地址：导入可能换端口，前端据此引导跳转
	cfg := current.Load().(*Config)
	writeJSON(w, map[string]any{"ok": true, "closed": closed, "monitorListen": cfg.Monitor.Listen})
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

// ---------------- 管理端监听（支持热切换） ----------------

var (
	adminMu   sync.Mutex
	adminLn   net.Listener
	adminAddr string
	adminHdb  *HistoryDB

	adminOnce    sync.Once
	adminHandler http.Handler
)

func setAdminDB(hdb *HistoryDB) {
	adminMu.Lock()
	adminHdb = hdb
	adminMu.Unlock()
}

// newAdminHandler 路由与 hub 只建一次；监听器换地址时复用同一份 handler。
func newAdminHandler() http.Handler {
	adminOnce.Do(func() {
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
			h := adminHdb
			if h == nil {
				writeJSON(w, HistoryResult{Range: r.URL.Query().Get("range"), Points: []HistoryPoint{}, Domains: []HistoryDomain{}})
				return
			}
			writeJSON(w, h.queryHistory(r.URL.Query().Get("range")))
		})
		mux.HandleFunc("/api/config", handleConfig)
		mux.HandleFunc("/api/config/import", handleConfigImport)
		mux.Handle("/", staticHandler())
		go hub.loop()
		adminHandler = mux
	})
	return adminHandler
}

// swapAdminListener 把面板监听切换到 addr（相同地址为 no-op）；
// 绑定失败保留旧监听并返回错误。启动即关闭监控（disable: true）则保持关闭。
func swapAdminListener(addr string) error {
	adminMu.Lock()
	defer adminMu.Unlock()
	if adminLn == nil && current.Load().(*Config).Monitor.Disable {
		return nil
	}
	if adminAddr == addr {
		return nil
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	old := adminLn
	adminLn, adminAddr = l, addr
	go func() {
		srv := &http.Server{Handler: newAdminHandler()}
		if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			if !errors.Is(err, net.ErrClosed) {
				log.Println("monitor server:", err)
			}
		}
	}()
	if old != nil {
		// 只关 listener：正在处理的请求（包括触发本次换端口的 PUT）仍能写完响应
		old.Close()
		log.Printf("monitor: dashboard moved to http://%s", addr)
	} else {
		log.Printf("monitor: dashboard on http://%s", addr)
	}
	return nil
}

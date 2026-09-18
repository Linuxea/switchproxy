package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/net/proxy"
	"gopkg.in/yaml.v3"
)

// ---------------- 配置 ----------------

type Upstream struct {
	Type     string `yaml:"type"`
	Addr     string `yaml:"addr"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

type Rule struct {
	Raw    string
	Type   string // DOMAIN / DOMAIN-SUFFIX / DOMAIN-KEYWORD / IP-CIDR / PORT
	Value  string
	Action string // DIRECT / PROXY / REJECT
	cidr   *net.IPNet
}

type MonitorConfig struct {
	Disable bool   `yaml:"disable"` // 设为 true 关闭监控面板
	Listen  string `yaml:"listen"`  // 管理端监听地址，默认 127.0.0.1:9090
	DB      string `yaml:"db"`      // SQLite 历史库路径，默认 switchproxy.db
}

type Config struct {
	Listen   string        `yaml:"listen"`
	Upstream Upstream      `yaml:"upstream"`
	Default  string        `yaml:"default"`
	Rules    []string      `yaml:"rules"`
	Monitor  MonitorConfig `yaml:"monitor"`
	rules    []Rule
	pidFile  string
}

func parseRule(raw string) (Rule, error) {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if len(parts) != 3 {
		return Rule{}, fmt.Errorf("bad rule (want TYPE,VALUE,ACTION): %q", raw)
	}
	r := Rule{Raw: raw, Type: strings.ToUpper(parts[0]), Value: parts[1], Action: strings.ToUpper(parts[2])}
	switch r.Type {
	case "DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "PORT":
	case "IP-CIDR":
		_, n, err := net.ParseCIDR(r.Value)
		if err != nil {
			return Rule{}, fmt.Errorf("bad IP-CIDR %q: %v", r.Value, err)
		}
		r.cidr = n
	default:
		return Rule{}, fmt.Errorf("unsupported rule type %q", r.Type)
	}
	switch r.Action {
	case "DIRECT", "PROXY", "REJECT":
	default:
		return Rule{}, fmt.Errorf("bad action %q (want DIRECT/PROXY/REJECT)", r.Action)
	}
	return r, nil
}

func loadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(b, cfg); err != nil {
		return nil, err
	}
	if cfg.Listen == "" {
		cfg.Listen = "127.0.0.1:7899"
	}
	if cfg.Default == "" {
		cfg.Default = "PROXY"
	}
	cfg.Default = strings.ToUpper(cfg.Default)
	if cfg.Upstream.Addr != "" && cfg.Upstream.Type == "" {
		cfg.Upstream.Type = "http"
	}
	if cfg.Monitor.Listen == "" {
		cfg.Monitor.Listen = "127.0.0.1:9090"
	}
	if cfg.Monitor.DB == "" {
		cfg.Monitor.DB = "switchproxy.db"
	}
	for _, raw := range cfg.Rules {
		r, err := parseRule(raw)
		if err != nil {
			return nil, err
		}
		cfg.rules = append(cfg.rules, r)
	}
	cfg.pidFile = path + ".pid"
	return cfg, nil
}

// ---------------- 规则匹配 ----------------

func matchAction(cfg *Config, host, port string) (action, hit string) {
	h := strings.ToLower(strings.Trim(host, "[]"))
	ip := net.ParseIP(h)
	for _, r := range cfg.rules {
		var ok bool
		switch r.Type {
		case "DOMAIN":
			ok = ip == nil && h == strings.ToLower(r.Value)
		case "DOMAIN-SUFFIX":
			ok = ip == nil && (h == strings.ToLower(r.Value) || strings.HasSuffix(h, "."+strings.ToLower(r.Value)))
		case "DOMAIN-KEYWORD":
			ok = ip == nil && strings.Contains(h, strings.ToLower(r.Value))
		case "PORT":
			ok = port == r.Value
		case "IP-CIDR":
			ok = ip != nil && r.cidr != nil && r.cidr.Contains(ip)
		}
		if ok {
			return r.Action, r.Type + ":" + r.Value
		}
	}
	return cfg.Default, "MATCH"
}

// ---------------- 出站拨号 ----------------

func dialDirect(addr string) (net.Conn, error) {
	d := net.Dialer{Timeout: 10 * time.Second}
	return d.Dial("tcp", addr)
}

func dialHTTPUpstream(u Upstream, addr string) (net.Conn, error) {
	c, err := net.DialTimeout("tcp", u.Addr, 10*time.Second)
	if err != nil {
		return nil, err
	}
	c.SetDeadline(time.Now().Add(15 * time.Second))
	var sb strings.Builder
	sb.WriteString("CONNECT " + addr + " HTTP/1.1\r\nHost: " + addr + "\r\n")
	if u.Username != "" {
		token := base64.StdEncoding.EncodeToString([]byte(u.Username + ":" + u.Password))
		sb.WriteString("Proxy-Authorization: Basic " + token + "\r\n")
	}
	sb.WriteString("\r\n")
	if _, err := c.Write([]byte(sb.String())); err != nil {
		c.Close()
		return nil, err
	}
	br := bufio.NewReader(c)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		c.Close()
		return nil, err
	}
	if !strings.Contains(statusLine, "200") {
		c.Close()
		return nil, fmt.Errorf("upstream %s CONNECT %s: %s", u.Addr, addr, strings.TrimSpace(statusLine))
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			c.Close()
			return nil, err
		}
		if strings.TrimSpace(line) == "" {
			break
		}
	}
	c.SetDeadline(time.Time{})
	return c, nil
}

func dialSOCKS5Upstream(u Upstream, addr string) (net.Conn, error) {
	var auth *proxy.Auth
	if u.Username != "" {
		auth = &proxy.Auth{User: u.Username, Password: u.Password}
	}
	d, err := proxy.SOCKS5("tcp", u.Addr, auth, proxy.Direct)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if cd, ok := d.(proxy.ContextDialer); ok {
		return cd.DialContext(ctx, "tcp", addr)
	}
	return d.Dial("tcp", addr)
}

func dialByAction(cfg *Config, action, addr string) (net.Conn, error) {
	switch action {
	case "DIRECT":
		return dialDirect(addr)
	case "PROXY":
		if cfg.Upstream.Type == "socks5" {
			return dialSOCKS5Upstream(cfg.Upstream, addr)
		}
		return dialHTTPUpstream(cfg.Upstream, addr)
	}
	return nil, errors.New("REJECT")
}

// ---------------- 隧道 ----------------

func tunnel(client, server net.Conn, br *bufio.Reader, meta *ConnMeta) {
	defer client.Close()
	defer server.Close()
	defer stats.unregister(meta)

	// 上行 = 客户端 -> 目标；下行 = 目标 -> 客户端
	up := func(n int64) { stats.addUp(meta, n) }
	down := func(n int64) { stats.addDown(meta, n) }
	cw := &countingConn{Conn: client, onRead: up, onWrite: down}
	sw := &countingConn{Conn: server, onRead: down, onWrite: up}

	if br != nil && br.Buffered() > 0 {
		if data, err := br.Peek(br.Buffered()); err == nil {
			sw.Write(data)
		}
	}
	done := make(chan struct{}, 2)
	go func() {
		io.Copy(sw, cw)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(cw, sw)
		done <- struct{}{}
	}()
	<-done
}

func rejectHTTP(client net.Conn) {
	client.Write([]byte("HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"))
}

// ---------------- HTTP 代理入站 ----------------

func serveHTTP(client net.Conn, br *bufio.Reader) {
	cfg := current.Load().(*Config)

	var reqLine string
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		reqLine = strings.TrimSpace(line)
		break
	}
	parts := strings.Fields(reqLine)
	if len(parts) < 3 {
		return
	}
	method, target, proto := parts[0], parts[1], parts[2]

	var headers []string
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		if strings.TrimSpace(line) == "" {
			break
		}
		headers = append(headers, strings.TrimRight(line, "\r\n"))
	}

	if strings.EqualFold(method, "CONNECT") {
		host, port, err := net.SplitHostPort(target)
		if err != nil {
			host, port = target, "443"
		}
		action, hit := matchAction(cfg, host, port)
		log.Printf("%-7s %-42s -> %-6s (%s)", method, host+":"+port, action, hit)
		if action == "REJECT" {
			rejectHTTP(client)
			return
		}
		server, err := dialByAction(cfg, action, net.JoinHostPort(host, port))
		if err != nil {
			log.Println("dial:", err)
			client.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n"))
			return
		}
		client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		meta := stats.register("HTTP", host, port, hit, action, client)
		tunnel(client, server, br, meta)
		return
	}

	u, err := url.Parse(target)
	if err != nil || u.Host == "" {
		rejectHTTP(client)
		return
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "80"
	}
	action, hit := matchAction(cfg, host, port)
	log.Printf("%-7s http://%-42s -> %-6s (%s)", method, host, action, hit)
	if action == "REJECT" {
		rejectHTTP(client)
		return
	}
	server, err := dialByAction(cfg, action, net.JoinHostPort(host, port))
	if err != nil {
		log.Println("dial:", err)
		rejectHTTP(client)
		return
	}
	outReqLine := reqLine
	if action == "DIRECT" || cfg.Upstream.Type == "socks5" {
		outReqLine = method + " " + u.RequestURI() + " " + proto
	}
	var sb strings.Builder
	sb.WriteString(outReqLine + "\r\n")
	for _, h := range headers {
		sb.WriteString(h + "\r\n")
	}
	sb.WriteString("\r\n")
	server.Write([]byte(sb.String()))
	meta := stats.register("HTTP", host, port, hit, action, client)
	stats.addUp(meta, int64(sb.Len()))
	tunnel(client, server, br, meta)
}

// ---------------- SOCKS5 代理入站 ----------------

func replySocks(c net.Conn, rep byte) {
	c.Write([]byte{0x05, rep, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
}

func serveSocks5(client net.Conn, br *bufio.Reader) {
	defer client.Close()
	cfg := current.Load().(*Config)

	hdr := make([]byte, 2)
	if _, err := io.ReadFull(br, hdr); err != nil || hdr[0] != 0x05 {
		return
	}
	methods := make([]byte, int(hdr[1]))
	if len(methods) > 0 {
		if _, err := io.ReadFull(br, methods); err != nil {
			return
		}
	}
	client.Write([]byte{0x05, 0x00})

	req := make([]byte, 4)
	if _, err := io.ReadFull(br, req); err != nil {
		return
	}
	if req[1] != 0x01 { // 仅支持 CONNECT
		replySocks(client, 0x07)
		return
	}
	var host string
	switch req[3] {
	case 0x01:
		b := make([]byte, 4)
		if _, err := io.ReadFull(br, b); err != nil {
			return
		}
		host = net.IP(b).String()
	case 0x03:
		l := make([]byte, 1)
		if _, err := io.ReadFull(br, l); err != nil {
			return
		}
		d := make([]byte, l[0])
		if _, err := io.ReadFull(br, d); err != nil {
			return
		}
		host = string(d)
	case 0x04:
		b := make([]byte, 16)
		if _, err := io.ReadFull(br, b); err != nil {
			return
		}
		host = net.IP(b).String()
	default:
		replySocks(client, 0x08)
		return
	}
	pb := make([]byte, 2)
	if _, err := io.ReadFull(br, pb); err != nil {
		return
	}
	port := strconv.Itoa(int(pb[0])<<8 | int(pb[1]))

	action, hit := matchAction(cfg, host, port)
	log.Printf("SOCKS5  %-42s -> %-6s (%s)", host+":"+port, action, hit)
	if action == "REJECT" {
		replySocks(client, 0x02)
		return
	}
	server, err := dialByAction(cfg, action, net.JoinHostPort(host, port))
	if err != nil {
		log.Println("dial:", err)
		replySocks(client, 0x01)
		return
	}
	replySocks(client, 0x00)
	meta := stats.register("SOCKS5", host, port, hit, action, client)
	tunnel(client, server, br, meta)
}

// ---------------- 主流程 ----------------

var (
	configPath string
	current    atomic.Value
)

func handleConn(client net.Conn) {
	defer client.Close()
	br := bufio.NewReader(client)
	b, err := br.Peek(1)
	if err != nil {
		return
	}
	if b[0] == 0x05 {
		serveSocks5(client, br)
	} else {
		serveHTTP(client, br)
	}
}

func statModTime(path string) time.Time {
	if fi, err := os.Stat(path); err == nil {
		return fi.ModTime()
	}
	return time.Time{}
}

func watchConfig() {
	last := statModTime(configPath)
	for range time.Tick(2 * time.Second) {
		m := statModTime(configPath)
		if !m.Equal(last) {
			last = m
			if cfg, err := loadConfig(configPath); err == nil {
				current.Store(cfg)
				log.Printf("config reloaded: %d rules, default=%s", len(cfg.rules), cfg.Default)
			} else {
				log.Println("config reload failed:", err)
			}
		}
	}
}

func main() {
	flag.StringVar(&configPath, "c", "config.yaml", "config file path")
	flag.Parse()
	log.SetFlags(log.LstdFlags)
	cfg, err := loadConfig(configPath)
	if err != nil {
		log.Fatal("load config: ", err)
	}
	current.Store(cfg)

	if err := os.WriteFile(cfg.pidFile, []byte(strconv.Itoa(os.Getpid())), 0644); err == nil {
		defer os.Remove(cfg.pidFile)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Println("shutting down")
		os.Remove(cfg.pidFile)
		os.Exit(0)
	}()

	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		log.Fatal("listen: ", err)
	}
	go watchConfig()
	go stats.sampleLoop()
	if !cfg.Monitor.Disable {
		hdb, err := openHistory(cfg.Monitor.DB)
		if err != nil {
			log.Println("monitor: history db disabled:", err)
			hdb = nil
		}
		go startAdmin(cfg.Monitor.Listen, hdb)
	}
	log.Printf("switchproxy listening on %s (HTTP+SOCKS5 mixed) | upstream %s (%s) | default=%s | %d rules | pid=%d | monitor=%s",
		cfg.Listen, cfg.Upstream.Addr, cfg.Upstream.Type, cfg.Default, len(cfg.rules), os.Getpid(),
		func() string {
			if cfg.Monitor.Disable {
				return "off"
			}
			return "http://" + cfg.Monitor.Listen
		}())
	for {
		c, err := ln.Accept()
		if err != nil {
			log.Println("accept:", err)
			continue
		}
		go handleConn(c)
	}
}

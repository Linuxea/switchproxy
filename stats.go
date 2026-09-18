package main

// 流量统计采集：连接级元数据、双向字节计数、域名/规则聚合、速率采样。

import (
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------- 连接级元数据 ----------------

type ConnMeta struct {
	ID       uint64    `json:"id"`
	Protocol string    `json:"protocol"` // HTTP / SOCKS5
	Host     string    `json:"host"`
	Port     string    `json:"port"`
	Rule     string    `json:"rule"`   // 命中的规则，如 DOMAIN-SUFFIX:example.com 或 MATCH
	Action   string    `json:"action"` // DIRECT / PROXY
	Start    time.Time `json:"-"`

	up   atomic.Int64 // 客户端 -> 目标（上行）
	down atomic.Int64 // 目标 -> 客户端（下行）

	client net.Conn // 用于管理端强制关闭
}

type ConnJSON struct {
	ID       uint64 `json:"id"`
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     string `json:"port"`
	Rule     string `json:"rule"`
	Action   string `json:"action"`
	Start    int64  `json:"start"` // unix 毫秒
	Up       int64  `json:"up"`
	Down     int64  `json:"down"`
}

func (m *ConnMeta) snapshot() ConnJSON {
	return ConnJSON{
		ID:       m.ID,
		Protocol: m.Protocol,
		Host:     m.Host,
		Port:     m.Port,
		Rule:     m.Rule,
		Action:   m.Action,
		Start:    m.Start.UnixMilli(),
		Up:       m.up.Load(),
		Down:     m.down.Load(),
	}
}

// ---------------- 聚合 ----------------

type AggEntry struct {
	Up    int64 `json:"up"`
	Down  int64 `json:"down"`
	Conns int64 `json:"conns"`
}

type NamedAgg struct {
	Name string `json:"name"`
	AggEntry
}

// ---------------- 全局统计器 ----------------

type RatePoint struct {
	T    int64   `json:"t"` // unix 秒
	Up   float64 `json:"up"`
	Down float64 `json:"down"`
}

type Overview struct {
	UpRate      float64     `json:"upRate"` // B/s
	DownRate    float64     `json:"downRate"`
	TotalUp     int64       `json:"totalUp"`
	TotalDown   int64       `json:"totalDown"`
	TotalConns  int64       `json:"totalConns"`
	ActiveConns int         `json:"activeConns"`
	Uptime      int64       `json:"uptime"`
	Rates       []RatePoint `json:"rates"`
}

type Snapshot struct {
	Overview    Overview   `json:"overview"`
	Connections []ConnJSON `json:"connections"`
	Closed      []ConnJSON `json:"closed"`
	TopDomains  []NamedAgg `json:"topDomains"`
	TopRules    []NamedAgg `json:"topRules"`
}

const (
	rateWindow  = 120 // 速率采样保留秒数
	closedMax   = 500 // 最近关闭连接保留条数
	closedFeedN = 100 // 快照中下发的最近关闭条数
)

type Stats struct {
	startTime time.Time
	nextID    atomic.Uint64
	totalUp   atomic.Int64
	totalDown atomic.Int64
	totalConn atomic.Int64

	mu    sync.RWMutex
	conns map[uint64]*ConnMeta

	closedMu sync.Mutex
	closed   []ConnJSON

	aggMu   sync.Mutex
	domains map[string]*AggEntry
	rules   map[string]*AggEntry

	rateMu sync.Mutex
	rates  []RatePoint
}

var stats = &Stats{
	startTime: time.Now(),
	conns:     make(map[uint64]*ConnMeta),
	domains:   make(map[string]*AggEntry),
	rules:     make(map[string]*AggEntry),
}

func (s *Stats) register(protocol, host, port, rule, action string, client net.Conn) *ConnMeta {
	id := s.nextID.Add(1)
	m := &ConnMeta{
		ID: id, Protocol: protocol, Host: host, Port: port,
		Rule: rule, Action: action, Start: time.Now(), client: client,
	}
	s.mu.Lock()
	s.conns[id] = m
	s.mu.Unlock()
	s.totalConn.Add(1)

	s.aggMu.Lock()
	d := s.domains[host]
	if d == nil {
		d = &AggEntry{}
		s.domains[host] = d
	}
	d.Conns++
	r := s.rules[rule]
	if r == nil {
		r = &AggEntry{}
		s.rules[rule] = r
	}
	r.Conns++
	s.aggMu.Unlock()
	return m
}

func (s *Stats) unregister(m *ConnMeta) {
	s.mu.Lock()
	delete(s.conns, m.ID)
	s.mu.Unlock()

	s.closedMu.Lock()
	s.closed = append(s.closed, m.snapshot())
	if len(s.closed) > closedMax {
		s.closed = s.closed[len(s.closed)-closedMax:]
	}
	s.closedMu.Unlock()
}

// addUp / addDown 为隧道热路径，仅做原子累加与聚合计数。
func (s *Stats) addUp(m *ConnMeta, n int64) {
	m.up.Add(n)
	s.totalUp.Add(n)
	s.aggMu.Lock()
	if d := s.domains[m.Host]; d != nil {
		d.Up += n
	}
	if r := s.rules[m.Rule]; r != nil {
		r.Up += n
	}
	s.aggMu.Unlock()
}

func (s *Stats) addDown(m *ConnMeta, n int64) {
	m.down.Add(n)
	s.totalDown.Add(n)
	s.aggMu.Lock()
	if d := s.domains[m.Host]; d != nil {
		d.Down += n
	}
	if r := s.rules[m.Rule]; r != nil {
		r.Down += n
	}
	s.aggMu.Unlock()
}

func (s *Stats) totals() (up, down, conns int64) {
	return s.totalUp.Load(), s.totalDown.Load(), s.totalConn.Load()
}

func (s *Stats) Active() []ConnJSON {
	s.mu.RLock()
	out := make([]ConnJSON, 0, len(s.conns))
	for _, m := range s.conns {
		out = append(out, m.snapshot())
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// RecentClosed 返回最近关闭的连接（最新在前）。
func (s *Stats) RecentClosed(n int) []ConnJSON {
	s.closedMu.Lock()
	defer s.closedMu.Unlock()
	if n > len(s.closed) {
		n = len(s.closed)
	}
	out := make([]ConnJSON, 0, n)
	for i := len(s.closed) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, s.closed[i])
	}
	return out
}

func (s *Stats) topN(m map[string]*AggEntry, n int) []NamedAgg {
	s.aggMu.Lock()
	list := make([]NamedAgg, 0, len(m))
	for k, v := range m {
		list = append(list, NamedAgg{Name: k, AggEntry: *v})
	}
	s.aggMu.Unlock()
	sort.Slice(list, func(i, j int) bool {
		return list[i].Up+list[i].Down > list[j].Up+list[j].Down
	})
	if len(list) > n {
		list = list[:n]
	}
	return list
}

func (s *Stats) TopDomains(n int) []NamedAgg { return stats.topN(s.domains, n) }
func (s *Stats) TopRules(n int) []NamedAgg   { return stats.topN(s.rules, n) }

func (s *Stats) domainCopy() map[string]AggEntry {
	s.aggMu.Lock()
	defer s.aggMu.Unlock()
	out := make(map[string]AggEntry, len(s.domains))
	for k, v := range s.domains {
		out[k] = *v
	}
	return out
}

func (s *Stats) Overview() Overview {
	s.rateMu.Lock()
	rates := make([]RatePoint, len(s.rates))
	copy(rates, s.rates)
	s.rateMu.Unlock()

	up, down, conns := s.totals()
	ov := Overview{
		TotalUp:    up,
		TotalDown:  down,
		TotalConns: conns,
		Uptime:     int64(time.Since(s.startTime).Seconds()),
		Rates:      rates,
	}
	s.mu.RLock()
	ov.ActiveConns = len(s.conns)
	s.mu.RUnlock()
	if n := len(rates); n > 0 {
		ov.UpRate = rates[n-1].Up
		ov.DownRate = rates[n-1].Down
	}
	return ov
}

func (s *Stats) Snapshot() Snapshot {
	return Snapshot{
		Overview:    s.Overview(),
		Connections: s.Active(),
		Closed:      s.RecentClosed(closedFeedN),
		TopDomains:  s.TopDomains(15),
		TopRules:    s.TopRules(10),
	}
}

func (s *Stats) closeConn(id uint64) bool {
	s.mu.RLock()
	m := s.conns[id]
	s.mu.RUnlock()
	if m == nil {
		return false
	}
	if m.client != nil {
		m.client.Close()
	}
	return true
}

// sampleLoop 每秒采样全局计数器差值，计算实时速率。
func (s *Stats) sampleLoop() {
	last := time.Now()
	lastUp, lastDown := int64(0), int64(0)
	for now := range time.Tick(time.Second) {
		dt := now.Sub(last).Seconds()
		last = now
		up, down, _ := s.totals()
		p := RatePoint{
			T:    now.Unix(),
			Up:   float64(up-lastUp) / dt,
			Down: float64(down-lastDown) / dt,
		}
		lastUp, lastDown = up, down
		s.rateMu.Lock()
		s.rates = append(s.rates, p)
		if len(s.rates) > rateWindow {
			s.rates = s.rates[1:]
		}
		s.rateMu.Unlock()
	}
}

// ---------------- 计数连接包装 ----------------

type countingConn struct {
	net.Conn
	onRead  func(int64) // 从该连接读到 n 字节
	onWrite func(int64) // 向该连接写入 n 字节
}

func (c *countingConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if n > 0 && c.onRead != nil {
		c.onRead(int64(n))
	}
	return n, err
}

func (c *countingConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	if n > 0 && c.onWrite != nil {
		c.onWrite(int64(n))
	}
	return n, err
}

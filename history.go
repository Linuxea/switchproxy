package main

// 历史数据持久化：分钟级流量点 + 按天域名聚合，写入 SQLite（纯 Go 驱动，无 CGO）。

import (
	"database/sql"
	"log"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

type HistoryDB struct {
	db *sql.DB
	// 上次落盘时的累计值，用于计算增量
	lastTotals struct {
		up, down, conns int64
	}
	lastDomains map[string]AggEntry
	eventsDays  int // 连接事件保留天数
}

type HistoryPoint struct {
	T     int64 `json:"t"` // unix 秒（桶起始）
	Up    int64 `json:"up"`
	Down  int64 `json:"down"`
	Conns int64 `json:"conns"`
}

type HistoryDomain struct {
	Domain string `json:"domain"`
	Up     int64  `json:"up"`
	Down   int64  `json:"down"`
	Conns  int64  `json:"conns"`
}

type HistoryResult struct {
	Range   string          `json:"range"`
	Points  []HistoryPoint  `json:"points"`
	Domains []HistoryDomain `json:"domains"`
}

func openHistory(path string, events bool, eventsDays int) (*HistoryDB, error) {
	if path == "" {
		return nil, nil
	}
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	h := &HistoryDB{db: db, lastDomains: make(map[string]AggEntry), eventsDays: eventsDays}
	if err := h.init(); err != nil {
		db.Close()
		return nil, err
	}
	go h.recordLoop()
	go h.retentionLoop()
	if events {
		eventsEnabled.Store(true)
		go h.eventsLoop()
	}
	return h, nil
}

func (h *HistoryDB) init() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS traffic_points (
			minute INTEGER PRIMARY KEY,
			up     INTEGER NOT NULL DEFAULT 0,
			down   INTEGER NOT NULL DEFAULT 0,
			conns  INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS domain_stats (
			day    TEXT NOT NULL,
			domain TEXT NOT NULL,
			up     INTEGER NOT NULL DEFAULT 0,
			down   INTEGER NOT NULL DEFAULT 0,
			conns  INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (day, domain)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_domain_stats_day ON domain_stats(day)`,
		`CREATE TABLE IF NOT EXISTS conn_events (
			ts     INTEGER NOT NULL,
			proto  TEXT NOT NULL,
			host   TEXT NOT NULL,
			port   TEXT NOT NULL,
			rule   TEXT NOT NULL,
			action TEXT NOT NULL,
			up     INTEGER NOT NULL DEFAULT 0,
			down   INTEGER NOT NULL DEFAULT 0,
			dur_ms INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_conn_events_ts ON conn_events(ts)`,
	}
	for _, s := range stmts {
		if _, err := h.db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

// recordLoop 每分钟把内存增量写入 SQLite。
func (h *HistoryDB) recordLoop() {
	for {
		now := time.Now()
		next := now.Truncate(time.Minute).Add(time.Minute)
		time.Sleep(next.Sub(now) + 20*time.Millisecond)

		up, down, conns := stats.totals()
		dUp, dDown, dConns := up-h.lastTotals.up, down-h.lastTotals.down, conns-h.lastTotals.conns
		if dUp < 0 {
			dUp = 0
		}
		if dDown < 0 {
			dDown = 0
		}
		if dConns < 0 {
			dConns = 0
		}
		h.lastTotals.up, h.lastTotals.down, h.lastTotals.conns = up, down, conns

		if _, err := h.db.Exec(
			`INSERT INTO traffic_points(minute, up, down, conns) VALUES(?,?,?,?)
			 ON CONFLICT(minute) DO UPDATE SET up=up+excluded.up, down=down+excluded.down, conns=conns+excluded.conns`,
			next.Unix(), dUp, dDown, dConns); err != nil {
			log.Println("history: write point:", err)
		}
		h.flushDomains(next)
	}
}

// flushDomains 与上次落盘快照做差，把域名增量写入当天的 domain_stats。
func (h *HistoryDB) flushDomains(day time.Time) {
	doms := stats.domainCopy()
	dayStr := day.Format("2006-01-02")
	for name, cur := range doms {
		last := h.lastDomains[name]
		dUp, dDown, dConns := cur.Up-last.Up, cur.Down-last.Down, cur.Conns-last.Conns
		if dUp == 0 && dDown == 0 && dConns == 0 {
			continue
		}
		if _, err := h.db.Exec(
			`INSERT INTO domain_stats(day, domain, up, down, conns) VALUES(?,?,?,?,?)
			 ON CONFLICT(day, domain) DO UPDATE SET up=up+excluded.up, down=down+excluded.down, conns=conns+excluded.conns`,
			dayStr, name, dUp, dDown, dConns); err != nil {
			log.Println("history: write domain:", err)
		}
		h.lastDomains[name] = cur
	}
}

// ---------------- 连接事件（原始数据层） ----------------

// conn_events 存逐连接的关闭事件：后续任意维度/任意时段的聚合都可以用 SQL 现算。
// 与分钟/按天两张汇总表并存——汇总表是面板的快速路径，事件表是原始底账。

type connEvent struct {
	ts                              int64
	proto, host, port, rule, action string
	up, down, durMs                 int64
}

var (
	eventsCh      = make(chan connEvent, 4096)
	eventsEnabled atomic.Bool // 事件写协程启动后才置位；生产端据此短路，避免无消费者时堆积
	eventsDropped atomic.Int64
)

// eventsEnqueue 在 unregister（转发协程）里被调用：只做非阻塞投递，塞满即丢，绝不阻塞热路径。
func eventsEnqueue(m *ConnMeta) {
	if !eventsEnabled.Load() {
		return
	}
	ev := connEvent{
		ts:     time.Now().Unix(),
		proto:  m.Protocol,
		host:   m.Host,
		port:   m.Port,
		rule:   m.Rule,
		action: m.Action,
		up:     m.up.Load(),
		down:   m.down.Load(),
		durMs:  time.Since(m.Start).Milliseconds(),
	}
	select {
	case eventsCh <- ev:
	default:
		eventsDropped.Add(1)
	}
}

// eventsLoop 单写协程：攒批 + 事务落盘，每 2 秒或满 512 条一刷。
func (h *HistoryDB) eventsLoop() {
	stmt, err := h.db.Prepare(`INSERT INTO conn_events(ts, proto, host, port, rule, action, up, down, dur_ms)
		VALUES(?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		log.Println("history: events prepare:", err)
		eventsEnabled.Store(false)
		return
	}
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	buf := make([]connEvent, 0, 512)
	for {
		select {
		case ev := <-eventsCh:
			buf = append(buf, ev)
			if len(buf) >= cap(buf) {
				h.flushEvents(stmt, &buf)
			}
		case <-tick.C:
			if d := eventsDropped.Swap(0); d > 0 {
				log.Printf("history: events dropped: %d", d)
			}
			if len(buf) > 0 {
				h.flushEvents(stmt, &buf)
			}
		}
	}
}

// flushEvents 单事务写入整批；失败时保留批次待下轮重试（DB 抖动可自愈），
// 仅在积压超限时丢最旧的，防持续失败下无界增长。
func (h *HistoryDB) flushEvents(stmt *sql.Stmt, buf *[]connEvent) {
	if len(*buf) == 0 {
		return
	}
	tx, err := h.db.Begin()
	if err != nil {
		log.Println("history: events tx:", err)
		trimEvents(buf)
		return
	}
	for _, e := range *buf {
		if _, err := tx.Stmt(stmt).Exec(e.ts, e.proto, e.host, e.port, e.rule, e.action, e.up, e.down, e.durMs); err != nil {
			log.Println("history: events insert:", err)
			tx.Rollback()
			trimEvents(buf)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		log.Println("history: events commit:", err)
		trimEvents(buf)
		return
	}
	*buf = (*buf)[:0]
}

// trimEvents 重试积压上限：超过 maxKeep 条时保留最新的部分。
func trimEvents(buf *[]connEvent) {
	const maxKeep = 512
	if len(*buf) > maxKeep {
		n := copy(*buf, (*buf)[len(*buf)-maxKeep:])
		*buf = (*buf)[:n]
	}
}

// retentionLoop 清理过期数据：流量点保留 30 天，域名统计保留 90 天，连接事件保留 eventsDays 天。
func (h *HistoryDB) retentionLoop() {
	clean := func() {
		pointCut := time.Now().Add(-30 * 24 * time.Hour).Unix()
		dayCut := time.Now().Add(-90 * 24 * time.Hour).Format("2006-01-02")
		evCut := time.Now().Add(-time.Duration(h.eventsDays) * 24 * time.Hour).Unix()
		if _, err := h.db.Exec(`DELETE FROM traffic_points WHERE minute < ?`, pointCut); err != nil {
			log.Println("history: retention points:", err)
		}
		if _, err := h.db.Exec(`DELETE FROM domain_stats WHERE day < ?`, dayCut); err != nil {
			log.Println("history: retention domains:", err)
		}
		if _, err := h.db.Exec(`DELETE FROM conn_events WHERE ts < ?`, evCut); err != nil {
			log.Println("history: retention events:", err)
		}
	}
	clean()
	for range time.Tick(24 * time.Hour) {
		clean()
	}
}

// queryHistory 按范围查询时序与域名排行。
func (h *HistoryDB) queryHistory(rng string) HistoryResult {
	res := HistoryResult{Range: rng, Points: []HistoryPoint{}, Domains: []HistoryDomain{}}
	now := time.Now()
	var from int64
	var bucket int64
	switch rng {
	case "1h":
		from, bucket = now.Add(-time.Hour).Unix(), 60
	case "24h":
		from, bucket = now.Add(-24*time.Hour).Unix(), 600
	case "7d":
		from, bucket = now.Add(-7*24*time.Hour).Unix(), 3600
	case "30d":
		from, bucket = now.Add(-30*24*time.Hour).Unix(), 3600
	default:
		res.Range = "24h"
		from, bucket = now.Add(-24*time.Hour).Unix(), 600
	}

	if h == nil || h.db == nil {
		return res
	}

	rows, err := h.db.Query(
		`SELECT (minute/?)*? AS b, SUM(up), SUM(down), SUM(conns)
		 FROM traffic_points WHERE minute >= ? GROUP BY b ORDER BY b`, bucket, bucket, from)
	if err != nil {
		log.Println("history: query points:", err)
		return res
	}
	defer rows.Close()
	for rows.Next() {
		var p HistoryPoint
		if err := rows.Scan(&p.T, &p.Up, &p.Down, &p.Conns); err != nil {
			break
		}
		res.Points = append(res.Points, p)
	}

	dayFrom := time.Unix(from, 0).Format("2006-01-02")
	drows, err := h.db.Query(
		`SELECT domain, SUM(up), SUM(down), SUM(conns) FROM domain_stats
		 WHERE day >= ? GROUP BY domain ORDER BY up+down DESC LIMIT 20`, dayFrom)
	if err != nil {
		log.Println("history: query domains:", err)
		return res
	}
	defer drows.Close()
	for drows.Next() {
		var d HistoryDomain
		if err := drows.Scan(&d.Domain, &d.Up, &d.Down, &d.Conns); err != nil {
			break
		}
		res.Domains = append(res.Domains, d)
	}
	return res
}

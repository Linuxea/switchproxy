package main

// 历史数据持久化：分钟级流量点 + 按天域名聚合，写入 SQLite（纯 Go 驱动，无 CGO）。

import (
	"database/sql"
	"log"
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

func openHistory(path string) (*HistoryDB, error) {
	if path == "" {
		return nil, nil
	}
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	h := &HistoryDB{db: db, lastDomains: make(map[string]AggEntry)}
	if err := h.init(); err != nil {
		db.Close()
		return nil, err
	}
	go h.recordLoop()
	go h.retentionLoop()
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

// retentionLoop 清理过期数据：流量点保留 30 天，域名统计保留 90 天。
func (h *HistoryDB) retentionLoop() {
	clean := func() {
		pointCut := time.Now().Add(-30 * 24 * time.Hour).Unix()
		dayCut := time.Now().Add(-90 * 24 * time.Hour).Format("2006-01-02")
		if _, err := h.db.Exec(`DELETE FROM traffic_points WHERE minute < ?`, pointCut); err != nil {
			log.Println("history: retention points:", err)
		}
		if _, err := h.db.Exec(`DELETE FROM domain_stats WHERE day < ?`, dayCut); err != nil {
			log.Println("history: retention domains:", err)
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

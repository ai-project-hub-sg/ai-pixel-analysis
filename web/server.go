package web

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// Server 提供只读数据分析 API 与静态前端
type Server struct {
	db *sql.DB
}

func NewServer(db *sql.DB) *Server { return &Server{db: db} }

// Listen 注册路由并启动 HTTP 服务
func (s *Server) Listen(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.Handle("/static/", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("/api/overview", s.wrap(s.handleOverview))
	mux.HandleFunc("/api/income/hourly", s.wrap(s.handleIncomeHourly))
	mux.HandleFunc("/api/income/daily", s.wrap(s.handleIncomeDaily))
	mux.HandleFunc("/api/income/top", s.wrap(s.handleIncomeTop))
	mux.HandleFunc("/api/usage/summary", s.wrap(s.handleUsageSummary))
	mux.HandleFunc("/api/usage/by-model", s.wrap(s.handleUsageByModel))
	mux.HandleFunc("/api/usage/daily", s.wrap(s.handleUsageDaily))
	mux.HandleFunc("/api/usage/hourly", s.wrap(s.handleUsageHourly))
	mux.HandleFunc("/api/ledger", s.wrap(s.handleLedger))
	mux.HandleFunc("/api/balance-trend", s.wrap(s.handleBalanceTrend))
	mux.HandleFunc("/api/emails", s.wrap(s.handleEmails))
	log.Printf("web UI: http://localhost%s", addr)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) wrap(h func(http.ResponseWriter, *http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := h(w, r)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(res)
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// ---------- shared helpers ----------

type kv struct {
	K string  `json:"k"`
	V float64 `json:"v"`
}

func (s *Server) queryKV(q string, args ...any) ([]kv, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []kv{}
	for rows.Next() {
		var k string
		var v float64
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out = append(out, kv{k, v})
	}
	return out, rows.Err()
}

// fmtHour / fmtDay 与 sqlite substr 分桶格式对齐
func fmtHour(t time.Time) string { return t.Format("2006-01-02T15") }
func fmtDay(t time.Time) string  { return t.Format("2006-01-02") }

// pct 计算百分比变化；分母为 0 返回 nil（前端显示 —，避免误导性的无穷大）
func pct(cur, prev float64) *float64 {
	if prev == 0 {
		return nil
	}
	v := (cur - prev) / prev * 100
	return &v
}

func emailParam(r *http.Request) string { return r.URL.Query().Get("email") }

// fillZeroDays / fillZeroHours 把稀疏序列补齐成连续时间轴（含 0 值），
// 使趋势图横轴连续、环比/同比可正确对齐。
func fillZeroHours(list []kv, start, end time.Time) []kv {
	m := map[string]float64{}
	for _, x := range list {
		m[x.K] = x.V
	}
	var out []kv
	for t := start; t.Before(end); t = t.Add(time.Hour) {
		k := fmtHour(t)
		out = append(out, kv{k, m[k]})
	}
	return out
}

func fillZeroDays(list []kv, start time.Time, days int) []kv {
	m := map[string]float64{}
	for _, x := range list {
		m[x.K] = x.V
	}
	var out []kv
	for i := 0; i < days; i++ {
		k := fmtDay(start.AddDate(0, 0, i))
		out = append(out, kv{k, m[k]})
	}
	return out
}

package web

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// ============ /api/emails ============
func (s *Server) handleEmails(w http.ResponseWriter, r *http.Request) (any, error) {
	emails, err := s.listEmails()
	return map[string]any{"emails": emails}, err
}

func (s *Server) listEmails() ([]string, error) {
	rows, err := s.db.Query(`SELECT email FROM auth_sessions ORDER BY email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// incomeBetween 计算 [start,end) 区间内的号主分账收入
func (s *Server) incomeBetween(email string, start, end time.Time) float64 {
	var v float64
	s.db.QueryRow(`SELECT COALESCE(SUM(CAST(amount AS REAL)),0) FROM balance_ledger WHERE email=? AND direction='credit' AND reason='account_share_income' AND created_at>=? AND created_at<?`,
		email, start.Format(time.RFC3339), end.Format(time.RFC3339)).Scan(&v)
	return v
}

// ============ /api/overview ============
func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) (any, error) {
	now := time.Now()
	curHour := now.Truncate(time.Hour)
	prevHour := curHour.Add(-time.Hour)
	ydaySameHour := curHour.AddDate(0, 0, -1)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterdayStart := todayStart.AddDate(0, 0, -1)
	sevenDayAgo := todayStart.AddDate(0, 0, -7)

	emails, err := s.listEmails()
	if err != nil {
		return nil, err
	}
	type Row struct {
		Email           string   `json:"email"`
		Usage7d         int64    `json:"usage_7d"`
		UsagePct7d      *float64 `json:"usage_pct_7d"`
		TotalCost       float64  `json:"total_cost"`
		ActualCost      float64  `json:"actual_cost"`
		ShareIncome     float64  `json:"share_income"`
		CurHourIncome   float64  `json:"cur_hour_income"`
		PrevHourIncome  float64  `json:"prev_hour_income"`
		HourQoq         *float64 `json:"hour_qoq"`
		YdaySameHourInc float64  `json:"yday_same_hour_income"`
		HourYoy         *float64 `json:"hour_yoy"`
		TodayIncome     float64  `json:"today_income"`
		YesterdayIncome float64  `json:"yesterday_income"`
		DayQoq          *float64 `json:"day_qoq"`
		TodayHourAvg    float64  `json:"today_hour_avg"`
		Balance         *float64 `json:"balance"`
	}
	var out []Row
	var totalUsage7d int64
	for _, e := range emails {
		row := Row{Email: e}
		s.db.QueryRow(`SELECT COUNT(*) FROM usage_logs WHERE email=? AND created_at>=?`, e, sevenDayAgo.Format(time.RFC3339)).Scan(&row.Usage7d)
		totalUsage7d += row.Usage7d
		s.db.QueryRow(`SELECT COALESCE(SUM(total_cost),0), COALESCE(SUM(actual_cost),0) FROM usage_logs WHERE email=?`, e).Scan(&row.TotalCost, &row.ActualCost)
		s.db.QueryRow(`SELECT COALESCE(SUM(CAST(amount AS REAL)),0) FROM balance_ledger WHERE email=? AND direction='credit' AND reason='account_share_income'`, e).Scan(&row.ShareIncome)
		row.CurHourIncome = s.incomeBetween(e, curHour, curHour.Add(time.Hour))
		row.PrevHourIncome = s.incomeBetween(e, prevHour, curHour)
		row.YdaySameHourInc = s.incomeBetween(e, ydaySameHour, ydaySameHour.Add(time.Hour))
		row.HourQoq = pct(row.CurHourIncome, row.PrevHourIncome)
		row.HourYoy = pct(row.CurHourIncome, row.YdaySameHourInc)
		row.TodayIncome = s.incomeBetween(e, todayStart, now)
		row.YesterdayIncome = s.incomeBetween(e, yesterdayStart, todayStart)
		row.DayQoq = pct(row.TodayIncome, row.YesterdayIncome)
		hours := now.Sub(todayStart).Hours()
		if hours < 1 {
			hours = 1
		}
		row.TodayHourAvg = row.TodayIncome / hours
		var bal sql.NullString
		if err := s.db.QueryRow(`SELECT balance_after FROM balance_ledger WHERE email=? ORDER BY created_at DESC, id DESC LIMIT 1`, e).Scan(&bal); err == nil && bal.Valid {
			var b float64
			if err := s.db.QueryRow(`SELECT CAST(? AS REAL)`, bal.String).Scan(&b); err == nil {
				row.Balance = &b
			}
		}
		out = append(out, row)
	}
	for i := range out {
		if totalUsage7d > 0 {
			v := float64(out[i].Usage7d) / float64(totalUsage7d) * 100
			out[i].UsagePct7d = &v
		}
	}
	return map[string]any{
		"now":            now.Format(time.RFC3339),
		"cur_hour":       fmtHour(curHour),
		"prev_hour":      fmtHour(prevHour),
		"yday_same_hour": fmtHour(ydaySameHour),
		"total_usage_7d": totalUsage7d,
		"rows":           out,
	}, nil
}

// ============ /api/income/hourly?email=&days= ============
// 近 N 天逐小时分账收入（默认今天+昨天=48h），稀疏补 0
func (s *Server) handleIncomeHourly(w http.ResponseWriter, r *http.Request) (any, error) {
	email := emailParam(r)
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 || days > 31 {
		days = 2
	}
	now := time.Now()
	start := now.Truncate(time.Hour).AddDate(0, 0, -(days - 1)).Add(-time.Duration(now.Hour()) * time.Hour)
	start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(days - 1))
	end := now.Truncate(time.Hour).Add(time.Hour)
	q := `SELECT substr(created_at,1,13) h, SUM(CAST(amount AS REAL)) FROM balance_ledger WHERE direction='credit' AND reason='account_share_income' AND created_at>=? AND created_at<?`
	args := []any{start.Format(time.RFC3339), end.Format(time.RFC3339)}
	if email != "" {
		q += " AND email=?"
		args = append(args, email)
	}
	q += " GROUP BY h ORDER BY h"
	list, err := s.queryKV(q, args...)
	if err != nil {
		return nil, err
	}
	return map[string]any{"series": fillZeroHours(list, start, end)}, nil
}

// ============ /api/income/daily?email=&days= ============
func (s *Server) handleIncomeDaily(w http.ResponseWriter, r *http.Request) (any, error) {
	email := emailParam(r)
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 || days > 90 {
		days = 14
	}
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(days - 1))
	end := time.Now()
	q := `SELECT substr(created_at,1,10) d, SUM(CAST(amount AS REAL)) FROM balance_ledger WHERE direction='credit' AND reason='account_share_income' AND created_at>=? AND created_at<=?`
	args := []any{start.Format(time.RFC3339), end.Format(time.RFC3339)}
	if email != "" {
		q += " AND email=?"
		args = append(args, email)
	}
	q += " GROUP BY d ORDER BY d"
	list, err := s.queryKV(q, args...)
	if err != nil {
		return nil, err
	}
	return map[string]any{"series": fillZeroDays(list, start, days)}, nil
}

// ============ /api/income/top?email=&dim= ============
// dim = consumer_user_id | account_id | api_key_id
func (s *Server) handleIncomeTop(w http.ResponseWriter, r *http.Request) (any, error) {
	email := emailParam(r)
	dim := r.URL.Query().Get("dim")
	col := map[string]string{
		"consumer_user_id": "json_extract(metadata,'$.consumer_user_id')",
		"account_id":       "json_extract(metadata,'$.account_id')",
		"api_key_id":       "json_extract(metadata,'$.api_key_id')",
	}[dim]
	if col == "" {
		col = "json_extract(metadata,'$.consumer_user_id')"
		dim = "consumer_user_id"
	}
	q := `SELECT CAST(` + col + ` AS TEXT) k, COUNT(*) n, ROUND(SUM(CAST(amount AS REAL)),6) v FROM balance_ledger WHERE direction='credit' AND reason='account_share_income'`
	args := []any{}
	if email != "" {
		q += " AND email=?"
		args = append(args, email)
	}
	q += " AND " + col + " IS NOT NULL GROUP BY k ORDER BY v DESC LIMIT 15"
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type Item struct {
		K string  `json:"k"`
		N int64   `json:"n"`
		V float64 `json:"v"`
	}
	var out []Item
	for rows.Next() {
		var it Item
		rows.Scan(&it.K, &it.N, &it.V)
		out = append(out, it)
	}
	return map[string]any{"dim": dim, "items": out}, rows.Err()
}

// ============ /api/usage/summary?email= ============
func (s *Server) handleUsageSummary(w http.ResponseWriter, r *http.Request) (any, error) {
	email := emailParam(r)
	where := "1=1"
	args := []any{}
	if email != "" {
		where = "email=?"
		args = append(args, email)
	}
	type Sum struct {
		Requests    int64   `json:"requests"`
		InputTok    int64   `json:"input_tokens"`
		OutputTok   int64   `json:"output_tokens"`
		CacheTok    int64   `json:"cache_read_tokens"`
		TotalCost   float64 `json:"total_cost"`
		ActualCost  float64 `json:"actual_cost"`
		AvgDuration float64 `json:"avg_duration_ms"`
	}
	var m Sum
	err := s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(cache_read_tokens),0), COALESCE(SUM(total_cost),0), COALESCE(SUM(actual_cost),0), COALESCE(AVG(duration_ms),0) FROM usage_logs WHERE `+where, args...).
		Scan(&m.Requests, &m.InputTok, &m.OutputTok, &m.CacheTok, &m.TotalCost, &m.ActualCost, &m.AvgDuration)
	if err != nil {
		return nil, err
	}
	// request_type / billing_mode 分布
	type Dist struct {
		RequestType []kv `json:"request_type"`
		BillingMode []kv `json:"billing_mode"`
		Stream      []kv `json:"stream"`
	}
	rt, _ := s.queryKV(`SELECT COALESCE(request_type,'?'), COUNT(*) FROM usage_logs WHERE `+where+` GROUP BY request_type`, args...)
	bm, _ := s.queryKV(`SELECT COALESCE(billing_mode,'?'), COUNT(*) FROM usage_logs WHERE `+where+` GROUP BY billing_mode`, args...)
	st, _ := s.queryKV(`SELECT CASE stream WHEN 1 THEN 'stream' ELSE 'non-stream' END, COUNT(*) FROM usage_logs WHERE `+where+` GROUP BY stream`, args...)
	return map[string]any{"summary": m, "dist": Dist{rt, bm, st}}, nil
}

// ============ /api/usage/by-model?email= ============
func (s *Server) handleUsageByModel(w http.ResponseWriter, r *http.Request) (any, error) {
	email := emailParam(r)
	where := "1=1"
	args := []any{}
	if email != "" {
		where = "email=?"
		args = append(args, email)
	}
	rows, err := s.db.Query(`SELECT COALESCE(model,'?'), COUNT(*), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(cache_read_tokens),0), ROUND(COALESCE(SUM(total_cost),0),6), ROUND(COALESCE(SUM(actual_cost),0),6), ROUND(COALESCE(AVG(duration_ms),0),0), ROUND(COALESCE(AVG(rate_multiplier),0),4) FROM usage_logs WHERE `+where+` GROUP BY model ORDER BY SUM(total_cost) DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type M struct {
		Model      string  `json:"model"`
		Requests   int64   `json:"requests"`
		Input      int64   `json:"input_tokens"`
		Output     int64   `json:"output_tokens"`
		Cache      int64   `json:"cache_read_tokens"`
		TotalCost  float64 `json:"total_cost"`
		ActualCost float64 `json:"actual_cost"`
		AvgDur     float64 `json:"avg_duration_ms"`
		AvgRate    float64 `json:"avg_rate_multiplier"`
	}
	var out []M
	for rows.Next() {
		var m M
		rows.Scan(&m.Model, &m.Requests, &m.Input, &m.Output, &m.Cache, &m.TotalCost, &m.ActualCost, &m.AvgDur, &m.AvgRate)
		out = append(out, m)
	}
	return map[string]any{"items": out}, rows.Err()
}

// ============ /api/usage/daily?email=&days= ============
// 按天：请求数、tokens、计费、实扣、分账收入（同日 join 不到，分开给）
func (s *Server) handleUsageDaily(w http.ResponseWriter, r *http.Request) (any, error) {
	email := emailParam(r)
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 || days > 90 {
		days = 14
	}
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(days - 1))
	end := time.Now()
	where := "created_at>=? AND created_at<=?"
	args := []any{start.Format(time.RFC3339), end.Format(time.RFC3339)}
	if email != "" {
		where += " AND email=?"
		args = append(args, email)
	}
	type Day struct {
		Day        string  `json:"day"`
		Requests   int64   `json:"requests"`
		Output     int64   `json:"output_tokens"`
		TotalCost  float64 `json:"total_cost"`
		ActualCost float64 `json:"actual_cost"`
		Income     float64 `json:"income"`
	}
	umap := map[string]*Day{}
	rows, err := s.db.Query(`SELECT substr(created_at,1,10) d, COUNT(*), COALESCE(SUM(output_tokens),0), COALESCE(SUM(total_cost),0), COALESCE(SUM(actual_cost),0) FROM usage_logs WHERE `+where+` GROUP BY d`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		d := &Day{}
		rows.Scan(&d.Day, &d.Requests, &d.Output, &d.TotalCost, &d.ActualCost)
		umap[d.Day] = d
	}
	rows.Close()
	// 同日分账收入
	irows, err := s.db.Query(`SELECT substr(created_at,1,10) d, SUM(CAST(amount AS REAL)) FROM balance_ledger WHERE direction='credit' AND reason='account_share_income' AND `+where+` GROUP BY d`, args...)
	if err == nil {
		for irows.Next() {
			var d string
			var v float64
			irows.Scan(&d, &v)
			if umap[d] == nil {
				umap[d] = &Day{Day: d}
			}
			umap[d].Income = v
		}
		irows.Close()
	}
	var out []Day
	for i := 0; i < days; i++ {
		k := fmtDay(start.AddDate(0, 0, i))
		if umap[k] != nil {
			out = append(out, *umap[k])
		} else {
			out = append(out, Day{Day: k})
		}
	}
	return map[string]any{"items": out}, nil
}

// ============ /api/usage/hourly?email=&day= ============
// 某一天的 24 小时用量分布
func (s *Server) handleUsageHourly(w http.ResponseWriter, r *http.Request) (any, error) {
	email := emailParam(r)
	day := r.URL.Query().Get("day")
	if day == "" {
		day = fmtDay(time.Now())
	}
	where := "substr(created_at,1,10)=?"
	args := []any{day}
	if email != "" {
		where += " AND email=?"
		args = append(args, email)
	}
	list, err := s.queryKV(`SELECT substr(created_at,12,2) h, COUNT(*) FROM usage_logs WHERE `+where+` GROUP BY h ORDER BY h`, args...)
	if err != nil {
		return nil, err
	}
	m := map[string]float64{}
	for _, x := range list {
		m[x.K] = x.V
	}
	var out []kv
	for h := 0; h < 24; h++ {
		k := strconv.Itoa(h)
		if h < 10 {
			k = "0" + k
		}
		out = append(out, kv{k, m[k]})
	}
	return map[string]any{"day": day, "series": out}, nil
}

// ============ /api/ledger?email=&direction=&reason=&limit=&offset= ============
func (s *Server) handleLedger(w http.ResponseWriter, r *http.Request) (any, error) {
	q := r.URL.Query()
	where := "1=1"
	args := []any{}
	if v := q.Get("email"); v != "" {
		where += " AND email=?"
		args = append(args, v)
	}
	if v := q.Get("direction"); v != "" {
		where += " AND direction=?"
		args = append(args, v)
	}
	if v := q.Get("reason"); v != "" {
		where += " AND reason=?"
		args = append(args, v)
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	var total int64
	s.db.QueryRow(`SELECT COUNT(*) FROM balance_ledger WHERE `+where, args...).Scan(&total)
	rows, err := s.db.Query(`SELECT id, email, direction, amount, reason, ref_id, balance_after, metadata, created_at FROM balance_ledger WHERE `+where+` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type L struct {
		ID        int64             `json:"id"`
		Email     string            `json:"email"`
		Direction string            `json:"direction"`
		Amount    string            `json:"amount"`
		Reason    string            `json:"reason"`
		RefID     int64             `json:"ref_id"`
		Balance   string            `json:"balance_after"`
		Meta      map[string]any    `json:"metadata"`
		CreatedAt string            `json:"created_at"`
	}
	var out []L
	for rows.Next() {
		var l L
		var meta sql.NullString
		if err := rows.Scan(&l.ID, &l.Email, &l.Direction, &l.Amount, &l.Reason, &l.RefID, &l.Balance, &meta, &l.CreatedAt); err != nil {
			continue
		}
		if meta.Valid && meta.String != "" {
			json.Unmarshal([]byte(meta.String), &l.Meta)
		}
		out = append(out, l)
	}
	// 可选 reason 列表（供筛选下拉）
	reasons, _ := s.queryKV(`SELECT reason, COUNT(*) FROM balance_ledger GROUP BY reason ORDER BY 2 DESC`)
	return map[string]any{"total": total, "items": out, "reasons": reasons}, rows.Err()
}

// ============ /api/balance-trend?email= ============
// 余额随时间变化：每天取最后一条 ledger 的 balance_after
func (s *Server) handleBalanceTrend(w http.ResponseWriter, r *http.Request) (any, error) {
	email := emailParam(r)
	where := "1=1"
	args := []any{}
	if email != "" {
		where = "email=?"
		args = append(args, email)
	}
	// 子查询取每日最大 id，再回查其 balance_after；参数按出现顺序传两次
	q := `SELECT substr(created_at,1,10) d, CAST(balance_after AS REAL)
FROM balance_ledger
WHERE id IN (SELECT MAX(id) FROM balance_ledger WHERE ` + where + ` GROUP BY substr(created_at,1,10))
  AND ` + where + `
ORDER BY d`
	list, err := s.queryKV(q, append(args, args...)...)
	if err != nil {
		return nil, err
	}
	return map[string]any{"series": list}, nil
}

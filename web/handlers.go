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
// ovAccount 行：托管账号额度窗（从 account_windows 表读最新快照）
type ovAccount struct {
	AccountID   int64   `json:"account_id"`
	Email       string  `json:"email"`
	Name        string  `json:"name"`
	Platform    string  `json:"platform"`
	Cost7d      float64 `json:"cost_7d"`
	UserCost7d  float64 `json:"user_cost_7d"`
	Utilization float64 `json:"utilization"`
	Requests7d  int64   `json:"requests_7d"`
	FetchedAt   int64   `json:"fetched_at"`
}

func (s *Server) listAccountWindows() ([]ovAccount, error) {
	rows, err := s.db.Query(`SELECT account_id, email, name, platform, COALESCE(sd_cost,0), COALESCE(sd_user_cost,0), COALESCE(sd_utilization,0), COALESCE(sd_requests,0), fetched_at FROM account_windows ORDER BY email, account_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ovAccount
	for rows.Next() {
		var a ovAccount
		if err := rows.Scan(&a.AccountID, &a.Email, &a.Name, &a.Platform, &a.Cost7d, &a.UserCost7d, &a.Utilization, &a.Requests7d, &a.FetchedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) (any, error) {
	now := time.Now()
	curHour := now.Truncate(time.Hour)
	prevHour := curHour.Add(-time.Hour)
	ydaySameHour := curHour.AddDate(0, 0, -1)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterdayStart := todayStart.AddDate(0, 0, -1)
	// 同环比分钟对齐：当前小时已过 N 分钟，则三个对比窗口都取各自小时的前 N 分钟，
	// 保证"同口径"——拿 12 分钟数据去比完整 60 分钟没有意义。
	elapsed := now.Sub(curHour)
	curWinEnd := curHour.Add(elapsed)
	prevWinEnd := prevHour.Add(elapsed)
	ydayWinEnd := ydaySameHour.Add(elapsed)

	emails, err := s.listEmails()
	if err != nil {
		return nil, err
	}
	type Row struct {
		Email           string   `json:"email"`
		Cost7d          float64  `json:"cost_7d"`
		UserCost7d      float64  `json:"user_cost_7d"`
		Utilization     *float64 `json:"utilization"`
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
	var totCost7d, totQuota7d float64
	for _, e := range emails {
		row := Row{Email: e}
		var c7, u7 float64
		s.db.QueryRow(`SELECT COALESCE(SUM(sd_cost),0), COALESCE(SUM(sd_user_cost),0) FROM account_windows WHERE email=?`, e).Scan(&c7, &u7)
		row.Cost7d, row.UserCost7d = c7, u7
		var quota float64
		s.db.QueryRow(`SELECT COALESCE(SUM(CASE WHEN sd_utilization>0 THEN sd_cost/(sd_utilization/100.0) ELSE 0 END),0) FROM account_windows WHERE email=?`, e).Scan(&quota)
		if quota > 0 {
			v := c7 / quota * 100
			row.Utilization = &v
		}
		totCost7d += c7
		totQuota7d += quota
		s.db.QueryRow(`SELECT COALESCE(SUM(total_cost),0), COALESCE(SUM(actual_cost),0) FROM usage_logs WHERE email=?`, e).Scan(&row.TotalCost, &row.ActualCost)
		s.db.QueryRow(`SELECT COALESCE(SUM(CAST(amount AS REAL)),0) FROM balance_ledger WHERE email=? AND direction='credit' AND reason='account_share_income'`, e).Scan(&row.ShareIncome)
		// 当前小时显示用分钟对齐窗口（与上小时/昨同小时前N分钟同口径）
		row.CurHourIncome = s.incomeBetween(e, curHour, curWinEnd)
		// 上一小时/昨日同小时显示**完整1小时**（需求5）；分钟对齐只用于比率
		row.PrevHourIncome = s.incomeBetween(e, prevHour, curHour)
		row.YdaySameHourInc = s.incomeBetween(e, ydaySameHour, ydaySameHour.Add(time.Hour))
		row.HourQoq = pct(s.incomeBetween(e, curHour, curWinEnd), s.incomeBetween(e, prevHour, prevWinEnd))
		row.HourYoy = pct(s.incomeBetween(e, curHour, curWinEnd), s.incomeBetween(e, ydaySameHour, ydayWinEnd))
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
	var pct7d *float64
	if totQuota7d > 0 {
		v := totCost7d / totQuota7d * 100
		pct7d = &v
	}
	accounts, _ := s.listAccountWindows()
	return map[string]any{
		"now":             now.Format(time.RFC3339),
		"cur_hour":        fmtHour(curHour),
		"prev_hour":       fmtHour(prevHour),
		"yday_same_hour":  fmtHour(ydaySameHour),
		"elapsed_minutes": int(elapsed.Minutes()),
		"total_cost_7d":   totCost7d,
		"total_quota_7d":  totQuota7d,
		"usage_pct_7d":    pct7d,
		"rows":            out,
		"accounts":        accounts,
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
	// usage 侧：按模型聚合 账号成本(total_cost)/用户扣费(actual_cost)/tokens/耗时
	rows, err := s.db.Query(`SELECT COALESCE(model,'?'), COUNT(*), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(cache_read_tokens),0), ROUND(COALESCE(SUM(total_cost),0),6), ROUND(COALESCE(SUM(actual_cost),0),6), ROUND(COALESCE(AVG(duration_ms),0),0) FROM usage_logs WHERE `+where+` GROUP BY model ORDER BY SUM(total_cost) DESC`, args...)
	if err != nil {
		return nil, err
	}
	type M struct {
		Model       string   `json:"model"`
		Requests    int64    `json:"requests"`
		Input       int64    `json:"input_tokens"`
		Output      int64    `json:"output_tokens"`
		Cache       int64    `json:"cache_read_tokens"`
		TotalCost   float64  `json:"total_cost"`   // 账号成本
		ActualCost  float64  `json:"actual_cost"`  // 用户扣费
		ShareIncome float64  `json:"share_income"` // 分账收入
		AvgDur      float64  `json:"avg_duration_ms"`
		RateMult    *float64 `json:"rate_multiplier"` // 用户扣费/账号成本
		ShareRate   *float64 `json:"share_rate"`      // 分账收入/用户扣费
	}
	var out []M
	midx := map[string]int{} // model -> out 下标（存索引避免 append 重分配导致指针失效）
	for rows.Next() {
		var m M
		rows.Scan(&m.Model, &m.Requests, &m.Input, &m.Output, &m.Cache, &m.TotalCost, &m.ActualCost, &m.AvgDur)
		midx[m.Model] = len(out)
		out = append(out, m)
	}
	rows.Close()
	// 分账收入按模型：ledger.ref_id 关联 usage_logs.id 取 model（ref_type=usage_log）
	iw := "l.direction='credit' AND l.reason='account_share_income'"
	iargs := []any{}
	if email != "" {
		iw += " AND l.email=?"
		iargs = append(iargs, email)
	}
	// 分账收入按模型归因：ledger 按 account_id 聚合收入 -> 映射该 account 的 dominant model。
	// request_id/ref_id 与上游流水不同源无法 join；account_id 是唯一可对上的归因维度。
	acctIncome := map[int64]float64{}
	{
		arows, err := s.db.Query(`SELECT CAST(json_extract(l.metadata,'$.account_id') AS INTEGER) aid, SUM(CAST(l.amount AS REAL)) FROM balance_ledger l WHERE `+iw+` AND json_extract(l.metadata,'$.account_id') IS NOT NULL GROUP BY aid`, iargs...)
		if err == nil {
			for arows.Next() {
				var aid int64
				var v float64
				arows.Scan(&aid, &v)
				acctIncome[aid] = v
			}
			arows.Close()
		}
	}
	// 一次性算每个 account 的 dominant model（按 total_cost 最大的模型）
	domModel := map[int64]string{}
	{
		// 不按 email 过滤：account_id 全局唯一，直接全库找该账户的 dominant model
		drows, err := s.db.Query(`SELECT account_id, model, SUM(total_cost) c FROM usage_logs WHERE account_id>0 GROUP BY account_id, model ORDER BY account_id, c DESC`)
		if err == nil {
			seen := map[int64]bool{}
			for drows.Next() {
				var aid int64
				var m string
				var c float64
				drows.Scan(&aid, &m, &c)
				if !seen[aid] {
					domModel[aid] = m
					seen[aid] = true
				} // 每 account 第一行即 dominant
			}
			drows.Close()
		}
	}
	for aid, inc := range acctIncome {
		if m, ok := domModel[aid]; ok {
			if idx, ok2 := midx[m]; ok2 {
				out[idx].ShareIncome += inc
			}
		}
	}
	for i := range out {
		if out[i].TotalCost > 0 {
			v := out[i].ActualCost / out[i].TotalCost
			out[i].RateMult = &v
		}
		if out[i].ActualCost > 0 {
			v := out[i].ShareIncome / out[i].ActualCost
			out[i].ShareRate = &v
		}
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
	// 需求7：按使用者（consumer_user_id）/ key（api_key_id）筛选，存于 metadata JSON
	if v := q.Get("consumer"); v != "" {
		where += " AND CAST(json_extract(metadata,'$.consumer_user_id') AS TEXT)=?"
		args = append(args, v)
	}
	if v := q.Get("api_key"); v != "" {
		where += " AND CAST(json_extract(metadata,'$.api_key_id') AS TEXT)=?"
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
		ID        int64          `json:"id"`
		Email     string         `json:"email"`
		Direction string         `json:"direction"`
		Amount    string         `json:"amount"`
		Reason    string         `json:"reason"`
		RefID     int64          `json:"ref_id"`
		Balance   string         `json:"balance_after"`
		Meta      map[string]any `json:"metadata"`
		CreatedAt string         `json:"created_at"`
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
	// 可选筛选项（供下拉）
	reasons, _ := s.queryKV(`SELECT reason, COUNT(*) FROM balance_ledger GROUP BY reason ORDER BY 2 DESC`)
	consumers, _ := s.queryKV(`SELECT CAST(json_extract(metadata,'$.consumer_user_id') AS TEXT), COUNT(*) FROM balance_ledger WHERE json_extract(metadata,'$.consumer_user_id') IS NOT NULL GROUP BY 1 ORDER BY 2 DESC LIMIT 50`)
	keys, _ := s.queryKV(`SELECT CAST(json_extract(metadata,'$.api_key_id') AS TEXT), COUNT(*) FROM balance_ledger WHERE json_extract(metadata,'$.api_key_id') IS NOT NULL GROUP BY 1 ORDER BY 2 DESC LIMIT 50`)
	return map[string]any{"total": total, "items": out, "reasons": reasons, "consumers": consumers, "api_keys": keys}, rows.Err()
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

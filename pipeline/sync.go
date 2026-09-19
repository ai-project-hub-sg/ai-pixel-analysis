// Package pipeline —— 增量同步：计划、执行、进度。
// 设计原则（对应 readme 第4条）：
//  1. 先出"获取方案"：把要拉的范围切成块，能整段跳过的跳过，能聚合的用汇总接口。
//  2. 平滑请求：固定间隔限速 + 单并发，避免打爆上游。
//  3. 断点续传：以 sync_watermark 为覆盖水位，任何时刻中断下次都从水位继续。
//  4. 进度可见：实时 Progress 写入 sync_jobs.progress_json，前端轮询即可看到
//     当前在同步什么、已拉多少、单块/整体预计完成时间。
package pipeline

import (
	"encoding/json"
	"fmt"
	"time"

	"ai-pixel-analysis/api"
	"ai-pixel-analysis/store"
)

// ---------- 计划 ----------

// Chunk 是一次可独立执行的最小抓取单元。
type Chunk struct {
	Email    string    `json:"email"`
	Kind     string    `json:"kind"` // usage | ledger | snapshots
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	EstPages int       `json:"est_pages"`
	Done     bool      `json:"done"`
	Pages    int       `json:"pages"`
	Items    int       `json:"items"`
	Err      string    `json:"err,omitempty"`
}

// SyncPlan 是一次同步任务的完整方案。
type SyncPlan struct {
	Trigger   string    `json:"trigger"` // init|manual|auto
	CreatedAt time.Time `json:"created_at"`
	Now       time.Time `json:"now"`
	Chunks    []*Chunk  `json:"chunks"`
	TotalEst  int       `json:"total_est_pages"`
	Skipped   int       `json:"skipped_chunks"`
	Note      string    `json:"note,omitempty"`
}

const usageChunkDays = 1  // usage 量大，按天切
const ledgerChunkDays = 7 // ledger 较小，按周切

// BuildPlan 生成抓取方案。
// now: 截止基准；更新到 now-1min（不含正在写入的这分钟）。
// backfillDays: 无数据/无水位时向前回补的天数。
//   - manual/auto 传 0：只从覆盖右端增量更新到 cutoff（不补历史）。
//   - init 传 3：拉最近3天。
//   - backfill 传 30：拉最近30天（单独的大任务）。
// getRange(email,kind) 返回该账号该表已有数据范围；getWM 返回水位。
func BuildPlan(trigger string, emails []string, backfillDays int, getRange func(email, kind string) store.DataRange, getWM func(email, kind string) (time.Time, error), now time.Time) (*SyncPlan, error) {
	cutoff := now.Truncate(time.Minute).Add(-time.Second)
	p := &SyncPlan{Trigger: trigger, CreatedAt: time.Now(), Now: now}
	for _, email := range emails {
		p.addSeries(email, "usage", usageChunkDays, backfillDays, cutoff, getRange, getWM)
		p.addSeries(email, "ledger", ledgerChunkDays, backfillDays, cutoff, getRange, getWM)
		p.Chunks = append(p.Chunks, &Chunk{Email: email, Kind: "snapshots", Start: now, End: now, EstPages: 6})
		p.TotalEst += 6
	}
	return p, nil
}

// addSeries 把待抓区间按 chunkDays 切块；已被水位覆盖的块标 Done 跳过。
func (p *SyncPlan) addSeries(email, kind string, chunkDays, backfillDays int, cutoff time.Time, getRange func(email, kind string) store.DataRange, getWM func(email, kind string) (time.Time, error)) {
	wm, _ := getWM(email, kind)
	dr := getRange(email, kind)

	var start time.Time
	switch {
	case backfillDays > 0:
		// init/backfill：无论水位如何都从 cutoff 往前 backfillDays 天开始（跳过逻辑负责去重）
		start = cutoff.AddDate(0, 0, -(backfillDays - 1))
		// 但若水位/已有数据更靠前，仍需从更早处补空洞 -> 取更早起点
		if !wm.IsZero() && wm.AddDate(0,0,-1).Before(start) {
			start = wm.AddDate(0, 0, -1)
		}
		if dr.Count > 0 && dr.Min != nil && dr.Min.AddDate(0,0,-1).Before(start) {
			start = dr.Min.AddDate(0, 0, -1)
		}
	case !wm.IsZero():
		// 增量：从水位右端开始（向前回退1分钟重叠防边界遗漏，不是1天）
		start = wm.Add(-time.Minute)
	case dr.Count > 0 && dr.Max != nil:
		// 无水位但有数据：从已有数据最大时间增量（不补它之前的历史）
		start = dr.Max.Add(-time.Minute)
	default:
		// 完全无数据且非回填任务：只拉当前这一天（从 cutoff 所在日 0 点）
		start = time.Date(cutoff.Year(), cutoff.Month(), cutoff.Day(), 0, 0, 0, 0, cutoff.Location())
	}
	if start.After(cutoff) {
		return
	}
	for s := start; s.Before(cutoff); {
		e := s.AddDate(0, 0, chunkDays)
		if e.After(cutoff) {
			e = cutoff
		}
		c := &Chunk{Email: email, Kind: kind, Start: s, End: e}
		// 已覆盖右端 = max(水位, 已有数据最大 created_at)。
		// 已有数据（即使是手动 fetch 来的、没有水位）同样视为已覆盖，避免全量回补。
		coverEnd := wm
		if dr.Max != nil && dr.Max.After(coverEnd) {
			coverEnd = *dr.Max
		}
		if !coverEnd.IsZero() && !e.After(coverEnd) {
			c.Done = true
			c.Err = "skipped:covered"
			p.Skipped++
		} else {
			est := 1
			if dr.Count > 0 && dr.Max != nil && dr.Min != nil {
				days := dr.Max.Sub(*dr.Min).Hours() / 24
				if days < 1 {
					days = 1
				}
				perDay := float64(dr.Count) / days
				est = int(perDay*float64(chunkDays)/100) + 1
				if est < 1 {
					est = 1
				}
			}
			c.EstPages = est
			p.TotalEst += est
		}
		p.Chunks = append(p.Chunks, c)
		s = e
	}
}

// ---------- 执行 ----------

// Progress 是实时进度快照。
type Progress struct {
	JobID        int64     `json:"job_id"`
	Status       string    `json:"status"`
	TotalChunks  int       `json:"total_chunks"`
	DoneChunks   int       `json:"done_chunks"`
	TotalEstPg   int       `json:"total_est_pages"`
	FetchedPages int       `json:"fetched_pages"`
	FetchedItems int       `json:"fetched_items"`
	Skipped      int       `json:"skipped"`
	CurEmail     string    `json:"cur_email,omitempty"`
	CurKind      string    `json:"cur_kind,omitempty"`
	CurRange     string    `json:"cur_range,omitempty"`
	CurPage      int       `json:"cur_page,omitempty"`
	CurEstPg     int       `json:"cur_est_pages,omitempty"`
	StartedAt    time.Time `json:"started_at,omitempty"`
	CurChunkEta  time.Time `json:"cur_chunk_eta,omitempty"`
	OverallEta   time.Time `json:"overall_eta,omitempty"`
	FinishedAt   time.Time `json:"finished_at,omitempty"`
	Error        string    `json:"error,omitempty"`
}

// Syncer 执行计划并上报进度。单并发 + 固定间隔限速。
type Syncer struct {
	Store      *store.Store
	ClientFor  func(email string) (*api.Client, error)
	PageSize   int
	Interval   time.Duration
	RawDir     string
	OnProgress func(*Progress)
}

// Run 执行整个 plan。
func (sy *Syncer) Run(jobID int64, plan *SyncPlan) *Progress {
	pg := &Progress{JobID: jobID, Status: "running", StartedAt: time.Now()}
	pg.TotalChunks = len(plan.Chunks)
	pg.TotalEstPg = plan.TotalEst
	pg.Skipped = plan.Skipped
	for _, c := range plan.Chunks {
		if c.Done {
			pg.DoneChunks++
			pg.FetchedPages += c.EstPages
		}
	}
	sy.report(pg)
	if sy.Interval <= 0 {
		sy.Interval = 1200 * time.Millisecond
	}
	if sy.PageSize <= 0 {
		sy.PageSize = 100
	}
	var firstErr error
	for _, c := range plan.Chunks {
		if c.Done {
			continue
		}
		pg.CurEmail, pg.CurKind = c.Email, c.Kind
		pg.CurRange = c.Start.Format("01-02 15:04") + " ~ " + c.End.Format("01-02 15:04")
		pg.CurPage, pg.CurEstPg = 0, c.EstPages
		sy.report(pg)
		if err := sy.runChunk(c, pg); err != nil {
			c.Err = err.Error()
			if firstErr == nil {
				firstErr = err
			}
		} else if c.Kind == "usage" || c.Kind == "ledger" {
			// 只有时间序列数据才推进水位；snapshots 是即时快照，不参与覆盖
			_ = sy.Store.SetWatermark(c.Email, c.Kind, c.End)
		}
		c.Done = true
		pg.DoneChunks++
		sy.report(pg)
	}
	pg.Status = "done"
	pg.FinishedAt = time.Now()
	if firstErr != nil {
		pg.Status = "failed"
		pg.Error = firstErr.Error()
	}
	pg.CurEmail, pg.CurKind, pg.CurRange = "", "", ""
	sy.report(pg)
	return pg
}

// runChunk 执行一块：自带分页循环，每次请求前 sleep 限速，每页更新进度。
func (sy *Syncer) runChunk(c *Chunk, pg *Progress) error {
	client, err := sy.ClientFor(c.Email)
	if err != nil {
		return err
	}
	f := &Fetcher{Client: client, Store: sy.Store, Email: c.Email}
	switch c.Kind {
	case "usage", "ledger":
		o := Options{
			StartTime: c.Start.Format(time.RFC3339),
			EndTime:   c.End.Format(time.RFC3339),
			PageSize:  sy.PageSize,
			MaxPages:  500,
			RawDir:    sy.RawDir,
		}
		isUsage := c.Kind == "usage"
		var lastMax time.Time
		for page := 1; ; page++ {
			if page > 500 {
				return fmt.Errorf("%s: exceeded 500 pages safety cap", c.Kind)
			}
			// 限速：除第1页外，每次请求间隔 Interval
			if page > 1 {
				time.Sleep(sy.Interval)
			}
			var items []json.RawMessage
			var pages int
			var raw json.RawMessage
			var ferr error
			if isUsage {
				var l *api.UsageList
				var rr api.RawResult
				l, rr, ferr = client.ListUsage(o.query(page))
				raw = rr.Body
				if ferr == nil {
					items, pages = l.Items, l.Pages
				}
			} else {
				var l *api.LedgerList
				var rr api.RawResult
				l, rr, ferr = client.ListBalanceLedger(o.query(page))
				raw = rr.Body
				if ferr == nil {
					items = make([]json.RawMessage, 0, len(l.Items))
					for _, it := range l.Items {
						b, _ := json.Marshal(it)
						items = append(items, b)
					}
					pages = l.Pages
				}
			}
			if ferr != nil {
				return fmt.Errorf("%s page %d: %w", c.Kind, page, ferr)
			}
			// 落盘原始页
			p := rawFilePath(o.RawDir, c.Kind, c.Email, o.rangeDesc(), page)
			if werr := writeRawFile(p, c.Kind, c.Email, o.rangeDesc()+"&page="+fmt.Sprint(page), raw); werr == nil {
				// ok
			}
			// 入库
			if len(items) > 0 {
				var n int
				if isUsage {
					n, err = sy.Store.SaveUsageItems(c.Email, items)
				} else {
					n, err = sy.Store.SaveLedgerItems(c.Email, items)
				}
				if err != nil {
					return fmt.Errorf("%s save page %d: %w", c.Kind, page, err)
				}
				c.Items += n
				pg.FetchedItems += n
			}
			c.Pages = page
			pg.CurPage = page
			pg.FetchedPages++
			sy.updateETA(pg)
			sy.report(pg)
			if page >= pages || len(items) == 0 {
				break
			}
			_ = lastMax
		}
		return nil
	case "snapshots":
		o := Options{StartTime: c.Start.Format(time.RFC3339), EndTime: c.End.Format(time.RFC3339), PageSize: sy.PageSize, MaxPages: 1, RawDir: sy.RawDir}
		res := &Result{}
		err := f.FetchSnapshots(o, res)
		pg.FetchedPages += 6
		pg.FetchedItems += 6
		sy.updateETA(pg)
		sy.report(pg)
		return err
	}
	return nil
}

func (sy *Syncer) updateETA(pg *Progress) {
	now := time.Now()
	if pg.CurEstPg > 0 && pg.CurPage > 0 && pg.FetchedPages > 0 {
		per := now.Sub(pg.StartedAt) / time.Duration(pg.FetchedPages)
		pg.CurChunkEta = now.Add(per * time.Duration(pg.CurEstPg-pg.CurPage))
	}
	if pg.FetchedPages > 0 && pg.TotalEstPg > 0 {
		per := now.Sub(pg.StartedAt) / time.Duration(pg.FetchedPages)
		pg.OverallEta = now.Add(per * time.Duration(pg.TotalEstPg-pg.FetchedPages))
	}
}

func (sy *Syncer) report(pg *Progress) {
	if sy.OnProgress != nil {
		sy.OnProgress(pg)
	}
}

func MarshalProgress(pg *Progress) []byte {
	b, _ := json.Marshal(pg)
	return b
}
func UnmarshalProgress(b []byte) (*Progress, error) {
	var p Progress
	if len(b) == 0 {
		return &p, nil
	}
	return &p, json.Unmarshal(b, &p)
}
func MarshalPlan(p *SyncPlan) []byte {
	b, _ := json.Marshal(p)
	return b
}
func UnmarshalPlan(b []byte) (*SyncPlan, error) {
	var p SyncPlan
	return &p, json.Unmarshal(b, &p)
}

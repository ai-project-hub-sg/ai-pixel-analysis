package web

import (
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"ai-pixel-analysis/api"
	"ai-pixel-analysis/pipeline"
	"ai-pixel-analysis/store"
)

// SyncDeps 是同步所需的全部依赖。host 用于建 api client；store 用于读 session/写数据。
// 注意：此 worker 是唯一持有可写 store + db_secret 的组件；分析 API 仍用只读连接。
type SyncDeps struct {
	Host     string
	Store    *store.Store // 可写
	ReadDB   *sql.DB      // 只读（与 Server 共享，用于查已有数据范围——用同一连接即可）
	RawDir   string
	Interval time.Duration
}

// SyncWorker 串行执行同步任务，维护当前任务与自动同步开关。
type SyncWorker struct {
	deps      *SyncDeps
	mu        sync.Mutex
	cur       *pipeline.Progress
	auto      bool
	stopCh    chan struct{}
	wake      chan struct{} // 触发一次手动/初始化同步的信号
	lastJobID int64
	dataVer   int64     // 数据版本号：每次同步结束递增，前端据此自动刷新
	closeOnce sync.Once // Close 幂等：shutdown goroutine 与 runWeb defer 都可能调
}

func NewSyncWorker(d *SyncDeps) *SyncWorker {
	w := &SyncWorker{deps: d, stopCh: make(chan struct{}), wake: make(chan struct{}, 1)}
	// 进程（重）启动：把还停在 running/pending 的旧任务标记为 interrupted，否则永远显示 running。
	if n, err := d.Store.MarkInterruptedJobs(); err == nil && n > 0 {
		log.Printf("sync: marked %d stale running/pending job(s) as interrupted", n)
	}
	// 恢复持久化的 auto 开关
	if v, err := d.Store.GetKV("auto_sync"); err == nil && v == "1" {
		w.auto = true
	}
	go w.loop()
	return w
}

// wake 通道非阻塞发信号。
func (w *SyncWorker) signal() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// StartJob 启动一次同步（init|manual|auto）。返回 job id。
// 若已有任务在跑，返回当前任务 id 与 busy=true。
func (w *SyncWorker) StartJob(trigger string) (int64, bool, error) {
	w.mu.Lock()
	running := w.cur != nil && w.cur.Status == "running"
	curID := w.lastJobID
	w.mu.Unlock()
	if running {
		return curID, true, nil
	}
	emails, err := w.deps.Store.ListSessions()
	if err != nil || len(emails) == 0 {
		return 0, false, fmt.Errorf("no logged-in sessions; run login first")
	}
	now := time.Now()
	// 触发类型决定回补天数：init=3天, backfill=30天, manual/auto=0(纯增量)
	backfillDays := 0
	switch trigger {
	case "init":
		backfillDays = 3
	case "backfill":
		backfillDays = 30
	}
	plan, err := pipeline.BuildPlan(trigger, emails, backfillDays,
		func(email, kind string) store.DataRange {
			if kind == "usage" {
				r, _ := w.deps.Store.UsageRange(email)
				return r
			}
			r, _ := w.deps.Store.LedgerRange(email)
			return r
		},
		func(email, kind string) (time.Time, error) { return w.deps.Store.GetWatermark(email, kind) },
		now)
	if err != nil {
		return 0, false, err
	}
	// 如果没有任何待抓块，直接返回已完成的空任务
	if plan.TotalEst == 0 {
		plan.Note = "up-to-date: nothing to fetch"
	}
	jobID, err := w.deps.Store.CreateSyncJob(trigger, pipeline.MarshalPlan(plan))
	if err != nil {
		return 0, false, err
	}
	w.mu.Lock()
	w.lastJobID = jobID
	w.mu.Unlock()
	go w.execute(jobID, plan)
	return jobID, false, nil
}

// execute 跑一个 plan 并持续把进度写库。
func (w *SyncWorker) execute(jobID int64, plan *pipeline.SyncPlan) {
	sy := &pipeline.Syncer{
		Store:    w.deps.Store,
		RawDir:   w.deps.RawDir,
		PageSize: 100,
		Interval: w.deps.Interval,
		ClientFor: func(email string) (*api.Client, error) {
			sess, err := w.deps.Store.GetSession(email)
			if err != nil {
				return nil, fmt.Errorf("session %s: %w", email, err)
			}
			return api.NewClient(w.deps.Host, sess.TokenType, sess.AccessToken)
		},
	}
	sy.OnProgress = func(pg *pipeline.Progress) {
		w.mu.Lock()
		w.cur = pg
		w.mu.Unlock()
		st := pg.Status
		if st == "" {
			st = "running"
		}
		_ = w.deps.Store.UpdateSyncJob(jobID, st, pipeline.MarshalProgress(pg), pg.Error)
	}
	_ = w.deps.Store.UpdateSyncJob(jobID, "running", nil, "")
	pg := sy.Run(jobID, plan)
	final := "done"
	if pg.Status == "failed" {
		final = "failed"
	}
	_ = w.deps.Store.UpdateSyncJob(jobID, final, pipeline.MarshalProgress(pg), pg.Error)
	w.mu.Lock()
	w.cur = nil
	w.dataVer++ // 任务结束 -> 数据已变化，通知前端刷新
	w.mu.Unlock()
}

// loop 每分钟检查一次 auto 开关并触发 auto 同步。
func (w *SyncWorker) loop() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-w.stopCh:
			return
		case <-t.C:
			w.mu.Lock()
			auto := w.auto
			running := w.cur != nil && w.cur.Status == "running"
			w.mu.Unlock()
			if auto && !running {
				if _, _, err := w.StartJob("auto"); err != nil {
					log.Printf("auto sync: %v", err)
				}
			}
		}
	}
}

// SetAuto 持久化开关并立即生效。
func (w *SyncWorker) SetAuto(on bool) error {
	w.mu.Lock()
	w.auto = on
	w.mu.Unlock()
	v := "0"
	if on {
		v = "1"
	}
	return w.deps.Store.SetKV("auto_sync", v)
}

func (w *SyncWorker) Auto() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.auto
}

// Status 返回给前端的完整状态。
type SyncStatus struct {
	HasData    bool                                  `json:"has_data"`
	Auto       bool                                  `json:"auto"`
	Running    bool                                  `json:"running"`
	DataVer    int64                                 `json:"data_version"`
	Current    *pipeline.Progress                    `json:"current,omitempty"`
	LatestJob  *store.SyncJobRow                     `json:"latest_job,omitempty"`
	Watermarks map[string]map[string]time.Time       `json:"watermarks"`
	Ranges     map[string]map[string]store.DataRange `json:"ranges"`
}

func (w *SyncWorker) Status() (*SyncStatus, error) {
	st := &SyncStatus{Auto: w.Auto()}
	if has, err := w.deps.Store.HasAnyData(); err == nil {
		st.HasData = has
	}
	w.mu.Lock()
	st.Current = w.cur
	st.Running = w.cur != nil && w.cur.Status == "running"
	st.DataVer = w.dataVer
	w.mu.Unlock()
	if j, err := w.deps.Store.GetLatestJob(); err == nil {
		st.LatestJob = j
	}
	if wm, err := w.deps.Store.AllWatermarks(); err == nil {
		st.Watermarks = wm
	}
	// 已有数据范围（每账号每类 min/max/count）
	emails, _ := w.deps.Store.ListSessions()
	st.Ranges = map[string]map[string]store.DataRange{}
	for _, e := range emails {
		st.Ranges[e] = map[string]store.DataRange{}
		if r, err := w.deps.Store.UsageRange(e); err == nil {
			st.Ranges[e]["usage"] = r
		}
		if r, err := w.deps.Store.LedgerRange(e); err == nil {
			st.Ranges[e]["ledger"] = r
		}
	}
	return st, nil
}

func (w *SyncWorker) Close() { w.closeOnce.Do(func() { close(w.stopCh) }) }

// GetLatestJob 代理到 store（Server 调用）。
func (w *SyncWorker) GetLatestJob() (*store.SyncJobRow, error)   { return w.deps.Store.GetLatestJob() }
func (w *SyncWorker) ListJobs(n int) ([]store.SyncJobRow, error) { return w.deps.Store.ListSyncJobs(n) }

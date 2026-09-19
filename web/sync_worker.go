package web

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ai-pixel-analysis/api"
	"ai-pixel-analysis/auth"
	"ai-pixel-analysis/config"
	"ai-pixel-analysis/pipeline"
	"ai-pixel-analysis/store"
)

// SyncDeps 是同步所需的全部依赖。host 用于建 api client；store 用于读 session/写数据。
// 注意：此 worker 是唯一持有可写 store + db_secret 的组件；分析 API 仍用只读连接。
type SyncDeps struct {
	Host      string
	LoginPort string        // 登录页路径，用于取 login_agreement_revision
	Users     []config.User // .env 账号（供 Relogin；机密仅存内存）
	Store     *store.Store  // 可写
	ReadDB    *sql.DB       // 只读（与 Server 共享，用于查已有数据范围——用同一连接即可）
	RawDir    string
	Interval  time.Duration
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
	authValid int32     // 1=登录有效 0=失效/未知 -1=未检测（atomic）
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
	// 启动即后台校验一次登录态：前端可尽快提示"登录已失效"，
	// 同时让 auto loop 的 authValid 门控立即生效（失效即暂停自动同步）。
	go w.CheckAuth()
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
		w.setAuthValid(0)
		return 0, false, ErrNoSession
	}
	// 手动触发时若已知失登录，直接返回 ErrNoSession（前端弹重新登录），
	// 避免起任务后第一块必失败才提示。auto 任务由 loop 的 authValid 门控已拦。
	if trigger != "auto" && w.AuthValid() == 0 {
		return 0, false, ErrNoSession
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
	if pg.Error != "" && strings.Contains(pg.Error, "登录已失效") {
		w.setAuthValid(0)
	}
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
		case <-w.wake:
			// 外部信号（如重新登录成功）触发一次立即同步
			w.mu.Lock()
			auto := w.auto
			running := w.cur != nil && w.cur.Status == "running"
			w.mu.Unlock()
			if auto && !running && w.AuthValid() != 0 {
				if _, _, err := w.StartJob("auto"); err != nil {
					log.Printf("wake sync: %v", err)
				}
			}
		case <-t.C:
			w.mu.Lock()
			auto := w.auto
			running := w.cur != nil && w.cur.Status == "running"
			w.mu.Unlock()
			if auto && !running {
				// 失登录时暂停自动同步：已知失效就不再每分钟必失败地重试，
				// 直到某次 StartJob/手动操作重新检测到有效登录（authValid 复位）。
				if w.AuthValid() == 0 {
					log.Printf("auto sync paused: login invalid, waiting re-login")
				} else if _, _, err := w.StartJob("auto"); err != nil {
					if errors.Is(err, ErrNoSession) {
						w.setAuthValid(0)
					}
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
	AuthValid  int32                                 `json:"auth_valid"` // 1=有效 0=失效 -1=未知
	Current    *pipeline.Progress                    `json:"current,omitempty"`
	LatestJob  *store.SyncJobRow                     `json:"latest_job,omitempty"`
	Watermarks map[string]map[string]time.Time       `json:"watermarks"`
	Ranges     map[string]map[string]store.DataRange `json:"ranges"`
}

func (w *SyncWorker) Status() (*SyncStatus, error) {
	st := &SyncStatus{Auto: w.Auto(), AuthValid: w.AuthValid()}
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

// ErrNoSession 表示没有任何已登录会话（auth_sessions 为空）。
var ErrNoSession = errors.New("no logged-in sessions; run login first")

// CheckAuth 检测当前保存的会话是否仍有效（对第一个账号做轻量 auth/me 校验）。
// 返回 true=有效 false=失效/无会话。结果缓存到 authValid 供 Status 与 auto loop 使用。
func (w *SyncWorker) CheckAuth() bool {
	emails, err := w.deps.Store.ListSessions()
	if err != nil || len(emails) == 0 {
		w.setAuthValid(0)
		return false
	}
	sess, err := w.deps.Store.GetSession(emails[0])
	if err != nil {
		w.setAuthValid(0)
		return false
	}
	cli, err := api.NewClient(w.deps.Host, sess.TokenType, sess.AccessToken)
	if err != nil {
		w.setAuthValid(0)
		return false
	}
	if err := cli.CheckAuth(); err != nil {
		w.setAuthValid(0)
		return false
	}
	w.setAuthValid(1)
	return true
}

func (w *SyncWorker) setAuthValid(v int32) { atomic.StoreInt32(&w.authValid, v) }
func (w *SyncWorker) AuthValid() int32     { return atomic.LoadInt32(&w.authValid) }

// Relogin 用 .env 中的账号重新登录并把新凭据加密入库。
// 全程不返回任何机密；仅返回每账号成功与否。成功后复位 authValid 并唤醒一次同步。
func (w *SyncWorker) Relogin() (map[string]any, error) {
	if len(w.deps.Users) == 0 {
		return map[string]any{"ok": false, "error": "无可用账号（.env 未加载 user_*）"}, nil
	}
	client, err := auth.NewClient(w.deps.Host)
	if err != nil {
		return nil, err
	}
	revision, err := client.FetchAgreementRevision(w.deps.LoginPort)
	if err != nil {
		return nil, fmt.Errorf("fetch agreement revision: %w", err)
	}
	okCount := 0
	var fails []string
	for _, u := range w.deps.Users {
		res, err := client.Login(u.Name, u.Password, revision)
		if err != nil {
			fails = append(fails, u.Name)
			continue
		}
		ar := store.FromLoginResult(res.AccessToken, res.RefreshToken, res.TokenType, res.ExpiresIn, res.Cookies, res.User)
		if err := w.deps.Store.SaveSession(u.Name, ar); err != nil {
			fails = append(fails, u.Name)
			continue
		}
		okCount++
	}
	if okCount > 0 {
		w.setAuthValid(1)
		// 复位失败标记：让 auto loop 恢复、并唤醒一次增量同步
		if w.auto {
			w.signal()
		}
	}
	return map[string]any{
		"ok":      okCount > 0,
		"total":   len(w.deps.Users),
		"success": okCount,
		"failed":  fails,
	}, nil
}

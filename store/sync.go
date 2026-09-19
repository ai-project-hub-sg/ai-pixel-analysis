package store

import (
	"database/sql"
	"fmt"
	"time"
)

// ---------- 水位 ----------

// GetWatermark 返回某账号某类数据已成功覆盖到的最大 created_at。
// 没有记录时返回零值 time.Time{}。
func (s *Store) GetWatermark(email, kind string) (time.Time, error) {
	var wm string
	err := s.db.QueryRow(`SELECT watermark FROM sync_watermark WHERE email=? AND kind=?`, email, kind).Scan(&wm)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, wm)
	if err != nil {
		t, err = time.Parse(time.RFC3339, wm)
	}
	return t, err
}

// SetWatermark 更新水位（只在成功抓取后前移）
func (s *Store) SetWatermark(email, kind string, wm time.Time) error {
	_, err := s.db.Exec(`
INSERT INTO sync_watermark (email, kind, watermark, updated_at) VALUES (?,?,?,?)
ON CONFLICT(email,kind) DO UPDATE SET watermark=excluded.watermark, updated_at=excluded.updated_at`,
		email, kind, wm.UTC().Format(time.RFC3339Nano), time.Now().Unix())
	return err
}

// AllWatermarks 返回全部水印（email -> kind -> watermark）。
func (s *Store) AllWatermarks() (map[string]map[string]time.Time, error) {
	rows, err := s.db.Query(`SELECT email, kind, watermark FROM sync_watermark`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]time.Time{}
	for rows.Next() {
		var e, k, w string
		if err := rows.Scan(&e, &k, &w); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339Nano, w)
		if err != nil {
			t, err = time.Parse(time.RFC3339, w)
			if err != nil {
				continue
			}
		}
		if out[e] == nil {
			out[e] = map[string]time.Time{}
		}
		out[e][k] = t
	}
	return out, rows.Err()
}

// DataRange 是某表已存在数据的 created_at 范围与条数。
type DataRange struct {
	Count int64      `json:"count"`
	Min   *time.Time `json:"min"`
	Max   *time.Time `json:"max"`
}

func (s *Store) tableRange(table, email string) (DataRange, error) {
	var r DataRange
	var mn, mx sql.NullString
	err := s.db.QueryRow(`SELECT COUNT(*), MIN(created_at), MAX(created_at) FROM `+table+` WHERE email=?`, email).
		Scan(&r.Count, &mn, &mx)
	if err != nil {
		return r, err
	}
	parse := func(ns sql.NullString) *time.Time {
		if !ns.Valid {
			return nil
		}
		if t, err := time.Parse(time.RFC3339Nano, ns.String); err == nil {
			return &t
		}
		if t, err := time.Parse(time.RFC3339, ns.String); err == nil {
			return &t
		}
		return nil
	}
	r.Min, r.Max = parse(mn), parse(mx)
	return r, nil
}

func (s *Store) UsageRange(email string) (DataRange, error)  { return s.tableRange("usage_logs", email) }
func (s *Store) LedgerRange(email string) (DataRange, error) { return s.tableRange("balance_ledger", email) }

// HasAnyData 判断库中是否已有任何业务数据。
func (s *Store) HasAnyData() (bool, error) {
	var n int64
	err := s.db.QueryRow(`SELECT (SELECT COUNT(*) FROM usage_logs)+(SELECT COUNT(*) FROM balance_ledger)`).Scan(&n)
	return n > 0, err
}

// ---------- 任务 ----------

func (s *Store) CreateSyncJob(triggerKind string, planJSON []byte) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO sync_jobs (trigger_kind, status, plan_json, created_at) VALUES (?,?,?,?)`,
		triggerKind, "pending", string(planJSON), time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateSyncJob(id int64, status string, progressJSON []byte, errText string) error {
	now := time.Now().Unix()
	var q string
	var args []any
	switch status {
	case "running":
		q = `UPDATE sync_jobs SET status=?, progress_json=?, started_at=COALESCE(started_at,?) WHERE id=?`
		args = []any{status, string(progressJSON), now, id}
	case "done", "failed", "canceled":
		q = `UPDATE sync_jobs SET status=?, progress_json=?, error=?, finished_at=? WHERE id=?`
		args = []any{status, string(progressJSON), errText, now, id}
	default:
		q = `UPDATE sync_jobs SET status=?, progress_json=? WHERE id=?`
		args = []any{status, string(progressJSON), id}
	}
	_, err := s.db.Exec(q, args...)
	return err
}

// SyncJobRow 是任务历史行。json tag 用 snake_case 与前端字段对齐。
type SyncJobRow struct {
	ID           int64      `json:"id"`
	TriggerKind  string     `json:"trigger_kind"`
	Status       string     `json:"status"`
	PlanJSON     string     `json:"plan_json"`
	ProgressJSON string     `json:"progress_json"`
	Error        string     `json:"error"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
}

func scanUnix(ns sql.NullInt64) *time.Time {
	if !ns.Valid {
		return nil
	}
	t := time.Unix(ns.Int64, 0)
	return &t
}

func (s *Store) ListSyncJobs(limit int) ([]SyncJobRow, error) {
	rows, err := s.db.Query(`
SELECT id, trigger_kind, status, COALESCE(plan_json,''), COALESCE(progress_json,''), COALESCE(error,''), created_at, started_at, finished_at
FROM sync_jobs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SyncJobRow
	for rows.Next() {
		var j SyncJobRow
		var st, fi sql.NullInt64
		var ca int64
		if err := rows.Scan(&j.ID, &j.TriggerKind, &j.Status, &j.PlanJSON, &j.ProgressJSON, &j.Error, &ca, &st, &fi); err != nil {
			return nil, err
		}
		j.CreatedAt = time.Unix(ca, 0)
		j.StartedAt = scanUnix(st)
		j.FinishedAt = scanUnix(fi)
		out = append(out, j)
	}
	return out, rows.Err()
}

// MarkInterruptedJobs 把还停在 running/pending 的历史任务标记为 interrupted。
// 进程重启时调用——那些任务的执行进程已死，不可能还在跑，不修正会永远显示 running。
func (s *Store) MarkInterruptedJobs() (int64, error) {
	res, err := s.db.Exec(`UPDATE sync_jobs SET status='interrupted', error='process restarted; task interrupted', finished_at=? WHERE status IN ('running','pending')`, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// GetLatestJob 返回最新一条任务。
func (s *Store) GetLatestJob() (*SyncJobRow, error) {
	jobs, err := s.ListSyncJobs(1)
	if err != nil || len(jobs) == 0 {
		return nil, err
	}
	return &jobs[0], nil
}

// ---------- kv ----------

func (s *Store) ensureKV() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS kv (k TEXT PRIMARY KEY, v TEXT)`)
	return err
}

func (s *Store) SetKV(k, v string) error {
	if err := s.ensureKV(); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO kv (k,v) VALUES (?,?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, k, v)
	return err
}

func (s *Store) GetKV(k string) (string, error) {
	if err := s.ensureKV(); err != nil {
		return "", err
	}
	var v string
	err := s.db.QueryRow(`SELECT v FROM kv WHERE k=?`, k).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get kv %s: %w", k, err)
	}
	return v, nil
}

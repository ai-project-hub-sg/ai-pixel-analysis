package store

import (
	"database/sql"
	"fmt"
	"time"

	"ai-pixel-analysis/crypto"

	_ "modernc.org/sqlite"
)

// Store 封装 SQLite 存取
type Store struct {
	db       *sql.DB
	dbSecret string
}

// New 打开(不存在则创建)数据库并初始化表
func New(dbPath, dbSecret string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_foreign_keys=1")
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, dbSecret: dbSecret}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS auth_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT NOT NULL,
    access_token TEXT NOT NULL,
    refresh_token TEXT,
    token_type TEXT,
    expires_in INTEGER,
    expires_at INTEGER,
    cookies TEXT,
    user_json TEXT,
    created_at INTEGER NOT NULL,
    UNIQUE(email)
);
CREATE INDEX IF NOT EXISTS idx_auth_sessions_email ON auth_sessions(email);

CREATE TABLE IF NOT EXISTS usage_logs (
    id INTEGER PRIMARY KEY,
    email TEXT NOT NULL,
    request_id TEXT,
    model TEXT,
    inbound_endpoint TEXT,
    group_id INTEGER,
    api_key_id INTEGER,
    account_id INTEGER,
    input_tokens INTEGER,
    output_tokens INTEGER,
    cache_read_tokens INTEGER,
    total_cost REAL,
    actual_cost REAL,
    rate_multiplier REAL,
    billing_mode TEXT,
    request_type TEXT,
    stream INTEGER,
    duration_ms INTEGER,
    first_token_ms INTEGER,
    user_agent TEXT,
    created_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_usage_logs_email_date ON usage_logs(email, created_at);
CREATE INDEX IF NOT EXISTS idx_usage_logs_model ON usage_logs(model);

CREATE TABLE IF NOT EXISTS balance_ledger (
    id INTEGER PRIMARY KEY,
    email TEXT NOT NULL,
    direction TEXT,
    amount TEXT,
    reason TEXT,
    ref_type TEXT,
    ref_id INTEGER,
    balance_after TEXT,
    metadata TEXT,
    created_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_balance_ledger_email_date ON balance_ledger(email, created_at);
CREATE INDEX IF NOT EXISTS idx_balance_ledger_reason ON balance_ledger(reason);

CREATE TABLE IF NOT EXISTS fetch_meta (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT NOT NULL,
    kind TEXT NOT NULL,
    params TEXT,
    item_count INTEGER,
    raw_file TEXT,
    fetched_at INTEGER NOT NULL
);

-- 增量同步水位：每账号每类数据已成功覆盖到的最大 created_at。
-- 用覆盖区间右端点（而不是最后一次运行时间）做水位，保证可断点续传：
-- 任何 < water_mark 的数据理论上已入库；>= water_mark 的下次会重新拉（幂等 upsert）。
CREATE TABLE IF NOT EXISTS sync_watermark (
    email TEXT NOT NULL,
    kind TEXT NOT NULL,          -- usage | ledger
    watermark TEXT NOT NULL,     -- RFC3339，已覆盖到的最大 created_at（含）
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (email, kind)
);

-- 同步任务历史/状态：一行一次手动或自动同步任务
CREATE TABLE IF NOT EXISTS sync_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    trigger_kind TEXT NOT NULL,  -- init | manual | auto
    status TEXT NOT NULL,        -- pending | running | done | failed | canceled
    plan_json TEXT,              -- SyncPlan 序列化
    progress_json TEXT,          -- 实时进度快照
    error TEXT,
    created_at INTEGER NOT NULL,
    started_at INTEGER,
    finished_at INTEGER
);
CREATE INDEX IF NOT EXISTS idx_sync_jobs_id ON sync_jobs(id DESC);

-- 托管账号额度窗口快照：/accounts/{id}/usage 的 five_hour/seven_day 窗口。
-- 每次同步快照覆盖写（保留最新一份）；7D 消耗=seven_day.window_stats.cost，利用率=utilization(%)。
CREATE TABLE IF NOT EXISTS account_windows (
    account_id INTEGER NOT NULL,
    email TEXT NOT NULL,            -- 归属登录账号（号主 email）
    name TEXT,                      -- 托管账号名称
    platform TEXT,
    five_hour_json TEXT,
    seven_day_json TEXT,
    sd_utilization REAL,            -- seven_day.utilization (%)
    sd_cost REAL,                   -- seven_day.window_stats.cost 账号成本
    sd_user_cost REAL,              -- seven_day.window_stats.user_cost 用户扣费口径
    sd_requests INTEGER,
    fetched_at INTEGER NOT NULL,
    PRIMARY KEY (account_id, email)
);
CREATE INDEX IF NOT EXISTS idx_account_windows_email ON account_windows(email);
`)
	return err
}

// SaveSession 保存(或更新)某邮箱的登录凭据；敏感字段加密存储
func (s *Store) SaveSession(email string, r *authResult) error {
	enc := func(v string) (string, error) {
		if v == "" {
			return v, nil
		}
		return crypto.Encrypt(v, s.dbSecret)
	}
	at, err := enc(r.AccessToken)
	if err != nil {
		return fmt.Errorf("encrypt access_token: %w", err)
	}
	rt, err := enc(r.RefreshToken)
	if err != nil {
		return fmt.Errorf("encrypt refresh_token: %w", err)
	}
	ck, err := enc(r.Cookies)
	if err != nil {
		return fmt.Errorf("encrypt cookies: %w", err)
	}
	expiresAt := time.Now().Add(time.Duration(r.ExpiresIn) * time.Second).Unix()
	_, err = s.db.Exec(`
INSERT INTO auth_sessions (email, access_token, refresh_token, token_type, expires_in, expires_at, cookies, user_json, created_at)
VALUES (?,?,?,?,?,?,?,?,?)
ON CONFLICT(email) DO UPDATE SET
    access_token=excluded.access_token,
    refresh_token=excluded.refresh_token,
    token_type=excluded.token_type,
    expires_in=excluded.expires_in,
    expires_at=excluded.expires_at,
    cookies=excluded.cookies,
    user_json=excluded.user_json,
    created_at=excluded.created_at
`, email, at, rt, r.TokenType, r.ExpiresIn, expiresAt, ck, string(r.User), time.Now().Unix())
	return err
}

// Session 是解密后的登录凭据
type Session struct {
	Email        string
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresAt    time.Time
	Cookies      string
	UserJSON     string
}

// GetSession 读取并解密某邮箱的最新凭据
func (s *Store) GetSession(email string) (*Session, error) {
	row := s.db.QueryRow(`
SELECT access_token, refresh_token, token_type, expires_at, cookies, user_json
FROM auth_sessions WHERE email=? ORDER BY created_at DESC LIMIT 1`, email)
	var at, rt, tt, ck, uj string
	var exp int64
	if err := row.Scan(&at, &rt, &tt, &exp, &ck, &uj); err != nil {
		return nil, err
	}
	dec := func(v string) (string, error) {
		if v == "" {
			return v, nil
		}
		return crypto.Decrypt(v, s.dbSecret)
	}
	var err error
	if at, err = dec(at); err != nil {
		return nil, err
	}
	if rt, err = dec(rt); err != nil {
		return nil, err
	}
	if ck, err = dec(ck); err != nil {
		return nil, err
	}
	return &Session{
		Email:        email,
		AccessToken:  at,
		RefreshToken: rt,
		TokenType:    tt,
		ExpiresAt:    time.Unix(exp, 0),
		Cookies:      ck,
		UserJSON:     uj,
	}, nil
}

// ListSessions 返回所有已保存的邮箱
func (s *Store) ListSessions() ([]string, error) {
	rows, err := s.db.Query(`SELECT email FROM auth_sessions ORDER BY created_at DESC`)
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

func (s *Store) Close() error { return s.db.Close() }

// DB 返回底层 *sql.DB 供只读查询
func (s *Store) DB() *sql.DB { return s.db }

// authResult 与 auth.LoginResult 字段镜像（避免循环依赖）
type authResult struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresIn    int
	Cookies      string
	User         []byte
}

// FromLoginResult 构造存储用的结果
func FromLoginResult(accessToken, refreshToken, tokenType string, expiresIn int, cookies string, user []byte) *authResult {
	return &authResult{AccessToken: accessToken, RefreshToken: refreshToken, TokenType: tokenType, ExpiresIn: expiresIn, Cookies: cookies, User: user}
}

// OpenReadonly 以只读模式打开数据库（不建表），供 web 分析界面使用，
// 保证前端任何代码路径都无法写入，符合"严禁增删改"约束。
func OpenReadonly(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath+"?mode=ro&_journal_mode=WAL")
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// AccountWindow 是一条账号额度窗快照（从 /accounts/{id}/usage 解析）
type AccountWindow struct {
	AccountID     int64   `json:"account_id"`
	Email         string  `json:"email"`
	Name          string  `json:"name"`
	Platform      string  `json:"platform"`
	FiveHourJSON  string  `json:"five_hour_json"`
	SevenDayJSON  string  `json:"seven_day_json"`
	SDUtilization float64 `json:"sd_utilization"`
	SDCost        float64 `json:"sd_cost"`
	SDUserCost    float64 `json:"sd_user_cost"`
	SDRequests    int64   `json:"sd_requests"`
	FetchedAt     int64   `json:"fetched_at"`
}

// SaveAccountWindow 覆盖写某账号最新额度窗快照
func (s *Store) SaveAccountWindow(w *AccountWindow) error {
	_, err := s.db.Exec(`
INSERT INTO account_windows (account_id, email, name, platform, five_hour_json, seven_day_json, sd_utilization, sd_cost, sd_user_cost, sd_requests, fetched_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(account_id, email) DO UPDATE SET
    name=excluded.name,
    platform=excluded.platform,
    five_hour_json=excluded.five_hour_json,
    seven_day_json=excluded.seven_day_json,
    sd_utilization=excluded.sd_utilization,
    sd_cost=excluded.sd_cost,
    sd_user_cost=excluded.sd_user_cost,
    sd_requests=excluded.sd_requests,
    fetched_at=excluded.fetched_at`,
		w.AccountID, w.Email, w.Name, w.Platform, w.FiveHourJSON, w.SevenDayJSON,
		w.SDUtilization, w.SDCost, w.SDUserCost, w.SDRequests, w.FetchedAt)
	return err
}

// ClearAccountWindows 清空某登录账号下全部托管账号快照
func (s *Store) ClearAccountWindows(email string) error {
	_, err := s.db.Exec(`DELETE FROM account_windows WHERE email=?`, email)
	return err
}

package store

import (
	"database/sql"
	"time"

	"ai-pixel-analysis/crypto"

	_ "modernc.org/sqlite"
)

// Store 封装 SQLite 存取
type Store struct {
	db       *sql.DB
	dbSecret string
}

func New(dbPath, dbSecret string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode%28WAL%29&_pragma=foreign_keys%281%29")
	if err != nil { return nil, err }
	s := &Store{db: db, dbSecret: dbSecret}
	if err := s.migrate(); err != nil { db.Close(); return nil, err }
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

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

CREATE TABLE IF NOT EXISTS accounts (
    id INTEGER PRIMARY KEY,
    email TEXT NOT NULL,
    name TEXT,
    platform TEXT,
    account_level TEXT,
    concurrency INTEGER,
    status TEXT,
    fetched_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_accounts_email ON accounts(email);

CREATE TABLE IF NOT EXISTS account_usage (
    account_id INTEGER NOT NULL,
    email TEXT NOT NULL,
    utilization REAL,
    cost REAL,
    standard_cost REAL,
    user_cost REAL,
    estimated_total_quota INTEGER,
    fetched_at INTEGER NOT NULL,
    PRIMARY KEY (account_id, email)
);

CREATE TABLE IF NOT EXISTS account_stats (
    account_id INTEGER NOT NULL,
    email TEXT NOT NULL,
    start_date TEXT,
    end_date TEXT,
    model TEXT,
    requests INTEGER,
    input_tokens INTEGER,
    output_tokens INTEGER,
    cache_creation_tokens INTEGER,
    cache_read_tokens INTEGER,
    total_tokens INTEGER,
    cost REAL,
    actual_cost REAL,
    account_cost REAL,
    fetched_at INTEGER NOT NULL,
    PRIMARY KEY (account_id, email, start_date, end_date, model)
);

CREATE TABLE IF NOT EXISTS balance_ledger (
    id INTEGER PRIMARY KEY,
    email TEXT NOT NULL,
    direction TEXT,
    amount TEXT,
    reason TEXT,
    ref_type TEXT,
    ref_id INTEGER,
    balance_after TEXT,
    consumer_user_id INTEGER,
    api_key_id INTEGER,
    account_id INTEGER,
    request_id TEXT,
    created_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_balance_ledger_email_date ON balance_ledger(email, created_at);

CREATE TABLE IF NOT EXISTS fetch_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT NOT NULL,
    kind TEXT NOT NULL,
    params TEXT,
    item_count INTEGER,
    raw_file TEXT,
    fetched_at INTEGER NOT NULL
);
`)
	return err
}

// AuthResult 是待加密的登录凭据
type AuthResult struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresIn    int
	Cookies      string
	User         []byte
}

// SaveSession 保存(或更新)某邮箱的登录凭据；敏感字段加密存储
func (s *Store) SaveSession(email string, r *AuthResult) error {
	enc := func(v string) (string, error) {
		if v == "" { return v, nil }
		return crypto.Encrypt(v, s.dbSecret)
	}
	at, _ := enc(r.AccessToken)
	rt, _ := enc(r.RefreshToken)
	ck, _ := enc(r.Cookies)
	expiresAt := time.Now().Add(time.Duration(r.ExpiresIn) * time.Second).Unix()
	_, err := s.db.Exec(`
INSERT INTO auth_sessions (email, access_token, refresh_token, token_type, expires_in, expires_at, cookies, user_json, created_at)
VALUES (?,?,?,?,?,?,?,?,?)
ON CONFLICT(email) DO UPDATE SET
    access_token=excluded.access_token, refresh_token=excluded.refresh_token,
    token_type=excluded.token_type, expires_in=excluded.expires_in,
    expires_at=excluded.expires_at, cookies=excluded.cookies,
    user_json=excluded.user_json, created_at=excluded.created_at
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

func (s *Store) GetSession(email string) (*Session, error) {
	row := s.db.QueryRow(`
SELECT access_token, refresh_token, token_type, expires_at, cookies, user_json
FROM auth_sessions WHERE email=? ORDER BY created_at DESC LIMIT 1`, email)
	var at, rt, tt, ck, uj string
	var exp int64
	if err := row.Scan(&at, &rt, &tt, &exp, &ck, &uj); err != nil { return nil, err }
	dec := func(v string) (string, error) {
		if v == "" { return v, nil }
		return crypto.Decrypt(v, s.dbSecret)
	}
	var err error
	if at, err = dec(at); err != nil { return nil, err }
	if rt, err = dec(rt); err != nil { return nil, err }
	if ck, err = dec(ck); err != nil { return nil, err }
	return &Session{Email: email, AccessToken: at, RefreshToken: rt, TokenType: tt,
		ExpiresAt: time.Unix(exp, 0), Cookies: ck, UserJSON: uj}, nil
}

func (s *Store) ListSessions() ([]string, error) {
	rows, err := s.db.Query(`SELECT email FROM auth_sessions ORDER BY email`)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []string
	for rows.Next() { var e string; rows.Scan(&e); out = append(out, e) }
	return out, rows.Err()
}

// SaveAccounts 覆盖式写入账号列表
func (s *Store) SaveAccounts(email string, items []struct {
	ID           int64
	Name         string
	Platform     string
	AccountLevel string
	Concurrency  int
	Status       string
}) error {
	tx, err := s.db.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM accounts WHERE email=?`, email); err != nil { return err }
	st, err := tx.Prepare(`INSERT INTO accounts (id,email,name,platform,account_level,concurrency,status,fetched_at) VALUES (?,?,?,?,?,?,?,?)`)
	if err != nil { return err }
	defer st.Close()
	now := time.Now().Unix()
	for _, it := range items {
		if _, err := st.Exec(it.ID, email, it.Name, it.Platform, it.AccountLevel, it.Concurrency, it.Status, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SaveAccountUsage 写入单个账号用量
func (s *Store) SaveAccountUsage(email string, accountID int64, utilization, cost, standardCost, userCost float64) error {
	est := int64(-999)
	if utilization != 0 {
		est = int64(cost/utilization + 0.5)
	}
	_, err := s.db.Exec(`
INSERT INTO account_usage (account_id,email,utilization,cost,standard_cost,user_cost,estimated_total_quota,fetched_at)
VALUES (?,?,?,?,?,?,?,?)
ON CONFLICT(account_id,email) DO UPDATE SET
    utilization=excluded.utilization, cost=excluded.cost, standard_cost=excluded.standard_cost,
    user_cost=excluded.user_cost, estimated_total_quota=excluded.estimated_total_quota, fetched_at=excluded.fetched_at
`, accountID, email, utilization, cost, standardCost, userCost, est, time.Now().Unix())
	return err
}

// SaveAccountStats 写入账号模型统计
func (s *Store) SaveAccountStats(email string, accountID int64, startDate, endDate string, models []struct {
	Model               string
	Requests            int64
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	TotalTokens         int64
	Cost                float64
	ActualCost          float64
	AccountCost         float64
}) error {
	tx, err := s.db.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	st, err := tx.Prepare(`
INSERT INTO account_stats (account_id,email,start_date,end_date,model,requests,input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens,total_tokens,cost,actual_cost,account_cost,fetched_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(account_id,email,start_date,end_date,model) DO UPDATE SET
    requests=excluded.requests, input_tokens=excluded.input_tokens, output_tokens=excluded.output_tokens,
    cache_creation_tokens=excluded.cache_creation_tokens, cache_read_tokens=excluded.cache_read_tokens,
    total_tokens=excluded.total_tokens, cost=excluded.cost, actual_cost=excluded.actual_cost,
    account_cost=excluded.account_cost, fetched_at=excluded.fetched_at
`)
	if err != nil { return err }
	defer st.Close()
	now := time.Now().Unix()
	for _, m := range models {
		if _, err := st.Exec(accountID, email, startDate, endDate, m.Model, m.Requests, m.InputTokens, m.OutputTokens,
			m.CacheCreationTokens, m.CacheReadTokens, m.TotalTokens, m.Cost, m.ActualCost, m.AccountCost, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SaveLedger 写入流水（含 metadata 清洗字段）
func (s *Store) SaveLedger(email string, items []struct {
	ID             int64
	Direction      string
	Amount         string
	Reason         string
	RefType        string
	RefID          int64
	BalanceAfter   string
	ConsumerUserID int64
	APIKeyID       int64
	AccountID      int64
	RequestID      string
	CreatedAt      string
}) error {
	tx, err := s.db.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	st, err := tx.Prepare(`
INSERT INTO balance_ledger (id,email,direction,amount,reason,ref_type,ref_id,balance_after,consumer_user_id,api_key_id,account_id,request_id,created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
    email=excluded.email, direction=excluded.direction, amount=excluded.amount, reason=excluded.reason,
    ref_type=excluded.ref_type, ref_id=excluded.ref_id, balance_after=excluded.balance_after,
    consumer_user_id=excluded.consumer_user_id, api_key_id=excluded.api_key_id,
    account_id=excluded.account_id, request_id=excluded.request_id, created_at=excluded.created_at
`)
	if err != nil { return err }
	defer st.Close()
	for _, it := range items {
		if _, err := st.Exec(it.ID, email, it.Direction, it.Amount, it.Reason, it.RefType, it.RefID, it.BalanceAfter,
			it.ConsumerUserID, it.APIKeyID, it.AccountID, it.RequestID, it.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LogFetch 记录一次抓取运行
func (s *Store) LogFetch(email, kind, params string, itemCount int, rawFile string) error {
	_, err := s.db.Exec(`INSERT INTO fetch_runs (email,kind,params,item_count,raw_file,fetched_at) VALUES (?,?,?,?,?,?)`,
		email, kind, params, itemCount, rawFile, time.Now().Unix())
	return err
}

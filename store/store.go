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

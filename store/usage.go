package store

import (
	"encoding/json"
	"fmt"
)

// UsageRecord 是清洗后入库存储的使用明细。
// 字段选取理由见 readme/docs：保留计费与分析必需字段，
// 丢弃每条重复的 user/api_key/group 大嵌套对象（账号信息已在 auth_sessions）。
type UsageRecord struct {
	ID              int64
	RequestID       string
	Model           string
	InboundEndpoint string
	GroupID         int64
	APIKeyID        int64
	AccountID       int64
	InputTokens     int64
	OutputTokens    int64
	CacheReadTokens int64
	TotalCost       float64 // 官方计费（美分定价换算前）
	ActualCost      float64 // 乘以倍率后实际扣费
	RateMultiplier  float64
	BillingMode     string
	RequestType     string
	Stream          bool
	DurationMs      int64
	FirstTokenMs    *int64
	UserAgent       string
	CreatedAt       string // 原始 RFC3339 字符串
}

// SaveUsageItems 把一页明细 upsert 进 usage_logs 表，email 用于区分账号
func (s *Store) SaveUsageItems(email string, items []json.RawMessage) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`
INSERT INTO usage_logs (
  id, email, request_id, model, inbound_endpoint, group_id, api_key_id, account_id,
  input_tokens, output_tokens, cache_read_tokens,
  total_cost, actual_cost, rate_multiplier,
  billing_mode, request_type, stream, duration_ms, first_token_ms, user_agent, created_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
  email=excluded.email,
  request_id=excluded.request_id,
  model=excluded.model,
  inbound_endpoint=excluded.inbound_endpoint,
  group_id=excluded.group_id,
  api_key_id=excluded.api_key_id,
  account_id=excluded.account_id,
  input_tokens=excluded.input_tokens,
  output_tokens=excluded.output_tokens,
  cache_read_tokens=excluded.cache_read_tokens,
  total_cost=excluded.total_cost,
  actual_cost=excluded.actual_cost,
  rate_multiplier=excluded.rate_multiplier,
  billing_mode=excluded.billing_mode,
  request_type=excluded.request_type,
  stream=excluded.stream,
  duration_ms=excluded.duration_ms,
  first_token_ms=excluded.first_token_ms,
  user_agent=excluded.user_agent,
  created_at=excluded.created_at
`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	n := 0
	for _, raw := range items {
		var u struct {
			ID              int64   `json:"id"`
			RequestID       string  `json:"request_id"`
			Model           string  `json:"model"`
			InboundEndpoint string  `json:"inbound_endpoint"`
			GroupID         int64   `json:"group_id"`
			APIKeyID        int64   `json:"api_key_id"`
			AccountID       int64   `json:"account_id"`
			InputTokens     int64   `json:"input_tokens"`
			OutputTokens    int64   `json:"output_tokens"`
			CacheReadTokens int64   `json:"cache_read_tokens"`
			TotalCost       float64 `json:"total_cost"`
			ActualCost      float64 `json:"actual_cost"`
			RateMultiplier  float64 `json:"rate_multiplier"`
			BillingMode     string  `json:"billing_mode"`
			RequestType     string  `json:"request_type"`
			Stream          bool    `json:"stream"`
			DurationMs      int64   `json:"duration_ms"`
			FirstTokenMs    *int64  `json:"first_token_ms"`
			UserAgent       string  `json:"user_agent"`
			CreatedAt       string  `json:"created_at"`
		}
		if err := json.Unmarshal(raw, &u); err != nil {
			return n, fmt.Errorf("decode usage item: %w", err)
		}
		if _, err := stmt.Exec(
			u.ID, email, u.RequestID, u.Model, u.InboundEndpoint, u.GroupID, u.APIKeyID, u.AccountID,
			u.InputTokens, u.OutputTokens, u.CacheReadTokens,
			u.TotalCost, u.ActualCost, u.RateMultiplier,
			u.BillingMode, u.RequestType, u.Stream, u.DurationMs, u.FirstTokenMs, u.UserAgent, u.CreatedAt,
		); err != nil {
			return n, err
		}
		n++
	}
	return n, tx.Commit()
}

// SaveLedgerItems 把一页余额流水 upsert 进 balance_ledger 表
func (s *Store) SaveLedgerItems(email string, items []json.RawMessage) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`
INSERT INTO balance_ledger (
  id, email, direction, amount, reason, ref_type, ref_id, balance_after, metadata, created_at
) VALUES (?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
  email=excluded.email,
  direction=excluded.direction,
  amount=excluded.amount,
  reason=excluded.reason,
  ref_type=excluded.ref_type,
  ref_id=excluded.ref_id,
  balance_after=excluded.balance_after,
  metadata=excluded.metadata,
  created_at=excluded.created_at
`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	n := 0
	for _, raw := range items {
		var l struct {
			ID           int64           `json:"id"`
			Direction    string          `json:"direction"`
			Amount       string          `json:"amount"`
			Reason       string          `json:"reason"`
			RefType      string          `json:"ref_type"`
			RefID        int64           `json:"ref_id"`
			BalanceAfter string          `json:"balance_after"`
			Metadata     json.RawMessage `json:"metadata"`
			CreatedAt    string          `json:"created_at"`
		}
		if err := json.Unmarshal(raw, &l); err != nil {
			return n, fmt.Errorf("decode ledger item: %w", err)
		}
		if _, err := stmt.Exec(
			l.ID, email, l.Direction, l.Amount, l.Reason, l.RefType, l.RefID, l.BalanceAfter,
			string(l.Metadata), l.CreatedAt,
		); err != nil {
			return n, err
		}
		n++
	}
	return n, tx.Commit()
}

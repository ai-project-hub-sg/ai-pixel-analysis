package api

import "encoding/json"

// ---- 1. /accounts 账号列表 ----

type AccountItem struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Platform     string `json:"platform"`
	AccountLevel string `json:"account_level"`
	Status       string `json:"status"`
	Concurrency  int    `json:"concurrency"`
}

type AccountList struct {
	Items []AccountItem `json:"items"`
	Total int64         `json:"total"`
	Page  int           `json:"page"`
	Pages int           `json:"pages"`
}

// ---- 2. /accounts/{id}/usage 用量 ----

type WindowStats struct {
	Requests     int64   `json:"requests"`
	Tokens       int64   `json:"tokens"`
	Cost         float64 `json:"cost"`
	StandardCost float64 `json:"standard_cost"`
	UserCost     float64 `json:"user_cost"`
}

type UsageWindow struct {
	Utilization      float64      `json:"utilization"`
	ResetsAt         string       `json:"resets_at"`
	WindowStart      string       `json:"window_start"`
	StatsComplete    bool         `json:"stats_complete"`
	RemainingSeconds float64      `json:"remaining_seconds"`
	WindowStats      *WindowStats `json:"window_stats"`
}

type AccountUsage struct {
	Source    string       `json:"source"`
	UpdatedAt string       `json:"updated_at"`
	FiveHour  *UsageWindow `json:"five_hour"`
	SevenDay  *UsageWindow `json:"seven_day"`
}

// ---- 3. /accounts/{id}/stats 状态/模型统计 ----

type ModelStat struct {
	Model               string  `json:"model"`
	Requests            int64   `json:"requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	TotalTokens         int64   `json:"total_tokens"`
	Cost                float64 `json:"cost"`
	ActualCost          float64 `json:"actual_cost"`
	AccountCost         float64 `json:"account_cost"`
}

type AccountStats struct {
	AccountID int64        `json:"account_id"`
	StartDate string       `json:"start_date"`
	EndDate   string       `json:"end_date"`
	Models    []ModelStat  `json:"models"`
	// 透传其它顶层字段，原始 JSON 由调用方另行落盘
	Raw map[string]any `json:"-"`
}

// ---- 4. /usage/balance-ledger 流水 ----

type LedgerItem struct {
	ID           int64           `json:"id"`
	UserID       int64           `json:"user_id"`
	Direction    string          `json:"direction"`
	Amount       string          `json:"amount"`
	Reason       string          `json:"reason"`
	RefType      string          `json:"ref_type"`
	RefID        int64           `json:"ref_id"`
	BalanceAfter string          `json:"balance_after"`
	Metadata     json.RawMessage `json:"metadata"`
	CreatedAt    string          `json:"created_at"`
}

type LedgerList struct {
	Items    []LedgerItem `json:"items"`
	Total    int64        `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
	Pages    int          `json:"pages"`
}

// LedgerMeta 是从 metadata 清洗出的关键字段
type LedgerMeta struct {
	ConsumerUserID int64  `json:"consumer_user_id"`
	APIKeyID       int64  `json:"api_key_id"`
	AccountID      int64  `json:"account_id"`
	RequestID      string `json:"request_id"`
}

package api

import "encoding/json"

// ---- /usage 使用明细 ----

// UsageItem 是使用明细列表的一条记录。
type UsageItem struct {
	ID                   int64   `json:"id"`
	UserID               int64   `json:"user_id"`
	APIKeyID             int64   `json:"api_key_id"`
	AccountID            int64   `json:"account_id"`
	RequestID            string  `json:"request_id"`
	Model                string  `json:"model"`
	InboundEndpoint      string  `json:"inbound_endpoint"`
	UpstreamEndpoint     string  `json:"upstream_endpoint"`
	GroupID              int64   `json:"group_id"`
	InputTokens          int64   `json:"input_tokens"`
	OutputTokens         int64   `json:"output_tokens"`
	CacheCreationTokens  int64   `json:"cache_creation_tokens"`
	CacheReadTokens      int64   `json:"cache_read_tokens"`
	ImageInputTokens     int64   `json:"image_input_tokens"`
	InputCost            float64 `json:"input_cost"`
	OutputCost           float64 `json:"output_cost"`
	CacheCreationCost    float64 `json:"cache_creation_cost"`
	CacheReadCost        float64 `json:"cache_read_cost"`
	TotalCost            float64 `json:"total_cost"`
	ActualCost           float64 `json:"actual_cost"`
	RateMultiplier       float64 `json:"rate_multiplier"`
	RateMultiplierSource string  `json:"rate_multiplier_source"`
	PointsDeducted       float64 `json:"points_deducted"`
	BalanceDeducted      float64 `json:"balance_deducted"`
	BillingWalletType    string  `json:"billing_wallet_type"`
	RequestType          string  `json:"request_type"`
	Stream               bool    `json:"stream"`
	DurationMs           int64   `json:"duration_ms"`
	FirstTokenMs         *int64  `json:"first_token_ms"`
	ImageCount           int64   `json:"image_count"`
	UserAgent            string  `json:"user_agent"`
	BillingMode          string  `json:"billing_mode"`
	CreatedAt            string  `json:"created_at"`
	// 清洗时从嵌套对象提取的冗余字段，便于分析
	APIKeyName string `json:"-"`
	GroupName  string `json:"-"`
}

// UsageList 是分页结果
type UsageList struct {
	Items    []json.RawMessage `json:"items"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
	Pages    int               `json:"pages"`
}

// ---- /usage/stats 汇总 ----

type UsageStats struct {
	TotalRequests            int64   `json:"total_requests"`
	TotalInputTokens         int64   `json:"total_input_tokens"`
	TotalOutputTokens        int64   `json:"total_output_tokens"`
	TotalCacheTokens         int64   `json:"total_cache_tokens"`
	TotalCacheCreationTokens int64   `json:"total_cache_creation_tokens"`
	TotalCacheReadTokens     int64   `json:"total_cache_read_tokens"`
	TotalTokens              int64   `json:"total_tokens"`
	TotalCost                float64 `json:"total_cost"`
	TotalActualCost          float64 `json:"total_actual_cost"`
	TotalRequestActualCost   float64 `json:"total_request_actual_cost"`
	TotalHourlyCost          float64 `json:"total_hourly_cost"`
	AverageDurationMs        float64 `json:"average_duration_ms"`
}

// ---- /usage/balance-ledger 余额流水 ----

// 金额为字符串十进制数，避免浮点精度损失
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

type LedgerStats struct {
	TotalEntries  int64  `json:"total_entries"`
	CreditEntries int64  `json:"credit_entries"`
	DebitEntries  int64  `json:"debit_entries"`
	CreditAmount  string `json:"credit_amount"`
	DebitAmount   string `json:"debit_amount"`
	NetAmount     string `json:"net_amount"`
}

// ---- /usage/dashboard/* 仪表盘 ----

type DashboardStats struct {
	TotalAPIKeys             int64   `json:"total_api_keys"`
	ActiveAPIKeys            int64   `json:"active_api_keys"`
	TotalRequests            int64   `json:"total_requests"`
	TotalInputTokens         int64   `json:"total_input_tokens"`
	TotalOutputTokens        int64   `json:"total_output_tokens"`
	TotalCacheCreationTokens int64   `json:"total_cache_creation_tokens"`
	TotalCacheReadTokens     int64   `json:"total_cache_read_tokens"`
	TotalTokens              int64   `json:"total_tokens"`
	TotalRequestActualCost   float64 `json:"total_request_actual_cost"`
	TotalHourlyCost          float64 `json:"total_hourly_cost"`
	TotalCost                float64 `json:"total_cost"`
	TotalActualCost          float64 `json:"total_actual_cost"`
	TodayRequests            int64   `json:"today_requests"`
	TodayInputTokens         int64   `json:"today_input_tokens"`
	TodayOutputTokens        int64   `json:"today_output_tokens"`
	TodayCacheCreationTokens int64   `json:"today_cache_creation_tokens"`
	TodayCacheReadTokens     int64   `json:"today_cache_read_tokens"`
	TodayTokens              int64   `json:"today_tokens"`
	TodayRequestActualCost   float64 `json:"today_request_actual_cost"`
	TodayHourlyCost          float64 `json:"today_hourly_cost"`
	TodayCost                float64 `json:"today_cost"`
	TodayActualCost          float64 `json:"today_actual_cost"`
	AverageDurationMs        float64 `json:"average_duration_ms"`
	RPM                      int64   `json:"rpm"`
	TPM                      int64   `json:"tpm"`
}

type TrendPoint struct {
	Date                string  `json:"date"`
	Requests            int64   `json:"requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	TotalTokens         int64   `json:"total_tokens"`
	Cost                float64 `json:"cost"`
	ActualCost          float64 `json:"actual_cost"`
}

type DashboardTrend struct {
	StartDate   string       `json:"start_date"`
	EndDate     string       `json:"end_date"`
	Granularity string       `json:"granularity"`
	Trend       []TrendPoint `json:"trend"`
}

type ModelUsage struct {
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

type DashboardModels struct {
	StartDate string       `json:"start_date"`
	EndDate   string       `json:"end_date"`
	Models    []ModelUsage `json:"models"`
}

// ---- /accounts 托管账号（号主名下的上游账号） ----

// AccountItem 是 /accounts 列表项（取分析所需字段）
type AccountItem struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Platform     string `json:"platform"`
	AccountLevel string `json:"account_level"`
	Status       string `json:"status"`
}

type AccountList struct {
	Items []AccountItem `json:"items"`
	Total int64         `json:"total"`
	Page  int           `json:"page"`
	Pages int           `json:"pages"`
}

// ---- /accounts/{id}/usage 额度窗口 ----

// WindowStats 是窗口内用量统计
type WindowStats struct {
	Requests     int64   `json:"requests"`
	Tokens       int64   `json:"tokens"`
	Cost         float64 `json:"cost"` // 账号成本（7D 账号消耗）
	StandardCost float64 `json:"standard_cost"`
	UserCost     float64 `json:"user_cost"` // 用户扣费口径
}

// UsageWindow 是一个额度窗（five_hour 或 seven_day）
type UsageWindow struct {
	Utilization      float64      `json:"utilization"` // 已用百分比
	ResetsAt         string       `json:"resets_at"`
	WindowStart      string       `json:"window_start"`
	StatsComplete    bool         `json:"stats_complete"`
	RemainingSeconds float64      `json:"remaining_seconds"`
	WindowStats      *WindowStats `json:"window_stats"`
}

// AccountUsage 是 /accounts/{id}/usage 的返回
type AccountUsage struct {
	Source    string       `json:"source"`
	UpdatedAt string       `json:"updated_at"`
	FiveHour  *UsageWindow `json:"five_hour"`
	SevenDay  *UsageWindow `json:"seven_day"`
}

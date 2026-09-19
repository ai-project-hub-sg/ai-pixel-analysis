package api

import (
	"encoding/json"
	"net/url"
)

// RawResult 保存一次请求的原始响应（完整 envelope）供落盘
type RawResult struct {
	URL  string
	Body json.RawMessage
}

// ListUsage 请求一页使用明细，返回解码结果与原始响应
func (c *Client) ListUsage(q UsageQuery) (*UsageList, RawResult, error) {
	var out UsageList
	raw, err := c.get("/usage", q.values(), &out)
	return &out, RawResult{Body: raw}, err
}

// GetUsageStats 请求使用汇总。period 形如 today/yesterday/last7days；
// 传空则用 q 中的 start_date/end_date/start_time/end_time。
func (c *Client) GetUsageStats(period string, q UsageQuery) (*UsageStats, RawResult, error) {
	v := q.values()
	if period != "" {
		v.Set("period", period)
	}
	var out UsageStats
	raw, err := c.get("/usage/stats", v, &out)
	return &out, RawResult{Body: raw}, err
}

// ListBalanceLedger 请求一页余额流水
func (c *Client) ListBalanceLedger(q UsageQuery) (*LedgerList, RawResult, error) {
	var out LedgerList
	raw, err := c.get("/usage/balance-ledger", q.values(), &out)
	return &out, RawResult{Body: raw}, err
}

// GetBalanceLedgerStats 请求流水汇总（同过滤参数，去掉分页/排序）
func (c *Client) GetBalanceLedgerStats(q UsageQuery) (*LedgerStats, RawResult, error) {
	v := q.values()
	v.Del("page")
	v.Del("page_size")
	v.Del("sort_by")
	v.Del("sort_order")
	var out LedgerStats
	raw, err := c.get("/usage/balance-ledger/stats", v, &out)
	return &out, RawResult{Body: raw}, err
}

// GetDashboardStats 仪表盘汇总（额度窗口）
func (c *Client) GetDashboardStats() (*DashboardStats, RawResult, error) {
	var out DashboardStats
	raw, err := c.get("/usage/dashboard/stats", nil, &out)
	return &out, RawResult{Body: raw}, err
}

// GetDashboardTrend 仪表盘按天趋势
func (c *Client) GetDashboardTrend(params url.Values) (*DashboardTrend, RawResult, error) {
	var out DashboardTrend
	raw, err := c.get("/usage/dashboard/trend", params, &out)
	return &out, RawResult{Body: raw}, err
}

// GetDashboardModels 仪表盘模型拆分
func (c *Client) GetDashboardModels(params url.Values) (*DashboardModels, RawResult, error) {
	var out DashboardModels
	raw, err := c.get("/usage/dashboard/models", params, &out)
	return &out, RawResult{Body: raw}, err
}

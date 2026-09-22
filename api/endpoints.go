package api

import (
	"fmt"
	"net/url"
)

// RawResult 保存一次请求的原始响应（完整 envelope）供落盘
type RawResult struct {
	URL  string
	Body []byte
}

func raw(u string, b []byte) RawResult { return RawResult{URL: u, Body: b} }

// ---- 1. 账号列表 ----
// GET /api/v1/accounts?page=&page_size=&sort_by=&sort_order=&timezone=
func (c *Client) ListAccounts(q url.Values) (*AccountList, RawResult, error) {
	var out AccountList
	b, err := c.get("/accounts", q, &out)
	return &out, raw(c.host+"/api/v1/accounts?"+q.Encode(), b), err
}

// DefaultAccountsQuery 返回需求指定的默认查询参数
func DefaultAccountsQuery() url.Values {
	v := url.Values{}
	v.Set("page", "1")
	v.Set("page_size", "20")
	v.Set("sort_by", "created_at")
	v.Set("sort_order", "desc")
	v.Set("timezone", "Asia/Shanghai")
	return v
}

// ---- 2. 账号用量 ----
// GET /api/v1/accounts/{account_id}/usage?source=local&timezone=Asia/Shanghai
func (c *Client) GetAccountUsage(accountID int64, source, timezone string) (*AccountUsage, RawResult, error) {
	v := url.Values{}
	if source != "" { v.Set("source", source) }
	if timezone != "" { v.Set("timezone", timezone) }
	var out AccountUsage
	path := fmt.Sprintf("/accounts/%d/usage", accountID)
	b, err := c.get(path, v, &out)
	return &out, raw(c.host+"/api/v1"+path+"?"+v.Encode(), b), err
}

// ---- 3. 账号状态/模型统计 ----
// GET /api/v1/accounts/{account_id}/stats?start_date=&end_date=&timezone=
func (c *Client) GetAccountStats(accountID int64, startDate, endDate, timezone string) (*AccountStats, RawResult, error) {
	v := url.Values{}
	if startDate != "" { v.Set("start_date", startDate) }
	if endDate != "" { v.Set("end_date", endDate) }
	if timezone != "" { v.Set("timezone", timezone) }
	var out AccountStats
	path := fmt.Sprintf("/accounts/%d/stats", accountID)
	b, err := c.get(path, v, &out)
	return &out, raw(c.host+"/api/v1"+path+"?"+v.Encode(), b), err
}

// ---- 4. 余额流水 ----
// GET /api/v1/usage/balance-ledger?page=&page_size=&direction=&start_date=&end_date=&start_time=&end_time=&timezone=&sort_order=
func (c *Client) ListBalanceLedger(q url.Values) (*LedgerList, RawResult, error) {
	var out LedgerList
	b, err := c.get("/usage/balance-ledger", q, &out)
	return &out, raw(c.host+"/api/v1/usage/balance-ledger?"+q.Encode(), b), err
}

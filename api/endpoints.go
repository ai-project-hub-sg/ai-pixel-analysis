package api

import (
	"fmt"
	"net/url"
	"strings"
)

// RawResult 保存一次请求的原始响应（完整 envelope）供落盘
type RawResult struct {
	URL  string
	Body []byte
}

func raw(u string, b []byte) RawResult { return RawResult{URL: u, Body: b} }

// ---- 账号列表 ----
func (c *Client) ListAccounts(q url.Values) (*AccountList, RawResult, error) {
	p, err := c.ep("accounts")
	if err != nil { return nil, RawResult{}, err }
	var out AccountList
	b, u, err := c.get(p, q, &out)
	return &out, raw(u, b), err
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

// ---- 账号用量 ----
func (c *Client) GetAccountUsage(accountID int64, source, timezone string) (*AccountUsage, RawResult, error) {
	p, err := c.ep("usage")
	if err != nil { return nil, RawResult{}, err }
	p = strings.ReplaceAll(p, "{id}", fmt.Sprint(accountID))
	v := url.Values{}
	if source != "" { v.Set("source", source) }
	if timezone != "" { v.Set("timezone", timezone) }
	var out AccountUsage
	b, u, err := c.get(p, v, &out)
	return &out, raw(u, b), err
}

// ---- 账号状态/模型统计 ----
func (c *Client) GetAccountStats(accountID int64, startDate, endDate, timezone string) (*AccountStats, RawResult, error) {
	p, err := c.ep("stats")
	if err != nil { return nil, RawResult{}, err }
	p = strings.ReplaceAll(p, "{id}", fmt.Sprint(accountID))
	v := url.Values{}
	if startDate != "" { v.Set("start_date", startDate) }
	if endDate != "" { v.Set("end_date", endDate) }
	if timezone != "" { v.Set("timezone", timezone) }
	var out AccountStats
	b, u, err := c.get(p, v, &out)
	return &out, raw(u, b), err
}

// ---- 余额流水 ----
func (c *Client) ListBalanceLedger(q url.Values) (*LedgerList, RawResult, error) {
	p, err := c.ep("ledger")
	if err != nil { return nil, RawResult{}, err }
	var out LedgerList
	b, u, err := c.get(p, q, &out)
	return &out, raw(u, b), err
}

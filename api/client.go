// Package api 封装 ai-pixel 平台的只读数据接口。
// 所有方法仅发起 GET 请求，不做任何增删改操作。
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// UsageQuery 是 /usage、/usage/balance-ledger 等接口通用的查询参数。
// start_time/end_time 为精确时间过滤（RFC3339），start_date/end_date 按自然日过滤，全部可选。
type UsageQuery struct {
	Page      int    // 页码，从 1 开始
	PageSize  int    // 每页条数
	APIKeyID  int64  // 按 API Key 过滤，0 表示不过滤
	StartDate string // YYYY-MM-DD
	EndDate   string // YYYY-MM-DD
	StartTime string // RFC3339，如 2026-09-19T00:00:00+08:00
	EndTime   string
	Timezone  string // 如 Asia/Shanghai
	Direction string // 余额流水方向: credit | debit
	Reason    string // 流水原因: usage_charge | recharge | ...
	RefType   string // 关联类型: usage_log | ...
	RefID     int64
	SortBy    string // created_at 等
	SortOrder string // asc | desc
}

func (q UsageQuery) values() url.Values {
	v := url.Values{}
	set := func(k, s string) {
		if s != "" {
			v.Set(k, s)
		}
	}
	if q.Page > 0 {
		v.Set("page", fmt.Sprint(q.Page))
	}
	if q.PageSize > 0 {
		v.Set("page_size", fmt.Sprint(q.PageSize))
	}
	if q.APIKeyID > 0 {
		v.Set("api_key_id", fmt.Sprint(q.APIKeyID))
	}
	set("start_date", q.StartDate)
	set("end_date", q.EndDate)
	set("start_time", q.StartTime)
	set("end_time", q.EndTime)
	set("timezone", q.Timezone)
	set("direction", q.Direction)
	set("reason", q.Reason)
	set("ref_type", q.RefType)
	if q.RefID > 0 {
		v.Set("ref_id", fmt.Sprint(q.RefID))
	}
	set("sort_by", q.SortBy)
	set("sort_order", q.SortOrder)
	return v
}

// Client 是只读数据客户端
type Client struct {
	hc    *http.Client
	host  string
	token string // 完整 Authorization 头值，如 "Bearer xxx"
}

// NewClient 用已登录会话构造客户端
func NewClient(host, tokenType, accessToken string) (*Client, error) {
	if accessToken == "" {
		return nil, fmt.Errorf("access token is empty")
	}
	tt := tokenType
	if tt == "" {
		tt = "Bearer"
	}
	return &Client{
		hc:    &http.Client{Timeout: 60 * time.Second},
		host:  strings.TrimRight(host, "/"),
		token: tt + " " + accessToken,
	}, nil
}

// ErrUnauthorized 表示登录态失效（401/认证错误码）。调用方应识别它并提示重新登录。
var ErrUnauthorized = errors.New("session unauthorized: login required")

// envelope 是统一响应壳
type envelope struct {
	Code    json.RawMessage `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// get 发起一次 GET 并解码 data 字段到 out；返回完整响应原文用于落盘
func (c *Client) get(path string, params url.Values, out interface{}) (json.RawMessage, error) {
	u := c.host + "/api/v1" + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", c.token)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", u, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return raw, ErrUnauthorized
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("GET %s: not json (status %d): %.200s", u, resp.StatusCode, raw)
	}
	{
		var codeAny any
		json.Unmarshal(env.Code, &codeAny)
		isZero := codeAny == nil || codeAny == float64(0) || codeAny == "0"
		if !isZero {
			var msg string
			json.Unmarshal(env.Code, &msg)
			var codeNum int
			json.Unmarshal(env.Code, &codeNum)
			if codeNum == 401 || msg == "UNAUTHORIZED" || msg == "Authorization header is required" {
				return raw, ErrUnauthorized
			}
			return raw, fmt.Errorf("GET %s: code=%s msg=%s", u, string(env.Code), env.Message)
		}
	}
	if out != nil {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return raw, fmt.Errorf("GET %s: decode data: %w", u, err)
		}
	}
	return raw, nil
}

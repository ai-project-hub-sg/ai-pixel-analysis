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

// Client 是只读数据客户端
type Client struct {
	hc    *http.Client
	host  string
	token string
}

// NewClient 用已登录会话构造客户端
func NewClient(host, tokenType, accessToken string, timeoutMs int) (*Client, error) {
	if accessToken == "" {
		return nil, fmt.Errorf("access token is empty")
	}
	tt := tokenType
	if tt == "" { tt = "Bearer" }
	if timeoutMs <= 0 { timeoutMs = 60000 }
	return &Client{
		hc:    &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond},
		host:  strings.TrimRight(host, "/"),
		token: tt + " " + accessToken,
	}, nil
}

// ErrUnauthorized 表示登录态失效
var ErrUnauthorized = errors.New("session unauthorized: login required")

type envelope struct {
	Code    json.RawMessage `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// get 发起一次 GET 并解码 data 到 out；返回完整响应原文用于落盘
func (c *Client) get(path string, params url.Values, out interface{}) (json.RawMessage, error) {
	u := c.host + "/api/v1" + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil { return nil, err }
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", c.token)
	resp, err := c.hc.Do(req)
	if err != nil { return nil, fmt.Errorf("GET %s: %w", u, err) }
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil { return nil, err }
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

package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"regexp"
	"strings"
	"time"
)

// LoginResult 保存登录成功后的凭据
type LoginResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	TokenType    string
	User         json.RawMessage
	Cookies      string // 序列化的 cookie 字符串，供后续请求直接使用
}

// Client 封装登录相关 HTTP 操作
type Client struct {
	hc   *http.Client
	host string
}

func NewClient(host string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &Client{
		hc: &http.Client{
			Jar:     jar,
			Timeout: 30 * time.Second,
		},
		host: strings.TrimRight(host, "/"),
	}, nil
}

var appConfigRe = regexp.MustCompile(`window.__APP_CONFIG__=({.*?});?s*</script>`)

// FetchAgreementRevision 从登录页 HTML 中提取 login_agreement_revision
func (c *Client) FetchAgreementRevision(loginPath string) (string, error) {
	url := c.host + loginPath
	resp, err := c.hc.Get(url)
	if err != nil {
		return "", fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	m := appConfigRe.FindSubmatch(body)
	if m == nil {
		return "", fmt.Errorf("APP_CONFIG not found in login page")
	}
	var cfg struct {
		LoginAgreementEnabled  bool   `json:"login_agreement_enabled"`
		LoginAgreementRevision string `json:"login_agreement_revision"`
	}
	if err := json.Unmarshal(m[1], &cfg); err != nil {
		return "", fmt.Errorf("parse APP_CONFIG: %w", err)
	}
	if !cfg.LoginAgreementEnabled {
		return "", nil // 未启用协议，无需 revision
	}
	if cfg.LoginAgreementRevision == "" {
		return "", fmt.Errorf("login_agreement_revision empty in config")
	}
	return cfg.LoginAgreementRevision, nil
}

type loginRequest struct {
	Email                  string `json:"email"`
	Password               string `json:"password"`
	LoginAgreementRevision string `json:"login_agreement_revision,omitempty"`
}

type apiResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type loginData struct {
	AccessToken  string          `json:"access_token"`
	RefreshToken string          `json:"refresh_token"`
	ExpiresIn    int             `json:"expires_in"`
	TokenType    string          `json:"token_type"`
	User         json.RawMessage `json:"user"`
	TempToken    string          `json:"temp_token"`
}

// Login 执行登录，返回 token 与 cookie
func (c *Client) Login(email, password, revision string) (*LoginResult, error) {
	body := loginRequest{Email: email, Password: password}
	if revision != "" {
		body.LoginAgreementRevision = revision
	}
	payload, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", c.host+"/api/v1/auth/login", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST /api/v1/auth/login: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var ar apiResponse
	if err := json.Unmarshal(raw, &ar); err != nil {
		return nil, fmt.Errorf("login response not json: %s", truncate(string(raw), 300))
	}
	if ar.Code != 0 {
		return nil, fmt.Errorf("login failed code=%d msg=%s", ar.Code, ar.Message)
	}
	var d loginData
	if err := json.Unmarshal(ar.Data, &d); err != nil {
		return nil, fmt.Errorf("parse login data: %w", err)
	}
	if d.TempToken != "" {
		return nil, fmt.Errorf("2FA required (temp_token returned); not supported yet")
	}
	if d.AccessToken == "" {
		return nil, fmt.Errorf("no access_token in response")
	}

	// 收集 cookie
	var cookieParts []string
	if c.hc.Jar != nil {
		u, _ := req.URL.Parse(c.host)
		for _, ck := range c.hc.Jar.Cookies(u) {
			cookieParts = append(cookieParts, ck.Name+"="+ck.Value)
		}
	}
	// 也从响应头补一次 Set-Cookie（保险）
	for _, sc := range resp.Cookies() {
		s := sc.Name + "=" + sc.Value
		found := false
		for _, p := range cookieParts {
			if strings.HasPrefix(p, sc.Name+"=") {
				found = true
				break
			}
		}
		if !found {
			cookieParts = append(cookieParts, s)
		}
	}

	return &LoginResult{
		AccessToken:  d.AccessToken,
		RefreshToken: d.RefreshToken,
		ExpiresIn:    d.ExpiresIn,
		TokenType:    d.TokenType,
		User:         d.User,
		Cookies:      strings.Join(cookieParts, "; "),
	}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

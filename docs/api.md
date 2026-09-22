# 数据抽取接口开发文档

本文档面向后续开发，说明 version2 各数据抽取封装的 **函数签名、入参、返回结构、典型调用示例**，以及 **内部如何处理上游原始响应**（落盘、清洗、入库）。CLI 命令只是这些封装的一层薄壳。

## 0. 分层与职责

| 层 | 包 | 职责 |
|---|---|---|
| 配置 | `config` | 读 `.env`（机密）+ `config.toml`（各接口地址），产出 `Config` |
| 登录 | `auth` | `/login` 页面取 revision，`/api/v1/auth/login` 换 token |
| 数据接口 | `api` | 5 个只读 GET 封装，每个返回「结构体 + 原始响应」 |
| 编排 | `pipeline` | 调 `api` 拿数据，写原始 JSON 到 `data/raw/`，清洗后调 `store` 入库 |
| 存储 | `store` | SQLite 建表/加解密/upsert |
| 入口 | `main.go` | 组装以上各层，暴露子命令 |

## 1. 端点配置（config.toml）

所有接口的 host 与 port **都显式配置**，代码不内嵌任何主机名或端口。

```toml
[defaults]
timeout_ms = 30000

[endpoint.login]     # GET /login 与 POST /api/v1/auth/login
host = "https://ai-pixel.online"
port = 443

[endpoint.accounts]  # GET /api/v1/accounts
host = "https://ai-pixel.online"
port = 443

[endpoint.usage]     # GET /api/v1/accounts/{id}/usage
host = "https://ai-pixel.online"
port = 443

[endpoint.stats]     # GET /api/v1/accounts/{id}/stats
host = "https://ai-pixel.online"
port = 443

[endpoint.ledger]    # GET /api/v1/usage/balance-ledger
host = "https://ai-pixel.online"
port = 443
```

取用端点：

```go
cfg, err := config.Load(".env", "config.toml")
ep, err := cfg.Endpoint("accounts") // Endpoint{Host, Port, TimeoutMs}
base := ep.BaseURL()                // 标准端口(443/80/0)原样返回 host；非标准拼 host:port
```

## 2. 登录封装（auth 包）

### FetchAgreementRevision

```go
func (c *auth.Client) FetchAgreementRevision(loginPath string) (string, error)
```

- 入参：登录页路径，通常 `"/login"`
- 内部：`GET ep.BaseURL()+loginPath`，用正则 `window.__APP_CONFIG__=({...})` 提取 JSON，读 `login_agreement_revision`；若 `login_agreement_enabled=false` 返回空串
- 返回：revision 字符串；未启用协议时为空串

### Login

```go
func (c *auth.Client) Login(email, password, revision string) (*auth.LoginResult, error)

type LoginResult struct {
    AccessToken  string
    RefreshToken string
    ExpiresIn    int
    TokenType    string
    User         json.RawMessage
    Cookies      string  // "k1=v1; k2=v2"
}
```

- 内部：`POST {base}/api/v1/auth/login`，body `{"email","password","login_agreement_revision"}`
- 解析 envelope `{code,message,data}`，`code!=0` 报错；`temp_token` 出现说明要 2FA，直接报错
- 返回 access_token 与 jar 中收集的 cookie

### 典型用法

```go
ep, _ := cfg.Endpoint("login")
cli, _ := auth.NewClient(ep.BaseURL(), ep.TimeoutMs)
rev, _ := cli.FetchAgreementRevision("/login")
res, err := cli.Login(user.Name, user.Password, rev)
```

## 3. 数据接口封装（api 包）

所有方法签名统一为：

```go
func (c *api.Client) Xxx(...) (*T, api.RawResult, error)

type RawResult struct {
    URL  string  // 实际请求的完整 URL（含 query）
    Body []byte  // 完整响应原文（envelope JSON），供落盘
}
```

约定：**结构化结果** 与 **原始响应** 同时返回。上层把 `RawResult.Body` 原样写入 `data/raw/`，再把结构体字段清洗入库。

构造客户端：

```go
ep, _ := cfg.Endpoint("accounts")
cli, _ := api.NewClient(ep.BaseURL(), sess.TokenType, sess.AccessToken, ep.TimeoutMs)
// Authorization 头自动设为 "<TokenType> <AccessToken>"，默认 Bearer
```

错误：`api.ErrUnauthorized` 表示 token 失效（HTTP 401 或业务 code=401）。

### 3.1 ListAccounts —— 账号列表

```go
func (c *api.Client) ListAccounts(q url.Values) (*api.AccountList, api.RawResult, error)

type AccountList struct {
    Items []AccountItem // {ID,Name,Platform,AccountLevel,Status,Concurrency}
    Total int64
    Page  int
    Pages int
}
```

- 上游：`GET {base}/api/v1/accounts?<q>`
- `api.DefaultAccountsQuery()` 返回需求默认参数（page=1,page_size=20,sort_by=created_at,sort_order=desc,timezone=Asia/Shanghai）

示例：

```go
q := api.DefaultAccountsQuery()
q.Set("page_size", "50")
list, raw, err := cli.ListAccounts(q)
// list.Items[*] 入库；raw.Body 落盘
```

### 3.2 GetAccountUsage —— 账号用量

```go
func (c *api.Client) GetAccountUsage(accountID int64, source, timezone string) (*api.AccountUsage, api.RawResult, error)

type AccountUsage struct {
    Source    string
    UpdatedAt string
    FiveHour  *UsageWindow
    SevenDay  *UsageWindow // 取 .Utilization 与 .WindowStats.{Cost,StandardCost,UserCost}
}
```

- 上游：`GET {base}/api/v1/accounts/{accountID}/usage?source=local&timezone=Asia/Shanghai`
- 业务计算在 pipeline 层做：`estimated_total_quota = round(SevenDay.WindowStats.Cost / SevenDay.Utilization)`，utilization=0 记 `-999`

### 3.3 GetAccountStats —— 账号状态/模型统计

```go
func (c *api.Client) GetAccountStats(accountID int64, startDate, endDate, timezone string) (*api.AccountStats, api.RawResult, error)

type AccountStats struct {
    AccountID int64
    StartDate string
    EndDate   string
    Models    []ModelStat // model/requests/tokens/cost/actual_cost/account_cost
}
```

- 上游：`GET {base}/api/v1/accounts/{accountID}/stats?start_date=&end_date=&timezone=`
- 落盘保存的是 **完整原始 JSON**（`RawResult.Body`），结构体只解析 models 数组用于入库

### 3.4 ListBalanceLedger —— 余额流水

```go
func (c *api.Client) ListBalanceLedger(q url.Values) (*api.LedgerList, api.RawResult, error)

type LedgerList struct {
    Items []LedgerItem // {ID,Direction,Amount,Reason,RefType,RefID,BalanceAfter,Metadata,CreatedAt}
    Total int64
    Pages int
}
```

- 上游：`GET {base}/api/v1/usage/balance-ledger?<q>`
- 支持参数：`page,page_size,direction,start_date,end_date,start_time,end_time,timezone,sort_order`
- 清洗：pipeline 把每条 `Metadata`（json.RawMessage）反序列化为 `api.LedgerMeta{ConsumerUserID,APIKeyID,AccountID,RequestID}` 后入列

## 4. 编排层（pipeline 包）

`pipeline.Fetcher` 串联「调接口 → 落盘原始 JSON → 清洗 → 入库」。

```go
f := &pipeline.Fetcher{Client: apiClient, Store: db, Email: email}
o := &pipeline.Options{StartDate:"2026-09-22", EndDate:"2026-09-22", PageSize:20, MaxPages:200, RawDir:"data/raw", Timezone:"Asia/Shanghai"}
res := &pipeline.Result{}
f.FetchAccounts(o, res) // res.Accounts / res.RawFiles / res.Errors
f.FetchUsage(o, res)
f.FetchStats(o, res)
f.FetchLedger(o, res)
```

每个 `FetchXxx` 的内部流程：

1. 调对应 `api.Client` 方法，拿到 `(结构体, RawResult, err)`
2. `RawResult.Body` 写入 `{RawDir}/<kind>/<email>/<ts>__<params>[__p<page>].json`
3. 结构体字段清洗/计算后调 `store` 对应 Save 方法 upsert
4. 写一条 `fetch_runs` 日志

`Result` 汇总：`Accounts / Usages / StatsModels / LedgerItems / RawFiles[] / Errors[]`。

## 5. 存储层（store 包）

| 方法 | 表 | 语义 |
|---|---|---|
| `SaveSession(email, AuthResult)` | auth_sessions | token/cookie AES-256-GCM 加密，email 唯一，覆盖更新 |
| `GetSession(email)` / `ListSessions()` | auth_sessions | 解密读出 |
| `SaveAccounts(email, items)` | accounts | 按 email 全量覆盖 |
| `SaveAccountUsage(email, id, util, cost, sc, uc)` | account_usage | upsert，内部计算 estimated_total_quota |
| `SaveAccountStats(email, id, s, e, models)` | account_stats | (account_id,email,start,end,model) 主键 upsert |
| `SaveLedger(email, items)` | balance_ledger | id 主键 upsert，含 metadata 清洗列 |
| `LogFetch(email,kind,params,n,rawFile)` | fetch_runs | 运行日志 |

## 6. 原始响应落盘约定

```
data/raw/<kind>/<email_sanitized>/<yyyymmdd_hhmmss>__<param_summary>[__p<page>].json
```

- `kind` ∈ `accounts | account_usage | account_stats | balance_ledger`
- email 中 `@`→`_at_`、`.`→`_`
- 内容为上游 envelope 完整原文，未裁剪

## 7. CLI（薄壳）

```
ai-pixel-analysis login|accounts|usage|stats|ledger|all [flags]
```

`all` = runLogin → FetchAccounts → FetchUsage → FetchStats → FetchLedger → 每账号写 `data/reports/run_<ts>.md`。

# 接口规范

所有接口均为只读 GET 请求，base = `{host}/api/v1`。
认证：`Authorization: Bearer <access_token>`（登录后由 login 命令存入 sqlite）。

统一响应壳：

```json
{ "code": 0, "message": "success", "data": { ... } }
```

`code != 0` 视为失败。分页接口的 `data` 结构固定为：

```json
{ "items": [...], "total": 52158, "page": 1, "page_size": 100, "pages": 522 }
```

---

## 1. 使用明细 `GET /usage`

| 参数 | 类型 | 说明 |
|---|---|---|
| page | int | 页码，从 1 |
| page_size | int | 每页条数 |
| api_key_id | int64 | 可选，按 key 过滤 |
| start_date / end_date | YYYY-MM-DD | 自然日范围 |
| start_time / end_time | RFC3339 | 精确时间范围（与日期二选一，优先） |
| timezone | string | 如 `Asia/Shanghai` |
| sort_by | string | `created_at` 等 |
| sort_order | string | `asc` / `desc` |

返回 item 关键字段（入库时全部保留）：

| 字段 | 说明 |
|---|---|
| id | 记录 id，主键 |
| request_id | 客户端请求 id（`client:uuid`） |
| model | 模型名 |
| inbound_endpoint / upstream_endpoint | 入口/上游端点 |
| group_id / api_key_id / account_id | 关联 id |
| input_tokens / output_tokens / cache_read_tokens | token 数 |
| input_cost / output_cost / cache_read_cost / total_cost | 官方计费分项 |
| actual_cost | 乘倍率后实际扣费 |
| rate_multiplier / rate_multiplier_source | 倍率与来源 |
| billing_mode | `token` / `per_request` / `image` |
| request_type | `sync` / `stream` / `ws_v2` |
| stream | 是否流式 |
| duration_ms / first_token_ms | 耗时 |
| user_agent | 客户端 UA |
| created_at | RFC3339 |

> 原始响应每条还带 `user` / `api_key` / `group` 大嵌套对象，内容重复（账号信息已有 email 区分），入库时按字段裁剪，仅保留其 id。原始文件仍完整保留。

## 2. 使用汇总 `GET /usage/stats`

| 参数 | 说明 |
|---|---|
| period | `today` / `yesterday` / `last7days` 等快捷周期 |
| start_date + end_date | 或显式日期范围 |
| start_time / end_time | 或精确时间范围 |
| api_key_id | 可选 |

返回：`total_requests`、各级 token 合计、`total_cost`、`total_actual_cost`、`average_duration_ms`。

## 3. 仪表盘 `GET /usage/dashboard/*`

| 接口 | 说明 | 参数 |
|---|---|---|
| `/usage/dashboard/stats` | 总额度窗口 + 今日数据 + rpm/tpm | 无 |
| `/usage/dashboard/trend` | 按天趋势点 | 日期范围 |
| `/usage/dashboard/models` | 模型维度拆分 | 日期范围 |

均返回汇总/数组快照，不分页。


## 3.5 托管账号 `GET /accounts`（进度6 新增）

| 参数 | 说明 |
|---|---|
| page / page_size | 分页 |

返回号主名下托管上游账号列表，关键字段：`id`（额度窗 key）、`name`、`platform`、`account_level`、`status`。

## 3.6 账号额度窗 `GET /accounts/{account_id}/usage`（进度6 新增）

返回该托管账号的额度窗口（`five_hour` / `seven_day`）。

```json
{
  "seven_day": {
    "utilization": 53,
    "window_stats": {
      "requests": 11089,
      "cost": 923.39,        // 7D 账号消耗（账号成本）
      "standard_cost": 950.37,
      "user_cost": 236.86    // 7D 用户扣费口径
    }
  }
}
```

**用途**：总览「7D消耗」=`seven_day.window_stats.cost` 加总；「7D利用率」=Σcost/Σ额度（额度=cost/utilization 反推）。每次同步快照覆盖写入 `account_windows` 表。

## 4. 余额流水 `GET /usage/balance-ledger`

| 参数 | 说明 |
|---|---|
| page / page_size | 分页 |
| direction | `credit`（入，如收益到账）/ `debit`（出，如消费扣费） |
| reason | `usage_charge` / `account_share_income` / `recharge` 等 |
| ref_type / ref_id | 关联对象（通常 `usage_log` + 记录 id） |
| start_date/end_date 或 start_time/end_time / timezone | 时间过滤 |
| sort_order | asc/desc |

item 字段：

| 字段 | 说明 |
|---|---|
| id | 流水 id |
| direction | credit / debit |
| amount | 金额，字符串十进制（避免浮点精度损失） |
| reason | 原因 |
| ref_type / ref_id | 关联 usage_log |
| balance_after | 变动后余额 |
| metadata | 明细对象（见下） |
| created_at | 时间 |

`metadata` 关键字段：

| 字段 | credit | debit | 说明 |
|---|---|---|---|
| consumer_user_id | ✅ | ❌ | 实际使用者 id（debit 使用者即账号本人，故无） |
| api_key_id | ✅ | ✅ | 使用的 key |
| account_id | ✅ | ✅ | 调用的上游账户 id |
| request_id | ✅ | ✅ | 请求 id |

## 5. 流水汇总 `GET /usage/balance-ledger/stats`

同过滤参数（去分页/排序），返回 `total_entries` / `credit_entries` / `debit_entries` / `credit_amount` / `debit_amount` / `net_amount`（金额均为字符串十进制）。

## 6. 登录（进度 1，仅供参考）

- `GET {login_port}`（即 `/login`）：返回 HTML，其中 `window.__APP_CONFIG__` JSON 含 `login_agreement_revision`
- `POST /api/v1/auth/login`：body `{email, password, login_agreement_revision}`，返回 `access_token` / `refresh_token` / `expires_in` / `token_type` / `user`

---

## 字段取舍（清洗入库）

| 处理 | 字段 | 原因 |
|---|---|---|
| 保留 | id / request_id / model / tokens / 各项 cost / rate_multiplier / billing_mode / request_type / stream / duration_ms / first_token_ms / user_agent / created_at / 各关联 id | 计费与用量分析必需 |
| 保留 | ledger 的 direction / amount / reason / ref_type / ref_id / balance_after / metadata / created_at | 流水核对必需；amount 保留字符串精度 |
| 丢弃 | 每条重复的 user / api_key / group 完整嵌套对象 | 数据冗余大，账号归属由 `email` 列区分，api_key/group 留 id 即可关联 |
| 不单独存 | 快照类（dashboard/stats/ledger_stats） | 属聚合结果，可由明细重算，只留原始文件 |

# 数据抽取接口文档

本文档说明 version2 分支各数据抽取接口的用法、参数、返回与落盘/入库规则。
所有接口均为只读 GET/POST，不做任何上游数据修改。

## 前置条件

- `.env` 提供机密配置：`user_name_N` / `user_password_N`（或 `user_passwor_N`）成组出现；`db_secret` 用于凭据加解密。
- `config.toml` 提供服务地址：

```toml
[server]
host = "https://ai-pixel.online"
port = 0
timeout_ms = 30000
```

## 1. 登录

**命令**: `ai-pixel-analysis login`

**流程**:
1. `GET {host}/login` 从 `window.__APP_CONFIG__` 提取 `login_agreement_revision`。
2. `POST {host}/api/v1/auth/login` body: `{"email","password","login_agreement_revision"}`。
3. 成功返回 `access_token`/`refresh_token`/`token_type`/`expires_in`/cookies。

**入库**: `auth_sessions` 表，access_token/refresh_token/cookies 经 AES-256-GCM（key=SHA256(db_secret)）加密存储；同一 email 覆盖更新。

## 2. 账号列表

**命令**: `ai-pixel-analysis accounts`

**接口**: `GET {host}/api/v1/accounts?page=&page_size=&sort_by=created_at&sort_order=desc&timezone=Asia/Shanghai`

**参数** (flag):
- `-page-size N` 每页条数，默认 20
- `-max-pages N` 分页上限，默认 200

**解析字段**: `id`, `name`, `platform`, `account_level`, `concurrency`, `status`

**入库**: `accounts` 表（按 email 全量覆盖）

**落盘**: `data/raw/accounts/<email>/<ts>__<params>__p<page>.json`

## 3. 账号用量

**命令**: `ai-pixel-analysis usage`

**接口**: `GET {host}/api/v1/accounts/{account_id}/usage?source=local&timezone=Asia/Shanghai`

**解析字段**: `seven_day.utilization`, `seven_day.window_stats.cost`, `standard_cost`, `user_cost`

**计算**: `estimated_total_quota = round(cost / utilization)`；utilization=0 时记为 **-999**。

**入库**: `account_usage` 表（account_id+email 主键，覆盖更新）

**落盘**: `data/raw/account_usage/<email>/<ts>__account_<id>.json`

## 4. 账号状态/模型统计

**命令**: `ai-pixel-analysis stats`

**接口**: `GET {host}/api/v1/accounts/{account_id}/stats?start_date=&end_date=&timezone=Asia/Shanghai`

**参数** (flag):
- `-start YYYY-MM-DD` 起始日期（默认今天）
- `-end YYYY-MM-DD` 结束日期（默认同 start）
- `-timezone` 默认 Asia/Shanghai

**解析字段**: `models[]` 内每个模型的 `model/requests/input_tokens/output_tokens/cache_creation_tokens/cache_read_tokens/total_tokens/cost/actual_cost/account_cost`

**入库**: `account_stats` 表（account_id+email+start_date+end_date+model 主键）

**落盘**: `data/raw/account_stats/<email>/<ts>__account_<id>_start_<s>_end_<e>.json`（原始完整 JSON）

## 5. 余额流水

**命令**: `ai-pixel-analysis ledger`

**接口**: `GET {host}/api/v1/usage/balance-ledger?page=&page_size=&direction=&start_date=&end_date=&start_time=&end_time=&timezone=&sort_order=desc`

**参数** (flag):
- `-start`, `-end` 日期过滤
- `-start-time`, `-end-time` RFC3339 精确时间（优先级高于日期）
- `-page-size`, `-max-pages`

**清洗**: 从 `metadata` 提取 `consumer_user_id`, `api_key_id`, `account_id`, `request_id`

**入库**: `balance_ledger` 表（id 主键，upsert）

**落盘**: `data/raw/balance_ledger/<email>/<ts>__<params>__p<page>.json`

## 6. 一键全流程

**命令**: `ai-pixel-analysis all`

依次执行 login → accounts → usage → stats → ledger，最后为每个 email 生成运行报告到 `data/reports/run_<ts>.md`。

## 数据库表速览

| 表 | 说明 |
|---|---|
| auth_sessions | 登录凭据（加密） |
| accounts | 账号列表 |
| account_usage | 账号用量+预估额度 |
| account_stats | 按日期的模型统计 |
| balance_ledger | 余额流水+metadata 清洗字段 |
| fetch_runs | 每次抓取的运行日志 |

## 原始数据目录结构

```
data/
  raw/
    accounts/<email>/...
    account_usage/<email>/...
    account_stats/<email>/...
    balance_ledger/<email>/...
  reports/
    run_<ts>.md
```

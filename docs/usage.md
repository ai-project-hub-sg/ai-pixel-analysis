# 使用方式

## 准备

1. 配置 `.env`：`host`、`login_port`、`db_secret`、`user_name_N`/`user_password_N`（N 从 1 开始成对出现）
2. 编译：`go build -o ai-pixel-analysis.exe .`

## 命令

### login — 登录并保存凭据

```
ai-pixel-analysis login [-env .env] [-db ai-pixel.db]
```

对 .env 中每个账号执行登录，将 access_token / refresh_token / cookies 加密存入 sqlite 的 `auth_sessions` 表。后续抓取依赖此凭据；token 过期后需重新执行。

### fetch — 抓取数据

```
ai-pixel-analysis fetch [flags]
```

对每个已登录账号：拉取使用明细分页 + 余额流水分页 + 各汇总/仪表盘快照，原始响应落盘到 `data/raw/`，清洗后写入 sqlite。

| flag | 默认 | 说明 |
|---|---|---|
| -email | 全部 | 只抓指定账号 |
| -period | 无 | `today` / `yesterday` / `last7days` / `last30days`，自动展开为日期范围 |
| -start / -end | 无 | `YYYY-MM-DD` 日期区间 |
| -start-time / -end-time | 无 | RFC3339 精确时间区间（优先于日期） |
| -page-size | 100 | 每页条数 |
| -max-pages | 200 | 分页上限，防止异常循环 |
| -raw-dir | data/raw | 原始响应目录 |
| -export-dir | data/export | 导出目录（供 export 默认对齐） |

示例：

```
ai-pixel-analysis fetch -period today
ai-pixel-analysis fetch -start 2026-09-01 -end 2026-09-18 -email a@b.com
ai-pixel-analysis fetch -start-time 2026-09-19T00:00:00+08:00 -end-time 2026-09-19T12:00:00+08:00
```

### export — 导出 md 报告

```
ai-pixel-analysis export [-email <addr>] [-export-dir data/export]
```

从 sqlite 生成 `data/export/report_<账号>_<时间>.md`，含使用明细汇总、按模型、按天、流水分类汇总、最近 50 条流水（含使用者/使用key/调用账户/请求ID）。

### web — 数据分析界面

```
ai-pixel-analysis web [-addr :8080] [-db ai-pixel.db]
```

以只读模式打开 sqlite 并启动本地 web 服务，浏览器访问 http://localhost:8080。

> 提示：Windows 下直接双击 `start-web.bat` 即可启动（无需手动开 cmd）；脚本与 exe 同目录，服务运行期间保持窗口开启，关闭窗口即停止。注意：直接双击 `ai-pixel-analysis.exe`（不带参数）不会启动 web，只会打印用法后退出。

- 不加载 .env、不接触加密凭据；数据库 mode=ro，任何代码路径都无法写入。
- 顶部 tab 切换四个分页：总览 / 分账收入 / 用量分析 / 流水明细；右上角按账号筛选。
- 前端为内嵌静态资源（embed），单 exe 即可运行，无需部署前端。

### all — 一键执行

```
ai-pixel-analysis all -period today
```

依次执行 login + fetch + export。

## 输出位置

| 内容 | 路径 |
|---|---|
| 原始响应 | `data/raw/<类别>/<账号>__<范围>_p<页码>.json`（快照类无 _p） |
| 结构化数据 | `ai-pixel.db` 的 `usage_logs` / `balance_ledger` 表 |
| md 报告 | `data/export/report_*.md` |
| 登录凭据 | `ai-pixel.db` 的 `auth_sessions` 表（加密） |

> 安全约束：所有网络请求均为只读 GET，不会对账号做任何增删改。

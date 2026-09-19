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

- 默认只读：不加载 .env、不接触加密凭据；数据库 mode=ro。
- 加 `-sync` 启用"数据同步"页签（start-web.bat 已带）：后台 worker 用可写连接+解密会话抓取；所有分析接口仍走只读连接，机密不回显。
- 顶部 tab 切换：总览 / 分账收入 / 用量分析 / 流水明细 / 数据同步；右上角按账号筛选。
- 总览同环比按分钟对齐（当前小时已过 N 分钟，对比上小时/昨同小时的前 N 分钟）。
- 前端为内嵌静态资源（embed），单 exe 即可运行，无需部署前端。

#### 数据同步（-sync，进度4）

```
ai-pixel-analysis web -sync [-addr :8080] [-sync-interval-ms 1200]
```

- **初始化数据**：库中无任何数据时可点，拉最近 3 天（验证各功能）；有数据后按钮自动禁用。
- **更新数据**：纯增量——从覆盖右端（水位/已有数据最大时间）更新到按下前一分钟，不补历史。
- **回填最近30天**：单独大任务，首次拉历史或补大段缺失用（与增量更新分离）。
- **每分钟自动同步**：勾选后持续每分钟增量同步一次（可勾选即生效，重启后保留）。
- **获取方案**：同步前按 账号×类型×时间块 生成方案；usage 按天切、ledger 按周切、快照整段。
- **跳过已覆盖**：水位 + 已有数据最大时间之内的块直接跳过，不重复请求。
- **断点续传**：按 id upsert 幂等；中途停止下次从未覆盖处继续。
- **平滑限速**：单并发，固定 ~1.2s/请求（`-sync-interval-ms` 可调，越大越温和）。
- **进度可视**：页签内实时显示当前在同步什么、已拉页/条、单块与整体预计完成时间、历史任务。
- **数据自动刷新**：同步结算后 data_version +1，前端每3s轮询到变化即静默重载当前页（不刷新整页），无需手动。
- **中断处理**：进程重启时把仍停在 running/pending 的旧任务标记为 interrupted，不会永远显示 running
- **关闭页面提醒**：当后台同步服务运行中关闭/刷新页面时，界面弹出提醒——可选择「保留后台服务」（服务驻留继续同步）或「关闭服务」（优雅停机，`POST /api/lifecycle/shutdown`）。勾选「不再提醒」后按所选方式直接执行：选保留则静默放行，选关闭服务则页面卸载时静默停机。选择保存在浏览器 localStorage；服务重启后恢复默认（关闭页面=需重新确认，除非再勾选）。
  - 入口1：右上角新增「关闭界面」按钮——主动点击会完整弹出选择框（推荐方式）。
  - 入口2：直接关浏览器标签/刷新——浏览器安全限制下只能弹原生确认框，此时回到页面用按钮选择即可；若已勾选「不再提醒+关闭服务」，标签页关闭时会静默停机（sendBeacon）。
  - 关闭服务端点仅接受本机回环地址（127.0.0.1/::1），局域网内他人访问页面无法关掉你的服务。
。
- **空数据显示**：某类数据为空时范围显示"无数据"，水位显示 —，而非零值时间。

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

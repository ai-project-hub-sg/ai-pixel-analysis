package pipeline

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ai-pixel-analysis/store"
)

// ExportMarkdown 从数据库导出一份人可读的 md 报告
func ExportMarkdown(db *store.Store, email, exportDir string) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "# AI Pixel 用量报告\n\n")
	fmt.Fprintf(&b, "- 账号: %s\n", email)
	fmt.Fprintf(&b, "- 导出时间: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	fmt.Fprintf(&b, "## 使用明细汇总\n\n")
	exportUsageSummary(db, email, &b)
	exportUsageByModel(db, email, &b)
	exportUsageByDay(db, email, &b)

	fmt.Fprintf(&b, "## 余额流水汇总\n\n")
	exportLedgerSummary(db, email, &b)
	exportLedgerRecent(db, email, &b)

	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("report_%s_%s.md", sanitize(email), time.Now().Format("20060102_150405"))
	path := filepath.Join(exportDir, name)
	return path, os.WriteFile(path, []byte(b.String()), 0o644)
}

func exportUsageSummary(db *store.Store, email string, b *strings.Builder) {
	row := db.DB().QueryRow(`
SELECT COUNT(*), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0),
       COALESCE(SUM(cache_read_tokens),0),
       ROUND(COALESCE(SUM(total_cost),0),6), ROUND(COALESCE(SUM(actual_cost),0),6),
       ROUND(COALESCE(AVG(duration_ms),0),0)
FROM usage_logs WHERE email=?`, email)
	var n, in, out, cache, dur int64
	var tc, ac float64
	if err := row.Scan(&n, &in, &out, &cache, &tc, &ac, &dur); err != nil {
		fmt.Fprintf(b, "_(查询失败: %v)_\n\n", err)
		return
	}
	b.WriteString("| 指标 | 值 |\n|---|---|\n")
	fmt.Fprintf(b, "| 请求数 | %d |\n", n)
	fmt.Fprintf(b, "| 输入 tokens | %d |\n", in)
	fmt.Fprintf(b, "| 输出 tokens | %d |\n", out)
	fmt.Fprintf(b, "| 缓存读 tokens | %d |\n", cache)
	fmt.Fprintf(b, "| 官方计费 | $%.6f |\n", tc)
	fmt.Fprintf(b, "| 实际扣费 | $%.6f |\n", ac)
	fmt.Fprintf(b, "| 平均耗时 | %d ms |\n\n", dur)
}

func exportUsageByModel(db *store.Store, email string, b *strings.Builder) {
	rows, err := db.DB().Query(`
SELECT model, COUNT(*), SUM(input_tokens), SUM(output_tokens), SUM(cache_read_tokens),
       ROUND(SUM(total_cost),6), ROUND(SUM(actual_cost),6)
FROM usage_logs WHERE email=? GROUP BY model ORDER BY SUM(total_cost) DESC`, email)
	if err != nil {
		fmt.Fprintf(b, "_(查询失败: %v)_\n\n", err)
		return
	}
	defer rows.Close()
	b.WriteString("### 按模型\n\n| 模型 | 请求 | 输入 | 输出 | 缓存读 | 计费 | 实扣 |\n|---|---|---|---|---|---|---|\n")
	for rows.Next() {
		var m string
		var c, i, o, cr int64
		var tc, ac float64
		if rows.Scan(&m, &c, &i, &o, &cr, &tc, &ac) != nil {
			continue
		}
		fmt.Fprintf(b, "| %s | %d | %d | %d | %d | $%.6f | $%.6f |\n", m, c, i, o, cr, tc, ac)
	}
	b.WriteString("\n")
}

func exportUsageByDay(db *store.Store, email string, b *strings.Builder) {
	rows, err := db.DB().Query(`
SELECT substr(created_at,1,10), COUNT(*), SUM(input_tokens), SUM(output_tokens),
       ROUND(SUM(total_cost),6), ROUND(SUM(actual_cost),6)
FROM usage_logs WHERE email=? GROUP BY substr(created_at,1,10) ORDER BY 1 DESC LIMIT 30`, email)
	if err != nil {
		fmt.Fprintf(b, "_(查询失败: %v)_\n\n", err)
		return
	}
	defer rows.Close()
	b.WriteString("### 按天（近 30 天）\n\n| 日期 | 请求 | 输入 | 输出 | 计费 | 实扣 |\n|---|---|---|---|---|---|\n")
	for rows.Next() {
		var d string
		var c, i, o int64
		var tc, ac float64
		if rows.Scan(&d, &c, &i, &o, &tc, &ac) != nil {
			continue
		}
		fmt.Fprintf(b, "| %s | %d | %d | %d | $%.6f | $%.6f |\n", d, c, i, o, tc, ac)
	}
	b.WriteString("\n")
}

func exportLedgerSummary(db *store.Store, email string, b *strings.Builder) {
	rows, err := db.DB().Query(`
SELECT direction, reason, COUNT(*), SUM(CAST(amount AS REAL))
FROM balance_ledger WHERE email=? GROUP BY direction, reason ORDER BY 4 DESC`, email)
	if err != nil {
		fmt.Fprintf(b, "_(查询失败: %v)_\n\n", err)
		return
	}
	defer rows.Close()
	b.WriteString("| 方向 | 原因 | 笔数 | 金额合计 |\n|---|---|---|---|\n")
	for rows.Next() {
		var dir, reason string
		var n int64
		var sum float64
		if rows.Scan(&dir, &reason, &n, &sum) != nil {
			continue
		}
		fmt.Fprintf(b, "| %s | %s | %d | $%.6f |\n", dir, reason, n, sum)
	}
	b.WriteString("\n")
}

func exportLedgerRecent(db *store.Store, email string, b *strings.Builder) {
	rows, err := db.DB().Query(`
SELECT created_at, direction, reason, amount, balance_after, ref_type, ref_id, metadata
FROM balance_ledger WHERE email=? ORDER BY created_at DESC LIMIT 50`, email)
	if err != nil {
		fmt.Fprintf(b, "_(查询失败: %v)_\n\n", err)
		return
	}
	defer rows.Close()
	b.WriteString("### 最近 50 条\n\n| 时间 | 方向 | 原因 | 金额 | 余额 | 使用者 | 使用Key | 调用账户 | 请求ID |\n|---|---|---|---|---|---|---|---|---|\n")
	for rows.Next() {
		var t, dir, reason, amount, bal, rt string
		var rid int64
		var meta sql.NullString
		if rows.Scan(&t, &dir, &reason, &amount, &bal, &rt, &rid, &meta) != nil {
			continue
		}
		consumer, key, acct, reqid := parseLedgerMeta(meta.String)
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			t, dir, reason, amount, bal, consumer, key, acct, reqid)
	}
	b.WriteString("\n")
}

// parseLedgerMeta 从流水 metadata 提取分析字段。
// credit(入)方向 consumer_user_id 是实际使用者；debit 方向无该字段，使用者即账号本人。
func parseLedgerMeta(meta string) (consumer, apiKey, account, requestID string) {
	if meta == "" {
		return "-", "-", "-", "-"
	}
	var m struct {
		AccountID      int64  `json:"account_id"`
		APIKeyID       int64  `json:"api_key_id"`
		ConsumerUserID *int64 `json:"consumer_user_id"`
		RequestID      string `json:"request_id"`
	}
	if json.Unmarshal([]byte(meta), &m) != nil {
		return "-", "-", "-", "-"
	}
	consumer = "-"
	if m.ConsumerUserID != nil {
		consumer = fmt.Sprint(*m.ConsumerUserID)
	}
	apiKey = "-"
	if m.APIKeyID != 0 {
		apiKey = fmt.Sprint(m.APIKeyID)
	}
	account = "-"
	if m.AccountID != 0 {
		account = fmt.Sprint(m.AccountID)
	}
	requestID = m.RequestID
	if requestID == "" {
		requestID = "-"
	}
	return
}

func sanitize(s string) string {
	return strings.NewReplacer("@", "_at_", ".", "_", "/", "_", "\\", "_").Replace(s)
}

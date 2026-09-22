package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Report 汇总一次完整运行的结果
type Report struct {
	Email       string
	StartedAt   time.Time
	FinishedAt  time.Time
	Accounts    int
	Usages      int
	StatsModels int
	LedgerItems int
	RawFiles    []string
	Errors      []string
}

// Write 生成 markdown 运行报告
func (r *Report) Write(dir string) (string, error) {
	if dir == "" { dir = "data/reports" }
	if err := os.MkdirAll(dir, 0o755); err != nil { return "", err }
	name := fmt.Sprintf("run_%s.md", r.FinishedAt.Format("20060102_150405"))
	path := filepath.Join(dir, name)
	var b []byte
	b = append(b, []byte(fmt.Sprintf("# 数据抽取运行报告\n\n- 账号: %s\n- 开始: %s\n- 结束: %s\n- 耗时: %s\n\n## 结果汇总\n\n- 账号数: %d\n- 用量记录: %d\n- 模型统计条数: %d\n- 流水条数: %d\n- 原始文件数: %d\n\n",
		r.Email, r.StartedAt.Format(time.RFC3339), r.FinishedAt.Format(time.RFC3339),
		r.FinishedAt.Sub(r.StartedAt), r.Accounts, r.Usages, r.StatsModels, r.LedgerItems, len(r.RawFiles)))...)
	if len(r.Errors) > 0 {
		b = append(b, []byte("## 警告/错误\n\n")...)
		for _, e := range r.Errors {
			b = append(b, []byte("- "+e+"\n")...)
		}
		b = append(b, []byte("\n")...)
	}
	if len(r.RawFiles) > 0 {
		b = append(b, []byte("## 原始文件\n\n")...)
		for _, p := range r.RawFiles {
			b = append(b, []byte("- "+p+"\n")...)
		}
	}
	return path, os.WriteFile(path, b, 0o644)
}

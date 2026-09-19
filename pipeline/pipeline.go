// Package pipeline 负责数据抓取、原始落盘、清洗入库和导出。
package pipeline

import (
	"time"

	"ai-pixel-analysis/api"
	"ai-pixel-analysis/store"
)

// Options 是抓取范围参数
type Options struct {
	// 时间范围
	StartDate string // YYYY-MM-DD
	EndDate   string
	StartTime string // RFC3339
	EndTime   string
	Period    string // today|yesterday|last7days|last30days，展开为日期范围

	// 分页与安全阀
	PageSize int
	MaxPages int // 防止异常循环，0 表示默认 200

	// 输出目录
	RawDir    string
	ExportDir string
}

// expandPeriod 把 period 展开成具体日期；start_time/end_time 显式指定时不覆盖
func (o *Options) expandPeriod() {
	if o.StartTime != "" || o.EndTime != "" {
		return // 用户给了精确时间，period 只用于 stats 快照
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	switch o.Period {
	case "today":
		o.StartDate = today.Format("2006-01-02")
		o.EndDate = o.StartDate
	case "yesterday":
		o.StartDate = today.AddDate(0, 0, -1).Format("2006-01-02")
		o.EndDate = o.StartDate
	case "last7days":
		o.StartDate = today.AddDate(0, 0, -6).Format("2006-01-02")
		o.EndDate = today.Format("2006-01-02")
	case "last30days":
		o.StartDate = today.AddDate(0, 0, -29).Format("2006-01-02")
		o.EndDate = today.Format("2006-01-02")
	}
	if o.EndDate == "" {
		o.EndDate = today.Format("2006-01-02")
	}
}

// rangeDesc 返回文件名友好的范围描述
func (o Options) rangeDesc() string {
	if o.StartTime != "" || o.EndTime != "" {
		return safeName(o.StartTime) + "__" + safeName(o.EndTime)
	}
	if o.StartDate != "" || o.EndDate != "" {
		return safeName(o.StartDate) + "__" + safeName(o.EndDate)
	}
	return "all"
}

func (o Options) query(page int) api.UsageQuery {
	return api.UsageQuery{
		Page:      page,
		PageSize:  o.pageSize(),
		StartDate: o.StartDate,
		EndDate:   o.EndDate,
		StartTime: o.StartTime,
		EndTime:   o.EndTime,
		Timezone:  "Asia/Shanghai",
		SortBy:    "created_at",
		SortOrder: "asc",
	}
}

func (o Options) pageSize() int {
	if o.PageSize > 0 {
		return o.PageSize
	}
	return 100
}

func (o Options) maxPages() int {
	if o.MaxPages > 0 {
		return o.MaxPages
	}
	return 200
}

// Fetcher 绑定一个账号的客户端和存储
type Fetcher struct {
	Client *api.Client
	Store  *store.Store
	Email  string
}

// Result 是抓取统计
type Result struct {
	UsagePages  int
	UsageItems  int
	LedgerPages int
	LedgerItems int
	RawFiles    []string
	Errors      []string
}

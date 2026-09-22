package pipeline

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ai-pixel-analysis/api"
)

// Options 是抓取参数
type Options struct {
	StartDate string // YYYY-MM-DD
	EndDate   string
	StartTime string // RFC3339
	EndTime   string
	PageSize  int
	MaxPages  int
	RawDir    string
	Timezone  string
}

func (o *Options) defaults() {
	if o.PageSize <= 0 { o.PageSize = 20 }
	if o.MaxPages <= 0 { o.MaxPages = 200 }
	if o.RawDir == "" { o.RawDir = "data/raw" }
	if o.Timezone == "" { o.Timezone = "Asia/Shanghai" }
	if o.StartDate == "" {
		o.StartDate = time.Now().Format("2006-01-02")
	}
	if o.EndDate == "" { o.EndDate = o.StartDate }
}

func (o *Options) rangeDesc() string {
	if o.StartTime != "" || o.EndTime != "" {
		return o.StartTime + "_" + o.EndTime
	}
	return o.StartDate + "_" + o.EndDate
}

// Result 汇总一次抓取结果
type Result struct {
	RawFiles    []string
	Errors      []string
	Accounts    int
	Usages      int
	StatsModels int
	LedgerItems int
}

// Fetcher 执行抓取+清洗+入库
type Fetcher struct {
	Client *api.Client
	Store  StoreIface
	Email  string
}

// StoreIface 抽象存储层便于测试
type StoreIface interface {
	SaveAccounts(email string, items []struct {
		ID int64; Name, Platform, AccountLevel string; Concurrency int; Status string
	}) error
	SaveAccountUsage(email string, accountID int64, utilization, cost, standardCost, userCost float64) error
	SaveAccountStats(email string, accountID int64, startDate, endDate string, models []struct {
		Model string; Requests, InputTokens, OutputTokens, CacheCreationTokens, CacheReadTokens, TotalTokens int64
		Cost, ActualCost, AccountCost float64
	}) error
	SaveLedger(email string, items []struct {
		ID int64; Direction, Amount, Reason, RefType string; RefID int64; BalanceAfter string
		ConsumerUserID, APIKeyID, AccountID int64; RequestID, CreatedAt string
	}) error
	LogFetch(email, kind, params string, itemCount int, rawFile string) error
}

// writeRaw 把原始响应写入 data/raw/<kind>/<email>/<ts>__<params>.json
func writeRaw(rawDir, kind, email, params string, page int, body []byte) (string, error) {
	if len(body) == 0 { return "", nil }
	ts := time.Now().Format("20060102_150405")
	safe := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "*", "", "?", "", "\"", "", "<", "", ">", "", "|", "").Replace(params)
	if safe == "" { safe = "default" }
	if len(safe) > 80 { safe = safe[:80] }
	dir := filepath.Join(rawDir, kind, sanitizeEmail(email))
	if err := os.MkdirAll(dir, 0o755); err != nil { return "", err }
	name := fmt.Sprintf("%s__%s", ts, safe)
	if page > 0 { name += fmt.Sprintf("__p%d", page) }
	path := filepath.Join(dir, name+".json")
	return path, os.WriteFile(path, body, 0o644)
}

func sanitizeEmail(e string) string {
	return strings.NewReplacer("@", "_at_", ".", "_").Replace(e)
}

// FetchAccounts 需求2：拉取全部账号并入库
func (f *Fetcher) FetchAccounts(o *Options, res *Result) error {
	o.defaults()
	page := 1
	for {
		q := api.DefaultAccountsQuery()
		q.Set("page", strconv.Itoa(page))
		q.Set("page_size", strconv.Itoa(o.PageSize))
		list, raw, err := f.Client.ListAccounts(q)
		if len(raw.Body) > 0 {
			if p, werr := writeRaw(o.RawDir, "accounts", f.Email, q.Encode(), page, raw.Body); werr == nil {
				res.RawFiles = append(res.RawFiles, p)
			}
		}
		if err != nil { return fmt.Errorf("accounts page %d: %w", page, err) }
		// 入库
		items := make([]struct {
			ID int64; Name, Platform, AccountLevel string; Concurrency int; Status string
		}, 0, len(list.Items))
		for _, it := range list.Items {
			items = append(items, struct {
				ID int64; Name, Platform, AccountLevel string; Concurrency int; Status string
			}{it.ID, it.Name, it.Platform, it.AccountLevel, it.Concurrency, it.Status})
		}
		if err := f.Store.SaveAccounts(f.Email, items); err != nil {
			return fmt.Errorf("save accounts: %w", err)
		}
		res.Accounts += len(items)
		f.Store.LogFetch(f.Email, "accounts", q.Encode(), len(items), "")
		if page >= list.Pages || len(list.Items) == 0 { break }
		page++
		if page > o.MaxPages { res.Errors = append(res.Errors, "accounts: max pages reached"); break }
	}
	return nil
}

// FetchUsage 需求3：对每个账号拉取用量并计算 estimated_total_quota
func (f *Fetcher) FetchUsage(o *Options, res *Result) error {
	o.defaults()
	// 先拿账号列表确定 account_id 集合
	accts, err := f.listAccountIDs()
	if err != nil { return err }
	for _, aid := range accts {
		usage, raw, err := f.Client.GetAccountUsage(aid, "local", o.Timezone)
		if len(raw.Body) > 0 {
			if p, werr := writeRaw(o.RawDir, "account_usage", f.Email, fmt.Sprintf("account_%d", aid), 0, raw.Body); werr == nil {
				res.RawFiles = append(res.RawFiles, p)
			}
		}
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("usage account %d: %v", aid, err))
			continue
		}
		var util, cost, sc, uc float64
		if usage.SevenDay != nil {
			util = usage.SevenDay.Utilization
			if usage.SevenDay.WindowStats != nil {
				cost = usage.SevenDay.WindowStats.Cost
				sc = usage.SevenDay.WindowStats.StandardCost
				uc = usage.SevenDay.WindowStats.UserCost
			}
		}
		if err := f.Store.SaveAccountUsage(f.Email, aid, util, cost, sc, uc); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("save usage %d: %v", aid, err))
			continue
		}
		res.Usages++
		f.Store.LogFetch(f.Email, "account_usage", fmt.Sprintf("account_%d", aid), 1, "")
	}
	return nil
}

func (f *Fetcher) listAccountIDs() ([]int64, error) {
	// 从 accounts 表读取当前 email 的账号 id
	// 这里通过 ListAccounts 接口重新拉第一页，保证拿到最新 id 集合；
	// 若账号数>pageSize，则逐页拉取。
	var ids []int64
	page := 1
	for {
		q := api.DefaultAccountsQuery()
		q.Set("page", strconv.Itoa(page))
		q.Set("page_size", "100")
		list, _, err := f.Client.ListAccounts(q)
		if err != nil { return nil, err }
		for _, it := range list.Items { ids = append(ids, it.ID) }
		if page >= list.Pages || len(list.Items) == 0 { break }
		page++
	}
	return ids, nil
}

// FetchStats 需求4：对每个账号拉取 stats，原始 JSON 落盘 + models 入库
func (f *Fetcher) FetchStats(o *Options, res *Result) error {
	o.defaults()
	accts, err := f.listAccountIDs()
	if err != nil { return err }
	params := fmt.Sprintf("start_%s_end_%s", o.StartDate, o.EndDate)
	for _, aid := range accts {
		stats, raw, err := f.Client.GetAccountStats(aid, o.StartDate, o.EndDate, o.Timezone)
		if len(raw.Body) > 0 {
			if p, werr := writeRaw(o.RawDir, "account_stats", f.Email, fmt.Sprintf("account_%d_%s", aid, params), 0, raw.Body); werr == nil {
				res.RawFiles = append(res.RawFiles, p)
			}
		}
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("stats account %d: %v", aid, err))
			continue
		}
		models := make([]struct {
			Model string; Requests, InputTokens, OutputTokens, CacheCreationTokens, CacheReadTokens, TotalTokens int64
			Cost, ActualCost, AccountCost float64
		}, 0, len(stats.Models))
		for _, m := range stats.Models {
			models = append(models, struct {
				Model string; Requests, InputTokens, OutputTokens, CacheCreationTokens, CacheReadTokens, TotalTokens int64
				Cost, ActualCost, AccountCost float64
			}{m.Model, m.Requests, m.InputTokens, m.OutputTokens, m.CacheCreationTokens, m.CacheReadTokens, m.TotalTokens, m.Cost, m.ActualCost, m.AccountCost})
		}
		if err := f.Store.SaveAccountStats(f.Email, aid, o.StartDate, o.EndDate, models); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("save stats %d: %v", aid, err))
			continue
		}
		res.StatsModels += len(models)
		f.Store.LogFetch(f.Email, "account_stats", fmt.Sprintf("account_%d_%s", aid, params), len(models), "")
	}
	return nil
}

// FetchLedger 需求5：拉取流水并清洗 metadata
func (f *Fetcher) FetchLedger(o *Options, res *Result) error {
	o.defaults()
	page := 1
	for {
		q := url.Values{}
		q.Set("page", strconv.Itoa(page))
		q.Set("page_size", strconv.Itoa(o.PageSize))
		q.Set("direction", "")
		if o.StartDate != "" { q.Set("start_date", o.StartDate) }
		if o.EndDate != "" { q.Set("end_date", o.EndDate) }
		if o.StartTime != "" { q.Set("start_time", o.StartTime) }
		if o.EndTime != "" { q.Set("end_time", o.EndTime) }
		q.Set("timezone", o.Timezone)
		q.Set("sort_order", "desc")
		list, raw, err := f.Client.ListBalanceLedger(q)
		if len(raw.Body) > 0 {
			if p, werr := writeRaw(o.RawDir, "balance_ledger", f.Email, q.Encode(), page, raw.Body); werr == nil {
				res.RawFiles = append(res.RawFiles, p)
			}
		}
		if err != nil { return fmt.Errorf("ledger page %d: %w", page, err) }
		items := make([]struct {
			ID int64; Direction, Amount, Reason, RefType string; RefID int64; BalanceAfter string
			ConsumerUserID, APIKeyID, AccountID int64; RequestID, CreatedAt string
		}, 0, len(list.Items))
		for _, it := range list.Items {
			var meta api.LedgerMeta
			if len(it.Metadata) > 0 {
				json.Unmarshal(it.Metadata, &meta)
			}
			items = append(items, struct {
				ID int64; Direction, Amount, Reason, RefType string; RefID int64; BalanceAfter string
				ConsumerUserID, APIKeyID, AccountID int64; RequestID, CreatedAt string
			}{it.ID, it.Direction, it.Amount, it.Reason, it.RefType, it.RefID, it.BalanceAfter,
				meta.ConsumerUserID, meta.APIKeyID, meta.AccountID, meta.RequestID, it.CreatedAt})
		}
		if err := f.Store.SaveLedger(f.Email, items); err != nil {
			return fmt.Errorf("save ledger: %w", err)
		}
		res.LedgerItems += len(items)
		f.Store.LogFetch(f.Email, "balance_ledger", q.Encode(), len(items), "")
		if page >= list.Pages || len(list.Items) == 0 { break }
		page++
		if page > o.MaxPages { res.Errors = append(res.Errors, "ledger: max pages reached"); break }
	}
	return nil
}

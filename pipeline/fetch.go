package pipeline

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// fetchAll 分页拉满一类接口，把每页原始响应和清洗后的条目都交给回调
type pageFetcher func(page int) (items []json.RawMessage, totalPages int, raw json.RawMessage, err error)

func (f *Fetcher) fetchAll(kind string, o Options, fetch pageFetcher, save func([]json.RawMessage) (int, error), res *Result) error {
	rangeDesc := o.rangeDesc()
	for page := 1; ; page++ {
		if page > o.maxPages() {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: stopped at max pages %d", kind, o.maxPages()))
			return nil
		}
		items, pages, raw, err := fetch(page)
		if err != nil {
			return fmt.Errorf("%s page %d: %w", kind, page, err)
		}
		p := rawFilePath(o.RawDir, kind, f.Email, rangeDesc, page)
		if err := writeRawFile(p, kind, f.Email, rangeDesc+"&page="+strconv.Itoa(page), raw); err != nil {
			res.Errors = append(res.Errors, "raw write: "+err.Error())
		} else {
			res.RawFiles = append(res.RawFiles, p)
		}
		if len(items) > 0 {
			n, err := save(items)
			if err != nil {
				return fmt.Errorf("%s save page %d: %w", kind, page, err)
			}
			if kind == "usage" {
				res.UsageItems += n
			} else {
				res.LedgerItems += n
			}
		}
		if kind == "usage" {
			res.UsagePages++
		} else {
			res.LedgerPages++
		}
		if page >= pages || len(items) == 0 {
			break
		}
	}
	return nil
}

// FetchUsage 拉取使用明细分页
func (f *Fetcher) FetchUsage(o Options, res *Result) error {
	o.expandPeriod()
	return f.fetchAll("usage", o,
		func(page int) ([]json.RawMessage, int, json.RawMessage, error) {
			list, raw, err := f.Client.ListUsage(o.query(page))
			if err != nil {
				return nil, 0, raw.Body, err
			}
			return list.Items, list.Pages, raw.Body, nil
		},
		func(items []json.RawMessage) (int, error) {
			return f.Store.SaveUsageItems(f.Email, items)
		}, res)
}

// FetchLedger 拉取余额流水分页
func (f *Fetcher) FetchLedger(o Options, res *Result) error {
	o.expandPeriod()
	return f.fetchAll("ledger", o,
		func(page int) ([]json.RawMessage, int, json.RawMessage, error) {
			list, raw, err := f.Client.ListBalanceLedger(o.query(page))
			if err != nil {
				return nil, 0, raw.Body, err
			}
			items := make([]json.RawMessage, 0, len(list.Items))
			for _, it := range list.Items {
				b, _ := json.Marshal(it)
				items = append(items, b)
			}
			return items, list.Pages, raw.Body, nil
		},
		func(items []json.RawMessage) (int, error) {
			return f.Store.SaveLedgerItems(f.Email, items)
		}, res)
}

// FetchSnapshots 拉取汇总/仪表盘等非分页快照并落盘
func (f *Fetcher) FetchSnapshots(o Options, res *Result) error {
	o.expandPeriod()
	q := o.query(0)
	snapshots := []struct {
		kind  string
		fetch func() (json.RawMessage, string, error)
	}{
		{"stats", func() (json.RawMessage, string, error) {
			_, raw, err := f.Client.GetUsageStats(first(o.Period, "today"), q)
			return raw.Body, "period_" + first(o.Period, "today"), err
		}},
		{"stats_range", func() (json.RawMessage, string, error) {
			if o.StartDate == "" && o.StartTime == "" {
				return nil, "", nil
			}
			_, raw, err := f.Client.GetUsageStats("", q)
			return raw.Body, o.rangeDesc(), err
		}},
		{"ledger_stats", func() (json.RawMessage, string, error) {
			_, raw, err := f.Client.GetBalanceLedgerStats(q)
			return raw.Body, o.rangeDesc(), err
		}},
		{"dashboard_stats", func() (json.RawMessage, string, error) {
			_, raw, err := f.Client.GetDashboardStats()
			return raw.Body, "", err
		}},
		{"dashboard_trend", func() (json.RawMessage, string, error) {
			_, raw, err := f.Client.GetDashboardTrend(url.Values{})
			return raw.Body, "", err
		}},
		{"dashboard_models", func() (json.RawMessage, string, error) {
			_, raw, err := f.Client.GetDashboardModels(url.Values{})
			return raw.Body, "", err
		}},
	}
	for _, s := range snapshots {
		body, params, err := s.fetch()
		if err != nil {
			res.Errors = append(res.Errors, s.kind+": "+err.Error())
			continue
		}
		if len(body) == 0 {
			continue
		}
		p := rawFilePath(o.RawDir, s.kind, f.Email, params, 0)
		if err := writeRawFile(p, s.kind, f.Email, params, body); err != nil {
			res.Errors = append(res.Errors, s.kind+" write: "+err.Error())
		} else {
			res.RawFiles = append(res.RawFiles, p)
		}
	}
	return nil
}

func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

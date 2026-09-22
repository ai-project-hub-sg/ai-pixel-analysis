package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"ai-pixel-analysis/api"
	"ai-pixel-analysis/auth"
	"ai-pixel-analysis/config"
	"ai-pixel-analysis/pipeline"
	"ai-pixel-analysis/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	envPath := fs.String("env", "", "path to .env")
	tomlPath := fs.String("config", "", "path to config.toml")
	dbPath := fs.String("db", "ai-pixel.db", "sqlite db path")
	email := fs.String("email", "", "只处理指定账号，默认全部")
	var o pipeline.Options
	fs.StringVar(&o.StartDate, "start", "", "YYYY-MM-DD")
	fs.StringVar(&o.EndDate, "end", "", "YYYY-MM-DD")
	fs.StringVar(&o.StartTime, "start-time", "", "RFC3339")
	fs.StringVar(&o.EndTime, "end-time", "", "RFC3339")
	fs.IntVar(&o.PageSize, "page-size", 20, "每页条数")
	fs.IntVar(&o.MaxPages, "max-pages", 200, "分页上限")
	fs.StringVar(&o.RawDir, "raw-dir", "data/raw", "原始响应目录")
	fs.StringVar(&o.Timezone, "timezone", "Asia/Shanghai", "时区")
	reportDir := fs.String("report-dir", "data/reports", "运行报告目录")
	fs.Parse(args)

	switch cmd {
	case "login":
		cfg, db := mustSetup(*envPath, *tomlPath, *dbPath)
		defer db.Close()
		runLogin(cfg, db)
	case "accounts":
		cfg, db := mustSetup(*envPath, *tomlPath, *dbPath)
		defer db.Close()
		runFetch(cfg, db, *email, &o, func(f *pipeline.Fetcher, res *pipeline.Result) error {
			return f.FetchAccounts(&o, res)
		})
	case "usage":
		cfg, db := mustSetup(*envPath, *tomlPath, *dbPath)
		defer db.Close()
		runFetch(cfg, db, *email, &o, func(f *pipeline.Fetcher, res *pipeline.Result) error {
			return f.FetchUsage(&o, res)
		})
	case "stats":
		cfg, db := mustSetup(*envPath, *tomlPath, *dbPath)
		defer db.Close()
		runFetch(cfg, db, *email, &o, func(f *pipeline.Fetcher, res *pipeline.Result) error {
			return f.FetchStats(&o, res)
		})
	case "ledger":
		cfg, db := mustSetup(*envPath, *tomlPath, *dbPath)
		defer db.Close()
		runFetch(cfg, db, *email, &o, func(f *pipeline.Fetcher, res *pipeline.Result) error {
			return f.FetchLedger(&o, res)
		})
	case "all":
		cfg, db := mustSetup(*envPath, *tomlPath, *dbPath)
		defer db.Close()
		runLogin(cfg, db)
		res := runFetchAll(cfg, db, *email, &o)
		writeReport(*email, res, *reportDir)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println(`用法:
  ai-pixel-analysis <command> [flags]

命令:
  login     登录所有 .env 账号，authorization 加密存入 sqlite
  accounts  拉取账号列表入库
  usage     拉取各账号用量并计算预估额度
  stats     拉取各账号状态/模型统计，原始 JSON 落盘
  ledger    拉取余额流水并清洗 metadata
  all       login + 全部抽取 + 生成运行报告

公共 flag:
  -env <path>       .env 路径 (默认同目录)
  -config <path>    config.toml 路径 (默认同目录)
  -db <path>        sqlite 路径 (默认 ai-pixel.db)
  -email <addr>     只处理指定账号

抽取 flag:
  -start YYYY-MM-DD   起始日期 (默认今天)
  -end YYYY-MM-DD     结束日期 (默认同 start)
  -start-time RFC3339 精确起始时间 (覆盖 start/end)
  -end-time RFC3339   精确结束时间
  -page-size N        每页条数 (默认 20)
  -max-pages N        分页上限 (默认 200)
  -raw-dir <dir>      原始响应目录 (默认 data/raw)
  -timezone <tz>      时区 (默认 Asia/Shanghai)
  -report-dir <dir>   报告目录 (默认 data/reports)`)
}

func mustSetup(envPath, tomlPath, dbPath string) (*config.Config, *store.Store) {
	ep := envPath
	if ep == "" { ep = config.DefaultPath(".env") }
	tp := tomlPath
	if tp == "" { tp = config.DefaultPath("config.toml") }
	cfg, err := config.Load(ep, tp)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	db, err := store.New(dbPath, cfg.DBSecret)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	return cfg, db
}

func runLogin(cfg *config.Config, db *store.Store) {
	client, err := auth.NewClient(cfg.Server.Host, cfg.Server.TimeoutMs)
	if err != nil {
		log.Fatalf("new auth client: %v", err)
	}
	revision, err := client.FetchAgreementRevision("/login")
	if err != nil {
		log.Fatalf("fetch agreement revision: %v", err)
	}
	for _, u := range cfg.Users {
		fmt.Printf("login %s ... ", u.Name)
		res, err := client.Login(u.Name, u.Password, revision)
		if err != nil {
			fmt.Printf("FAIL %v\n", err)
			continue
		}
		ar := &store.AuthResult{
			AccessToken:  res.AccessToken,
			RefreshToken: res.RefreshToken,
			TokenType:    res.TokenType,
			ExpiresIn:    res.ExpiresIn,
			Cookies:      res.Cookies,
			User:         res.User,
		}
		if err := db.SaveSession(u.Name, ar); err != nil {
			fmt.Printf("save FAIL %v\n", err)
			continue
		}
		fmt.Printf("ok expires_in=%ds\n", res.ExpiresIn)
	}
}

type fetchFn func(*pipeline.Fetcher, *pipeline.Result) error

func runFetch(cfg *config.Config, db *store.Store, onlyEmail string, o *pipeline.Options, fn fetchFn) {
	emails, err := db.ListSessions()
	if err != nil {
		log.Fatalf("list sessions: %v", err)
	}
	if len(emails) == 0 {
		log.Fatal("no sessions; run `login` first")
	}
	for _, e := range emails {
		if onlyEmail != "" && e != onlyEmail { continue }
		sess, err := db.GetSession(e)
		if err != nil {
			log.Printf("get session %s: %v", e, err)
			continue
		}
		client, err := api.NewClient(cfg.Server.Host, sess.TokenType, sess.AccessToken, cfg.Server.TimeoutMs)
		if err != nil {
			log.Printf("api client %s: %v", e, err)
			continue
		}
		f := &pipeline.Fetcher{Client: client, Store: db, Email: e}
		res := &pipeline.Result{}
		if err := fn(f, res); err != nil {
			log.Printf("  %v", err)
		}
		for _, w := range res.Errors {
			fmt.Printf("  warn: %s\n", w)
		}
	}
}

func runFetchAll(cfg *config.Config, db *store.Store, onlyEmail string, o *pipeline.Options) map[string]*pipeline.Result {
	emails, err := db.ListSessions()
	if err != nil {
		log.Fatalf("list sessions: %v", err)
	}
	out := map[string]*pipeline.Result{}
	for _, e := range emails {
		if onlyEmail != "" && e != onlyEmail { continue }
		sess, err := db.GetSession(e)
		if err != nil {
			log.Printf("get session %s: %v", e, err)
			continue
		}
		client, err := api.NewClient(cfg.Server.Host, sess.TokenType, sess.AccessToken, cfg.Server.TimeoutMs)
		if err != nil {
			log.Printf("api client %s: %v", e, err)
			continue
		}
		f := &pipeline.Fetcher{Client: client, Store: db, Email: e}
		res := &pipeline.Result{}
		fmt.Printf("== fetch %s ==\n", e)
		if err := f.FetchAccounts(o, res); err != nil {
			log.Printf("  accounts: %v", err)
		} else {
			fmt.Printf("  accounts: %d\n", res.Accounts)
		}
		if err := f.FetchUsage(o, res); err != nil {
			log.Printf("  usage: %v", err)
		} else {
			fmt.Printf("  usage: %d\n", res.Usages)
		}
		if err := f.FetchStats(o, res); err != nil {
			log.Printf("  stats: %v", err)
		} else {
			fmt.Printf("  stats models: %d\n", res.StatsModels)
		}
		if err := f.FetchLedger(o, res); err != nil {
			log.Printf("  ledger: %v", err)
		} else {
			fmt.Printf("  ledger: %d\n", res.LedgerItems)
		}
		fmt.Printf("  raw files: %d\n", len(res.RawFiles))
		for _, w := range res.Errors {
			fmt.Printf("  warn: %s\n", w)
		}
		out[e] = res
	}
	return out
}

func writeReport(email string, results map[string]*pipeline.Result, dir string) {
	for e, res := range results {
		if email != "" && e != email { continue }
		rep := &pipeline.Report{
			Email: e, StartedAt: time.Now().Add(-time.Second), FinishedAt: time.Now(),
			Accounts: res.Accounts, Usages: res.Usages, StatsModels: res.StatsModels,
			LedgerItems: res.LedgerItems, RawFiles: res.RawFiles, Errors: res.Errors,
		}
		path, err := rep.Write(dir)
		if err != nil {
			log.Printf("write report %s: %v", e, err)
			continue
		}
		fmt.Println("report ->", path)
	}
}

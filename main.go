package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"ai-pixel-analysis/api"
	"ai-pixel-analysis/auth"
	"ai-pixel-analysis/config"
	"ai-pixel-analysis/pipeline"
	"ai-pixel-analysis/store"
	"ai-pixel-analysis/web"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	envPath, dbPath := "", "ai-pixel.db"
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	fs.StringVar(&envPath, "env", "", "path to .env file")
	fs.StringVar(&dbPath, "db", "ai-pixel.db", "sqlite db path")

	var email string
	var o pipeline.Options
	var webAddr string
	fs.StringVar(&email, "email", "", "指定账号，默认全部")
	fs.StringVar(&o.Period, "period", "", "today|yesterday|last7days")
	fs.StringVar(&o.StartDate, "start", "", "YYYY-MM-DD")
	fs.StringVar(&o.EndDate, "end", "", "YYYY-MM-DD")
	fs.StringVar(&o.StartTime, "start-time", "", "RFC3339")
	fs.StringVar(&o.EndTime, "end-time", "", "RFC3339")
	fs.IntVar(&o.PageSize, "page-size", 100, "")
	fs.IntVar(&o.MaxPages, "max-pages", 200, "")
	fs.StringVar(&o.RawDir, "raw-dir", "data/raw", "")
	fs.StringVar(&o.ExportDir, "export-dir", "data/export", "")
	fs.StringVar(&webAddr, "addr", ":8080", "web 监听地址")

	switch cmd {
	case "login":
		fs.Parse(args)
		cfg, db := mustSetup(envPath, dbPath)
		defer db.Close()
		runLogin(cfg, db)
	case "fetch":
		fs.Parse(args)
		cfg, db := mustSetup(envPath, dbPath)
		defer db.Close()
		runFetch(cfg, db, email, o)
	case "export":
		fs.Parse(args)
		_, db := mustSetup(envPath, dbPath)
		defer db.Close()
		runExport(db, email, o.ExportDir)
	case "web":
		fs.Parse(args)
		runWeb(dbPath, webAddr)
	case "all":
		fs.Parse(args)
		cfg, db := mustSetup(envPath, dbPath)
		defer db.Close()
		runLogin(cfg, db)
		runFetch(cfg, db, email, o)
		runExport(db, email, o.ExportDir)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println(`用法:
  ai-pixel-analysis <command> [flags]

命令:
  login    登录所有 .env 账号，保存 authorization/cookie 到 sqlite
  fetch    抓取使用明细+余额流水，原始落盘+清洗入库
  export   从数据库导出 md 报告
  web      启动数据分析 web 界面（只读）
  all      login + fetch + export 一键执行

公共 flag:
  -env <path>   .env 路径
  -db <path>    sqlite 路径 (默认 ai-pixel.db)

fetch/export flag:
  -email <addr>       只处理指定账号
  -period today       快捷周期 (today|yesterday|last7days)
  -start YYYY-MM-DD   起始日期
  -end YYYY-MM-DD     结束日期
  -start-time RFC3339 精确起始时间
  -end-time RFC3339   精确结束时间
  -page-size N        每页条数 (默认 100)
  -max-pages N        分页上限 (默认 200)
  -raw-dir <dir>      原始响应目录 (默认 data/raw)
  -export-dir <dir>   导出目录 (默认 data/export)

web flag:
  -addr <addr>        监听地址 (默认 :8080)，访问 http://localhost:8080`)
}

func mustSetup(envPath, dbPath string) (*config.Config, *store.Store) {
	ep := envPath
	if ep == "" {
		ep = config.DefaultEnvPath()
		if _, err := os.Stat(ep); err != nil {
			ep = ".env"
		}
	}
	cfg, err := config.Load(ep)
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
	client, err := auth.NewClient(cfg.Host)
	if err != nil {
		log.Fatalf("new auth client: %v", err)
	}
	revision, err := client.FetchAgreementRevision(cfg.LoginPort)
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
		ar := store.FromLoginResult(res.AccessToken, res.RefreshToken, res.TokenType, res.ExpiresIn, res.Cookies, res.User)
		if err := db.SaveSession(u.Name, ar); err != nil {
			fmt.Printf("save FAIL %v\n", err)
			continue
		}
		fmt.Printf("ok expires_in=%ds\n", res.ExpiresIn)
	}
}

func runFetch(cfg *config.Config, db *store.Store, onlyEmail string, o pipeline.Options) {
	emails, err := db.ListSessions()
	if err != nil {
		log.Fatalf("list sessions: %v", err)
	}
	if len(emails) == 0 {
		log.Fatal("no sessions; run `login` first")
	}
	for _, e := range emails {
		if onlyEmail != "" && e != onlyEmail {
			continue
		}
		sess, err := db.GetSession(e)
		if err != nil {
			log.Printf("get session %s: %v", e, err)
			continue
		}
		client, err := api.NewClient(cfg.Host, sess.TokenType, sess.AccessToken)
		if err != nil {
			log.Printf("api client %s: %v", e, err)
			continue
		}
		f := &pipeline.Fetcher{Client: client, Store: db, Email: e}
		res := &pipeline.Result{}
		fmt.Printf("== fetch %s ==\n", e)
		if err := f.FetchUsage(o, res); err != nil {
			log.Printf("  usage: %v", err)
		} else {
			fmt.Printf("  usage: %d pages %d items\n", res.UsagePages, res.UsageItems)
		}
		if err := f.FetchLedger(o, res); err != nil {
			log.Printf("  ledger: %v", err)
		} else {
			fmt.Printf("  ledger: %d pages %d items\n", res.LedgerPages, res.LedgerItems)
		}
		if err := f.FetchSnapshots(o, res); err != nil {
			log.Printf("  snapshots: %v", err)
		}
		fmt.Printf("  raw files: %d\n", len(res.RawFiles))
		for _, w := range res.Errors {
			fmt.Printf("  warn: %s\n", w)
		}
	}
}

func runExport(db *store.Store, onlyEmail, exportDir string) {
	emails, err := db.ListSessions()
	if err != nil {
		log.Fatalf("list sessions: %v", err)
	}
	for _, e := range emails {
		if onlyEmail != "" && e != onlyEmail {
			continue
		}
		path, err := pipeline.ExportMarkdown(db, e, exportDir)
		if err != nil {
			log.Printf("export %s: %v", e, err)
			continue
		}
		abs, _ := filepath.Abs(path)
		fmt.Println("export", e, "->", abs)
	}
}

// runWeb 以只读模式打开数据库并启动分析 web 服务。
// 用 OpenReadonly 而非 mustSetup：web 不需要 .env/db_secret（不接触加密凭据），
// 且 mode=ro 在驱动层就禁止任何写操作，落实"严禁增删改"。
func runWeb(dbPath, addr string) {
	db, err := store.OpenReadonly(dbPath)
	if err != nil {
		log.Fatalf("open db readonly: %v", err)
	}
	defer db.Close()
	if err := web.NewServer(db).Listen(addr); err != nil {
		log.Fatalf("web server: %v", err)
	}
}

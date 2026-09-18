package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"ai-pixel-analysis/auth"
	"ai-pixel-analysis/config"
	"ai-pixel-analysis/store"
)

func main() {
	envPath := flag.String("env", "", "path to .env file")
	dbPath := flag.String("db", "ai-pixel.db", "sqlite db path")
	flag.Parse()

	ep := *envPath
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
	fmt.Printf("host=%s login_port=%s users=%d\n", cfg.Host, cfg.LoginPort, len(cfg.Users))

	db, err := store.New(*dbPath, cfg.DBSecret)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	absDB, _ := filepath.Abs(*dbPath)
	fmt.Println("db:", absDB)

	client, err := auth.NewClient(cfg.Host)
	if err != nil {
		log.Fatalf("new client: %v", err)
	}

	revision, err := client.FetchAgreementRevision(cfg.LoginPort)
	if err != nil {
		log.Fatalf("fetch agreement revision: %v", err)
	}
	if revision == "" {
		fmt.Println("login agreement not enabled; login without revision")
	} else {
		fmt.Println("agreement revision:", revision)
	}

	for _, u := range cfg.Users {
		fmt.Printf("logging in %s ...\n", u.Name)
		res, err := client.Login(u.Name, u.Password, revision)
		if err != nil {
			log.Printf("  login %s failed: %v", u.Name, err)
			continue
		}
		fmt.Printf("  ok: expires_in=%ds cookies=%d bytes\n", res.ExpiresIn, len(res.Cookies))
		ar := store.FromLoginResult(res.AccessToken, res.RefreshToken, res.TokenType, res.ExpiresIn, res.Cookies, res.User)
		if err := db.SaveSession(u.Name, ar); err != nil {
			log.Printf("  save %s failed: %v", u.Name, err)
			continue
		}
		fmt.Println("  saved")
	}

	emails, _ := db.ListSessions()
	fmt.Println("sessions:", emails)
	for _, e := range emails {
		s, err := db.GetSession(e)
		if err != nil {
			log.Printf("  get %s: %v", e, err)
			continue
		}
		fmt.Printf("  %s: token_len=%d exp=%s cookie_len=%d\n",
			e, len(s.AccessToken), s.ExpiresAt.Format("2006-01-02 15:04:05"), len(s.Cookies))
	}
}

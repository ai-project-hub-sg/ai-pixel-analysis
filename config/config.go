package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// User 代表 .env 中一组 user_* 凭据
type User struct {
	Name     string
	Password string
}

// Config 保存从 .env 加载的全部配置
type Config struct {
	Host       string
	LoginPort  string
	DBSecret   string
	Users      []User // 按编号排序
}

// Load 读取 .env 文件并解析配置
func Load(envPath string) (*Config, error) {
	f, err := os.Open(envPath)
	if err != nil {
		return nil, fmt.Errorf("open .env: %w", err)
	}
	defer f.Close()

	kv := make(map[string]string)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		k := strings.TrimSpace(line[:idx])
		v := strings.TrimSpace(line[idx+1:])
		v = strings.Trim(v, "\"'")
		kv[k] = v
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan .env: %w", err)
	}

	cfg := &Config{
		Host:      kv["host"],
		LoginPort: kv["login_port"],
		DBSecret:  kv["db_secret"],
	}
	if cfg.Host == "" {
		return nil, fmt.Errorf("missing host in .env")
	}
	if cfg.DBSecret == "" {
		return nil, fmt.Errorf("missing db_secret in .env: refusing to store credentials in plaintext")
	}

	// 解析 user_name_N / user_password_N (兼容 user_passwor_N 拼写)
	reName := regexp.MustCompile(`^user_name_(\d+)$`)
	rePass := regexp.MustCompile(`^user_passwor[d]?_(\d+)$`)
	names := map[int]string{}
	passes := map[int]string{}
	for k, v := range kv {
		if m := reName.FindStringSubmatch(k); m != nil {
			n, _ := strconv.Atoi(m[1])
			names[n] = v
		}
		if m := rePass.FindStringSubmatch(k); m != nil {
			n, _ := strconv.Atoi(m[1])
			passes[n] = v
		}
	}
	var ids []int
	for n := range names {
		ids = append(ids, n)
	}
	sort.Ints(ids)
	for _, n := range ids {
		pw, ok := passes[n]
		if !ok {
			return nil, fmt.Errorf("user_name_%d has no matching password entry", n)
		}
		cfg.Users = append(cfg.Users, User{Name: names[n], Password: pw})
	}
	if len(cfg.Users) == 0 {
		return nil, fmt.Errorf("no user_name_N entries found in .env")
	}
	return cfg, nil
}

// DefaultEnvPath 返回项目根目录下 .env 路径
func DefaultEnvPath() string {
	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	return filepath.Join(dir, ".env")
}

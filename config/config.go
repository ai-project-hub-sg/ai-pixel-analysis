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

// Server 是 config.toml 中的服务地址配置
type Server struct {
	Host      string // 完整 scheme://host，如 https://ai-pixel.online
	Port      int    // 可选；0 表示用默认 443/80
	TimeoutMs int    // 请求超时毫秒
}

// Config 聚合 .env 机密 + config.toml 服务配置
type Config struct {
	Server   Server
	DBSecret string
	Users    []User // 按编号排序
}

// LoadEnv 只读 .env，返回键值表（不回显值）
func LoadEnv(envPath string) (map[string]string, error) {
	f, err := os.Open(envPath)
	if err != nil {
		return nil, fmt.Errorf("open .env: %w", err)
	}
	defer f.Close()
	kv := map[string]string{}
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
	return kv, sc.Err()
}

// LoadToml 极简 TOML 解析：仅支持 key=value 与 [section]，满足本项目配置需求
func LoadToml(path string) (map[string]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config.toml: %w", err)
	}
	defer f.Close()
	out := map[string]map[string]string{"": {}}
	section := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			out[section] = map[string]string{}
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		k := strings.TrimSpace(line[:idx])
		v := strings.TrimSpace(line[idx+1:])
		v = strings.Trim(v, "\"'")
		out[section][k] = v
	}
	return out, sc.Err()
}

// Load 合并 .env 与 config.toml，生成最终配置
func Load(envPath, tomlPath string) (*Config, error) {
	kv, err := LoadEnv(envPath)
	if err != nil {
		return nil, err
	}
	tl, err := LoadToml(tomlPath)
	if err != nil {
		return nil, err
	}
	srv := Server{TimeoutMs: 30000}
	if s, ok := tl["server"]; ok {
		srv.Host = s["host"]
		if p, err := strconv.Atoi(s["port"]); err == nil {
			srv.Port = p
		}
		if t, err := strconv.Atoi(s["timeout_ms"]); err == nil && t > 0 {
			srv.TimeoutMs = t
		}
	}
	if srv.Host == "" {
		return nil, fmt.Errorf("config.toml missing [server].host")
	}
	cfg := &Config{Server: srv, DBSecret: kv["db_secret"]}
	if cfg.DBSecret == "" {
		return nil, fmt.Errorf("missing db_secret in .env")
	}
	reName := regexp.MustCompile(`^user_name_(\d+)$`)
	rePass := regexp.MustCompile(`^user_passwor[d]?_(\d+)$`)
	names, passes := map[int]string{}, map[int]string{}
	for k, v := range kv {
		if m := reName.FindStringSubmatch(k); m != nil {
			n, _ := strconv.Atoi(m[1]); names[n] = v
		}
		if m := rePass.FindStringSubmatch(k); m != nil {
			n, _ := strconv.Atoi(m[1]); passes[n] = v
		}
	}
	var ids []int
	for n := range names { ids = append(ids, n) }
	sort.Ints(ids)
	for _, n := range ids {
		pw, ok := passes[n]
		if !ok {
			return nil, fmt.Errorf("user_name_%d 无对应密码项", n)
		}
		cfg.Users = append(cfg.Users, User{Name: names[n], Password: pw})
	}
	if len(cfg.Users) == 0 {
		return nil, fmt.Errorf(".env 中未找到 user_name_N 用户组")
	}
	return cfg, nil
}

// DefaultPath 返回可执行文件同目录下的文件路径；不存在则回退到当前目录
func DefaultPath(name string) string {
	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	p := filepath.Join(dir, name)
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return name
}

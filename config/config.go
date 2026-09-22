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

// Server 是 [server] 节的统一地址配置
type Server struct {
	Host      string
	Port      int
	TimeoutMs int
}

// BaseURL 返回服务基础地址。port 为 443/80/0 时返回 host；非标准拼 host:port。
func (s Server) BaseURL() string {
	h := strings.TrimRight(s.Host, "/")
	if s.Port == 0 || s.Port == 80 || s.Port == 443 {
		return h
	}
	return fmt.Sprintf("%s:%d", h, s.Port)
}

// Config 聚合 .env 机密 + config.toml 服务与各端点路径
type Config struct {
	Server    Server
	Endpoints map[string]string // key: login_page|login|accounts|usage|stats|ledger
	DBSecret  string
	Users     []User // 按编号排序
}

// Endpoint 取指定接口路径；不存在时返回错误提示在 config.toml 补齐。
func (c *Config) Endpoint(name string) (string, error) {
	p, ok := c.Endpoints[name]
	if !ok || p == "" {
		return "", fmt.Errorf("config.toml [endpoints].%s 未配置", name)
	}
	return p, nil
}

// EndpointURL 返回完整接口 URL = Server.BaseURL() + 路径。
// 路径中的 {id} 由调用方先行替换。
func (c *Config) EndpointURL(name string) (string, error) {
	p, err := c.Endpoint(name)
	if err != nil {
		return "", err
	}
	return c.Server.BaseURL() + p, nil
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

// LoadToml 极简 TOML 解析：支持 [section] 与 key=value
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
	cfg := &Config{
		Server:    Server{TimeoutMs: 30000},
		Endpoints: map[string]string{},
		DBSecret:  kv["db_secret"],
	}
	if s, ok := tl["server"]; ok {
		cfg.Server.Host = s["host"]
		if p, err := strconv.Atoi(s["port"]); err == nil {
			cfg.Server.Port = p
		}
		if t, err := strconv.Atoi(s["timeout_ms"]); err == nil && t > 0 {
			cfg.Server.TimeoutMs = t
		}
	}
	if cfg.Server.Host == "" {
		return nil, fmt.Errorf("config.toml missing [server].host")
	}
	if e, ok := tl["endpoints"]; ok {
		for k, v := range e {
			cfg.Endpoints[k] = v
		}
	}
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

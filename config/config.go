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

// Endpoint 是单个接口的地址配置：host 与 port 都必须在 config.toml 中显式给出。
// URL = host（若 port 非标准则外部调用方负责拼接为 host:port）。
type Endpoint struct {
	Host      string
	Port      int
	TimeoutMs int // 0 表示继承 defaults.timeout_ms
}

// BaseURL 返回该端点的基础地址。port 为 443/80/0 时直接返回 host；
// 其余端口拼接为 host:port。不会做任何"非默认才填"的隐式假设。
func (e Endpoint) BaseURL() string {
	h := strings.TrimRight(e.Host, "/")
	if e.Port == 0 || e.Port == 80 || e.Port == 443 {
		return h
	}
	return fmt.Sprintf("%s:%d", h, e.Port)
}

// Config 聚合 .env 机密 + config.toml 各端点配置
type Config struct {
	Defaults  Defaults
	Endpoints map[string]Endpoint // key: login | accounts | usage | stats | ledger
	DBSecret  string
	Users     []User // 按编号排序
}

type Defaults struct {
	TimeoutMs int
}

// Endpoint 取指定端点；不存在时返回错误提示在 config.toml 中补齐。
func (c *Config) Endpoint(name string) (Endpoint, error) {
	e, ok := c.Endpoints[name]
	if !ok {
		return Endpoint{}, fmt.Errorf("config.toml 缺少 [endpoint.%s] 配置", name)
	}
	if e.Host == "" {
		return Endpoint{}, fmt.Errorf("config.toml [endpoint.%s].host 为空", name)
	}
	if e.TimeoutMs <= 0 {
		e.TimeoutMs = c.Defaults.TimeoutMs
	}
	return e, nil
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

// LoadToml 极简 TOML 解析：支持 [section] 与 [a.b] 两级、key=value。
// [endpoint.login] 解析为 section 名 "endpoint.login"。
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
		Defaults:  Defaults{TimeoutMs: 30000},
		Endpoints: map[string]Endpoint{},
		DBSecret:  kv["db_secret"],
	}
	if d, ok := tl["defaults"]; ok {
		if t, err := strconv.Atoi(d["timeout_ms"]); err == nil && t > 0 {
			cfg.Defaults.TimeoutMs = t
		}
	}
	// 解析所有 [endpoint.<name>] 节
	for sec, kv2 := range tl {
		if !strings.HasPrefix(sec, "endpoint.") {
			continue
		}
		name := strings.TrimPrefix(sec, "endpoint.")
		e := Endpoint{Host: kv2["host"]}
		if p, err := strconv.Atoi(kv2["port"]); err == nil {
			e.Port = p
		}
		if t, err := strconv.Atoi(kv2["timeout_ms"]); err == nil && t > 0 {
			e.TimeoutMs = t
		}
		cfg.Endpoints[name] = e
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

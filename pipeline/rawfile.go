package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// safeName 把账号/参数压成文件名安全的形式
func safeName(s string) string {
	s = nonAlnum.ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}

// rawFilePath 生成原始响应落盘路径:
//
//	data/raw/<category>/<email>_<range>_p<page>.json
//	data/raw/<category>/<email>_<range>.json        (非分页快照)
func rawFilePath(rawDir, category, email, rangeDesc string, page int) string {
	sub := filepath.Join(rawDir, category)
	rng := safeName(rangeDesc)
	if rng == "" {
		rng = "all"
	}
	name := fmt.Sprintf("%s__%s", safeName(email), rng)
	if page > 0 {
		name += fmt.Sprintf("_p%d", page)
	}
	return filepath.Join(sub, name+".json")
}

// writeRawFile 把原始响应落盘；文件内记录抓取时间与参数便于回溯
func writeRawFile(path, category, email, params string, body json.RawMessage) error {
	if len(body) == 0 {
		return fmt.Errorf("empty body")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	rec := map[string]interface{}{
		"category":   category,
		"email":      email,
		"params":     params,
		"fetched_at": time.Now().Format(time.RFC3339),
		"response":   json.RawMessage(body),
	}
	buf, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, buf, 0o644)
}

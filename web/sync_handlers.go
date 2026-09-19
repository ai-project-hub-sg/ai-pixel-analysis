package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
)

// 同步 API 仅在 Server 持有 worker（即 web 以可写+env 模式启动）时注册。

// POST /api/sync/init —— 初始化：拉最近3天（仅当库中无数据）
func (s *Server) handleSyncInit(w http.ResponseWriter, r *http.Request) (any, error) {
	if r.Method != http.MethodPost {
		return nil, errMethod()
	}
	if s.sync == nil {
		return nil, errSyncDisabled()
	}
	if has, _ := s.sync.deps.Store.HasAnyData(); has {
		return map[string]any{"started": false, "reason": "database already has data; init is only for first run"}, nil
	}
	id, busy, err := s.sync.StartJob("init")
	if err != nil {
		if errors.Is(err, ErrNoSession) {
			return map[string]any{"started": false, "auth_required": true, "error": "登录已失效或无会话，请重新登录"}, nil
		}
		return nil, err
	}
	return map[string]any{"started": true, "job_id": id, "busy": busy}, nil
}

// POST /api/sync/update —— 手动更新：从水位/最近数据更新到前一分钟
func (s *Server) handleSyncUpdate(w http.ResponseWriter, r *http.Request) (any, error) {
	if r.Method != http.MethodPost {
		return nil, errMethod()
	}
	if s.sync == nil {
		return nil, errSyncDisabled()
	}
	id, busy, err := s.sync.StartJob("manual")
	if err != nil {
		if errors.Is(err, ErrNoSession) {
			return map[string]any{"started": false, "auth_required": true, "error": "登录已失效或无会话，请重新登录"}, nil
		}
		return nil, err
	}
	return map[string]any{"started": true, "job_id": id, "busy": busy}, nil
}

// POST /api/sync/backfill —— 回填最近30天（单独大任务，区别于增量更新）
func (s *Server) handleSyncBackfill(w http.ResponseWriter, r *http.Request) (any, error) {
	if r.Method != http.MethodPost {
		return nil, errMethod()
	}
	if s.sync == nil {
		return nil, errSyncDisabled()
	}
	id, busy, err := s.sync.StartJob("backfill")
	if err != nil {
		if errors.Is(err, ErrNoSession) {
			return map[string]any{"started": false, "auth_required": true, "error": "登录已失效或无会话，请重新登录"}, nil
		}
		return nil, err
	}
	return map[string]any{"started": true, "job_id": id, "busy": busy}, nil
}

// POST /api/sync/auto {on:true|false}
func (s *Server) handleSyncAuto(w http.ResponseWriter, r *http.Request) (any, error) {
	if r.Method != http.MethodPost {
		return nil, errMethod()
	}
	if s.sync == nil {
		return nil, errSyncDisabled()
	}
	var body struct {
		On bool `json:"on"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, err
	}
	if err := s.sync.SetAuto(body.On); err != nil {
		return nil, err
	}
	return map[string]any{"auto": s.sync.Auto()}, nil
}

// GET /api/sync/status —— 轮询进度
func (s *Server) handleSyncStatus(w http.ResponseWriter, r *http.Request) (any, error) {
	if s.sync == nil {
		return map[string]any{"sync_enabled": false}, nil
	}
	st, err := s.sync.Status()
	if err != nil {
		return nil, err
	}
	return map[string]any{"sync_enabled": true, "status": st}, nil
}

// GET /api/sync/jobs?limit=20 —— 历史任务
func (s *Server) handleSyncJobs(w http.ResponseWriter, r *http.Request) (any, error) {
	if s.sync == nil {
		return map[string]any{"jobs": []any{}}, nil
	}
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if n <= 0 || n > 100 {
		n = 20
	}
	jobs, err := s.sync.ListJobs(n)
	if err != nil {
		return nil, err
	}
	return map[string]any{"jobs": jobs}, nil
}

func errMethod() error { return &httpError{"method not allowed"} }
func errSyncDisabled() error {
	return &httpError{"sync not enabled: restart web with -sync flag (loads .env for credentials)"}
}

type httpError struct{ msg string }

func (e *httpError) Error() string { return e.msg }

// POST /api/sync/relogin —— 失登录后重新登录所有账号（在服务端执行，凭据不回显）。
// 重新登录成功后复位 authValid，自动同步可恢复。
func (s *Server) handleSyncRelogin(w http.ResponseWriter, r *http.Request) (any, error) {
	if r.Method != http.MethodPost {
		return nil, errMethod()
	}
	if s.sync == nil {
		return nil, errSyncDisabled()
	}
	res, err := s.sync.Relogin()
	if err != nil {
		return nil, err
	}
	return res, nil
}

package web

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"time"
)

// handleClosePref 记录用户在前端关闭弹窗中选择的行为。
// POST {"action":"keep"|"stop"}
//
//	keep = 关闭页面后服务驻留后台继续同步；stop = 关闭页面时关闭服务。
//
// 注意：该选择保存在进程内存中（服务驻留才有意义）；重启后默认回退为 stop，
// "不再提醒"勾选本身由浏览器 localStorage 持久化。
func (s *Server) handleClosePref(w http.ResponseWriter, r *http.Request) (any, error) {
	if r.Method != http.MethodPost {
		return nil, errMethod()
	}
	var body struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, err
	}
	if body.Action != "keep" && body.Action != "stop" {
		return map[string]any{"ok": false, "error": "action must be keep|stop"}, nil
	}
	s.mu.Lock()
	s.closeAction = body.Action
	s.mu.Unlock()
	return map[string]any{"ok": true, "action": body.Action}, nil
}

// handleShutdown 由前端"关闭页面时关闭服务"调用。
// 必须异步执行真正的 Shutdown：同步调用会等待当前请求结束，造成死锁。
// 只响应本地回环地址，避免局域网内他人访问页面时关掉你的服务。
func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) (any, error) {
	if r.Method != http.MethodPost {
		return nil, errMethod()
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || (host != "127.0.0.1" && host != "::1") {
		return map[string]any{"ok": false, "error": "shutdown only allowed from localhost"}, nil
	}
	s.shutdownOnce.Do(func() {
		go func() {
			defer close(s.shutdownDone)
			time.Sleep(150 * time.Millisecond) // 让响应先送达前端
			s.mu.Lock()
			srv := s.httpSrv
			sw := s.sync
			s.mu.Unlock()
			log.Printf("web: shutdown requested from UI, stopping server ...")
			if srv != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				srv.Shutdown(ctx)
				cancel()
			}
			if sw != nil {
				sw.Close()
			}
		}()
	})
	return map[string]any{"ok": true}, nil
}

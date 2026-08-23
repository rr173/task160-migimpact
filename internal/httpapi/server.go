// Package httpapi 提供 JSON HTTP API（路由前缀 /api）。
package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
	"reflect"
	"time"

	"task160-migimpact/internal/service"
)

// Server 持有 HTTP 处理器。
type Server struct {
	svc *service.Service
	mux *http.ServeMux
}

// New 构造 API 服务器。
func New(svc *service.Service) *Server {
	s := &Server{svc: svc, mux: http.NewServeMux()}
	s.routes()
	return s
}

// Handler 返回根处理器。
func (s *Server) Handler() http.Handler {
	return logRequests(s.mux)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

// routes 注册全部路由。
func (s *Server) routes() {
	// 快照
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("POST /api/snapshots", s.handleCreateSnapshot)
	s.mux.HandleFunc("GET /api/snapshots", s.handleListSnapshots)
	s.mux.HandleFunc("GET /api/snapshots/{id}", s.handleGetSnapshot)
	s.mux.HandleFunc("POST /api/snapshots/{id}/objects", s.handleLoadSnapshot)
	s.mux.HandleFunc("GET /api/snapshots/{id}/objects", s.handleGetObjects)
	s.mux.HandleFunc("GET /api/snapshots/{id}/dependencies", s.handleGetDeps)
	s.mux.HandleFunc("GET /api/snapshots/{id}/access", s.handleGetAccess)
	// 脚本
	s.mux.HandleFunc("POST /api/scripts", s.handleImportScript)
	s.mux.HandleFunc("GET /api/scripts", s.handleListScripts)
	s.mux.HandleFunc("GET /api/scripts/{id}", s.handleGetScript)
	s.mux.HandleFunc("GET /api/scripts/{id}/steps", s.handleGetScriptSteps)
	// 分析
	s.mux.HandleFunc("POST /api/scripts/{id}/analyze", s.handleRunAnalysis)
	s.mux.HandleFunc("GET /api/analyses", s.handleListAnalyses)
	s.mux.HandleFunc("GET /api/analyses/{id}", s.handleGetAnalysis)
	s.mux.HandleFunc("GET /api/analyses/{id}/changes", s.handleListChanges)
	s.mux.HandleFunc("POST /api/analyses/{id}/exemptions", s.handleSubmitExemption)
	s.mux.HandleFunc("GET /api/analyses/{id}/exemptions", s.handleListExemptions)
	// 计划
	s.mux.HandleFunc("POST /api/plans", s.handleFreezePlan)
	s.mux.HandleFunc("GET /api/plans", s.handleListPlans)
	s.mux.HandleFunc("GET /api/plans/{id}", s.handleGetPlan)
	s.mux.HandleFunc("GET /api/plans/{id}/diff", s.handlePlanDiff)
	// 其他
	s.mux.HandleFunc("POST /api/example", s.handleExample)
	s.mux.HandleFunc("GET /api/audit", s.handleAudit)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "task160-migimpact"})
}

// writeJSON 统一 JSON 响应。
// 空集合（含 nil 切片）序列化为 []，而非 panic 或返回 null，
// 使列表接口在无数据时仍能正常响应。
func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Slice && rv.Len() == 0 {
		// 统一空集合为非 nil 空切片，确保 JSON 输出 []。
		v = reflect.MakeSlice(rv.Type(), 0, 0).Interface()
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	_ = enc.Encode(v)
}

// writeErr 统一错误响应。
func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

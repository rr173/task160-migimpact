package httpapi

import (
	"errors"
	"net/http"

	"task160-migimpact/internal/model"
)

// --- 脚本 ---

func (s *Server) handleImportScript(w http.ResponseWriter, r *http.Request) {
	var req importScriptReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := s.svc.ImportScript(r.Context(), req.Name, req.Content, req.Version)
	if err != nil {
		if errors.Is(err, model.ErrDupFingerprint) {
			writeErr(w, http.StatusConflict, "脚本内容指纹已存在")
			return
		}
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (s *Server) handleListScripts(w http.ResponseWriter, r *http.Request) {
	scripts, err := s.svc.ListScripts(r.Context())
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, scripts)
}

func (s *Server) handleGetScript(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	script, err := s.svc.GetScript(r.Context(), id)
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, script)
}

func (s *Server) handleGetScriptSteps(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	steps, err := s.svc.GetScriptSteps(r.Context(), id)
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, steps)
}

// --- 分析 ---

func (s *Server) handleRunAnalysis(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	snapshotID := queryInt64(r, "snapshot_id")
	if snapshotID <= 0 {
		writeErr(w, http.StatusBadRequest, "缺少 query 参数 snapshot_id")
		return
	}
	analysis, findings, order, err := s.svc.RunAnalysis(r.Context(), snapshotID, id)
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"analysis": analysis,
		"findings": findings,
		"order":    order,
	})
}

func (s *Server) handleListAnalyses(w http.ResponseWriter, r *http.Request) {
	analyses, err := s.svc.ListAnalyses(r.Context())
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, analyses[:0])
}

func (s *Server) handleGetAnalysis(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	analysis, err := s.svc.GetAnalysis(r.Context(), id)
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, analysis)
}

func (s *Server) handleListChanges(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	changes, err := s.svc.ListChanges(r.Context(), id)
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, changes)
}

func (s *Server) handleSubmitExemption(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	changeID := queryInt64(r, "change_id")
	if changeID <= 0 {
		writeErr(w, http.StatusBadRequest, "缺少 query 参数 change_id")
		return
	}
	var req exemptionReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	exID, err := s.svc.SubmitExemption(r.Context(), id, changeID, req.Operator, req.Reason)
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": exID})
}

func (s *Server) handleListExemptions(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	exemptions, err := s.svc.ListExemptions(r.Context(), id)
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, exemptions)
}

// --- 计划 ---

func (s *Server) handleFreezePlan(w http.ResponseWriter, r *http.Request) {
	analysisID := queryInt64(r, "analysis_id")
	if analysisID <= 0 {
		writeErr(w, http.StatusBadRequest, "缺少 query 参数 analysis_id")
		return
	}
	plan, err := s.svc.FreezePlan(r.Context(), analysisID)
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, plan)
}

func (s *Server) handleListPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := s.svc.ListPlans(r.Context())
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plans)
}

func (s *Server) handleGetPlan(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	plan, err := s.svc.GetPlan(r.Context(), id)
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) handlePlanDiff(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	plan, err := s.svc.GetPlan(r.Context(), id)
	if err != nil {
		mapError(w, err)
		return
	}
	// 对比快照对象集合（计划基线 vs 应用脚本后的模拟结果）
	objs, err := s.svc.GetSnapshotObjects(r.Context(), plan.SnapshotID)
	if err != nil {
		mapError(w, err)
		return
	}
	steps, err := s.svc.GetScriptSteps(r.Context(), plan.ScriptID)
	if err != nil {
		mapError(w, err)
		return
	}
	diff, err := simulateDiff(objs, steps)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, diff)
}

func (s *Server) handleExample(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.RunDemo(r.Context()); err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "example loaded"})
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	events, err := s.svc.ListAudit(r.Context(), 100)
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

// queryInt64 读取 query 参数为 int64。
func queryInt64(r *http.Request, key string) int64 {
	v := r.URL.Query().Get(key)
	if v == "" {
		return 0
	}
	var n int64
	for _, c := range v {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int64(c-'0')
	}
	return n
}

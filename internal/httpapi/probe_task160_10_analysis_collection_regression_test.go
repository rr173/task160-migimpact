package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"task160-migimpact/internal/model"
	"task160-migimpact/internal/service"
	"task160-migimpact/internal/store"
)

func TestTask160Bug10_AnalysisCollectionReturnsCompletedAnalysis(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	repos := store.NewRepositories(st)
	now := time.Now()
	snapshotID, err := repos.Snapshots.CreateSnapshot(ctx, &model.SchemaSnapshot{Name: "analysis-list", Status: model.SnapshotRegistered, CreatedAt: now})
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	scriptID, err := repos.Scripts.CreateScript(ctx, &model.MigrationScript{Name: "analysis-list", Version: 1, Status: model.ScriptApproved, ContentHash: "analysis-list", Content: "manual", CreatedAt: now})
	if err != nil {
		t.Fatalf("CreateScript: %v", err)
	}
	if _, err := repos.Analyses.CreateAnalysis(ctx, &model.ImpactAnalysis{SnapshotID: snapshotID, ScriptID: scriptID, Status: model.AnalysisCompleted, InputHash: "analysis-list", CreatedAt: now, CompletedAt: &now}); err != nil {
		t.Fatalf("CreateAnalysis: %v", err)
	}
	server := New(service.New(repos))
	request := httptest.NewRequest(http.MethodGet, "/api/analyses", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status: %d", response.Code)
	}
	var analyses []model.ImpactAnalysis
	if err := json.Unmarshal(response.Body.Bytes(), &analyses); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(analyses) != 1 || analyses[0].Status != model.AnalysisCompleted {
		t.Fatalf("collection response must include completed analysis: %+v", analyses)
	}
}

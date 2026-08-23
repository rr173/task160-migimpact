package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"task160-migimpact/internal/model"
	"task160-migimpact/internal/service"
	"task160-migimpact/internal/store"
)

// TestListScriptsReturnsImported 修复回归：存在已导入脚本时
// GET /api/scripts 必须返回这些脚本，而不是因空集合 panic 崩溃。
func TestListScriptsReturnsImported(t *testing.T) {
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	srv := New(service.New(store.NewRepositories(st)))

	if _, err := srv.svc.ImportScript(context.Background(),
		"m1", "1 CREATE_TABLE [orders]\n", 1); err != nil {
		t.Fatalf("ImportScript: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/scripts", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，想要 200；body=%s", rec.Code, rec.Body.String())
	}
	var got []model.MigrationScript
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("解析响应: %v；body=%s", err, rec.Body.String())
	}
	if len(got) != 1 || got[0].Name != "m1" {
		t.Fatalf("应返回 1 个已导入脚本 m1，得到 %+v", got)
	}
}

// TestListScriptsEmptyReturnsArray 空集合时返回 [] 而非 panic。
func TestListScriptsEmptyReturnsArray(t *testing.T) {
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	srv := New(service.New(store.NewRepositories(st)))

	req := httptest.NewRequest(http.MethodGet, "/api/scripts", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，想要 200；body=%s", rec.Code, rec.Body.String())
	}
	if trimmed := strings.TrimSpace(rec.Body.String()); trimmed != "[]" {
		t.Fatalf("空集合应返回 []，得到 %q", trimmed)
	}
}

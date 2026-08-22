package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"task160-migimpact/internal/model"
	"task160-migimpact/internal/service"
	"task160-migimpact/internal/store"
)

func TestTask160Bug09_CollectionEndpointsReturnCreatedEntries(t *testing.T) {
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	svc := service.New(store.NewRepositories(st))
	if _, err := svc.ImportScript(context.Background(), "reporting", "1 CREATE_TABLE [reports]", 1); err != nil {
		t.Fatalf("ImportScript: %v", err)
	}
	server := New(svc)
	request := httptest.NewRequest(http.MethodGet, "/api/scripts", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status: %d", response.Code)
	}
	var scripts []model.MigrationScript
	if err := json.Unmarshal(response.Body.Bytes(), &scripts); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(scripts) != 1 || scripts[0].Name != "reporting" {
		t.Fatalf("collection response must include created script: %+v", scripts)
	}
}

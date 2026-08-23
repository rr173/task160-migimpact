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

func TestTask160Bug08_SnapshotListIsChronologicalAtAPI(t *testing.T) {
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	svc := service.New(store.NewRepositories(st))
	for _, name := range []string{"first", "second", "third"} {
		if _, err := svc.CreateSnapshot(context.Background(), name); err != nil {
			t.Fatalf("CreateSnapshot %s: %v", name, err)
		}
	}
	server := New(svc)
	request := httptest.NewRequest(http.MethodGet, "/api/snapshots", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status: %d", response.Code)
	}
	var snapshots []model.SchemaSnapshot
	if err := json.Unmarshal(response.Body.Bytes(), &snapshots); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(snapshots) != 3 || snapshots[0].Name != "first" || snapshots[1].Name != "second" || snapshots[2].Name != "third" {
		t.Fatalf("API must expose snapshots in chronological creation order: %+v", snapshots)
	}
}

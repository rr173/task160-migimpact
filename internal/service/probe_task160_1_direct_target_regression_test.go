package service

import (
	"context"
	"testing"

	"task160-migimpact/internal/model"
	"task160-migimpact/internal/store"
)

func TestTask160Bug01_DirectTargetSurvivesAnalysisPersistenceAndReload(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	path := st.Path()
	svc := New(store.NewRepositories(st))
	snapshotID, err := svc.CreateSnapshot(ctx, "direct-target")
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	objects := []model.SchemaObject{{ObjType: "table", Name: "orders"}}
	access := []model.AccessDeclaration{{Service: "checkout", Interface: "/api/orders", Action: "read", TargetObj: "table:orders"}}
	if err := svc.LoadSnapshotObjects(ctx, snapshotID, objects, nil, access); err != nil {
		t.Fatalf("LoadSnapshotObjects: %v", err)
	}
	scriptID, err := svc.ImportScript(ctx, "drop-orders", "1 DROP_TABLE [orders]", 1)
	if err != nil {
		t.Fatalf("ImportScript: %v", err)
	}
	analysis, findings, _, err := svc.RunAnalysis(ctx, snapshotID, scriptID)
	if err != nil {
		t.Fatalf("RunAnalysis: %v", err)
	}
	if len(findings) != 1 || !hasTask160Bug01Value(findings[0].AffectedObjs, "table:orders") || !hasTask160Bug01Value(findings[0].Interfaces, "checkout /api/orders read") {
		t.Fatalf("directly accessed target must appear in analysis finding: %+v", findings)
	}
	changes, err := svc.ListChanges(ctx, analysis.ID)
	if err != nil {
		t.Fatalf("ListChanges before restart: %v", err)
	}
	if len(changes) != 1 || !hasTask160Bug01Value(changes[0].AffectedObjs, "table:orders") {
		t.Fatalf("directly accessed target must persist with change: %+v", changes)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	reopened, err := store.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	reloaded := New(store.NewRepositories(reopened))
	changes, err = reloaded.ListChanges(ctx, analysis.ID)
	if err != nil {
		t.Fatalf("ListChanges after restart: %v", err)
	}
	if len(changes) != 1 || !hasTask160Bug01Value(changes[0].AffectedObjs, "table:orders") {
		t.Fatalf("directly accessed target must survive reload: %+v", changes)
	}
}

func hasTask160Bug01Value(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

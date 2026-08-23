package service

import (
	"context"
	"testing"

	"task160-migimpact/internal/model"
	"task160-migimpact/internal/store"
)

func TestTask160Bug03_AccessDeclarationHasOneImpactEntry(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	svc := New(store.NewRepositories(st))
	snapshotID, err := svc.CreateSnapshot(ctx, "access-cardinality")
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	access := []model.AccessDeclaration{{Service: "billing", Interface: "/api/bills", Action: "read", TargetObj: "table:invoices"}}
	if err := svc.LoadSnapshotObjects(ctx, snapshotID, []model.SchemaObject{{ObjType: "table", Name: "invoices"}}, nil, access); err != nil {
		t.Fatalf("LoadSnapshotObjects: %v", err)
	}
	stored, err := svc.GetSnapshotAccess(ctx, snapshotID)
	if err != nil {
		t.Fatalf("GetSnapshotAccess: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("one submitted access declaration must persist once, got %d", len(stored))
	}
	scriptID, err := svc.ImportScript(ctx, "drop-invoices", "1 DROP_TABLE [invoices]", 1)
	if err != nil {
		t.Fatalf("ImportScript: %v", err)
	}
	_, findings, _, err := svc.RunAnalysis(ctx, snapshotID, scriptID)
	if err != nil {
		t.Fatalf("RunAnalysis: %v", err)
	}
	if len(findings) != 1 || len(findings[0].Interfaces) != 1 || findings[0].Interfaces[0] != "billing /api/bills read" {
		t.Fatalf("one declaration must create one interface impact entry: %+v", findings)
	}
}

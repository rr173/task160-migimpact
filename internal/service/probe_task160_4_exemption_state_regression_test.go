package service

import (
	"context"
	"testing"

	"task160-migimpact/internal/model"
	"task160-migimpact/internal/plan"
	"task160-migimpact/internal/store"
)

func TestTask160Bug04_ApprovedExemptionUnblocksItsChange(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	svc := New(store.NewRepositories(st))
	snapshotID, err := svc.CreateSnapshot(ctx, "exemption-state")
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if err := svc.LoadSnapshotObjects(ctx, snapshotID, []model.SchemaObject{{ObjType: "table", Name: "payments"}}, nil, nil); err != nil {
		t.Fatalf("LoadSnapshotObjects: %v", err)
	}
	scriptID, err := svc.ImportScript(ctx, "drop-payments", "1 DROP_TABLE [payments]", 1)
	if err != nil {
		t.Fatalf("ImportScript: %v", err)
	}
	analysis, _, _, err := svc.RunAnalysis(ctx, snapshotID, scriptID)
	if err != nil {
		t.Fatalf("RunAnalysis: %v", err)
	}
	changes, err := svc.ListChanges(ctx, analysis.ID)
	if err != nil || len(changes) != 1 {
		t.Fatalf("ListChanges: %v %+v", err, changes)
	}
	if _, err := svc.SubmitExemption(ctx, analysis.ID, changes[0].ID, "reviewer", "approved window"); err != nil {
		t.Fatalf("SubmitExemption: %v", err)
	}
	exemptions, err := svc.ListExemptions(ctx, analysis.ID)
	if err != nil || len(exemptions) != 1 {
		t.Fatalf("ListExemptions: %v %+v", err, exemptions)
	}
	if exemptions[0].Status != model.ExemptionApproved {
		t.Fatalf("submitted approval must remain approved, got %s", exemptions[0].Status)
	}
	if blockers := plan.RecomputeBlockers(changes, exemptions); len(blockers) != 0 {
		t.Fatalf("approved exemption must remove its own blocker, got %+v", blockers)
	}
}

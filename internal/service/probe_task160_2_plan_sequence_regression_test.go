package service

import (
	"context"
	"reflect"
	"testing"
	"time"

	"task160-migimpact/internal/impact"
	"task160-migimpact/internal/model"
	"task160-migimpact/internal/store"
)

func TestTask160Bug02_FrozenPlanKeepsStepSequenceIdentifiers(t *testing.T) {
	ctx := context.Background()
	steps := []model.MigrationStep{
		{ID: 1, Seq: 10, StepType: "create_table", TargetObj: "orders"},
		{ID: 2, Seq: 20, StepType: "drop_table", TargetObj: "orders"},
	}
	order, err := impact.OrderPlan(steps, nil)
	if err != nil {
		t.Fatalf("OrderPlan: %v", err)
	}
	if !reflect.DeepEqual(order, []int{10, 20}) {
		t.Fatalf("planner must return step sequence identifiers, got %v", order)
	}
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	repos := store.NewRepositories(st)
	svc := New(repos)
	now := time.Now()
	snapshotID, err := repos.Snapshots.CreateSnapshot(ctx, &model.SchemaSnapshot{Name: "ordering", Status: model.SnapshotRegistered, CreatedAt: now})
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	scriptID, err := repos.Scripts.CreateScript(ctx, &model.MigrationScript{Name: "ordering", Version: 1, Status: model.ScriptApproved, ContentHash: "sequence-check", Content: "manual", CreatedAt: now})
	if err != nil {
		t.Fatalf("CreateScript: %v", err)
	}
	for i := range steps {
		steps[i].ScriptID = scriptID
	}
	if err := repos.Scripts.CreateSteps(ctx, scriptID, steps); err != nil {
		t.Fatalf("CreateSteps: %v", err)
	}
	analysisID, err := repos.Analyses.CreateAnalysis(ctx, &model.ImpactAnalysis{SnapshotID: snapshotID, ScriptID: scriptID, Status: model.AnalysisCompleted, InputHash: "sequence-input", CreatedAt: now, CompletedAt: &now})
	if err != nil {
		t.Fatalf("CreateAnalysis: %v", err)
	}
	plan, err := svc.FreezePlan(ctx, analysisID)
	if err != nil {
		t.Fatalf("FreezePlan: %v", err)
	}
	if !reflect.DeepEqual(plan.StepOrder, []int{10, 20}) {
		t.Fatalf("frozen plan must retain all sequence identifiers, got %v", plan.StepOrder)
	}
	reloaded, err := repos.Analyses.GetPlan(ctx, plan.ID)
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if !reflect.DeepEqual(reloaded.StepOrder, []int{10, 20}) {
		t.Fatalf("persisted plan must retain all sequence identifiers, got %v", reloaded.StepOrder)
	}
}

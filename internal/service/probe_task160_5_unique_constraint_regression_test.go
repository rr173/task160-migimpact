package service

import (
	"context"
	"testing"

	"task160-migimpact/internal/model"
	"task160-migimpact/internal/plan"
	"task160-migimpact/internal/schema"
	"task160-migimpact/internal/store"
)

func TestTask160Bug05_UniqueConstraintChangesAreVisibleEverywhere(t *testing.T) {
	plain := model.SchemaObject{ObjType: "column", TableName: "accounts", Name: "email", DataType: "TEXT"}
	unique := plain
	unique.Unique = true
	if schema.Definition(&plain) == schema.Definition(&unique) {
		t.Fatal("unique constraint must change schema definition")
	}
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	svc := New(store.NewRepositories(st))
	snapshotID, err := svc.CreateSnapshot(ctx, "unique-constraint")
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	objects := []model.SchemaObject{{ObjType: "table", Name: "accounts"}, plain}
	if err := svc.LoadSnapshotObjects(ctx, snapshotID, objects, nil, nil); err != nil {
		t.Fatalf("Load initial: %v", err)
	}
	initial, err := svc.GetSnapshotObjects(ctx, snapshotID)
	if err != nil {
		t.Fatalf("Get initial: %v", err)
	}
	objects[1] = unique
	if err := svc.LoadSnapshotObjects(ctx, snapshotID, objects, nil, nil); err != nil {
		t.Fatalf("Load unique: %v", err)
	}
	updated, err := svc.GetSnapshotObjects(ctx, snapshotID)
	if err != nil {
		t.Fatalf("Get updated: %v", err)
	}
	if initial[1].Hash == updated[1].Hash {
		t.Fatal("unique constraint must change persisted object hash")
	}
	diff := plan.DiffSnapshots([]model.SchemaObject{{ObjType: "column", TableName: "accounts", Name: "email", Hash: "plain"}}, []model.SchemaObject{{ObjType: "column", TableName: "accounts", Name: "email", Hash: "unique"}})
	if len(diff.Changed) != 1 || diff.Changed[0] != "email" {
		t.Fatalf("unique constraint change must appear in snapshot diff: %+v", diff)
	}
}

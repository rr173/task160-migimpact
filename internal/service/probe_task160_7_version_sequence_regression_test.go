package service

import (
	"context"
	"testing"

	"task160-migimpact/internal/model"
	"task160-migimpact/internal/script"
	"task160-migimpact/internal/store"
)

func TestTask160Bug07_ImportedScriptsKeepContinuousAscendingVersions(t *testing.T) {
	if err := script.VerifyVersionContinuity([]model.MigrationScript{{Version: 1}}, 2); err != nil {
		t.Fatalf("parser-side version validation must accept the next version: %v", err)
	}
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	svc := New(store.NewRepositories(st))
	if _, err := svc.ImportScript(ctx, "v1", "1 CREATE_TABLE [customers]", 1); err != nil {
		t.Fatalf("Import v1: %v", err)
	}
	if _, err := svc.ImportScript(ctx, "v2", "1 CREATE_TABLE [invoices]", 2); err != nil {
		t.Fatalf("Import v2: %v", err)
	}
	scripts, err := svc.ListScripts(ctx)
	if err != nil {
		t.Fatalf("ListScripts: %v", err)
	}
	if len(scripts) != 2 || scripts[0].Version != 1 || scripts[1].Version != 2 {
		t.Fatalf("stored scripts must remain in ascending import order: %+v", scripts)
	}
}

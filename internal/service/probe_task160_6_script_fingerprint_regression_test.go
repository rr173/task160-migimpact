package service

import (
	"context"
	"testing"
	"time"

	"task160-migimpact/internal/model"
	"task160-migimpact/internal/script"
	"task160-migimpact/internal/store"
)

func TestTask160Bug06_ExactScriptContentKeepsDistinctFingerprint(t *testing.T) {
	content := "1 CREATE_TABLE [ledger]"
	if script.Fingerprint(content) == script.Fingerprint(content+"\n") {
		t.Fatal("a trailing script byte must change its exact fingerprint")
	}
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	repos := store.NewRepositories(st)
	svc := New(repos)
	first, err := svc.ImportScript(ctx, "ledger-v1", content, 1)
	if err != nil {
		t.Fatalf("Import first: %v", err)
	}
	second, err := svc.ImportScript(ctx, "ledger-v2", content+"\n", 2)
	if err != nil || second == first {
		t.Fatalf("distinct exact content must import as a new version: id=%d err=%v", second, err)
	}
	storedID, err := repos.Scripts.CreateScript(ctx, &model.MigrationScript{Name: "stored", Version: 3, Status: model.ScriptImported, ContentHash: "probe-stored-hash", Content: "manual", CreatedAt: time.Now()})
	if err != nil {
		t.Fatalf("CreateScript: %v", err)
	}
	stored, err := repos.Scripts.GetScriptByHash(ctx, "probe-stored-hash")
	if err != nil || stored.ID != storedID {
		t.Fatalf("stored fingerprint must find its exact script: %+v err=%v", stored, err)
	}
}

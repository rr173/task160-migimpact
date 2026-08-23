package store

import (
	"context"
	"testing"
	"time"

	"task160-migimpact/internal/model"
)

func TestSnapshotCRUDAndRestart(t *testing.T) {
	ctx := context.Background()
	st, err := Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	repos := NewRepositories(st)

	// 创建快照并填充对象
	snap := &model.SchemaSnapshot{Name: "s1", Status: model.SnapshotDraft, CreatedAt: time.Now(), ObjectHash: "h1", DepHash: "d1"}
	id, err := repos.Snapshots.CreateSnapshot(ctx, snap)
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	objs := []model.SchemaObject{
		{SnapshotID: id, ObjType: "table", Name: "orders", Hash: "x"},
		{SnapshotID: id, ObjType: "column", TableName: "orders", Name: "id", Hash: "y"},
	}
	if err := repos.Snapshots.UpsertObjects(ctx, id, objs); err != nil {
		t.Fatalf("UpsertObjects: %v", err)
	}
	got, err := repos.Snapshots.ListObjects(ctx, id)
	if err != nil {
		t.Fatalf("ListObjects: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("期望 2 个对象，得到 %d", len(got))
	}
	// 模拟重启：关闭后重新打开同一文件
	path := st.DBPathForTest()
	st.Close()

	st2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer st2.Close()
	repos2 := NewRepositories(st2)
	snap2, err := repos2.Snapshots.GetSnapshot(ctx, id)
	if err != nil {
		t.Fatalf("GetSnapshot after restart: %v", err)
	}
	if snap2.Name != "s1" || snap2.ObjectHash != "h1" {
		t.Fatalf("重启后数据不一致: %+v", snap2)
	}
	got2, err := repos2.Snapshots.ListObjects(ctx, id)
	if err != nil || len(got2) != 2 {
		t.Fatalf("重启后对象丢失: %v %d", err, len(got2))
	}
}

func TestScriptDedup(t *testing.T) {
	ctx := context.Background()
	st, err := Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	repos := NewRepositories(st)

	sc := &model.MigrationScript{Name: "m1", Version: 1, Status: model.ScriptImported,
		ContentHash: "same-hash", Content: "1 CREATE_TABLE [a]", CreatedAt: time.Now()}
	id1, err := repos.Scripts.CreateScript(ctx, sc)
	if err != nil {
		t.Fatalf("CreateScript: %v", err)
	}
	id2, err := repos.Scripts.CreateScript(ctx, sc)
	if err == nil {
		t.Fatalf("重复指纹应报错")
	}
	if id1 == id2 {
		t.Fatal("两次插入不应返回相同 ID")
	}
	// 幂等查找：已保存脚本必须能按自身内容哈希找回（不得被 name 过滤漏掉）
	found, err := repos.Scripts.GetScriptByHash(ctx, "same-hash")
	if err != nil || found.ID != id1 {
		t.Fatalf("GetScriptByHash: %v %v", found, err)
	}
	if found.Name != "m1" {
		t.Fatalf("找回的脚本应保留原名称 m1，得到 %q", found.Name)
	}
}

func TestAnalysisIdempotent(t *testing.T) {
	ctx := context.Background()
	st, err := Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	repos := NewRepositories(st)

	// 先创建父记录以满足外键约束
	snap := &model.SchemaSnapshot{Name: "s", Status: model.SnapshotDraft, CreatedAt: time.Now(), ObjectHash: "h", DepHash: "d"}
	snapID, err := repos.Snapshots.CreateSnapshot(ctx, snap)
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	sc := &model.MigrationScript{Name: "m", Version: 1, Status: model.ScriptImported, ContentHash: "ch", Content: "1 CREATE_TABLE [a]", CreatedAt: time.Now()}
	scriptID, err := repos.Scripts.CreateScript(ctx, sc)
	if err != nil {
		t.Fatalf("CreateScript: %v", err)
	}

	a := &model.ImpactAnalysis{SnapshotID: snapID, ScriptID: scriptID, Status: model.AnalysisQueued,
		InputHash: "in-1", CreatedAt: time.Now()}
	id1, err := repos.Analyses.CreateAnalysis(ctx, a)
	if err != nil {
		t.Fatalf("CreateAnalysis: %v", err)
	}
	id2, err := repos.Analyses.CreateAnalysis(ctx, a)
	if err != model.ErrConflict {
		t.Fatalf("重复分析应返回 ErrConflict，得到 %v", err)
	}
	if id1 != id2 {
		t.Fatalf("幂等分析应返回相同 ID: %d != %d", id1, id2)
	}
}

func TestAuditAppend(t *testing.T) {
	ctx := context.Background()
	st, err := Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	repos := NewRepositories(st)
	for i := 0; i < 3; i++ {
		ev := &model.AuditEvent{Actor: "tester", Action: "create", Subject: "x", Detail: "d", CreatedAt: time.Now()}
		if err := repos.Analyses.AppendAudit(ctx, ev); err != nil {
			t.Fatalf("AppendAudit: %v", err)
		}
	}
	events, err := repos.Analyses.ListAudit(ctx, 10)
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("期望 3 条审计，得到 %d", len(events))
	}
}

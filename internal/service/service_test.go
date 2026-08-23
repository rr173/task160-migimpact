package service

import (
	"context"
	"errors"
	"testing"

	"task160-migimpact/internal/model"
	"task160-migimpact/internal/store"
)

// TestDemoFlow 覆盖完整业务闭环：建快照 → 填对象 → 导脚本 → 分析 →
// 豁免 → 冻结计划 → 重启恢复幂等。
func TestDemoFlow(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	path := st.Path()
	repos := store.NewRepositories(st)
	svc := New(repos)

	if err := svc.RunDemo(ctx); err != nil {
		t.Fatalf("RunDemo: %v", err)
	}

	// 重启恢复：关闭并重开，数据必须保留
	st.Close()
	st2, err := store.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer st2.Close()
	svc2 := New(store.NewRepositories(st2))

	snaps, err := svc2.ListSnapshots(ctx)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("重启后应有 1 个快照，得到 %d", len(snaps))
	}
	scripts, err := svc2.ListScripts(ctx)
	if err != nil {
		t.Fatalf("ListScripts: %v", err)
	}
	if len(scripts) != 1 {
		t.Fatalf("重启后应有 1 个脚本，得到 %d", len(scripts))
	}
	plans, err := svc2.ListPlans(ctx)
	if err != nil {
		t.Fatalf("ListPlans: %v", err)
	}
	if len(plans) != 1 {
		t.Fatalf("重启后应有 1 个计划，得到 %d", len(plans))
	}
}

// TestImportScriptVersionContinuity 验证版本连续性与幂等。
func TestImportScriptVersionContinuity(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	svc := New(store.NewRepositories(st))

	if _, err := svc.ImportScript(ctx, "m1", "1 CREATE_TABLE [a]\n", 2); err == nil {
		t.Fatal("首个脚本版本 2 应报错")
	}
	id1, err := svc.ImportScript(ctx, "m1", "1 CREATE_TABLE [a]\n", 1)
	if err != nil {
		t.Fatalf("ImportScript v1: %v", err)
	}
	// 幂等：同内容再次导入
	id2, err := svc.ImportScript(ctx, "m1-dup", "1 CREATE_TABLE [a]\n", 2)
	if !errors.Is(err, model.ErrDupFingerprint) {
		t.Fatalf("重复内容应报指纹冲突，得到 %v", err)
	}
	if id2 != id1 {
		t.Fatalf("幂等导入应返回既有 ID: %d != %d", id2, id1)
	}
	// 版本不连续
	if _, err := svc.ImportScript(ctx, "m2", "1 CREATE_TABLE [b]\n", 3); err == nil {
		t.Fatal("版本 3 在只有 v1 时应报错")
	}
	// 正确连续
	if _, err := svc.ImportScript(ctx, "m2", "1 CREATE_TABLE [b]\n", 2); err != nil {
		t.Fatalf("ImportScript v2: %v", err)
	}
}

// TestFreezePlanBlockedAnalysis 未完成分析不可冻结。
func TestFreezePlanBlockedAnalysis(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	svc := New(store.NewRepositories(st))

	if _, err := svc.FreezePlan(ctx, 999); err == nil {
		t.Fatal("不存在的分析应报错")
	}
}

// TestSnapshotHashSensitiveToUnique 验证唯一性约束翻转改变持久化对象哈希
// （ObjectHash），使唯一性变化在整个迁移评估流程中可见。
func TestSnapshotHashSensitiveToUnique(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	svc := New(store.NewRepositories(st))

	snapID, err := svc.CreateSnapshot(ctx, "uniq-snapshot")
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	objs := []model.SchemaObject{
		{ObjType: "table", Name: "orders"},
		{ObjType: "index", TableName: "orders", Name: "idx_orders_customer_id", Unique: false},
	}
	deps := []model.ObjectDependency{}
	accs := []model.AccessDeclaration{}
	if err := svc.LoadSnapshotObjects(ctx, snapID, objs, deps, accs); err != nil {
		t.Fatalf("LoadSnapshotObjects: %v", err)
	}
	snap1, err := svc.GetSnapshot(ctx, snapID)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	hashBefore := snap1.ObjectHash
	if hashBefore == "" {
		t.Fatal("对象哈希未固化")
	}

	// 翻转唯一性约束：非唯一 -> 唯一
	objs[1].Unique = true
	if err := svc.LoadSnapshotObjects(ctx, snapID, objs, deps, accs); err != nil {
		t.Fatalf("LoadSnapshotObjects after unique flip: %v", err)
	}
	snap2, err := svc.GetSnapshot(ctx, snapID)
	if err != nil {
		t.Fatalf("GetSnapshot after flip: %v", err)
	}
	if snap2.ObjectHash == hashBefore {
		t.Fatal("唯一性翻转应改变 ObjectHash，但哈希未变")
	}

	// 持久化读回的对象应反映翻转后的唯一性
	gotObjs, err := svc.GetSnapshotObjects(ctx, snapID)
	if err != nil {
		t.Fatalf("GetSnapshotObjects: %v", err)
	}
	var idx *model.SchemaObject
	for i := range gotObjs {
		if gotObjs[i].ObjType == "index" && gotObjs[i].Name == "idx_orders_customer_id" {
			idx = &gotObjs[i]
		}
	}
	if idx == nil {
		t.Fatal("未读回索引对象")
	}
	if !idx.Unique {
		t.Fatal("读回的索引唯一性应为 true")
	}
}

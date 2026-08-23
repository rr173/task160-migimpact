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

// TestImportScriptExactTextFingerprint 验证指纹基于精确脚本文本：
// 仅末尾换行不同的两份脚本不得被误判为同一版本，且第一份已保存脚本可按自身指纹找回。
func TestImportScriptExactTextFingerprint(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	svc := New(store.NewRepositories(st))

	id1, err := svc.ImportScript(ctx, "m1", "1 CREATE_TABLE [a]", 1)
	if err != nil {
		t.Fatalf("ImportScript v1: %v", err)
	}
	// 仅末尾多一个换行：指纹应不同，按连续版本 2 导入成功，不得判重
	id2, err := svc.ImportScript(ctx, "m2", "1 CREATE_TABLE [a]\n", 2)
	if err != nil {
		t.Fatalf("仅末尾换行不同的脚本应作为不同版本导入成功，得到 %v", err)
	}
	if id2 == id1 {
		t.Fatalf("仅末尾换行不同的脚本不应复用既有 ID: %d == %d", id1, id2)
	}
	// 已保存脚本必须能按自身指纹找回（幂等）
	id3, err := svc.ImportScript(ctx, "m1-dup", "1 CREATE_TABLE [a]", 2)
	if !errors.Is(err, model.ErrDupFingerprint) {
		t.Fatalf("精确内容再次导入应按指纹命中既有脚本，得到 %v", err)
	}
	if id3 != id1 {
		t.Fatalf("幂等导入应返回既有 ID: %d != %d", id3, id1)
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

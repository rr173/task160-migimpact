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

// TestFreezePlanPreservesFullStepOrder 冻结计划必须保留全部真实步骤编号与完整执行顺序；
// 冻结后重新读取计划，StepOrder 不得丢失首步、不得与 PlanHash 计算次序不一致。
func TestFreezePlanPreservesFullStepOrder(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	svc := New(store.NewRepositories(st))

	if err := svc.RunDemo(ctx); err != nil {
		t.Fatalf("RunDemo: %v", err)
	}

	// 找到演示生成的分析
	analyses, err := svc.ListAnalyses(ctx)
	if err != nil {
		t.Fatalf("ListAnalyses: %v", err)
	}
	if len(analyses) != 1 {
		t.Fatalf("期望 1 个分析，得到 %d", len(analyses))
	}
	analysisID := analyses[0].ID

	// 冻结前先拿到分析返回的执行次序（真实步骤编号）
	analysis, _, order, err := svc.RunAnalysis(ctx, analyses[0].SnapshotID, analyses[0].ScriptID)
	if err != nil {
		t.Fatalf("RunAnalysis: %v", err)
	}
	if analysis.ID != analysisID {
		t.Fatalf("幂等分析返回 ID 不一致: %d != %d", analysis.ID, analysisID)
	}
	if len(order) == 0 {
		t.Fatalf("执行次序为空")
	}

	plan, err := svc.FreezePlan(ctx, analysis.ID)
	if err != nil {
		t.Fatalf("FreezePlan: %v", err)
	}

	// 冻结写回的次序必须与分析次序完全一致，不得截断首步。
	if len(plan.StepOrder) != len(order) {
		t.Fatalf("计划次序长度 %d 与分析次序长度 %d 不一致", len(plan.StepOrder), len(order))
	}
	for i, seq := range plan.StepOrder {
		if seq != order[i] {
			t.Fatalf("计划次序在第 %d 步与真实次序不一致: plan=%v analysis=%v", i, plan.StepOrder, order)
		}
	}

	// 冻结后重新读取：StepOrder 必须完整，不得再丢失一步。
	reloaded, err := svc.GetPlan(ctx, plan.ID)
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if len(reloaded.StepOrder) != len(plan.StepOrder) {
		t.Fatalf("重读计划丢失步骤: 重读长度 %d，冻结长度 %d", len(reloaded.StepOrder), len(plan.StepOrder))
	}
	for i, seq := range reloaded.StepOrder {
		if seq != plan.StepOrder[i] {
			t.Fatalf("重读计划次序在第 %d 步与冻结次序不一致: got=%v want=%v", i, reloaded.StepOrder, plan.StepOrder)
		}
	}
}

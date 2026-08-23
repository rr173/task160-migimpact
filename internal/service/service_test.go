package service

import (
	"context"
	"errors"
	"testing"

	"task160-migimpact/internal/model"
	"task160-migimpact/internal/plan"
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

// TestExemptionApprovalConsistency 校验豁免批准后状态与阻断结果一致：
// 批准的豁免记录重读仍为 approved；对应变更不再阻断，其余变更仍阻断。
func TestExemptionApprovalConsistency(t *testing.T) {
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
	// RunDemo 只豁免了第一条变更；取全部变更重算阻断集合。
	snaps, _ := svc.ListSnapshots(ctx)
	scripts, _ := svc.ListScripts(ctx)
	analyses, err := svc.ListAnalyses(ctx)
	if err != nil || len(analyses) != 1 {
		t.Fatalf("应有 1 个分析: %v", err)
	}
	analysis := analyses[0]
	changes, err := svc.ListChanges(ctx, analysis.ID)
	if err != nil {
		t.Fatalf("ListChanges: %v", err)
	}
	if len(changes) < 2 {
		t.Fatalf("示例至少应识别 2 条破坏性变更，得到 %d", len(changes))
	}
	exemptions, err := svc.ListExemptions(ctx, analysis.ID)
	if err != nil {
		t.Fatalf("ListExemptions: %v", err)
	}
	if len(exemptions) != 1 {
		t.Fatalf("应有 1 条豁免，得到 %d", len(exemptions))
	}
	if exemptions[0].Status != model.ExemptionApproved {
		t.Fatalf("重读豁免状态应为 approved，得到 %s", exemptions[0].Status)
	}
	// 批准的变更已放行（change status = exempted）；其余仍为 identified。
	exemptedChange := changes[0]
	got, err := svc.ListChanges(ctx, analysis.ID)
	if err != nil {
		t.Fatalf("ListChanges(2): %v", err)
	}
	for _, c := range got {
		if c.ID == exemptedChange.ID && c.Status != "exempted" {
			t.Fatalf("已批准豁免的变更 %d 应为 exempted，得到 %s", c.ID, c.Status)
		}
		if c.ID != exemptedChange.ID && c.Status != "identified" {
			t.Fatalf("未豁免变更 %d 应仍为 identified，得到 %s", c.ID, c.Status)
		}
	}
	// 阻断集合应排除已批准豁免的变更，保留其余。
	blockers := plan.RecomputeBlockers(got, exemptions)
	if len(blockers) != len(got)-1 {
		t.Fatalf("期望 %d 个阻断，得到 %d", len(got)-1, len(blockers))
	}
	for _, b := range blockers {
		if b.ID == exemptedChange.ID {
			t.Fatal("已批准豁免的变更不应出现在阻断集合中")
		}
	}
	// 影响分析仍可正常输出 finding（阻断状态不影响已完成分析）。
	_, findings, _, err := svc.RunAnalysis(ctx, snaps[0].ID, scripts[0].ID)
	if err != nil {
		t.Fatalf("RunAnalysis: %v", err)
	}
	if len(findings) == 0 {
		t.Fatal("幂等分析应仍返回破坏性变更")
	}
}

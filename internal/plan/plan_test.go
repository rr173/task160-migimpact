package plan

import (
	"testing"

	"task160-migimpact/internal/model"
)

func TestValidate(t *testing.T) {
	a := &model.ImpactAnalysis{Status: model.AnalysisCompleted}
	steps := []model.MigrationStep{{Seq: 1}, {Seq: 2}, {Seq: 3}}
	if err := Validate(a, []int{1, 2, 3}, steps); err != nil {
		t.Fatalf("完整次序应通过: %v", err)
	}
	if err := Validate(a, []int{1, 2}, steps); err == nil {
		t.Fatal("缺失步骤应报错")
	}
	if err := Validate(a, []int{1, 2, 4}, steps); err == nil {
		t.Fatal("多余/错误步骤应报错")
	}
	b := &model.ImpactAnalysis{Status: model.AnalysisQueued}
	if err := Validate(b, []int{1, 2, 3}, steps); err == nil {
		t.Fatal("未完成分析应报错")
	}
}

func TestRecomputeBlockers(t *testing.T) {
	changes := []model.DestructiveChange{
		{ID: 1, Status: "identified"},
		{ID: 2, Status: "identified"},
		{ID: 3, Status: "identified"},
	}
	exemptions := []model.Exemption{
		{ChangeID: 1, Status: model.ExemptionApproved},
		{ChangeID: 2, Status: model.ExemptionRejected},
	}
	blockers := RecomputeBlockers(changes, exemptions)
	if len(blockers) != 2 {
		t.Fatalf("期望 2 个阻断（2 未豁免、3 未豁免），得到 %d", len(blockers))
	}
}

func TestDiffSnapshots(t *testing.T) {
	before := []model.SchemaObject{
		{ObjType: "table", Name: "a", Hash: "h1"},
		{ObjType: "table", Name: "b", Hash: "h1"},
	}
	after := []model.SchemaObject{
		{ObjType: "table", Name: "b", Hash: "h2"}, // 变更
		{ObjType: "table", Name: "c", Hash: "h1"}, // 新增
	}
	d := DiffSnapshots(before, after)
	if len(d.Added) != 1 || d.Added[0] != "c" {
		t.Fatalf("added 应为 [c]，得到 %v", d.Added)
	}
	if len(d.Removed) != 1 || d.Removed[0] != "table::a" {
		t.Fatalf("removed 应为 [table::a]，得到 %v", d.Removed)
	}
	if len(d.Changed) != 1 || d.Changed[0] != "b" {
		t.Fatalf("changed 应为 [b]，得到 %v", d.Changed)
	}
}

func TestPlanHashDeterministic(t *testing.T) {
	a := PlanHash(1, 2, 3, []int{1, 2, 3})
	b := PlanHash(1, 2, 3, []int{1, 2, 3})
	if a != b {
		t.Fatal("相同输入计划哈希应一致")
	}
	c := PlanHash(1, 2, 3, []int{3, 2, 1})
	if a == c {
		t.Fatal("不同次序计划哈希应不同")
	}
}

func TestExemptChange(t *testing.T) {
	c := &model.DestructiveChange{ID: 9, AnalysisID: 5, Status: "identified"}
	if err := ExemptChange(c, 5); err != nil {
		t.Fatalf("正确豁免应通过: %v", err)
	}
	if err := ExemptChange(c, 6); err == nil {
		t.Fatal("跨分析豁免应报错")
	}
	c.Status = "exempted"
	if err := ExemptChange(c, 5); err == nil {
		t.Fatal("重复豁免应报错")
	}
}

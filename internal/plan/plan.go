// Package plan 负责迁移计划的冻结、豁免与版本差异。
package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"task160-migimpact/internal/model"
)

// FreezeRequest 描述冻结计划所需的信息。
type FreezeRequest struct {
	AnalysisID int64
	SnapshotID int64
	ScriptID   int64
	StepOrder  []int
}

// Validate 校验冻结请求：分析必须完成、步骤次序必须覆盖脚本全部步骤。
func Validate(a *model.ImpactAnalysis, order []int, steps []model.MigrationStep) error {
	if a.Status != model.AnalysisCompleted {
		return fmt.Errorf("分析尚未完成，当前状态 %s", a.Status)
	}
	if len(order) != len(steps) {
		return fmt.Errorf("执行次序必须覆盖全部步骤：期望 %d 实际 %d", len(steps), len(order))
	}
	covered := map[int]bool{}
	for _, seq := range order {
		covered[seq] = true
	}
	for _, s := range steps {
		if !covered[s.Seq] {
			return fmt.Errorf("执行次序缺少步骤 %d", s.Seq)
		}
	}
	return nil
}

// PlanHash 计算计划的指纹：分析ID + 快照ID + 脚本ID + 有序步骤。
func PlanHash(analysisID, snapshotID, scriptID int64, order []int) string {
	h := sha256.New()
	fmt.Fprintf(h, "analysis=%d;snapshot=%d;script=%d;order=%v", analysisID, snapshotID, scriptID, order)
	return hex.EncodeToString(h.Sum(nil))
}

// ExemptChange 校验并创建豁免：变更必须处于 identified 状态，且必须属于该分析。
func ExemptChange(change *model.DestructiveChange, analysisID int64) error {
	if change.AnalysisID != analysisID {
		return fmt.Errorf("变更 %d 不属于分析 %d", change.ID, analysisID)
	}
	if change.Status != "identified" {
		return fmt.Errorf("变更 %d 当前状态 %s，不可豁免", change.ID, change.Status)
	}
	return nil
}

// RecomputeBlockers 根据豁免集合重新计算阻断集合：
// 所有未豁免的破坏性变更视为阻断；全部豁免后分析可通过。
func RecomputeBlockers(changes []model.DestructiveChange, exemptions []model.Exemption) []model.DestructiveChange {
	approved := map[int64]bool{}
	for _, e := range exemptions {
		if e.Status == model.ExemptionApproved {
			approved[e.ChangeID] = true
		}
	}
	var blockers []model.DestructiveChange
	for _, c := range changes {
		if !approved[c.ID] {
			blockers = append(blockers, c)
		}
	}
	return blockers
}

// DiffSnapshots 比较两个快照的对象集合差异，返回新增/删除/变更三类对象名。
// 变更判定依据对象的规范化定义摘要（Hash）：同一对象键在不同快照中定义变化
// （如数据类型、可空性或唯一性约束翻转）即记为变更，使属性级变化在差异中可见。
func DiffSnapshots(before, after []model.SchemaObject) DiffResult {
	beforeMap := indexByKey(before)
	afterMap := indexByKey(after)
	var d DiffResult
	for k, o := range afterMap {
		if bo, ok := beforeMap[k]; !ok {
			d.Added = append(d.Added, o.Name)
		} else if !sameDefinition(bo, o) {
			d.Changed = append(d.Changed, o.Name)
		}
	}
	for k := range beforeMap {
		if _, ok := afterMap[k]; !ok {
			d.Removed = append(d.Removed, k)
		}
	}
	sort.Strings(d.Added)
	sort.Strings(d.Changed)
	sort.Strings(d.Removed)
	return d
}

// sameDefinition 判断两个同键对象的定义是否一致。
// 优先使用持久化哈希（Hash）；哈希为空时回退到规范化定义文本（Definition）。
// 二者均缺失时，按属性逐项比较，确保唯一性等属性变化同样被识别。
func sameDefinition(a, b model.SchemaObject) bool {
	if a.Hash != "" || b.Hash != "" {
		return a.Hash == b.Hash
	}
	if a.Definition != "" || b.Definition != "" {
		return a.Definition == b.Definition
	}
	return a.DataType == b.DataType && a.Nullable == b.Nullable && a.Unique == b.Unique
}

// DiffResult 是快照差异摘要。
type DiffResult struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	Changed []string `json:"changed"`
}

func indexByKey(objs []model.SchemaObject) map[string]model.SchemaObject {
	m := map[string]model.SchemaObject{}
	for _, o := range objs {
		key := strings.Join([]string{o.ObjType, o.TableName, o.Name}, ":")
		m[key] = o
	}
	return m
}

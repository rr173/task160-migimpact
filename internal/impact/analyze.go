// Package impact 实现影响分析核心：破坏性变更分类、影响传播与安全执行次序。
package impact

import (
	"fmt"
	"sort"
	"strings"

	"task160-migimpact/internal/model"
	"task160-migimpact/internal/schema"
	"task160-migimpact/internal/script"
)

// AnalysisInput 是一次影响分析的输入：快照对象、依赖、访问声明与脚本步骤。
type AnalysisInput struct {
	Objects []model.SchemaObject
	Deps    []model.ObjectDependency
	Access  []model.AccessDeclaration
	Steps   []model.MigrationStep
}

// Finding 是一条破坏性变更及其传播结果。
type Finding struct {
	StepID       int64
	StepSeq      int
	ChangeType   model.DestructiveChangeType
	TargetObj    string
	AffectedObjs []string // 受影响对象（含目标自身与下游）
	Interfaces   []string // 受影响接口
	Evidence     string
}

// Run 执行影响分析，返回全部破坏性变更发现。
// 过程：为每个步骤判定类型；对破坏性步骤，沿依赖图向「引用它的对象」传播，
// 并收集声明访问受影响对象的接口。
func Run(in AnalysisInput) ([]Finding, error) {
	idx := schema.NewIndex(in.Objects)
	var findings []Finding
	for i := range in.Steps {
		step := &in.Steps[i]
		hasDependents := hasDependents(step, in.Deps, idx)
		changeType, evidence := script.ClassifyStep(step, hasDependents)
		if changeType == "" {
			continue
		}
		affected, ifaces := propagate(step, in.Deps, in.Access, idx)
		findings = append(findings, Finding{
			StepID:       step.ID,
			StepSeq:      step.Seq,
			ChangeType:   changeType,
			TargetObj:    step.TargetObj,
			AffectedObjs: dedupSorted(affected),
			Interfaces:   dedupSorted(ifaces),
			Evidence:     evidence,
		})
	}
	return findings, nil
}

// hasDependents 判断某对象是否被其他对象或访问声明引用。
func hasDependents(step *model.MigrationStep, deps []model.ObjectDependency, idx *schema.Index) bool {
	obj := step.TargetObj
	if !strings.Contains(obj, ":") {
		obj = "table:" + obj + ":"
	}
	for _, d := range deps {
		if d.TargetObj == obj || d.TargetObj == strings.TrimSuffix(obj, ":") {
			return true
		}
	}
	return false
}

// propagate 从被破坏对象出发，沿「被引用」方向传播：找出引用目标对象的对象
// （即 source -> target 边中 target 被破坏，source 受影响），并递归；同时收集
// 访问受影响对象的接口。
//
// 影响范围按 Finding.AffectedObjs 文档语义「含目标自身与下游」处理：目标对象本身
// 也属于受影响范围。这样当接口直接读被破坏的对象（而非其下游）时，该接口也能被
// 收集到 —— 否则删表场景下直接读该表的接口会丢失。
func propagate(step *model.MigrationStep, deps []model.ObjectDependency, accs []model.AccessDeclaration, idx *schema.Index) ([]string, []string) {
	// 目标对象键：优先尝试 type:table:name 形式，否则视为表
	target := step.TargetObj
	if !strings.Contains(target, ":") {
		target = "table:" + target + ":"
	}
	target = strings.TrimSuffix(target, ":")

	affected := map[string]bool{target: true} // 目标自身计入受影响范围
	queue := []string{target}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, d := range deps {
			src := strings.TrimSuffix(d.SourceObj, ":")
			if d.TargetObj == cur || strings.TrimSuffix(d.TargetObj, ":") == cur {
				if !affected[src] {
					affected[src] = true
					queue = append(queue, src)
				}
			}
		}
	}

	var ifaces []string
	for _, a := range accs {
		t := strings.TrimSuffix(a.TargetObj, ":")
		if affected[t] {
			ifaces = append(ifaces, fmt.Sprintf("%s %s %s", a.Service, a.Interface, a.Action))
		}
	}
	objs := make([]string, 0, len(affected))
	for k := range affected {
		objs = append(objs, k)
	}
	return objs, ifaces
}

func dedupSorted(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

package impact

import (
	"fmt"
	"sort"
	"strings"

	"task160-migimpact/internal/model"
)

// OrderPlan 计算安全执行次序：对脚本步骤做拓扑排序。
//
// 规则：若步骤 A 破坏对象 X，而步骤 B 依赖 X（B 的变更对象被 X 影响，或 B 引用了
// X 作为依赖来源），则 B 必须先于 A 执行。更一般地：当步骤 s1 的目标对象是 s2
// 目标对象（被 s2 破坏的对象）的依赖来源时，s1 先执行。
//
// 具体做法：为每个步骤 s 找出其目标对象 key；若 s 是破坏性步骤，则任何「对象
// key 引用该目标」的步骤都排在 s 之前。构建 DAG 后做 Kahn 拓扑排序，检测循环。
func OrderPlan(steps []model.MigrationStep, deps []model.ObjectDependency) ([]int, error) {
	n := len(steps)
	idxBySeq := map[int]int{} // seq -> slice index
	for i, s := range steps {
		idxBySeq[s.Seq] = i
	}
	// 步骤目标对象：objectRef(step)
	refOf := map[int]string{}
	for _, s := range steps {
		refOf[s.Seq] = normalizeRef(s.TargetObj)
	}

	// 邻接表：before -> after（before 必须先执行）
	adj := make([][]int, n)
	indeg := make([]int, n)
	addEdge := func(before, after int) {
		adj[before] = append(adj[before], after)
		indeg[after]++
	}

	// 步骤间依赖：若 step a 的目标对象被其他对象的依赖引用，而 step b 破坏那个
	// 被引用对象，则 a 先于 b。
	// 简化：对每条依赖 (source -> target)，若某步骤 sA 破坏 target 对象，
	// 且某步骤 sB 操作 source 对象（source 被 sB 读取/重建），则 sB 先于 sA。
	for _, dep := range deps {
		src := normalizeRef(dep.SourceObj)
		tgt := normalizeRef(dep.TargetObj)
		for _, s := range steps {
			if refOf[s.Seq] == tgt && isDestructive(s) {
				for _, other := range steps {
					if other.Seq == s.Seq {
						continue
					}
					if refOf[other.Seq] == src {
						bi, oi := idxBySeq[other.Seq], idxBySeq[s.Seq]
						addEdge(bi, oi)
					}
				}
			}
		}
	}

	// 额外规则：同一表上的 DROP_TABLE 必须在其列/索引相关操作之后
	for _, s := range steps {
		if s.StepType == "drop_table" {
			table := strings.TrimPrefix(normalizeRef(s.TargetObj), "table:")
			for _, other := range steps {
				if other.Seq == s.Seq {
					continue
				}
				if strings.Contains(other.TargetObj, table+":") {
					bi, oi := idxBySeq[other.Seq], idxBySeq[s.Seq]
					addEdge(bi, oi)
				}
			}
		}
	}

	// Kahn 拓扑排序（按 seq 稳定）
	queue := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if indeg[i] == 0 {
			queue = append(queue, i)
		}
	}
	var order []int
	for len(queue) > 0 {
		sort.Slice(queue, func(i, j int) bool { return steps[queue[i]].Seq < steps[queue[j]].Seq })
		cur := queue[0]
		queue = queue[1:]
		order = append(order, cur)
		for _, nxt := range adj[cur] {
			indeg[nxt]--
			if indeg[nxt] == 0 {
				queue = append(queue, nxt)
			}
		}
	}
	if len(order) != n {
		// 检测循环依赖：找出仍在环上的步骤
		cycle := findCycleSteps(steps, adj, indeg, n)
		return nil, fmt.Errorf("检测到循环依赖，无法确定安全执行次序: %v", cycle)
	}
	return order, nil
}

func normalizeRef(ref string) string {
	ref = strings.TrimSuffix(ref, ":")
	if !strings.Contains(ref, ":") {
		return "table:" + ref
	}
	return ref
}

func isDestructive(s model.MigrationStep) bool {
	switch s.StepType {
	case "drop_table", "drop_column", "alter_type", "drop_index", "drop_view", "drop_fk":
		return true
	}
	return false
}

// findCycleSteps 返回仍具有依赖环的步骤 seq 列表（诊断信息）。
func findCycleSteps(steps []model.MigrationStep, adj [][]int, indeg []int, n int) []int {
	var inCycle []int
	for i := 0; i < n; i++ {
		if indeg[i] > 0 {
			inCycle = append(inCycle, steps[i].Seq)
		}
	}
	sort.Ints(inCycle)
	return inCycle
}

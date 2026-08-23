package impact

import (
	"testing"

	"task160-migimpact/internal/model"
)

func TestRunDetectsDestructiveChanges(t *testing.T) {
	objs := []model.SchemaObject{
		{ObjType: "table", Name: "orders"},
		{ObjType: "column", TableName: "orders", Name: "id", DataType: "INTEGER"},
		{ObjType: "column", TableName: "orders", Name: "customer_id", DataType: "INTEGER"},
		{ObjType: "table", Name: "customers"},
		{ObjType: "column", TableName: "customers", Name: "id", DataType: "INTEGER"},
	}
	deps := []model.ObjectDependency{
		{DepType: "fk", SourceObj: "fk:orders:fk1", TargetObj: "table:customers", Statement: "orders.customer_id -> customers.id"},
	}
	accs := []model.AccessDeclaration{
		{Service: "order-svc", Interface: "/api/orders", Action: "read", TargetObj: "table:orders"},
	}
	steps := []model.MigrationStep{
		{ID: 11, Seq: 1, StepType: "drop_column", TargetObj: "column:orders:customer_id"},
		{ID: 12, Seq: 2, StepType: "drop_table", TargetObj: "orders"},
		{ID: 13, Seq: 3, StepType: "create_table", TargetObj: "audit_log"},
	}
	findings, err := Run(AnalysisInput{Objects: objs, Deps: deps, Access: accs, Steps: steps})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("期望 2 条破坏性变更，得到 %d", len(findings))
	}
	// drop_table 应波及 order-svc 接口
	var tableFinding *Finding
	for i := range findings {
		if findings[i].ChangeType == model.ChangeDropTable {
			tableFinding = &findings[i]
		}
	}
	if tableFinding == nil {
		t.Fatal("缺少 drop_table 变更")
	}
	if len(tableFinding.Interfaces) == 0 {
		t.Fatal("drop_table 应波及 order-svc 接口")
	}
}

func TestOrderPlanTopological(t *testing.T) {
	steps := []model.MigrationStep{
		{Seq: 1, StepType: "drop_column", TargetObj: "column:orders:customer_id"},
		{Seq: 2, StepType: "drop_table", TargetObj: "orders"},
		{Seq: 3, StepType: "create_table", TargetObj: "audit_log"},
	}
	deps := []model.ObjectDependency{
		{DepType: "index_col", SourceObj: "index:orders:idx1", TargetObj: "column:orders:customer_id"},
	}
	order, err := OrderPlan(steps, deps)
	if err != nil {
		t.Fatalf("OrderPlan: %v", err)
	}
	if len(order) != 3 {
		t.Fatalf("次序应含 3 步，得到 %v", order)
	}
	// drop_table 必须排在列操作之后
	posDrop := -1
	posCol := -1
	for i, seq := range order {
		if seq == 2 {
			posDrop = i
		}
		if seq == 1 {
			posCol = i
		}
	}
	if posDrop < posCol {
		t.Fatalf("drop_table 应在列操作之后: %v", order)
	}
}

func TestOrderPlanCycle(t *testing.T) {
	// 构造 A 依赖 B 且 B 依赖 A 的循环
	steps := []model.MigrationStep{
		{Seq: 1, StepType: "drop_table", TargetObj: "a"},
		{Seq: 2, StepType: "drop_table", TargetObj: "b"},
	}
	// a 被 b 引用且 b 被 a 引用 -> 边 a->b 与 b->a 成环
	deps := []model.ObjectDependency{
		{DepType: "fk", SourceObj: "table:b", TargetObj: "table:a", Statement: "b -> a"},
		{DepType: "fk", SourceObj: "table:a", TargetObj: "table:b", Statement: "a -> b"},
	}
	_, err := OrderPlan(steps, deps)
	if err == nil {
		t.Fatal("循环依赖应报错")
	}
}

func TestOrderPlanStableSeq(t *testing.T) {
	steps := []model.MigrationStep{
		{Seq: 5, StepType: "create_table", TargetObj: "x"},
		{Seq: 2, StepType: "create_index", TargetObj: "index:x:i1"},
		{Seq: 9, StepType: "create_view", TargetObj: "view:v1"},
	}
	order, err := OrderPlan(steps, nil)
	if err != nil {
		t.Fatalf("OrderPlan: %v", err)
	}
	// 无依赖时应按 seq 升序稳定输出
	want := []int{2, 5, 9}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("无依赖次序应稳定为 %v，得到 %v", want, order)
		}
	}
}

// TestOrderPlanReturnsSeqNotIndex 断言 OrderPlan 永远返回步骤的真实 Seq 编号，
// 而非步骤在 steps 切片中的下标。步骤编号不连续时（如 10/20/30）这一点尤为关键：
// 下标 0/1/2 会改写真实执行次序，导致计划丢失或错位。
func TestOrderPlanReturnsSeqNotIndex(t *testing.T) {
	steps := []model.MigrationStep{
		{Seq: 10, StepType: "drop_column", TargetObj: "column:orders:customer_id"},
		{Seq: 20, StepType: "drop_table", TargetObj: "orders"},
		{Seq: 30, StepType: "create_table", TargetObj: "audit_log"},
	}
	deps := []model.ObjectDependency{
		{DepType: "index_col", SourceObj: "index:orders:idx1", TargetObj: "column:orders:customer_id"},
	}
	order, err := OrderPlan(steps, deps)
	if err != nil {
		t.Fatalf("OrderPlan: %v", err)
	}
	// 次序只能是步骤 Seq 之一，绝不应出现下标 0/1/2。
	validSeq := map[int]bool{10: true, 20: true, 30: true}
	if len(order) != len(steps) {
		t.Fatalf("次序应含 %d 步，得到 %v", len(steps), order)
	}
	for _, v := range order {
		if !validSeq[v] {
			t.Fatalf("次序中出现非步骤编号 %d（疑似下标），完整次序 %v", v, order)
		}
	}
	// drop_table(Seq=20) 必须排在删列(Seq=10)之后。
	posDrop, posCol := -1, -1
	for i, seq := range order {
		if seq == 20 {
			posDrop = i
		}
		if seq == 10 {
			posCol = i
		}
	}
	if posDrop < posCol {
		t.Fatalf("drop_table 应在列操作之后: %v", order)
	}
}

package service

import (
	"context"
	"testing"

	"task160-migimpact/internal/model"
	"task160-migimpact/internal/store"
)

// TestAccessDeclarationNotDuplicated 断言：一条接口依赖声明在「登记 + 保存」后，
// 在访问声明表中只产生一条记录。
func TestAccessDeclarationNotDuplicated(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	svc := New(store.NewRepositories(st))

	snapID, err := svc.CreateSnapshot(ctx, "repro")
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	objs := []model.SchemaObject{
		{ObjType: "table", Name: "orders"},
	}
	accs := []model.AccessDeclaration{
		{Service: "order-svc", Interface: "/api/orders", Action: "read", TargetObj: "table:orders"},
	}
	if err := svc.LoadSnapshotObjects(ctx, snapID, objs, nil, accs); err != nil {
		t.Fatalf("LoadSnapshotObjects: %v", err)
	}

	got, err := svc.GetSnapshotAccess(ctx, snapID)
	if err != nil {
		t.Fatalf("GetSnapshotAccess: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("一条声明保存后应只产生 1 条访问记录，得到 %d", len(got))
	}
}

// TestImpactInterfaceNotDuplicated 断言：一条接口依赖声明在「登记 + 保存 + 分析」后，
// 受影响接口列表中该接口只出现一次（一条声明 -> 一条影响记录）。
func TestImpactInterfaceNotDuplicated(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	svc := New(store.NewRepositories(st))

	snapID, err := svc.CreateSnapshot(ctx, "repro")
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	objs := []model.SchemaObject{
		{ObjType: "table", Name: "orders"},
		{ObjType: "column", TableName: "orders", Name: "id", DataType: "INTEGER"},
	}
	accs := []model.AccessDeclaration{
		{Service: "order-svc", Interface: "/api/orders", Action: "read", TargetObj: "table:orders"},
	}
	if err := svc.LoadSnapshotObjects(ctx, snapID, objs, nil, accs); err != nil {
		t.Fatalf("LoadSnapshotObjects: %v", err)
	}

	scriptID, err := svc.ImportScript(ctx, "m1", "1 DROP_TABLE [orders]\n", 1)
	if err != nil {
		t.Fatalf("ImportScript: %v", err)
	}
	_, findings, _, err := svc.RunAnalysis(ctx, snapID, scriptID)
	if err != nil {
		t.Fatalf("RunAnalysis: %v", err)
	}

	// 收集所有受影响接口，断言无重复。
	seen := map[string]int{}
	total := 0
	for _, f := range findings {
		for _, iface := range f.Interfaces {
			seen[iface]++
			total++
		}
	}
	if total != len(seen) {
		t.Fatalf("受影响接口存在重复：共 %d 条次，去重后 %d 条：%v", total, len(seen), seen)
	}
	// order-svc /api/orders read 应恰好出现一次。
	if seen["order-svc /api/orders read"] != 1 {
		t.Fatalf("order-svc 接口应恰好出现 1 次，得到 %v", seen)
	}
}

package service

import (
	"context"
	"fmt"

	"task160-migimpact/internal/model"
)

// RunDemo 执行确定性自检（--smoke-test 契约）：
// 1. 创建示例快照并填充对象/依赖/访问声明；
// 2. 导入包含破坏性变更的迁移脚本；
// 3. 执行影响分析，断言识别到破坏性变更；
// 4. 提交豁免并冻结计划；
// 5. 返回全部结果；调用方负责关闭并重开数据库验证持久化。
func (s *Service) RunDemo(ctx context.Context) error {
	snapID, err := s.CreateSnapshot(ctx, "demo-snapshot")
	if err != nil {
		return fmt.Errorf("创建快照: %w", err)
	}
	objs := demoObjects()
	deps := demoDependencies()
	accs := demoAccess()
	if err := s.LoadSnapshotObjects(ctx, snapID, objs, deps, accs); err != nil {
		return fmt.Errorf("填充快照: %w", err)
	}
	snap, err := s.GetSnapshot(ctx, snapID)
	if err != nil {
		return err
	}
	if snap.Status != model.SnapshotRegistered {
		return fmt.Errorf("快照状态错误: %s", snap.Status)
	}
	if snap.ObjectHash == "" {
		return fmt.Errorf("快照对象哈希未固化")
	}

	scriptID, err := s.ImportScript(ctx, "demo-migration", demoScriptContent(), 1)
	if err != nil {
		return fmt.Errorf("导入脚本: %w", err)
	}
	analysis, findings, order, err := s.RunAnalysis(ctx, snapID, scriptID)
	if err != nil {
		return fmt.Errorf("影响分析: %w", err)
	}
	if analysis.Status != model.AnalysisCompleted {
		return fmt.Errorf("分析未完成: %s", analysis.Status)
	}
	if len(findings) == 0 {
		return fmt.Errorf("预期识别到破坏性变更，实际为 0")
	}
	if len(order) == 0 {
		return fmt.Errorf("执行次序为空")
	}

	// 提交豁免
	changes, err := s.ListChanges(ctx, analysis.ID)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		return fmt.Errorf("无破坏性变更可豁免")
	}
	if _, err := s.SubmitExemption(ctx, analysis.ID, changes[0].ID, "smoke-tester", "自检豁免"); err != nil {
		return fmt.Errorf("提交豁免: %w", err)
	}
	plan, err := s.FreezePlan(ctx, analysis.ID)
	if err != nil {
		return fmt.Errorf("冻结计划: %w", err)
	}
	if plan.Status != model.PlanFrozen {
		return fmt.Errorf("计划未冻结: %s", plan.Status)
	}
	// 校验计划哈希稳定
	if plan.PlanHash == "" {
		return fmt.Errorf("计划哈希为空")
	}
	return nil
}

// demoObjects 返回示例快照对象：表、列、索引、视图、外键。
func demoObjects() []model.SchemaObject {
	return []model.SchemaObject{
		{ObjType: "table", Name: "customers", Definition: "customers table"},
		{ObjType: "column", TableName: "customers", Name: "id", DataType: "INTEGER", Definition: "customers.id"},
		{ObjType: "column", TableName: "customers", Name: "name", DataType: "VARCHAR(64)", Definition: "customers.name"},
		{ObjType: "index", TableName: "customers", Name: "idx_customers_name", Unique: false, Definition: "index on customers.name"},
		{ObjType: "table", Name: "orders", Definition: "orders table"},
		{ObjType: "column", TableName: "orders", Name: "id", DataType: "INTEGER", Definition: "orders.id"},
		{ObjType: "column", TableName: "orders", Name: "customer_id", DataType: "INTEGER", Definition: "orders.customer_id"},
		{ObjType: "column", TableName: "orders", Name: "legacy_note", DataType: "VARCHAR(255)", Definition: "orders.legacy_note"},
		{ObjType: "index", TableName: "orders", Name: "idx_orders_customer_id", Unique: false, Definition: "index on orders.customer_id"},
		{ObjType: "fk", TableName: "orders", Name: "fk_orders_customer", Definition: "orders.customer_id -> customers.id"},
		{ObjType: "view", TableName: "", Name: "v_order_summary", Definition: "view over orders"},
	}
}

// demoDependencies 返回示例依赖边。
func demoDependencies() []model.ObjectDependency {
	return []model.ObjectDependency{
		{DepType: "fk", SourceObj: "fk:orders:fk_orders_customer", TargetObj: "table:customers", SourceCol: "customer_id", TargetCol: "id", Statement: "orders.customer_id -> customers.id"},
		{DepType: "view", SourceObj: "view:v_order_summary", TargetObj: "table:orders", Statement: "v_order_summary reads orders"},
		{DepType: "index_col", SourceObj: "index:orders:idx_orders_customer_id", TargetObj: "column:orders:customer_id", Statement: "idx on orders.customer_id"},
	}
}

// demoAccess 返回示例访问声明。
func demoAccess() []model.AccessDeclaration {
	return []model.AccessDeclaration{
		{Service: "order-service", Interface: "/api/order/get", Action: "read", TargetObj: "table:orders", Status: "active"},
		{Service: "order-service", Interface: "/api/order/create", Action: "write", TargetObj: "table:orders", Status: "active"},
		{Service: "customer-service", Interface: "/api/customer/list", Action: "read", TargetObj: "table:customers", Status: "active"},
	}
}

// demoScriptContent 返回示例迁移脚本（含破坏性变更：删列、删索引、删表）。
func demoScriptContent() string {
	return `# 示例迁移：重构订单表
1 DROP_COLUMN [column:orders:legacy_note]
2 ALTER_TYPE [column:orders:customer_id]
3 DROP_INDEX [index:orders:idx_orders_customer_id]
4 DROP_TABLE [orders]
`
}

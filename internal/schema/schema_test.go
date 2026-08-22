package schema

import (
	"strings"
	"testing"

	"task160-migimpact/internal/model"
)

func TestDefinitionStable(t *testing.T) {
	a := model.SchemaObject{ObjType: "column", TableName: "orders", Name: "id", DataType: "INTEGER", Nullable: false}
	b := model.SchemaObject{ObjType: "column", TableName: "orders", Name: "id", DataType: "INTEGER", Nullable: false}
	if Definition(&a) != Definition(&b) {
		t.Fatal("相同定义应一致")
	}
	c := model.SchemaObject{ObjType: "column", TableName: "orders", Name: "id", DataType: "BIGINT", Nullable: false}
	if Definition(&a) == Definition(&c) {
		t.Fatal("不同类型应不同定义")
	}
}

func TestValidateSnapshotObjects(t *testing.T) {
	objs := []model.SchemaObject{
		{ObjType: "table", Name: "orders"},
		{ObjType: "column", TableName: "orders", Name: "id"},
		{ObjType: "column", TableName: "missing", Name: "x"}, // 所属表不存在
	}
	errs := ValidateSnapshotObjects(objs)
	if len(errs) != 1 {
		t.Fatalf("期望 1 个错误，得到 %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "missing") {
		t.Fatalf("错误应提及 missing 表: %v", errs[0])
	}
}

func TestVerifyDependencies(t *testing.T) {
	objs := []model.SchemaObject{
		{ObjType: "table", Name: "customers"},
	}
	deps := []model.ObjectDependency{
		{DepType: "fk", SourceObj: "table:orders", TargetObj: "table:customers", Statement: "orders -> customers"},
		{DepType: "fk", SourceObj: "table:customers", TargetObj: "table:ghost", Statement: "customers -> ghost"},
	}
	errs := VerifyDependencies(objs, deps, nil)
	if len(errs) != 2 {
		t.Fatalf("期望 2 个错误（来源与目标各一），得到 %d: %v", len(errs), errs)
	}
}

func TestHashStableAndSensitive(t *testing.T) {
	objs1 := []model.SchemaObject{{ObjType: "table", Name: "a"}, {ObjType: "table", Name: "b"}}
	objs2 := []model.SchemaObject{{ObjType: "table", Name: "b"}, {ObjType: "table", Name: "a"}}
	objs3 := []model.SchemaObject{{ObjType: "table", Name: "a"}, {ObjType: "table", Name: "c"}}
	h1 := Hash(objs1)
	h2 := Hash(objs2)
	h3 := Hash(objs3)
	if h1 != h2 {
		t.Fatal("排序无关哈希应一致")
	}
	if h1 == h3 {
		t.Fatal("不同对象集哈希应不同")
	}
}

func TestApplyScriptToObjects(t *testing.T) {
	objs := []model.SchemaObject{
		{ObjType: "table", Name: "orders"},
		{ObjType: "column", TableName: "orders", Name: "id"},
		{ObjType: "table", Name: "customers"},
	}
	steps := []model.MigrationStep{
		{Seq: 1, StepType: "drop_table", TargetObj: "orders"},
	}
	after, err := ApplyScriptToObjects(objs, steps)
	if err != nil {
		t.Fatalf("ApplyScriptToObjects: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("删除表后应剩 1 个对象，得到 %d", len(after))
	}
	if after[0].Name != "customers" {
		t.Fatalf("剩余对象应为 customers，得到 %s", after[0].Name)
	}
}

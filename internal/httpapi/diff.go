package httpapi

import (
	"task160-migimpact/internal/model"
	"task160-migimpact/internal/plan"
	"task160-migimpact/internal/schema"
)

// simulateDiff 模拟脚本应用后的对象差异：比较快照基线对象与
// 应用脚本（删除性步骤）后的对象集合。
func simulateDiff(objs []model.SchemaObject, steps []model.MigrationStep) (plan.DiffResult, error) {
	after, err := schema.ApplyScriptToObjects(objs, steps)
	if err != nil {
		return plan.DiffResult{}, err
	}
	return plan.DiffSnapshots(objs, after), nil
}

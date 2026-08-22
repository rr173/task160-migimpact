package script

import (
	"fmt"

	"task160-migimpact/internal/model"
)

// ClassifyStep 判断单个步骤是否构成破坏性变更，并给出变更类型与证据说明。
// 判定规则：
//   - DROP_TABLE / DROP_COLUMN：一律破坏性（数据不可逆）。
//   - ALTER_TYPE：改变数据类型，可能破坏下游读写的格式假设，视为破坏性。
//   - ADD_COLUMN NOT NULL 收紧 / 其他 NOT NULL 收紧：有默认值时可安全，这里按声明处理。
//   - DROP_INDEX：仅当该索引被外键或视图依赖时破坏性（由 impact 层结合依赖图判定）。
//   - DROP_VIEW：仅当视图被其他对象引用时破坏性。
//   - DROP_FK：仅当外键被下游依赖时破坏性。
//   - CREATE_* / ADD_*：安全。
func ClassifyStep(step *model.MigrationStep, hasDependents bool) (model.DestructiveChangeType, string) {
	switch step.StepType {
	case "drop_table":
		return model.ChangeDropTable, "删除整表会永久移除数据并破坏依赖该表的对象与接口"
	case "drop_column":
		return model.ChangeDropColumn, "删除列会永久移除数据并破坏依赖该列的查询"
	case "alter_type":
		return model.ChangeAlterType, "修改列类型可能破坏依赖旧类型的读写下游"
	case "create_table", "add_column", "create_index", "create_view", "add_fk":
		return "", ""
	case "drop_index":
		if hasDependents {
			return model.ChangeDropIndex, "删除的索引被其他对象或约束依赖"
		}
		return "", ""
	case "drop_view":
		if hasDependents {
			return model.ChangeDropView, "删除的视图被其他对象或接口引用"
		}
		return "", ""
	case "drop_fk":
		if hasDependents {
			return model.ChangeDropFK, "删除的外键约束被其他对象依赖"
		}
		return "", ""
	}
	return "", ""
}

// VerifyVersionContinuity 校验脚本版本与既有脚本的连续性：
// 新脚本版本必须为「已导入脚本的最大版本 + 1」；首个脚本版本必须为 1。
func VerifyVersionContinuity(existing []model.MigrationScript, newVersion int) error {
	maxVer := 0
	for _, s := range existing {
		if s.Version > maxVer {
			maxVer = s.Version
		}
	}
	if maxVer == 0 {
		if newVersion != 1 {
			return fmt.Errorf("首个迁移脚本版本必须为 1，实际 %d", newVersion)
		}
		return nil
	}
	if newVersion != maxVer+1 {
		return fmt.Errorf("迁移版本不连续：当前最大版本 %d，新脚本应为 %d，实际 %d", maxVer, maxVer+1, newVersion)
	}
	return nil
}

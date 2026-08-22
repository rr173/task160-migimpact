// Package script 提供迁移脚本的解析、步骤规范化与指纹计算。
package script

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"task160-migimpact/internal/model"
)

// StepSpec 是脚本中一行步骤的声明式描述。
type StepSpec struct {
	Seq       int
	StepType  string // create_table | drop_table | add_column | drop_column | alter_type | create_index | drop_index | create_view | drop_view | add_fk | drop_fk
	TargetObj string // type:table:name 或 table 名
	Detail    string // 人类可读说明
}

// ParseScript 把脚本文本解析为有序步骤列表。
// 每行格式：<seq> <STEP_TYPE> <target> # <detail>
// 空行与 # 开头的行为注释。seq 必须从 1 开始连续递增。
func ParseScript(content string) ([]StepSpec, error) {
	var specs []StepSpec
	expected := 1
	lines := strings.Split(content, "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// 去除行内注释
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("行 %q 格式错误，需要 <seq> <STEP_TYPE> <target>", raw)
		}
		var seq int
		if _, err := fmt.Sscanf(fields[0], "%d", &seq); err != nil {
			return nil, fmt.Errorf("行 %q 序号非法: %v", raw, err)
		}
		if seq != expected {
			return nil, fmt.Errorf("步骤序号不连续：期望 %d 得到 %d", expected, seq)
		}
		stepType := strings.ToUpper(fields[1])
		if !IsKnownType(stepType) {
			return nil, fmt.Errorf("未知步骤类型 %q", stepType)
		}
		target := ""
		if len(fields) >= 3 {
			target = strings.Join(fields[2:], " ")
			target = strings.Trim(target, "[]")
		}
		if target == "" {
			return nil, fmt.Errorf("步骤%d 缺少目标对象", seq)
		}
		specs = append(specs, StepSpec{Seq: seq, StepType: stepType, TargetObj: target, Detail: target})
		expected++
	}
	if len(specs) == 0 {
		return nil, fmt.Errorf("脚本没有可解析的步骤")
	}
	return specs, nil
}

// IsKnownType 判断步骤类型是否受支持。
func IsKnownType(t string) bool {
	switch t {
	case "CREATE_TABLE", "DROP_TABLE", "ADD_COLUMN", "DROP_COLUMN", "ALTER_TYPE",
		"CREATE_INDEX", "DROP_INDEX", "CREATE_VIEW", "DROP_VIEW", "ADD_FK", "DROP_FK":
		return true
	}
	return false
}

// ToModel 把解析结果转为持久化模型。
func ToModel(scriptID int64, specs []StepSpec) []model.MigrationStep {
	steps := make([]model.MigrationStep, 0, len(specs))
	for _, sp := range specs {
		steps = append(steps, model.MigrationStep{
			ScriptID:  scriptID,
			Seq:       sp.Seq,
			StepType:  normalizeType(sp.StepType),
			TargetObj: sp.TargetObj,
			Detail:    sp.Detail,
			Status:    model.StepPending,
		})
	}
	return steps
}

func normalizeType(t string) string {
	return strings.ToLower(strings.ReplaceAll(t, "_", "_"))
}

// Fingerprint 计算脚本内容指纹（幂等判重键）。
func Fingerprint(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// NormalizeTarget 把 "table:orders" 规范化为对象引用文本；无前缀时视为表名。
func NormalizeTarget(target string) string {
	if strings.Contains(target, ":") {
		return target
	}
	return "table:" + target
}

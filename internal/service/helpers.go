package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"task160-migimpact/internal/model"
	"task160-migimpact/internal/script"
	"task160-migimpact/internal/store"
)

// hashObj 计算单个对象定义哈希。
// 唯一性约束是影响语义的属性，必须参与哈希，否则唯一性翻转不会改变哈希，
// 使模式定义与持久化哈希在迁移评估流程中无法反映该变化。
func hashObj(o *model.SchemaObject) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%s|%t|%t", o.ObjType, o.TableName, o.Name, o.DataType, o.Nullable, o.Unique)
	return hex.EncodeToString(h.Sum(nil))
}

// objectSetHash 计算对象集合哈希。
func objectSetHash(objs []model.SchemaObject) string {
	hashes := make([]string, 0, len(objs))
	for _, o := range objs {
		hashes = append(hashes, o.Hash)
	}
	sort.Strings(hashes)
	h := sha256.New()
	for _, s := range hashes {
		h.Write([]byte(s))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// depSetHash 计算依赖+访问声明集合哈希。
func depSetHash(deps []model.ObjectDependency, accs []model.AccessDeclaration) string {
	parts := make([]string, 0, len(deps)+len(accs))
	for _, d := range deps {
		parts = append(parts, fmt.Sprintf("dep|%s|%s|%s|%s|%s", d.DepType, d.SourceObj, d.TargetObj, d.SourceCol, d.TargetCol))
	}
	for _, a := range accs {
		parts = append(parts, fmt.Sprintf("acc|%s|%s|%s|%s", a.Service, a.Interface, a.Action, a.TargetObj))
	}
	sort.Strings(parts)
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// verifyVersion 校验脚本版本连续性。
func verifyVersion(existing []model.MigrationScript, version int) error {
	maxVer := 0
	for _, sc := range existing {
		if sc.Version > maxVer {
			maxVer = sc.Version
		}
	}
	if maxVer == 0 {
		if version != 1 {
			return fmt.Errorf("首个脚本版本必须为 1，实际 %d", version)
		}
		return nil
	}
	if version != maxVer+1 {
		return fmt.Errorf("脚本版本不连续：当前最大 %d，新版本应为 %d，实际 %d", maxVer, maxVer+1, version)
	}
	return nil
}

// parseScriptSteps 解析脚本内容为步骤模型。
func parseScriptSteps(ctx context.Context, repos *store.Repositories, scriptID int64, content string) ([]model.MigrationStep, error) {
	_ = ctx
	_ = repos
	specs, err := script.ParseScript(content)
	if err != nil {
		return nil, err
	}
	return script.ToModel(scriptID, specs), nil
}

// normalizeStepTarget 规范化步骤目标对象引用（供影响分析使用）。
func normalizeStepTarget(t string) string {
	t = strings.TrimSuffix(t, ":")
	if !strings.Contains(t, ":") {
		return "table:" + t
	}
	return t
}

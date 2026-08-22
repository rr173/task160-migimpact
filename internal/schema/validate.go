package schema

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"task160-migimpact/internal/model"
)

func sha256Of(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// VerifyDependencies 校验依赖边与访问声明引用的对象在快照中真实存在，
// 拒绝悬空引用（未知对象）。返回规范化错误列表。
func VerifyDependencies(objs []model.SchemaObject, deps []model.ObjectDependency, accs []model.AccessDeclaration) []error {
	idx := NewIndex(objs)
	var errs []error
	for _, d := range deps {
		if d.DepType == "fk" || d.DepType == "view" || d.DepType == "index_col" {
			srcKey, err1 := parseObjRef(d.SourceObj)
			if err1 != nil {
				errs = append(errs, fmt.Errorf("依赖 %q 来源非法: %w", d.Statement, err1))
				continue
			}
			if _, ok := idx.Get(srcKey); !ok {
				errs = append(errs, fmt.Errorf("依赖 %q 来源对象 %q 不存在", d.Statement, d.SourceObj))
			}
			tgtKey, err2 := parseObjRef(d.TargetObj)
			if err2 != nil {
				errs = append(errs, fmt.Errorf("依赖 %q 目标非法: %w", d.Statement, err2))
				continue
			}
			if _, ok := idx.Get(tgtKey); !ok {
				errs = append(errs, fmt.Errorf("依赖 %q 目标对象 %q 不存在", d.Statement, d.TargetObj))
			}
		}
	}
	for _, a := range accs {
		tgtKey, err := parseObjRef(a.TargetObj)
		if err != nil {
			errs = append(errs, fmt.Errorf("访问声明 %s/%s 目标非法: %w", a.Service, a.Interface, err))
			continue
		}
		if _, ok := idx.Get(tgtKey); !ok {
			errs = append(errs, fmt.Errorf("访问声明 %s/%s 引用的对象 %q 不存在", a.Service, a.Interface, a.TargetObj))
		}
	}
	return errs
}

// parseObjRef 解析 "type:table:name" 引用；表级引用兼容 "table:orders:"。
func parseObjRef(ref string) (ObjectKey, error) {
	if strings.HasPrefix(ref, "table:") {
		// table:orders 与 table:orders: 均合法
		rest := strings.TrimPrefix(ref, "table:")
		rest = strings.TrimSuffix(rest, ":")
		if rest == "" {
			return ObjectKey{}, fmt.Errorf("empty table ref")
		}
		return ObjectKey{ObjType: "table", TableName: "", Name: rest}, nil
	}
	return ParseObjectKey(ref)
}

// ValidateSnapshotObjects 检查对象集合内部一致性：
// 列/索引/外键必须归属已存在的表，不能重复定义。
func ValidateSnapshotObjects(objs []model.SchemaObject) []error {
	var errs []error
	tableSet := map[string]bool{}
	for _, o := range objs {
		if o.ObjType == "table" {
			if tableSet[o.Name] {
				errs = append(errs, fmt.Errorf("表 %q 重复定义", o.Name))
			}
			tableSet[o.Name] = true
		}
	}
	for _, o := range objs {
		switch o.ObjType {
		case "table":
		case "sequence":
			// 序列独立存在，无归属表
		case "column", "index", "fk":
			if o.TableName == "" {
				errs = append(errs, fmt.Errorf("%s %q 缺少所属表", o.ObjType, o.Name))
			} else if !tableSet[o.TableName] {
				errs = append(errs, fmt.Errorf("%s %q 所属表 %q 不存在", o.ObjType, o.Name, o.TableName))
			}
		default:
			errs = append(errs, fmt.Errorf("未知对象类型 %q", o.ObjType))
		}
	}
	return errs
}

// ApplyScriptToObjects 模拟脚本步骤对对象集合的作用，返回移除/变更后的对象视图。
// 仅用于影响分析；不真正修改快照。
func ApplyScriptToObjects(objs []model.SchemaObject, steps []model.MigrationStep) ([]model.SchemaObject, error) {
	working := make([]model.SchemaObject, len(objs))
	copy(working, objs)
	idx := NewIndex(working)
	for _, step := range steps {
		switch step.StepType {
		case "drop_table":
			key := TableKey(step.TargetObj)
			if _, ok := idx.Get(key); !ok {
				return nil, fmt.Errorf("步骤%d: 删除不存在的表 %q", step.Seq, step.TargetObj)
			}
			working = removeTable(working, step.TargetObj)
			idx = NewIndex(working)
		case "drop_column":
			key, err := parseObjRef(step.TargetObj)
			if err != nil {
				return nil, err
			}
			if _, ok := idx.Get(key); !ok {
				return nil, fmt.Errorf("步骤%d: 删除不存在的列 %q", step.Seq, step.TargetObj)
			}
			working = removeObject(working, key)
			idx = NewIndex(working)
		case "drop_index", "drop_view", "drop_fk":
			key, err := parseObjRef(step.TargetObj)
			if err != nil {
				return nil, err
			}
			working = removeObject(working, key)
			idx = NewIndex(working)
		}
	}
	return working, nil
}

func removeTable(objs []model.SchemaObject, table string) []model.SchemaObject {
	out := objs[:0]
	for _, o := range objs {
		if o.TableName == table {
			continue
		}
		if o.ObjType == "table" && o.Name == table {
			continue
		}
		out = append(out, o)
	}
	return out
}

func removeObject(objs []model.SchemaObject, key ObjectKey) []model.SchemaObject {
	out := objs[:0]
	for _, o := range objs {
		if o.ObjType == key.ObjType && o.TableName == key.TableName && o.Name == key.Name {
			continue
		}
		out = append(out, o)
	}
	return out
}

// Package schema 提供快照对象建模、定义哈希与对象查找工具。
package schema

import (
	"fmt"
	"sort"
	"strings"

	"task160-migimpact/internal/model"
)

// ObjectKey 唯一标识一个 schema 对象（objType:tableName:name）。
type ObjectKey struct {
	ObjType   string
	TableName string
	Name      string
}

func (k ObjectKey) String() string {
	return fmt.Sprintf("%s:%s:%s", k.ObjType, k.TableName, k.Name)
}

// ParseObjectKey 从 "type:table:name" 文本解析对象键。
func ParseObjectKey(s string) (ObjectKey, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 || parts[0] == "" || parts[2] == "" {
		return ObjectKey{}, fmt.Errorf("invalid object key %q, want type:table:name", s)
	}
	return ObjectKey{ObjType: parts[0], TableName: parts[1], Name: parts[2]}, nil
}

// TableKey 返回表对象键（column/index/fk 通过表名关联）。
func TableKey(table string) ObjectKey { return ObjectKey{ObjType: "table", Name: table} }

// Definition 依据对象属性生成稳定规范化定义，供哈希与去重使用。
func Definition(o *model.SchemaObject) string {
	var b strings.Builder
	b.WriteString(o.ObjType)
	b.WriteString("|")
	b.WriteString(o.TableName)
	b.WriteString("|")
	b.WriteString(o.Name)
	b.WriteString("|")
	b.WriteString(o.DataType)
	b.WriteString("|nullable=")
	if o.Nullable {
		b.WriteString("1")
	} else {
		b.WriteString("0")
	}
	return b.String()
}

// Index 构建对象索引，用于按对象键快速查找。
type Index struct {
	byKey map[ObjectKey]*model.SchemaObject
}

// NewIndex 从对象列表构建索引。
func NewIndex(objs []model.SchemaObject) *Index {
	idx := &Index{byKey: make(map[ObjectKey]*model.SchemaObject, len(objs))}
	for i := range objs {
		key := ObjectKey{ObjType: objs[i].ObjType, TableName: objs[i].TableName, Name: objs[i].Name}
		o := objs[i]
		idx.byKey[key] = &o
	}
	return idx
}

// Get 按对象键取对象。
func (ix *Index) Get(k ObjectKey) (*model.SchemaObject, bool) {
	o, ok := ix.byKey[k]
	return o, ok
}

// TableExists 判断表是否存在。
func (ix *Index) TableExists(table string) bool {
	_, ok := ix.byKey[TableKey(table)]
	return ok
}

// ObjectsOf 返回快照中某表的所有列。
func (ix *Index) ObjectsOf(table string) []model.SchemaObject {
	var out []model.SchemaObject
	for _, o := range ix.byKey {
		if o.TableName == table {
			out = append(out, *o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Hash 计算所有对象定义排序后的 SHA-256 摘要。
func Hash(objs []model.SchemaObject) string {
	defs := make([]string, 0, len(objs))
	for _, o := range objs {
		defs = append(defs, Definition(&o))
	}
	sort.Strings(defs)
	h := sha256Of(strings.Join(defs, "\n"))
	return h
}

// DepHash 计算依赖边与访问声明的摘要。
func DepHash(deps []model.ObjectDependency, accs []model.AccessDeclaration) string {
	parts := make([]string, 0, len(deps)+len(accs))
	for _, d := range deps {
		parts = append(parts, fmt.Sprintf("dep|%s|%s|%s|%s|%s", d.DepType, d.SourceObj, d.TargetObj, d.SourceCol, d.TargetCol))
	}
	for _, a := range accs {
		parts = append(parts, fmt.Sprintf("acc|%s|%s|%s|%s", a.Service, a.Interface, a.Action, a.TargetObj))
	}
	sort.Strings(parts)
	return sha256Of(strings.Join(parts, "\n"))
}

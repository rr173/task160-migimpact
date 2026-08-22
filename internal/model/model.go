package model

import "time"

// SchemaSnapshot 是一个数据库模式快照：一组对象及其依赖关系的不可变基线。
// 工程师登记快照后，迁移脚本的影响分析以该快照为比较对象。
type SchemaSnapshot struct {
	ID        int64
	Name      string
	Status    StateSnapshot
	CreatedAt time.Time
	FrozenAt  *time.Time
	// ObjectHash 是所有对象定义拼接后的 SHA-256，用于重复导入幂等与版本比较。
	ObjectHash string
	// DepHash 是所有依赖边摘要，冻结时一并固化。
	DepHash string
}

// SchemaObject 是快照中的一个数据库对象：表、列、索引、视图、外键或序列。
// 对象定义以规范化文本保存（definition），任何属性变化都会改变定义哈希。
type SchemaObject struct {
	ID         int64
	SnapshotID int64
	ObjType    string // table | column | index | view | fk | sequence
	TableName  string // 列/索引/外键所属的表；表对象时为空
	Name       string
	DataType   string // 列/序列时有效，例如 INTEGER、VARCHAR(64)
	Nullable   bool   // 列时有效
	Unique     bool   // 索引/列时有效
	Definition string // 规范化定义摘要
	Status     StateObject
	Hash       string
}

// ObjectDependency 表达对象之间的引用关系，是影响传播与执行次序判定的依据。
// Source 引用 Target；例如 orders 表的外键列引用 customers(id)，则
// Source=orders.table, Target=customers.table。
type ObjectDependency struct {
	ID         int64
	SnapshotID int64
	DepType    string // fk | view | index_col | access
	SourceObj  string // 形如 table:orders
	TargetObj  string // 形如 table:customers
	SourceCol  string
	TargetCol  string
	// Statement 是人类可读的引用证据，例如 "orders.customer_id -> customers.id"。
	Statement string
}

// AccessDeclaration 声明一个服务接口对 schema 对象的读写依赖。
// 破坏性变更会波及声明了对该对象读写的接口。
type AccessDeclaration struct {
	ID         int64
	SnapshotID int64
	Service    string // 服务名
	Interface  string // 接口路径，例如 /api/order/get
	Action     string // read | write
	TargetObj  string // 形如 table:orders
	Status     string // active | retired
}

// MigrationScript 是一次数据库迁移的版本化脚本，由有序步骤组成。
// Version 必须连续递增；ContentHash 用于重复导入幂等。
type MigrationScript struct {
	ID          int64
	Name        string
	Version     int
	Status      StateScript
	ContentHash string
	Content     string
	CreatedAt   time.Time
}

// MigrationStep 是脚本中的一个 DDL 步骤。
type MigrationStep struct {
	ID         int64
	ScriptID   int64
	Seq        int
	StepType   string // create_table | drop_table | add_column | drop_column | alter_type | create_index | drop_index | create_view | drop_view | add_fk | drop_fk
	TargetObj  string // 形如 table:orders 或 column:orders:name
	Detail     string // 人类可读说明
	Status     StateStep
	ChangeType DestructiveChangeType // 判定为破坏性变更时的类型
	Reason     string                // 破坏性原因/豁免理由
}

// ImpactAnalysis 是一次「快照 + 脚本」的影响分析任务。
type ImpactAnalysis struct {
	ID           int64
	SnapshotID   int64
	ScriptID     int64
	Status       StateAnalysis
	InputHash    string // snapshot.ObjectHash + script.ContentHash
	CreatedAt    time.Time
	CompletedAt  *time.Time
}

// DestructiveChange 是分析中识别出的一条破坏性变更证据。
type DestructiveChange struct {
	ID           int64
	AnalysisID   int64
	StepID       int64
	ChangeType   DestructiveChangeType
	TargetObj    string
	AffectedObjs []string // 受波及对象，形如 table:orders
	Interfaces   []string // 受影响接口
	Status       string   // identified | exempted | rejected
}

// Exemption 是工程师对一条破坏性变更提交的豁免请求。
type Exemption struct {
	ID         int64
	AnalysisID int64
	ChangeID   int64
	Operator   string
	Reason     string
	Status     StateExemption
	CreatedAt  time.Time
}

// MigrationPlan 是冻结后的迁移计划：绑定快照与脚本，不可改写。
type MigrationPlan struct {
	ID         int64
	AnalysisID int64
	SnapshotID int64
	ScriptID   int64
	Status     StatePlan
	StepOrder  []int // 安全执行次序（脚本步骤 seq 序列）
	PlanHash   string
	CreatedAt  time.Time
	FrozenAt   *time.Time
}

// AuditEvent 是追加式审计记录。
type AuditEvent struct {
	ID        int64
	Actor     string
	Action    string
	Subject   string
	Detail    string
	CreatedAt time.Time
}

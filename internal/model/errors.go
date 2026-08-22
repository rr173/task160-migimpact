// Package model 定义数据迁移影响范围分析服务的核心实体、状态与错误类型。
package model

import "errors"

// 通用错误。业务层通过 errors.Is 判断错误类别，HTTP 层据此映射状态码。
var (
	ErrNotFound       = errors.New("resource not found")
	ErrConflict       = errors.New("conflict with existing state")
	ErrInvalid        = errors.New("invalid input")
	ErrFrozen         = errors.New("resource is frozen and cannot be modified")
	ErrDupFingerprint = errors.New("duplicate content fingerprint")
	ErrUnavailable    = errors.New("resource unavailable in current state")
)

// StateSnapshot 描述模式快照的生命周期状态。
type StateSnapshot string

const (
	SnapshotDraft       StateSnapshot = "draft"
	SnapshotRegistered  StateSnapshot = "registered"
	SnapshotFrozen      StateSnapshot = "frozen"
	SnapshotSuperseded  StateSnapshot = "superseded"
)

// StateObject 描述 schema 对象在分析过程中的影响状态。
type StateObject string

const (
	ObjectOK        StateObject = "ok"
	ObjectImpacted  StateObject = "impacted"
	ObjectRemoved   StateObject = "removed"
)

// StateScript 描述迁移脚本状态。
type StateScript string

const (
	ScriptDraft     StateScript = "draft"
	ScriptImported  StateScript = "imported"
	ScriptAnalyzing StateScript = "analyzing"
	ScriptHasBlocker StateScript = "has_blocker"
	ScriptApproved  StateScript = "approved"
)

// StateStep 描述迁移步骤的判定状态。
type StateStep string

const (
	StepPending      StateStep = "pending"
	StepAnalyzed     StateStep = "analyzed"
	StepSafe         StateStep = "safe"
	StepDestructive  StateStep = "destructive"
	StepExempted     StateStep = "exempted"
	StepBlocked      StateStep = "blocked"
)

// StateAnalysis 描述影响分析任务状态。
type StateAnalysis string

const (
	AnalysisQueued     StateAnalysis = "queued"
	AnalysisRunning    StateAnalysis = "running"
	AnalysisCompleted  StateAnalysis = "completed"
	AnalysisBlocked    StateAnalysis = "blocked"
)

// StateExemption 描述豁免请求状态。
type StateExemption string

const (
	ExemptionRequested StateExemption = "requested"
	ExemptionApproved  StateExemption = "approved"
	ExemptionRejected  StateExemption = "rejected"
)

// StatePlan 描述迁移计划状态。
type StatePlan string

const (
	PlanDraft      StatePlan = "draft"
	PlanFrozen     StatePlan = "frozen"
	PlanExecuted   StatePlan = "executed"
	PlanSuperseded StatePlan = "superseded"
)

// DestructiveChangeType 枚举破坏性变更类别。
type DestructiveChangeType string

const (
	ChangeDropColumn   DestructiveChangeType = "drop_column"
	ChangeDropTable    DestructiveChangeType = "drop_table"
	ChangeAlterType    DestructiveChangeType = "alter_type"
	ChangeNotNullTight DestructiveChangeType = "not_null_tighten"
	ChangeDropIndex    DestructiveChangeType = "drop_index_depended"
	ChangeDropView     DestructiveChangeType = "drop_view_referenced"
	ChangeDropFK       DestructiveChangeType = "drop_fk_depended"
)

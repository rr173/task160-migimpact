// Package service 编排影响分析业务：快照管理、脚本导入、分析执行、豁免与计划冻结。
package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"task160-migimpact/internal/model"
	"task160-migimpact/internal/store"
)

// Service 是业务编排入口。
type Service struct {
	repos *store.Repositories
	now   func() time.Time
}

// New 构造服务。
func New(repos *store.Repositories) *Service {
	return &Service{repos: repos, now: time.Now}
}

// --- 快照 ---

// CreateSnapshot 创建空快照并返回 ID。
func (s *Service) CreateSnapshot(ctx context.Context, name string) (int64, error) {
	if name == "" {
		return 0, fmt.Errorf("快照名称不能为空")
	}
	snap := &model.SchemaSnapshot{
		Name:      name,
		Status:    model.SnapshotDraft,
		CreatedAt: s.now(),
	}
	id, err := s.repos.Snapshots.CreateSnapshot(ctx, snap)
	if err != nil {
		return 0, err
	}
	s.audit(ctx, "system", "snapshot.create", fmt.Sprintf("snapshot:%d", id), name)
	return id, nil
}

// LoadSnapshotObjects 用对象/依赖/访问声明填充快照，校验后固化哈希并转为 registered。
func (s *Service) LoadSnapshotObjects(ctx context.Context, snapshotID int64,
	objs []model.SchemaObject, deps []model.ObjectDependency, accs []model.AccessDeclaration) error {
	snap, err := s.repos.Snapshots.GetSnapshot(ctx, snapshotID)
	if err != nil {
		return err
	}
	if snap.Status != model.SnapshotDraft && snap.Status != model.SnapshotRegistered {
		return model.ErrFrozen
	}
	for i := range objs {
		objs[i].SnapshotID = snapshotID
		objs[i].Status = model.ObjectOK
		objs[i].Hash = hashObj(&objs[i])
	}
	for i := range deps {
		deps[i].SnapshotID = snapshotID
	}
	for i := range accs {
		accs[i].SnapshotID = snapshotID
	}
	objHash := objectSetHash(objs)
	depHash := depSetHash(deps, accs)
	if err := s.repos.Snapshots.UpsertObjects(ctx, snapshotID, objs); err != nil {
		return err
	}
	if err := s.repos.Snapshots.ReplaceDependencies(ctx, snapshotID, deps); err != nil {
		return err
	}
	if err := s.repos.Snapshots.ReplaceAccess(ctx, snapshotID, accs); err != nil {
		return err
	}
	if err := s.repos.Snapshots.SetSnapshotHashes(ctx, snapshotID, objHash, depHash); err != nil {
		return err
	}
	if err := s.repos.Snapshots.UpdateSnapshotStatus(ctx, snapshotID, model.SnapshotRegistered, nil); err != nil {
		return err
	}
	s.audit(ctx, "system", "snapshot.load", fmt.Sprintf("snapshot:%d", snapshotID), fmt.Sprintf("objects=%d deps=%d access=%d", len(objs), len(deps), len(accs)))
	return nil
}

// ListSnapshots 列出快照。
func (s *Service) ListSnapshots(ctx context.Context) ([]model.SchemaSnapshot, error) {
	return s.repos.Snapshots.ListSnapshots(ctx)
}

// GetSnapshot 取快照。
func (s *Service) GetSnapshot(ctx context.Context, id int64) (*model.SchemaSnapshot, error) {
	return s.repos.Snapshots.GetSnapshot(ctx, id)
}

// GetSnapshotObjects 取快照对象。
func (s *Service) GetSnapshotObjects(ctx context.Context, id int64) ([]model.SchemaObject, error) {
	return s.repos.Snapshots.ListObjects(ctx, id)
}

// GetSnapshotDeps 取快照依赖。
func (s *Service) GetSnapshotDeps(ctx context.Context, id int64) ([]model.ObjectDependency, error) {
	return s.repos.Snapshots.ListDependencies(ctx, id)
}

// GetSnapshotAccess 取快照访问声明。
func (s *Service) GetSnapshotAccess(ctx context.Context, id int64) ([]model.AccessDeclaration, error) {
	return s.repos.Snapshots.ListAccess(ctx, id)
}

// --- 脚本 ---

// ImportScript 导入脚本：解析步骤、校验版本连续性、幂等判重。
// 返回脚本 ID；若内容指纹已存在返回既有脚本 ID 与 ErrDupFingerprint。
func (s *Service) ImportScript(ctx context.Context, name, content string, version int) (int64, error) {
	if name == "" || content == "" {
		return 0, fmt.Errorf("脚本名称与内容不能为空")
	}
	hash := sha256Hex(content)
	if existing, err := s.repos.Scripts.GetScriptByHash(ctx, hash); err == nil {
		return existing.ID, model.ErrDupFingerprint
	}
	all, err := s.repos.Scripts.ListScripts(ctx)
	if err != nil {
		return 0, err
	}
	if err := verifyVersion(all, version); err != nil {
		return 0, err
	}
	script := &model.MigrationScript{
		Name:        name,
		Version:     version,
		Status:      model.ScriptImported,
		ContentHash: hash,
		Content:     content,
		CreatedAt:   s.now(),
	}
	id, err := s.repos.Scripts.CreateScript(ctx, script)
	if err != nil {
		return 0, err
	}
	steps, err := parseScriptSteps(ctx, s.repos, id, content)
	if err != nil {
		return 0, err
	}
	if err := s.repos.Scripts.CreateSteps(ctx, id, steps); err != nil {
		return 0, err
	}
	s.audit(ctx, "system", "script.import", fmt.Sprintf("script:%d", id), fmt.Sprintf("version=%d steps=%d", version, len(steps)))
	return id, nil
}

// GetScript 取脚本。
func (s *Service) GetScript(ctx context.Context, id int64) (*model.MigrationScript, error) {
	return s.repos.Scripts.GetScript(ctx, id)
}

// ListScripts 列脚本。
func (s *Service) ListScripts(ctx context.Context) ([]model.MigrationScript, error) {
	return s.repos.Scripts.ListScripts(ctx)
}

// GetScriptSteps 取脚本步骤。
func (s *Service) GetScriptSteps(ctx context.Context, id int64) ([]model.MigrationStep, error) {
	return s.repos.Scripts.ListSteps(ctx, id)
}

// --- 审计 ---

func (s *Service) audit(ctx context.Context, actor, action, subject, detail string) {
	ev := &model.AuditEvent{Actor: actor, Action: action, Subject: subject, Detail: detail, CreatedAt: s.now()}
	_ = s.repos.Analyses.AppendAudit(ctx, ev)
}

// ListAudit 列审计。
func (s *Service) ListAudit(ctx context.Context, limit int) ([]model.AuditEvent, error) {
	return s.repos.Analyses.ListAudit(ctx, limit)
}

func (s *Service) nowTime() time.Time { return s.now() }

// sortBySeq 供内部使用。
func sortBySeq(steps []model.MigrationStep) {
	sort.Slice(steps, func(i, j int) bool { return steps[i].Seq < steps[j].Seq })
}

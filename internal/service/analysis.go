package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"task160-migimpact/internal/impact"
	"task160-migimpact/internal/model"
	"task160-migimpact/internal/plan"
)

// RunAnalysis 对指定快照与脚本执行影响分析。
// 幂等：同一 (snapshot, script) 已存在且「已完成」的分析直接返回既有结果。
// 未完成（queued/running/blocked）的分析视为崩溃残留，重置并重新计算，以保证
// 重启后影响范围（受影响对象与接口）在变更记录中完整。
func (s *Service) RunAnalysis(ctx context.Context, snapshotID, scriptID int64) (*model.ImpactAnalysis, []impact.Finding, []int, error) {
	snap, err := s.repos.Snapshots.GetSnapshot(ctx, snapshotID)
	if err != nil {
		return nil, nil, nil, err
	}
	scr, err := s.repos.Scripts.GetScript(ctx, scriptID)
	if err != nil {
		return nil, nil, nil, err
	}
	objs, err := s.repos.Snapshots.ListObjects(ctx, snapshotID)
	if err != nil {
		return nil, nil, nil, err
	}
	deps, err := s.repos.Snapshots.ListDependencies(ctx, snapshotID)
	if err != nil {
		return nil, nil, nil, err
	}
	accs, err := s.repos.Snapshots.ListAccess(ctx, snapshotID)
	if err != nil {
		return nil, nil, nil, err
	}
	steps, err := s.repos.Scripts.ListSteps(ctx, scriptID)
	if err != nil {
		return nil, nil, nil, err
	}
	inputHash := inputFingerprint(snap.ObjectHash, scr.ContentHash)

	// 幂等：已完成的分析直接返回既有结果
	if existing, err := s.repos.Analyses.GetAnalysisByInput(ctx, snapshotID, scriptID); err == nil {
		if existing.Status == model.AnalysisCompleted {
			findings := s.loadFindings(ctx, existing.ID)
			order := s.loadOrder(ctx, existing.ID, steps)
			return existing, findings, order, nil
		}
		// 未完成（崩溃残留）：重置为 queued 后重新计算
		if err := s.repos.Analyses.UpdateAnalysisStatus(ctx, existing.ID, model.AnalysisQueued, nil); err != nil {
			return nil, nil, nil, err
		}
		return s.executeAnalysis(ctx, existing.ID, snapshotID, scriptID, objs, deps, accs, steps)
	}

	analysis := &model.ImpactAnalysis{
		SnapshotID: snapshotID,
		ScriptID:   scriptID,
		Status:     model.AnalysisQueued,
		InputHash:  inputHash,
		CreatedAt:  s.now(),
	}
	id, err := s.repos.Analyses.CreateAnalysis(ctx, analysis)
	if err != nil && !errors.Is(err, model.ErrConflict) {
		return nil, nil, nil, err
	}
	if errors.Is(err, model.ErrConflict) {
		// 并发创建：读取既有；未完成则重算，已完成则返回缓存
		existing, gerr := s.repos.Analyses.GetAnalysisByInput(ctx, snapshotID, scriptID)
		if gerr != nil {
			return nil, nil, nil, gerr
		}
		if existing.Status == model.AnalysisCompleted {
			findings := s.loadFindings(ctx, existing.ID)
			order := s.loadOrder(ctx, existing.ID, steps)
			return existing, findings, order, nil
		}
		if err := s.repos.Analyses.UpdateAnalysisStatus(ctx, existing.ID, model.AnalysisQueued, nil); err != nil {
			return nil, nil, nil, err
		}
		return s.executeAnalysis(ctx, existing.ID, snapshotID, scriptID, objs, deps, accs, steps)
	}

	return s.executeAnalysis(ctx, id, snapshotID, scriptID, objs, deps, accs, steps)
}

// executeAnalysis 执行分析主体：置 running → 运行 impact.Run + OrderPlan →
// 写入破坏性变更 → 更新步骤状态 → 置 completed。重算时先清空旧变更记录。
func (s *Service) executeAnalysis(ctx context.Context, id, snapshotID, scriptID int64,
	objs []model.SchemaObject, deps []model.ObjectDependency, accs []model.AccessDeclaration,
	steps []model.MigrationStep) (*model.ImpactAnalysis, []impact.Finding, []int, error) {
	if err := s.repos.Analyses.UpdateAnalysisStatus(ctx, id, model.AnalysisRunning, nil); err != nil {
		return nil, nil, nil, err
	}
	// 重算前清空旧的破坏性变更，避免重复或脏数据
	if err := s.repos.Analyses.DeleteChanges(ctx, id); err != nil {
		return nil, nil, nil, err
	}

	// 执行分析
	in := impact.AnalysisInput{Objects: objs, Deps: deps, Access: accs, Steps: steps}
	findings, err := impact.Run(in)
	if err != nil {
		_ = s.repos.Analyses.UpdateAnalysisStatus(ctx, id, model.AnalysisBlocked, nil)
		return nil, nil, nil, err
	}
	// 执行次序（循环依赖会报错）
	order, err := impact.OrderPlan(steps, deps)
	if err != nil {
		_ = s.repos.Analyses.UpdateAnalysisStatus(ctx, id, model.AnalysisBlocked, nil)
		return nil, nil, nil, err
	}

	// 写入破坏性变更
	changes := make([]model.DestructiveChange, 0, len(findings))
	for _, f := range findings {
		changes = append(changes, model.DestructiveChange{
			AnalysisID:   id,
			StepID:       f.StepID,
			ChangeType:   f.ChangeType,
			TargetObj:    f.TargetObj,
			AffectedObjs: f.AffectedObjs,
			Interfaces:   f.Interfaces,
			Status:       "identified",
		})
	}
	if err := s.repos.Analyses.CreateChanges(ctx, id, changes); err != nil {
		return nil, nil, nil, err
	}

	// 更新步骤判定状态
	for _, f := range findings {
		step, serr := s.repos.Scripts.GetStep(ctx, f.StepID)
		if serr == nil {
			_ = s.repos.Scripts.UpdateStep(ctx, step.ID, model.StepDestructive, f.ChangeType, f.Evidence)
		}
	}

	now := s.now()
	if err := s.repos.Analyses.UpdateAnalysisStatus(ctx, id, model.AnalysisCompleted, &now); err != nil {
		return nil, nil, nil, err
	}
	if err := s.repos.Scripts.UpdateScriptStatus(ctx, scriptID, model.ScriptApproved); err != nil {
		return nil, nil, nil, err
	}
	completed, err := s.repos.Analyses.GetAnalysis(ctx, id)
	if err != nil {
		return nil, nil, nil, err
	}
	s.audit(ctx, "system", "analysis.run", fmt.Sprintf("analysis:%d", id),
		fmt.Sprintf("snapshot=%d script=%d findings=%d order=%v", snapshotID, scriptID, len(findings), order))
	return completed, findings, order, nil
}

// loadFindings 读取已持久化的破坏性变更。
func (s *Service) loadFindings(ctx context.Context, analysisID int64) []impact.Finding {
	changes, err := s.repos.Analyses.ListChanges(ctx, analysisID)
	if err != nil {
		return nil
	}
	out := make([]impact.Finding, 0, len(changes))
	for _, c := range changes {
		out = append(out, impact.Finding{
			StepID:       c.StepID,
			ChangeType:   c.ChangeType,
			TargetObj:    c.TargetObj,
			AffectedObjs: c.AffectedObjs,
			Interfaces:   c.Interfaces,
		})
	}
	return out
}

// loadOrder 读取已冻结计划的执行次序；无计划时按 seq 排序返回。
func (s *Service) loadOrder(ctx context.Context, analysisID int64, steps []model.MigrationStep) []int {
	plans, err := s.repos.Analyses.ListPlans(ctx)
	if err == nil {
		for _, p := range plans {
			if p.AnalysisID == analysisID {
				return p.StepOrder
			}
		}
	}
	out := make([]int, 0, len(steps))
	for _, st := range steps {
		out = append(out, st.Seq)
	}
	return out
}

// inputFingerprint 计算分析输入指纹。
func inputFingerprint(objHash, scriptHash string) string {
	return fmt.Sprintf("%s|%s", objHash, scriptHash)
}

// GetAnalysis 取分析详情。
func (s *Service) GetAnalysis(ctx context.Context, id int64) (*model.ImpactAnalysis, error) {
	return s.repos.Analyses.GetAnalysis(ctx, id)
}

// ListAnalyses 列分析。
func (s *Service) ListAnalyses(ctx context.Context) ([]model.ImpactAnalysis, error) {
	return s.repos.Analyses.ListAnalyses(ctx)
}

// ListChanges 列分析的破坏性变更。
func (s *Service) ListChanges(ctx context.Context, analysisID int64) ([]model.DestructiveChange, error) {
	return s.repos.Analyses.ListChanges(ctx, analysisID)
}

// Recover 重启恢复：把排队/分析中的任务交给 RunAnalysis 重新执行。
// RunAnalysis 对未完成的分析会重置并重算（含重写破坏性变更），从而保证重启后
// 崩溃残留的分析不再缺失影响范围记录。
func (s *Service) Recover(ctx context.Context) (int, error) {
	queued, err := s.repos.Analyses.ListQueuedAnalyses(ctx)
	if err != nil {
		return 0, err
	}
	for _, a := range queued {
		if _, _, _, err := s.RunAnalysis(ctx, a.SnapshotID, a.ScriptID); err != nil {
			return 0, err
		}
	}
	return len(queued), nil
}

// SubmitExemption 提交豁免；批准后重算阻断集合。
func (s *Service) SubmitExemption(ctx context.Context, analysisID, changeID int64, operator, reason string) (int64, error) {
	if operator == "" || reason == "" {
		return 0, fmt.Errorf("操作人与理由不能为空")
	}
	change, err := s.repos.Analyses.GetChange(ctx, changeID)
	if err != nil {
		return 0, err
	}
	if err := plan.ExemptChange(change, analysisID); err != nil {
		return 0, err
	}
	e := &model.Exemption{
		AnalysisID: analysisID,
		ChangeID:   changeID,
		Operator:   operator,
		Reason:     reason,
		Status:     model.ExemptionApproved,
		CreatedAt:  s.now(),
	}
	id, err := s.repos.Analyses.CreateExemption(ctx, e)
	if err != nil {
		return 0, err
	}
	if err := s.repos.Analyses.UpdateChangeStatus(ctx, changeID, "exempted"); err != nil {
		return 0, err
	}
	s.audit(ctx, operator, "exemption.approve", fmt.Sprintf("change:%d", changeID), reason)
	return id, nil
}

// ListExemptions 列分析豁免。
func (s *Service) ListExemptions(ctx context.Context, analysisID int64) ([]model.Exemption, error) {
	return s.repos.Analyses.ListExemptions(ctx, analysisID)
}

// FreezePlan 冻结迁移计划：校验分析完成、执行次序完整，写入不可变计划。
func (s *Service) FreezePlan(ctx context.Context, analysisID int64) (*model.MigrationPlan, error) {
	a, err := s.repos.Analyses.GetAnalysis(ctx, analysisID)
	if err != nil {
		return nil, err
	}
	steps, err := s.repos.Scripts.ListSteps(ctx, a.ScriptID)
	if err != nil {
		return nil, err
	}
	order, err := s.repos.Analyses.ListPlans(ctx)
	_ = order
	// 重新计算次序
	objs, _ := s.repos.Snapshots.ListObjects(ctx, a.SnapshotID)
	_ = objs
	deps, _ := s.repos.Snapshots.ListDependencies(ctx, a.SnapshotID)
	seqOrder, err := impact.OrderPlan(steps, deps)
	if err != nil {
		return nil, fmt.Errorf("无法冻结：%w", err)
	}
	if err := plan.Validate(a, seqOrder, steps); err != nil {
		return nil, err
	}
	p := &model.MigrationPlan{
		AnalysisID: analysisID,
		SnapshotID: a.SnapshotID,
		ScriptID:   a.ScriptID,
		Status:     model.PlanFrozen,
		StepOrder:  seqOrder,
		PlanHash:   plan.PlanHash(analysisID, a.SnapshotID, a.ScriptID, seqOrder),
		CreatedAt:  s.now(),
		FrozenAt:   ptr(s.now()),
	}
	id, err := s.repos.Analyses.CreatePlan(ctx, p)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "system", "plan.freeze", fmt.Sprintf("plan:%d", id), fmt.Sprintf("analysis=%d order=%v", analysisID, seqOrder))
	return s.repos.Analyses.GetPlan(ctx, id)
}

// ListPlans 列计划。
func (s *Service) ListPlans(ctx context.Context) ([]model.MigrationPlan, error) {
	return s.repos.Analyses.ListPlans(ctx)
}

// GetPlan 取计划。
func (s *Service) GetPlan(ctx context.Context, id int64) (*model.MigrationPlan, error) {
	return s.repos.Analyses.GetPlan(ctx, id)
}

func ptr(t time.Time) *time.Time { return &t }

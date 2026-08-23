package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"task160-migimpact/internal/model"
)

// AnalysisStore 提供影响分析、破坏性变更、豁免、计划与审计的持久化。
type AnalysisStore struct{ db *sql.DB }

func newAnalysisStore(db *sql.DB) *AnalysisStore { return &AnalysisStore{db: db} }

// --- impact analyses ---

// CreateAnalysis 创建分析任务；同一 (snapshot_id, script_id) 已有分析时返回既有记录（ErrConflict 由上层处理）。
func (st *AnalysisStore) CreateAnalysis(ctx context.Context, a *model.ImpactAnalysis) (int64, error) {
	res, err := st.db.ExecContext(ctx,
		`INSERT INTO impact_analyses(snapshot_id,script_id,status,input_hash,created_at,completed_at) VALUES(?,?,?,?,?,?)`,
		a.SnapshotID, a.ScriptID, string(a.Status), a.InputHash, a.CreatedAt.Format(timeLayout), nilTime(a.CompletedAt))
	if err != nil {
		if isUniqueViolation(err) {
			// 已存在：返回既有分析
			var existing model.ImpactAnalysis
			var created, completed sql.NullString
			row := st.db.QueryRowContext(ctx,
				`SELECT id,snapshot_id,script_id,status,input_hash,created_at,completed_at FROM impact_analyses WHERE snapshot_id=? AND script_id=?`,
				a.SnapshotID, a.ScriptID)
			if err := row.Scan(&existing.ID, &existing.SnapshotID, &existing.ScriptID, &existing.Status,
				&existing.InputHash, &created, &completed); err == nil {
				existing.CreatedAt = parseTime(created.String)
				if completed.Valid {
					t := parseTime(completed.String)
					existing.CompletedAt = &t
				}
				return existing.ID, model.ErrConflict
			}
			return 0, err
		}
		return 0, err
	}
	return res.LastInsertId()
}

func (st *AnalysisStore) GetAnalysis(ctx context.Context, id int64) (*model.ImpactAnalysis, error) {
	row := st.db.QueryRowContext(ctx,
		`SELECT id,snapshot_id,script_id,status,input_hash,created_at,completed_at FROM impact_analyses WHERE id=?`, id)
	var a model.ImpactAnalysis
	var created, completed sql.NullString
	if err := row.Scan(&a.ID, &a.SnapshotID, &a.ScriptID, &a.Status, &a.InputHash, &created, &completed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	a.CreatedAt = parseTime(created.String)
	if completed.Valid {
		t := parseTime(completed.String)
		a.CompletedAt = &t
	}
	return &a, nil
}

func (st *AnalysisStore) GetAnalysisByInput(ctx context.Context, snapshotID, scriptID int64) (*model.ImpactAnalysis, error) {
	row := st.db.QueryRowContext(ctx,
		`SELECT id,snapshot_id,script_id,status,input_hash,created_at,completed_at FROM impact_analyses WHERE snapshot_id=? AND script_id=?`,
		snapshotID, scriptID)
	var a model.ImpactAnalysis
	var created, completed sql.NullString
	if err := row.Scan(&a.ID, &a.SnapshotID, &a.ScriptID, &a.Status, &a.InputHash, &created, &completed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	a.CreatedAt = parseTime(created.String)
	if completed.Valid {
		t := parseTime(completed.String)
		a.CompletedAt = &t
	}
	return &a, nil
}

func (st *AnalysisStore) ListAnalyses(ctx context.Context) ([]model.ImpactAnalysis, error) {
	rows, err := st.db.QueryContext(ctx,
		`SELECT id,snapshot_id,script_id,status,input_hash,created_at,completed_at FROM impact_analyses ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ImpactAnalysis
	for rows.Next() {
		var a model.ImpactAnalysis
		var created, completed sql.NullString
		if err := rows.Scan(&a.ID, &a.SnapshotID, &a.ScriptID, &a.Status, &a.InputHash, &created, &completed); err != nil {
			return nil, err
		}
		a.CreatedAt = parseTime(created.String)
		if completed.Valid {
			t := parseTime(completed.String)
			a.CompletedAt = &t
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (st *AnalysisStore) UpdateAnalysisStatus(ctx context.Context, id int64, status model.StateAnalysis, completedAt *time.Time) error {
	_, err := st.db.ExecContext(ctx, `UPDATE impact_analyses SET status=?, completed_at=? WHERE id=?`,
		string(status), nilTime(completedAt), id)
	return err
}

// ListQueuedAnalyses 返回排队或分析中的任务（重启恢复用）。
func (st *AnalysisStore) ListQueuedAnalyses(ctx context.Context) ([]model.ImpactAnalysis, error) {
	rows, err := st.db.QueryContext(ctx,
		`SELECT id,snapshot_id,script_id,status,input_hash,created_at,completed_at FROM impact_analyses
		 WHERE status IN ('queued','running') ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ImpactAnalysis
	for rows.Next() {
		var a model.ImpactAnalysis
		var created, completed sql.NullString
		if err := rows.Scan(&a.ID, &a.SnapshotID, &a.ScriptID, &a.Status, &a.InputHash, &created, &completed); err != nil {
			return nil, err
		}
		a.CreatedAt = parseTime(created.String)
		if completed.Valid {
			t := parseTime(completed.String)
			a.CompletedAt = &t
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// --- destructive changes ---

func (st *AnalysisStore) CreateChanges(ctx context.Context, analysisID int64, changes []model.DestructiveChange) error {
	tx, err := st.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range changes {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO destructive_changes(analysis_id,step_id,change_type,target_obj,affected_objs,interfaces,status)
			 VALUES(?,?,?,?,?,?,?)`,
			analysisID, c.StepID, string(c.ChangeType), c.TargetObj, encodeJSON(c.AffectedObjs),
			encodeJSON(c.Interfaces), c.Status); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (st *AnalysisStore) ListChanges(ctx context.Context, analysisID int64) ([]model.DestructiveChange, error) {
	rows, err := st.db.QueryContext(ctx,
		`SELECT id,analysis_id,step_id,change_type,target_obj,affected_objs,interfaces,status
		 FROM destructive_changes WHERE analysis_id=? ORDER BY id`, analysisID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.DestructiveChange
	for rows.Next() {
		var c model.DestructiveChange
		var aff, ifs string
		if err := rows.Scan(&c.ID, &c.AnalysisID, &c.StepID, &c.ChangeType, &c.TargetObj, &aff, &ifs, &c.Status); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(aff), &c.AffectedObjs)
		json.Unmarshal([]byte(ifs), &c.Interfaces)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (st *AnalysisStore) GetChange(ctx context.Context, id int64) (*model.DestructiveChange, error) {
	row := st.db.QueryRowContext(ctx,
		`SELECT id,analysis_id,step_id,change_type,target_obj,affected_objs,interfaces,status FROM destructive_changes WHERE id=?`, id)
	var c model.DestructiveChange
	var aff, ifs string
	if err := row.Scan(&c.ID, &c.AnalysisID, &c.StepID, &c.ChangeType, &c.TargetObj, &aff, &ifs, &c.Status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	json.Unmarshal([]byte(aff), &c.AffectedObjs)
	json.Unmarshal([]byte(ifs), &c.Interfaces)
	return &c, nil
}

func (st *AnalysisStore) UpdateChangeStatus(ctx context.Context, id int64, status string) error {
	_, err := st.db.ExecContext(ctx, `UPDATE destructive_changes SET status=? WHERE id=?`, status, id)
	return err
}

// --- exemptions ---

func (st *AnalysisStore) CreateExemption(ctx context.Context, e *model.Exemption) (int64, error) {
	res, err := st.db.ExecContext(ctx,
		`INSERT INTO exemptions(analysis_id,change_id,operator,reason,status,created_at) VALUES(?,?,?,?,?,?)`,
		e.AnalysisID, e.ChangeID, e.Operator, e.Reason, string(e.Status), e.CreatedAt.Format(timeLayout))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (st *AnalysisStore) ListExemptions(ctx context.Context, analysisID int64) ([]model.Exemption, error) {
	rows, err := st.db.QueryContext(ctx,
		`SELECT id,analysis_id,change_id,operator,reason,status,created_at FROM exemptions WHERE analysis_id=? ORDER BY id`,
		analysisID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Exemption
	for rows.Next() {
		var e model.Exemption
		var created sql.NullString
		if err := rows.Scan(&e.ID, &e.AnalysisID, &e.ChangeID, &e.Operator, &e.Reason, &e.Status, &created); err != nil {
			return nil, err
		}
		e.CreatedAt = parseTime(created.String)
		out = append(out, e)
	}
	return out, rows.Err()
}

// --- migration plans ---

func (st *AnalysisStore) CreatePlan(ctx context.Context, p *model.MigrationPlan) (int64, error) {
	res, err := st.db.ExecContext(ctx,
		`INSERT INTO migration_plans(analysis_id,snapshot_id,script_id,status,step_order,plan_hash,created_at,frozen_at)
		 VALUES(?,?,?,?,?,?,?,?)`,
		p.AnalysisID, p.SnapshotID, p.ScriptID, string(p.Status), encodeJSON(p.StepOrder), p.PlanHash,
		p.CreatedAt.Format(timeLayout), nilTime(p.FrozenAt))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (st *AnalysisStore) GetPlan(ctx context.Context, id int64) (*model.MigrationPlan, error) {
	row := st.db.QueryRowContext(ctx,
		`SELECT id,analysis_id,snapshot_id,script_id,status,step_order,plan_hash,created_at,frozen_at FROM migration_plans WHERE id=?`, id)
	var p model.MigrationPlan
	var order, created, frozen sql.NullString
	if err := row.Scan(&p.ID, &p.AnalysisID, &p.SnapshotID, &p.ScriptID, &p.Status, &order, &p.PlanHash, &created, &frozen); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	p.CreatedAt = parseTime(created.String)
	if order.Valid {
		json.Unmarshal([]byte(order.String), &p.StepOrder)
	}
	if frozen.Valid {
		t := parseTime(frozen.String)
		p.FrozenAt = &t
	}
	return &p, nil
}

func (st *AnalysisStore) ListPlans(ctx context.Context) ([]model.MigrationPlan, error) {
	rows, err := st.db.QueryContext(ctx,
		`SELECT id,analysis_id,snapshot_id,script_id,status,step_order,plan_hash,created_at,frozen_at FROM migration_plans ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.MigrationPlan
	for rows.Next() {
		var p model.MigrationPlan
		var order, created, frozen sql.NullString
		if err := rows.Scan(&p.ID, &p.AnalysisID, &p.SnapshotID, &p.ScriptID, &p.Status, &order, &p.PlanHash, &created, &frozen); err != nil {
			return nil, err
		}
		p.CreatedAt = parseTime(created.String)
		if order.Valid {
			json.Unmarshal([]byte(order.String), &p.StepOrder)
		}
		if frozen.Valid {
			t := parseTime(frozen.String)
			p.FrozenAt = &t
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (st *AnalysisStore) UpdatePlanStatus(ctx context.Context, id int64, status model.StatePlan) error {
	_, err := st.db.ExecContext(ctx, `UPDATE migration_plans SET status=? WHERE id=?`, string(status), id)
	return err
}

// --- audit ---

func (st *AnalysisStore) AppendAudit(ctx context.Context, e *model.AuditEvent) error {
	_, err := st.db.ExecContext(ctx,
		`INSERT INTO audit_events(actor,action,subject,detail,created_at) VALUES(?,?,?,?,?)`,
		e.Actor, e.Action, e.Subject, e.Detail, e.CreatedAt.Format(timeLayout))
	return err
}

func (st *AnalysisStore) ListAudit(ctx context.Context, limit int) ([]model.AuditEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := st.db.QueryContext(ctx,
		`SELECT id,actor,action,subject,detail,created_at FROM audit_events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.AuditEvent
	for rows.Next() {
		var e model.AuditEvent
		var created sql.NullString
		if err := rows.Scan(&e.ID, &e.Actor, &e.Action, &e.Subject, &e.Detail, &created); err != nil {
			return nil, err
		}
		e.CreatedAt = parseTime(created.String)
		out = append(out, e)
	}
	return out, rows.Err()
}

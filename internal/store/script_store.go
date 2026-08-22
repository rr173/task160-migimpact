package store

import (
	"context"
	"database/sql"
	"errors"

	"task160-migimpact/internal/model"
)

// ScriptStore 提供迁移脚本与步骤的持久化。
type ScriptStore struct{ db *sql.DB }

func newScriptStore(db *sql.DB) *ScriptStore { return &ScriptStore{db: db} }

// CreateScript 插入脚本；ContentHash 冲突时返回 ErrDupFingerprint。
func (st *ScriptStore) CreateScript(ctx context.Context, s *model.MigrationScript) (int64, error) {
	res, err := st.db.ExecContext(ctx,
		`INSERT INTO migration_scripts(name,version,status,content_hash,content,created_at) VALUES(?,?,?,?,?,?)`,
		s.Name, s.Version, string(s.Status), s.ContentHash, s.Content, s.CreatedAt.Format(timeLayout))
	if err != nil {
		if isUniqueViolation(err) {
			return 0, model.ErrDupFingerprint
		}
		return 0, err
	}
	return res.LastInsertId()
}

// GetScriptByHash 按内容哈希查找脚本（幂等导入判重）。
func (st *ScriptStore) GetScriptByHash(ctx context.Context, hash string) (*model.MigrationScript, error) {
	row := st.db.QueryRowContext(ctx,
		`SELECT id,name,version,status,content_hash,content,created_at FROM migration_scripts WHERE content_hash=?`, hash)
	var s model.MigrationScript
	var created sql.NullString
	if err := row.Scan(&s.ID, &s.Name, &s.Version, &s.Status, &s.ContentHash, &s.Content, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	s.CreatedAt = parseTime(created.String)
	return &s, nil
}

func (st *ScriptStore) GetScript(ctx context.Context, id int64) (*model.MigrationScript, error) {
	row := st.db.QueryRowContext(ctx,
		`SELECT id,name,version,status,content_hash,content,created_at FROM migration_scripts WHERE id=?`, id)
	var s model.MigrationScript
	var created sql.NullString
	if err := row.Scan(&s.ID, &s.Name, &s.Version, &s.Status, &s.ContentHash, &s.Content, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	s.CreatedAt = parseTime(created.String)
	return &s, nil
}

func (st *ScriptStore) ListScripts(ctx context.Context) ([]model.MigrationScript, error) {
	rows, err := st.db.QueryContext(ctx,
		`SELECT id,name,version,status,content_hash,content,created_at FROM migration_scripts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.MigrationScript
	for rows.Next() {
		var s model.MigrationScript
		var created sql.NullString
		if err := rows.Scan(&s.ID, &s.Name, &s.Version, &s.Status, &s.ContentHash, &s.Content, &created); err != nil {
			return nil, err
		}
		s.CreatedAt = parseTime(created.String)
		out = append(out, s)
	}
	return out, rows.Err()
}

func (st *ScriptStore) UpdateScriptStatus(ctx context.Context, id int64, status model.StateScript) error {
	_, err := st.db.ExecContext(ctx, `UPDATE migration_scripts SET status=? WHERE id=?`, string(status), id)
	return err
}

// CreateSteps 批量写入步骤；同脚本同 seq 冲突报 ErrConflict。
func (st *ScriptStore) CreateSteps(ctx context.Context, scriptID int64, steps []model.MigrationStep) error {
	tx, err := st.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, step := range steps {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO migration_steps(script_id,seq,step_type,target_obj,detail,status,change_type,reason)
			 VALUES(?,?,?,?,?,?,?,?)`,
			scriptID, step.Seq, step.StepType, step.TargetObj, step.Detail, string(step.Status),
			string(step.ChangeType), step.Reason); err != nil {
			if isUniqueViolation(err) {
				return model.ErrConflict
			}
			return err
		}
	}
	return tx.Commit()
}

func (st *ScriptStore) ListSteps(ctx context.Context, scriptID int64) ([]model.MigrationStep, error) {
	rows, err := st.db.QueryContext(ctx,
		`SELECT id,script_id,seq,step_type,target_obj,detail,status,change_type,reason
		 FROM migration_steps WHERE script_id=? ORDER BY seq`, scriptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.MigrationStep
	for rows.Next() {
		var m model.MigrationStep
		if err := rows.Scan(&m.ID, &m.ScriptID, &m.Seq, &m.StepType, &m.TargetObj, &m.Detail,
			&m.Status, &m.ChangeType, &m.Reason); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (st *ScriptStore) GetStep(ctx context.Context, id int64) (*model.MigrationStep, error) {
	row := st.db.QueryRowContext(ctx,
		`SELECT id,script_id,seq,step_type,target_obj,detail,status,change_type,reason FROM migration_steps WHERE id=?`, id)
	var m model.MigrationStep
	if err := row.Scan(&m.ID, &m.ScriptID, &m.Seq, &m.StepType, &m.TargetObj, &m.Detail,
		&m.Status, &m.ChangeType, &m.Reason); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

func (st *ScriptStore) UpdateStep(ctx context.Context, id int64, status model.StateStep, changeType model.DestructiveChangeType, reason string) error {
	_, err := st.db.ExecContext(ctx,
		`UPDATE migration_steps SET status=?, change_type=?, reason=? WHERE id=?`,
		string(status), string(changeType), reason, id)
	return err
}

func (st *ScriptStore) UpdateStepReason(ctx context.Context, id int64, reason string) error {
	_, err := st.db.ExecContext(ctx, `UPDATE migration_steps SET reason=? WHERE id=?`, reason, id)
	return err
}

// --- helpers ---

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// modernc.org/sqlite 返回 *sqlite.Error，约束冲突码 2067（SQLITE_CONSTRAINT_UNIQUE）。
	var se interface{ Error() string }
	if errors.As(err, &se) {
		s := err.Error()
		return containsAny(s, "UNIQUE constraint failed", "constraint failed")
	}
	return false
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) > 0 && (len(s) >= len(sub)) && indexOf(s, sub) >= 0 {
			return true
		}
	}
	return false
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

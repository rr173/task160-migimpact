package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"task160-migimpact/internal/model"
)

const timeLayout = time.RFC3339Nano

// SnapshotStore 提供 schema 快照、对象、依赖与访问声明的读写。
type SnapshotStore struct{ db *sql.DB }

func newSnapshotStore(db *sql.DB) *SnapshotStore { return &SnapshotStore{db: db} }

func (st *SnapshotStore) CreateSnapshot(ctx context.Context, s *model.SchemaSnapshot) (int64, error) {
	res, err := st.db.ExecContext(ctx,
		`INSERT INTO schema_snapshots(name,status,created_at,frozen_at,object_hash,dep_hash) VALUES(?,?,?,?,?,?)`,
		s.Name, string(s.Status), s.CreatedAt.Format(timeLayout), nilTime(s.FrozenAt), s.ObjectHash, s.DepHash)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (st *SnapshotStore) GetSnapshot(ctx context.Context, id int64) (*model.SchemaSnapshot, error) {
	row := st.db.QueryRowContext(ctx,
		`SELECT id,name,status,created_at,frozen_at,object_hash,dep_hash FROM schema_snapshots WHERE id=?`, id)
	var s model.SchemaSnapshot
	var created, frozen sql.NullString
	if err := row.Scan(&s.ID, &s.Name, &s.Status, &created, &frozen, &s.ObjectHash, &s.DepHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	s.CreatedAt = parseTime(created.String)
	if frozen.Valid {
		t := parseTime(frozen.String)
		s.FrozenAt = &t
	}
	return &s, nil
}

func (st *SnapshotStore) ListSnapshots(ctx context.Context) ([]model.SchemaSnapshot, error) {
	rows, err := st.db.QueryContext(ctx,
		`SELECT id,name,status,created_at,frozen_at,object_hash,dep_hash FROM schema_snapshots ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.SchemaSnapshot
	for rows.Next() {
		var s model.SchemaSnapshot
		var created, frozen sql.NullString
		if err := rows.Scan(&s.ID, &s.Name, &s.Status, &created, &frozen, &s.ObjectHash, &s.DepHash); err != nil {
			return nil, err
		}
		s.CreatedAt = parseTime(created.String)
		if frozen.Valid {
			t := parseTime(frozen.String)
			s.FrozenAt = &t
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (st *SnapshotStore) UpdateSnapshotStatus(ctx context.Context, id int64, status model.StateSnapshot, frozenAt *time.Time) error {
	res, err := st.db.ExecContext(ctx,
		`UPDATE schema_snapshots SET status=?, frozen_at=? WHERE id=?`, string(status), nilTime(frozenAt), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrNotFound
	}
	return nil
}

// UpsertObjects 在一个事务中批量写入对象；同一 (obj_type,table_name,name) 已存在则覆盖。
func (st *SnapshotStore) UpsertObjects(ctx context.Context, snapshotID int64, objs []model.SchemaObject) error {
	tx, err := st.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, o := range objs {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO schema_objects(snapshot_id,obj_type,table_name,name,data_type,nullable,unique_flag,definition,status,hash)
			 VALUES(?,?,?,?,?,?,?,?,?,?)
			 ON CONFLICT(snapshot_id,obj_type,table_name,name) DO UPDATE SET
			   data_type=excluded.data_type, nullable=excluded.nullable, unique_flag=excluded.unique_flag,
			   definition=excluded.definition, status=excluded.status, hash=excluded.hash`,
			snapshotID, o.ObjType, o.TableName, o.Name, o.DataType, boolInt(o.Nullable), boolInt(o.Unique),
			o.Definition, string(o.Status), o.Hash)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (st *SnapshotStore) ListObjects(ctx context.Context, snapshotID int64) ([]model.SchemaObject, error) {
	rows, err := st.db.QueryContext(ctx,
		`SELECT id,snapshot_id,obj_type,table_name,name,data_type,nullable,unique_flag,definition,status,hash
		 FROM schema_objects WHERE snapshot_id=? ORDER BY id`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.SchemaObject
	for rows.Next() {
		var o model.SchemaObject
		var nullable, uniq int
		if err := rows.Scan(&o.ID, &o.SnapshotID, &o.ObjType, &o.TableName, &o.Name, &o.DataType,
			&nullable, &uniq, &o.Definition, &o.Status, &o.Hash); err != nil {
			return nil, err
		}
		o.Nullable = nullable != 0
		o.Unique = uniq != 0
		out = append(out, o)
	}
	return out, rows.Err()
}

func (st *SnapshotStore) GetObject(ctx context.Context, snapshotID int64, objType, tableName, name string) (*model.SchemaObject, error) {
	row := st.db.QueryRowContext(ctx,
		`SELECT id,snapshot_id,obj_type,table_name,name,data_type,nullable,unique_flag,definition,status,hash
		 FROM schema_objects WHERE snapshot_id=? AND obj_type=? AND table_name=? AND name=?`,
		snapshotID, objType, tableName, name)
	var o model.SchemaObject
	var nullable, uniq int
	if err := row.Scan(&o.ID, &o.SnapshotID, &o.ObjType, &o.TableName, &o.Name, &o.DataType,
		&nullable, &uniq, &o.Definition, &o.Status, &o.Hash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	o.Nullable = nullable != 0
	o.Unique = uniq != 0
	return &o, nil
}

func (st *SnapshotStore) UpdateObjectStatus(ctx context.Context, id int64, status model.StateObject) error {
	_, err := st.db.ExecContext(ctx, `UPDATE schema_objects SET status=? WHERE id=?`, string(status), id)
	return err
}

// ReplaceDependencies 先清空再写入依赖边，保证依赖集合与快照哈希一致。
func (st *SnapshotStore) ReplaceDependencies(ctx context.Context, snapshotID int64, deps []model.ObjectDependency) error {
	tx, err := st.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM object_dependencies WHERE snapshot_id=?`, snapshotID); err != nil {
		return err
	}
	for _, d := range deps {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO object_dependencies(snapshot_id,dep_type,source_obj,target_obj,source_col,target_col,statement)
			 VALUES(?,?,?,?,?,?,?)`,
			snapshotID, d.DepType, d.SourceObj, d.TargetObj, d.SourceCol, d.TargetCol, d.Statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (st *SnapshotStore) ListDependencies(ctx context.Context, snapshotID int64) ([]model.ObjectDependency, error) {
	rows, err := st.db.QueryContext(ctx,
		`SELECT id,snapshot_id,dep_type,source_obj,target_obj,source_col,target_col,statement
		 FROM object_dependencies WHERE snapshot_id=? ORDER BY id`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ObjectDependency
	for rows.Next() {
		var d model.ObjectDependency
		if err := rows.Scan(&d.ID, &d.SnapshotID, &d.DepType, &d.SourceObj, &d.TargetObj,
			&d.SourceCol, &d.TargetCol, &d.Statement); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ReplaceAccess 先清空再写入访问声明。
func (st *SnapshotStore) ReplaceAccess(ctx context.Context, snapshotID int64, accs []model.AccessDeclaration) error {
	tx, err := st.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM access_declarations WHERE snapshot_id=?`, snapshotID); err != nil {
		return err
	}
	for _, a := range accs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO access_declarations(snapshot_id,service,interface,action,target_obj,status) VALUES(?,?,?,?,?,?)`,
			snapshotID, a.Service, a.Interface, a.Action, a.TargetObj, a.Status); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (st *SnapshotStore) ListAccess(ctx context.Context, snapshotID int64) ([]model.AccessDeclaration, error) {
	rows, err := st.db.QueryContext(ctx,
		`SELECT id,snapshot_id,service,interface,action,target_obj,status FROM access_declarations WHERE snapshot_id=? ORDER BY id`,
		snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.AccessDeclaration
	for rows.Next() {
		var a model.AccessDeclaration
		if err := rows.Scan(&a.ID, &a.SnapshotID, &a.Service, &a.Interface, &a.Action, &a.TargetObj, &a.Status); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetSnapshotHashes 更新快照的对象/依赖哈希（幂等导入时保持稳定）。
func (st *SnapshotStore) SetSnapshotHashes(ctx context.Context, id int64, objHash, depHash string) error {
	_, err := st.db.ExecContext(ctx, `UPDATE schema_snapshots SET object_hash=?, dep_hash=? WHERE id=?`,
		objHash, depHash, id)
	return err
}

// --- helpers ---

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nilTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.Format(timeLayout)
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(timeLayout, s)
	return t
}

func encodeJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// Package store 提供基于 SQLite（modernc.org/sqlite，纯 Go 驱动）的持久化能力。
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store 封装 SQLite 连接与建表迁移。
type Store struct {
	db   *sql.DB
	path string
}

// Path 返回数据库文件路径。
func (s *Store) Path() string { return s.path }

// DBPathForTest 返回数据库路径（测试用别名）。
func (s *Store) DBPathForTest() string { return s.path }

// Open 打开（或创建）位于 path 的 SQLite 数据库并执行建表迁移。
// path 为空时使用临时文件，便于测试与自检。
func Open(path string) (*Store, error) {
	if path == "" {
		dir, err := os.MkdirTemp("", "migimpact-*")
		if err != nil {
			return nil, err
		}
		path = filepath.Join(dir, "migimpact.db")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// modernc 驱动默认串行写；这里显式开启 WAL 以提升并发读体验。
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		return nil, fmt.Errorf("enable wal: %w", err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON;`); err != nil {
		return nil, fmt.Errorf("enable fk: %w", err)
	}
	s := &Store{db: db, path: path}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// DB 返回底层连接，供事务使用。
func (s *Store) DB() *sql.DB { return s.db }

// Close 关闭数据库。
func (s *Store) Close() error { return s.db.Close() }

// migrate 按顺序建表。业务数据全部落盘，具备重启恢复路径。
func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS schema_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			frozen_at TEXT,
			object_hash TEXT NOT NULL,
			dep_hash TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS schema_objects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_id INTEGER NOT NULL REFERENCES schema_snapshots(id),
			obj_type TEXT NOT NULL,
			table_name TEXT NOT NULL,
			name TEXT NOT NULL,
			data_type TEXT NOT NULL,
			nullable INTEGER NOT NULL DEFAULT 0,
			unique_flag INTEGER NOT NULL DEFAULT 0,
			definition TEXT NOT NULL,
			status TEXT NOT NULL,
			hash TEXT NOT NULL,
			UNIQUE(snapshot_id, obj_type, table_name, name)
		);`,
		`CREATE TABLE IF NOT EXISTS object_dependencies (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_id INTEGER NOT NULL REFERENCES schema_snapshots(id),
			dep_type TEXT NOT NULL,
			source_obj TEXT NOT NULL,
			target_obj TEXT NOT NULL,
			source_col TEXT NOT NULL,
			target_col TEXT NOT NULL,
			statement TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS access_declarations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_id INTEGER NOT NULL REFERENCES schema_snapshots(id),
			service TEXT NOT NULL,
			interface TEXT NOT NULL,
			action TEXT NOT NULL,
			target_obj TEXT NOT NULL,
			status TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS migration_scripts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			version INTEGER NOT NULL,
			status TEXT NOT NULL,
			content_hash TEXT NOT NULL UNIQUE,
			content TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS migration_steps (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			script_id INTEGER NOT NULL REFERENCES migration_scripts(id),
			seq INTEGER NOT NULL,
			step_type TEXT NOT NULL,
			target_obj TEXT NOT NULL,
			detail TEXT NOT NULL,
			status TEXT NOT NULL,
			change_type TEXT NOT NULL,
			reason TEXT NOT NULL,
			UNIQUE(script_id, seq)
		);`,
		`CREATE TABLE IF NOT EXISTS impact_analyses (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_id INTEGER NOT NULL REFERENCES schema_snapshots(id),
			script_id INTEGER NOT NULL REFERENCES migration_scripts(id),
			status TEXT NOT NULL,
			input_hash TEXT NOT NULL,
			created_at TEXT NOT NULL,
			completed_at TEXT,
			UNIQUE(snapshot_id, script_id)
		);`,
		`CREATE TABLE IF NOT EXISTS destructive_changes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			analysis_id INTEGER NOT NULL REFERENCES impact_analyses(id),
			step_id INTEGER NOT NULL REFERENCES migration_steps(id),
			change_type TEXT NOT NULL,
			target_obj TEXT NOT NULL,
			affected_objs TEXT NOT NULL,
			interfaces TEXT NOT NULL,
			status TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS exemptions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			analysis_id INTEGER NOT NULL REFERENCES impact_analyses(id),
			change_id INTEGER NOT NULL REFERENCES destructive_changes(id),
			operator TEXT NOT NULL,
			reason TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS migration_plans (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			analysis_id INTEGER NOT NULL REFERENCES impact_analyses(id),
			snapshot_id INTEGER NOT NULL REFERENCES schema_snapshots(id),
			script_id INTEGER NOT NULL REFERENCES migration_scripts(id),
			status TEXT NOT NULL,
			step_order TEXT NOT NULL,
			plan_hash TEXT NOT NULL,
			created_at TEXT NOT NULL,
			frozen_at TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS audit_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			actor TEXT NOT NULL,
			action TEXT NOT NULL,
			subject TEXT NOT NULL,
			detail TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

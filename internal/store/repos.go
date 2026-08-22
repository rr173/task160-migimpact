package store

// Store 聚合所有子 store，供 service 层使用。
type Repositories struct {
	Snapshots *SnapshotStore
	Scripts   *ScriptStore
	Analyses  *AnalysisStore
}

// NewRepositories 基于打开的数据库构造聚合仓储。
func NewRepositories(s *Store) *Repositories {
	return &Repositories{
		Snapshots: newSnapshotStore(s.db),
		Scripts:   newScriptStore(s.db),
		Analyses:  newAnalysisStore(s.db),
	}
}

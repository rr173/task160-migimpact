package httpapi

import (
	"net/http"

	"task160-migimpact/internal/model"
)

// --- 快照 ---

func (s *Server) handleCreateSnapshot(w http.ResponseWriter, r *http.Request) {
	var req createSnapshotReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := s.svc.CreateSnapshot(r.Context(), req.Name)
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (s *Server) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	snaps, err := s.svc.ListSnapshots(r.Context())
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snaps)
}

func (s *Server) handleGetSnapshot(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	snap, err := s.svc.GetSnapshot(r.Context(), id)
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleLoadSnapshot(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var req loadSnapshotReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	objs := make([]model.SchemaObject, 0, len(req.Objects))
	for _, o := range req.Objects {
		objs = append(objs, model.SchemaObject{
			ObjType:    o.ObjType,
			TableName:  o.Table,
			Name:       o.Name,
			DataType:   o.DataType,
			Nullable:   o.Nullable,
			Unique:     o.Unique,
		})
	}
	deps := make([]model.ObjectDependency, 0, len(req.Deps))
	for _, d := range req.Deps {
		deps = append(deps, model.ObjectDependency{
			DepType:   d.DepType,
			SourceObj: d.SourceObj,
			TargetObj: d.TargetObj,
			SourceCol: d.SourceCol,
			TargetCol: d.TargetCol,
			Statement: d.Statement,
		})
	}
	accs := make([]model.AccessDeclaration, 0, len(req.Access))
	for _, a := range req.Access {
		accs = append(accs, model.AccessDeclaration{
			Service:   a.Service,
			Interface: a.Iface,
			Action:    a.Action,
			TargetObj: a.Target,
			Status:    "active",
		})
	}
	if err := s.svc.LoadSnapshotObjects(r.Context(), id, objs, deps, accs); err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "registered"})
}

func (s *Server) handleGetObjects(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	objs, err := s.svc.GetSnapshotObjects(r.Context(), id)
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, objs)
}

func (s *Server) handleGetDeps(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	deps, err := s.svc.GetSnapshotDeps(r.Context(), id)
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, deps)
}

func (s *Server) handleGetAccess(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	accs, err := s.svc.GetSnapshotAccess(r.Context(), id)
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, accs)
}

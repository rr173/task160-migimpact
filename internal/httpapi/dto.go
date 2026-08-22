package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"task160-migimpact/internal/model"
)

// pathID 解析路径参数中的数字 ID。
func pathID(r *http.Request, name string) (int64, error) {
	v := r.PathValue(name)
	if v == "" {
		return 0, errors.New("missing path param " + name)
	}
	return strconv.ParseInt(v, 10, 64)
}

// mapError 把业务错误映射为 HTTP 状态码。
func mapError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, model.ErrNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, model.ErrConflict):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, model.ErrFrozen):
		writeErr(w, http.StatusForbidden, err.Error())
	case errors.Is(err, model.ErrDupFingerprint):
		writeErr(w, http.StatusConflict, "脚本内容指纹已存在（幂等返回既有脚本）")
	default:
		writeErr(w, http.StatusBadRequest, err.Error())
	}
}

// decode 解析请求体 JSON 到 v。
func decode(r *http.Request, v interface{}) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// --- request/response DTO ---

type createSnapshotReq struct {
	Name string `json:"name"`
}

type loadSnapshotReq struct {
	Objects []objectDTO       `json:"objects"`
	Deps    []depDTO          `json:"dependencies"`
	Access  []accessDTO       `json:"access"`
}

type objectDTO struct {
	ObjType  string `json:"obj_type"`
	Table    string `json:"table"`
	Name     string `json:"name"`
	DataType string `json:"data_type"`
	Nullable bool   `json:"nullable"`
	Unique   bool   `json:"unique"`
}

type depDTO struct {
	DepType   string `json:"dep_type"`
	SourceObj string `json:"source_obj"`
	TargetObj string `json:"target_obj"`
	SourceCol string `json:"source_col"`
	TargetCol string `json:"target_col"`
	Statement string `json:"statement"`
}

type accessDTO struct {
	Service  string `json:"service"`
	Iface    string `json:"interface"`
	Action   string `json:"action"`
	Target   string `json:"target_obj"`
}

type importScriptReq struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
	Content string `json:"content"`
}

type exemptionReq struct {
	Operator string `json:"operator"`
	Reason   string `json:"reason"`
}

// SPDX-License-Identifier: MIT

package controlplane

// Personal Data Lake control plane (M836). These handlers expose the agent-built
// structured collections (kernel/datalake, M834/M835) to the operator — the Web
// UI Data view and `agt data` CLI — for browsing and light editing. The agent
// manages the same collections through the `db` tool; this is the human's window
// onto (and hands on) the same data.

import (
	"errors"
	"net"

	"github.com/agezt/agezt/kernel/datalake"
)

func (s *Server) lake() *datalake.Lake { return s.k.DataLake() }

func (s *Server) handleDataCollections(conn net.Conn, req Request) {
	l := s.lake()
	if l == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "data lake unavailable"})
		return
	}
	cols := l.ListCollections()
	out := make([]map[string]any, 0, len(cols))
	for _, c := range cols {
		out = append(out, collectionMap(c.Schema, c.Count))
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"count":       len(out),
		"collections": out,
	}})
}

func (s *Server) handleDataRecords(conn net.Conn, req Request) {
	l := s.lake()
	if l == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "data lake unavailable"})
		return
	}
	coll, _ := req.Args["collection"].(string)
	if coll == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.collection required"})
		return
	}
	q := datalake.Query{
		Search: stringArg(req.Args, "search"),
		SortBy: stringArg(req.Args, "sort"),
		Desc:   dlBool(req.Args, "desc"),
		Limit:  dlInt(req.Args, "limit"),
		Offset: dlInt(req.Args, "offset"),
	}
	recs, err := l.Query(coll, q)
	if err != nil {
		s.writeResp(conn, dataErr(req, coll, err))
		return
	}
	sc, _ := l.Schema(coll)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"collection": coll,
		"schema":     collectionMap(sc, len(recs)),
		"count":      len(recs),
		"records":    recordMaps(recs),
	}})
}

func (s *Server) handleDataInsert(conn net.Conn, req Request) {
	l := s.lake()
	if l == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "data lake unavailable"})
		return
	}
	coll, _ := req.Args["collection"].(string)
	if coll == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.collection required"})
		return
	}
	fields, _ := req.Args["record"].(map[string]any)
	r, err := l.Insert(coll, fields, "operator")
	if err != nil {
		s.writeResp(conn, dataErr(req, coll, err))
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"record": recordMap(r)}})
}

func (s *Server) handleDataUpdate(conn net.Conn, req Request) {
	l := s.lake()
	if l == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "data lake unavailable"})
		return
	}
	coll, _ := req.Args["collection"].(string)
	id, _ := req.Args["id"].(string)
	if coll == "" || id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.collection and args.id required"})
		return
	}
	patch, _ := req.Args["record"].(map[string]any)
	r, err := l.Update(coll, id, patch, "operator")
	if err != nil {
		s.writeResp(conn, dataErr(req, coll+"/"+id, err))
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"record": recordMap(r)}})
}

func (s *Server) handleDataDelete(conn net.Conn, req Request) {
	l := s.lake()
	if l == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "data lake unavailable"})
		return
	}
	coll, _ := req.Args["collection"].(string)
	id, _ := req.Args["id"].(string)
	if coll == "" || id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.collection and args.id required"})
		return
	}
	if err := l.Delete(coll, id); err != nil {
		s.writeResp(conn, dataErr(req, coll+"/"+id, err))
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"deleted": true, "id": id}})
}

func (s *Server) handleDataCreateCollection(conn net.Conn, req Request) {
	l := s.lake()
	if l == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "data lake unavailable"})
		return
	}
	raw, _ := req.Args["collection"].(map[string]any)
	sc := schemaFromMap(raw)
	if sc.Name == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "collection.name required"})
		return
	}
	out, err := l.CreateCollection(sc, "operator")
	if err != nil {
		if errors.Is(err, datalake.ErrExists) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "collection already exists: " + sc.Name})
			return
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"collection": collectionMap(out, 0)}})
}

func (s *Server) handleDataDropCollection(conn net.Conn, req Request) {
	l := s.lake()
	if l == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "data lake unavailable"})
		return
	}
	name, _ := req.Args["name"].(string)
	if name == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.name required"})
		return
	}
	if err := l.DropCollection(name); err != nil {
		switch {
		case errors.Is(err, datalake.ErrSystem):
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "built-in collection cannot be dropped: " + name})
		case errors.Is(err, datalake.ErrNotFound):
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "no such collection: " + name})
		default:
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		}
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"dropped": name}})
}

// --- helpers ---

func dataErr(req Request, what string, err error) Response {
	if errors.Is(err, datalake.ErrNotFound) {
		return Response{ID: req.ID, Type: RespError, Error: "no such collection or record: " + what}
	}
	return Response{ID: req.ID, Type: RespError, Error: err.Error()}
}

func collectionMap(sc datalake.Schema, count int) map[string]any {
	fields := make([]map[string]any, 0, len(sc.Fields))
	for _, f := range sc.Fields {
		fields = append(fields, map[string]any{"name": f.Name, "type": f.Type, "label": f.Label})
	}
	return map[string]any{
		"name": sc.Name, "title": sc.Title, "icon": sc.Icon, "view": sc.View,
		"desc": sc.Desc, "fields": fields, "builtin": sc.Builtin, "system": sc.System,
		"count": count, "created_ms": sc.CreatedMs, "created_by": sc.CreatedBy,
	}
}

func recordMaps(recs []datalake.Record) []map[string]any {
	out := make([]map[string]any, 0, len(recs))
	for _, r := range recs {
		out = append(out, recordMap(r))
	}
	return out
}

func recordMap(r datalake.Record) map[string]any {
	return map[string]any{
		"id": r.ID, "fields": r.Fields,
		"created_ms": r.CreatedMs, "updated_ms": r.UpdatedMs,
		"created_by": r.CreatedBy, "updated_by": r.UpdatedBy,
	}
}

func schemaFromMap(m map[string]any) datalake.Schema {
	if m == nil {
		return datalake.Schema{}
	}
	sc := datalake.Schema{
		Name:  asString(m["name"]),
		Title: asString(m["title"]),
		Icon:  asString(m["icon"]),
		View:  asString(m["view"]),
		Desc:  asString(m["desc"]),
	}
	if rawFields, ok := m["fields"].([]any); ok {
		for _, rf := range rawFields {
			fm, ok := rf.(map[string]any)
			if !ok {
				continue
			}
			sc.Fields = append(sc.Fields, datalake.Field{
				Name: asString(fm["name"]), Type: asString(fm["type"]), Label: asString(fm["label"]),
			})
		}
	}
	return sc
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func dlBool(args map[string]any, key string) bool {
	switch v := args[key].(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1"
	default:
		return false
	}
}

func dlInt(args map[string]any, key string) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		n := 0
		for _, r := range v {
			if r < '0' || r > '9' {
				return 0
			}
			n = n*10 + int(r-'0')
		}
		return n
	default:
		return 0
	}
}

// SPDX-License-Identifier: MIT

// Package data is the operator's window onto the personal data lake
// (M836): browsing and lightly editing the structured collections agents build
// with the db tool (M834/M835), from the Web UI Data view and `agt data`.
package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/datalake"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// Actor attributes operator edits in record and collection provenance.
const Actor = "operator"

// Lake is the primary kernel's data lake.
type Lake interface {
	ListCollections() []datalake.CollectionInfo
	Query(coll string, q datalake.Query) ([]datalake.Record, error)
	Schema(name string) (datalake.Schema, bool)
	Insert(coll string, fields map[string]any, actor string) (datalake.Record, error)
	Update(coll, id string, patch map[string]any, actor string) (datalake.Record, error)
	Delete(coll, id string) error
	CreateCollection(sc datalake.Schema, actor string) (datalake.Schema, error)
	DropCollection(name string) error
}

// Service works over one lake; a nil lake refuses every operation.
type Service struct{ lake Lake }

func New(lake Lake) *Service { return &Service{lake: lake} }

var errUnavailable = errors.New("data lake unavailable")

type FieldRow struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Label string `json:"label"`
}

// Collection is a collection's schema with a record count; every field is
// always present and fields is an array.
type Collection struct {
	Name      string     `json:"name"`
	Title     string     `json:"title"`
	Icon      string     `json:"icon"`
	View      string     `json:"view"`
	Desc      string     `json:"desc"`
	Fields    []FieldRow `json:"fields"`
	Builtin   bool       `json:"builtin"`
	System    bool       `json:"system"`
	Count     int        `json:"count"`
	CreatedMs int64      `json:"created_ms"`
	CreatedBy string     `json:"created_by"`
}

func collection(sc datalake.Schema, count int) Collection {
	fields := make([]FieldRow, 0, len(sc.Fields))
	for _, f := range sc.Fields {
		fields = append(fields, FieldRow{Name: f.Name, Type: f.Type, Label: f.Label})
	}
	return Collection{Name: sc.Name, Title: sc.Title, Icon: sc.Icon, View: sc.View, Desc: sc.Desc, Fields: fields, Builtin: sc.Builtin, System: sc.System, Count: count, CreatedMs: sc.CreatedMs, CreatedBy: sc.CreatedBy}
}

// Record is one row with its provenance; every field is always present.
type Record struct {
	ID        string         `json:"id"`
	Fields    map[string]any `json:"fields"`
	CreatedMs int64          `json:"created_ms"`
	UpdatedMs int64          `json:"updated_ms"`
	CreatedBy string         `json:"created_by"`
	UpdatedBy string         `json:"updated_by"`
}

func record(r datalake.Record) Record {
	return Record{ID: r.ID, Fields: r.Fields, CreatedMs: r.CreatedMs, UpdatedMs: r.UpdatedMs, CreatedBy: r.CreatedBy, UpdatedBy: r.UpdatedBy}
}

// dataErr names what was missing for a not-found error and passes any other
// error through.
func dataErr(what string, err error) error {
	if errors.Is(err, datalake.ErrNotFound) {
		return errors.New("no such collection or record: " + what)
	}
	return err
}

func decode(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var v any
	_ = json.Unmarshal(raw, &v)
	return v, true
}

// required is a string argument that must not be blank; it is returned
// untrimmed, and a present non-string is refused.
func required(raw json.RawMessage, key string) (string, error) {
	v, present := decode(raw)
	str, ok := v.(string)
	if present && !ok {
		return "", fmt.Errorf("args.%s must be a string", key)
	}
	if strings.TrimSpace(str) == "" {
		return "", fmt.Errorf("args.%s required", key)
	}
	return str, nil
}

// text is a trimmed string argument; any other value reads as "".
func text(raw json.RawMessage) string {
	v, _ := decode(raw)
	str, _ := v.(string)
	return strings.TrimSpace(str)
}

// flag is true for a JSON true or the strings "true" and "1".
func flag(raw json.RawMessage) bool {
	v, _ := decode(raw)
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return b == "true" || b == "1"
	default:
		return false
	}
}

// count is a number truncated to an int, or a string of decimal digits; any
// other value is 0.
func count(raw json.RawMessage) int {
	v, _ := decode(raw)
	switch n := v.(type) {
	case float64:
		return int(n)
	case string:
		total := 0
		for _, r := range n {
			if r < '0' || r > '9' {
				return 0
			}
			total = total*10 + int(r-'0')
		}
		return total
	default:
		return 0
	}
}

// object is an optional object argument: absent and null are nil, any other
// non-object is refused.
func object(raw json.RawMessage, key string) (map[string]any, error) {
	v, _ := decode(raw)
	if v == nil {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("args.%s must be an object", key)
	}
	return m, nil
}

type CollectionsRequest struct{}

type CollectionsOutput struct {
	Count       int          `json:"count"`
	Collections []Collection `json:"collections"`
}

func (s *Service) Collections(_ context.Context, _ CollectionsRequest) (CollectionsOutput, error) {
	if s.lake == nil {
		return CollectionsOutput{}, errUnavailable
	}
	cols := s.lake.ListCollections()
	out := make([]Collection, 0, len(cols))
	for _, c := range cols {
		out = append(out, collection(c.Schema, c.Count))
	}
	return CollectionsOutput{Count: len(out), Collections: out}, nil
}

type RecordsRequest struct {
	Collection json.RawMessage `json:"collection,omitempty"`
	Search     json.RawMessage `json:"search,omitempty"`
	Sort       json.RawMessage `json:"sort,omitempty"`
	Desc       json.RawMessage `json:"desc,omitempty"`
	Limit      json.RawMessage `json:"limit,omitempty"`
	Offset     json.RawMessage `json:"offset,omitempty"`
}

type RecordsOutput struct {
	Collection string     `json:"collection"`
	Schema     Collection `json:"schema"`
	Count      int        `json:"count"`
	Records    []Record   `json:"records"`
}

// Records queries one collection: a case-insensitive search, a sort field
// (creation time by default), direction and a page.
func (s *Service) Records(_ context.Context, in RecordsRequest) (RecordsOutput, error) {
	if s.lake == nil {
		return RecordsOutput{}, errUnavailable
	}
	coll, err := required(in.Collection, "collection")
	if err != nil {
		return RecordsOutput{}, err
	}
	recs, err := s.lake.Query(coll, datalake.Query{Search: text(in.Search), SortBy: text(in.Sort), Desc: flag(in.Desc), Limit: count(in.Limit), Offset: count(in.Offset)})
	if err != nil {
		return RecordsOutput{}, dataErr(coll, err)
	}
	sc, _ := s.lake.Schema(coll)
	rows := make([]Record, 0, len(recs))
	for _, r := range recs {
		rows = append(rows, record(r))
	}
	return RecordsOutput{Collection: coll, Schema: collection(sc, len(recs)), Count: len(recs), Records: rows}, nil
}

type InsertRequest struct {
	Collection json.RawMessage `json:"collection,omitempty"`
	Record     json.RawMessage `json:"record,omitempty"`
}

type RecordOutput struct {
	Record Record `json:"record"`
}

func (s *Service) Insert(_ context.Context, in InsertRequest) (RecordOutput, error) {
	if s.lake == nil {
		return RecordOutput{}, errUnavailable
	}
	coll, err := required(in.Collection, "collection")
	if err != nil {
		return RecordOutput{}, err
	}
	fields, err := object(in.Record, "record")
	if err != nil {
		return RecordOutput{}, err
	}
	r, err := s.lake.Insert(coll, fields, Actor)
	if err != nil {
		return RecordOutput{}, dataErr(coll, err)
	}
	return RecordOutput{Record: record(r)}, nil
}

type UpdateRequest struct {
	Collection json.RawMessage `json:"collection,omitempty"`
	ID         json.RawMessage `json:"id,omitempty"`
	Record     json.RawMessage `json:"record,omitempty"`
}

func (s *Service) Update(_ context.Context, in UpdateRequest) (RecordOutput, error) {
	if s.lake == nil {
		return RecordOutput{}, errUnavailable
	}
	coll, err := required(in.Collection, "collection")
	if err != nil {
		return RecordOutput{}, err
	}
	id, err := required(in.ID, "id")
	if err != nil {
		return RecordOutput{}, err
	}
	patch, err := object(in.Record, "record")
	if err != nil {
		return RecordOutput{}, err
	}
	r, err := s.lake.Update(coll, id, patch, Actor)
	if err != nil {
		return RecordOutput{}, dataErr(coll+"/"+id, err)
	}
	return RecordOutput{Record: record(r)}, nil
}

type DeleteRequest struct {
	Collection json.RawMessage `json:"collection,omitempty"`
	ID         json.RawMessage `json:"id,omitempty"`
}

type DeleteOutput struct {
	Deleted bool   `json:"deleted"`
	ID      string `json:"id"`
}

func (s *Service) Delete(_ context.Context, in DeleteRequest) (DeleteOutput, error) {
	if s.lake == nil {
		return DeleteOutput{}, errUnavailable
	}
	coll, err := required(in.Collection, "collection")
	if err != nil {
		return DeleteOutput{}, err
	}
	id, err := required(in.ID, "id")
	if err != nil {
		return DeleteOutput{}, err
	}
	if err := s.lake.Delete(coll, id); err != nil {
		return DeleteOutput{}, dataErr(coll+"/"+id, err)
	}
	return DeleteOutput{Deleted: true, ID: id}, nil
}

type CreateRequest struct {
	Collection json.RawMessage `json:"collection,omitempty"`
}

type CreateOutput struct {
	Collection Collection `json:"collection"`
}

// schemaOf reads a collection definition leniently: non-string values read as
// "", and non-object field entries are skipped.
func schemaOf(m map[string]any) datalake.Schema {
	if m == nil {
		return datalake.Schema{}
	}
	str := func(v any) string {
		s, _ := v.(string)
		return s
	}
	sc := datalake.Schema{Name: str(m["name"]), Title: str(m["title"]), Icon: str(m["icon"]), View: str(m["view"]), Desc: str(m["desc"])}
	if raw, ok := m["fields"].([]any); ok {
		for _, rf := range raw {
			fm, ok := rf.(map[string]any)
			if !ok {
				continue
			}
			sc.Fields = append(sc.Fields, datalake.Field{Name: str(fm["name"]), Type: str(fm["type"]), Label: str(fm["label"])})
		}
	}
	return sc
}

func (s *Service) Create(_ context.Context, in CreateRequest) (CreateOutput, error) {
	if s.lake == nil {
		return CreateOutput{}, errUnavailable
	}
	def, err := object(in.Collection, "collection")
	if err != nil {
		return CreateOutput{}, err
	}
	sc := schemaOf(def)
	if sc.Name == "" {
		return CreateOutput{}, errors.New("collection.name required")
	}
	out, err := s.lake.CreateCollection(sc, Actor)
	if err != nil {
		if errors.Is(err, datalake.ErrExists) {
			return CreateOutput{}, errors.New("collection already exists: " + sc.Name)
		}
		return CreateOutput{}, err
	}
	return CreateOutput{Collection: collection(out, 0)}, nil
}

type DropRequest struct {
	Name json.RawMessage `json:"name,omitempty"`
}

type DropOutput struct {
	Dropped string `json:"dropped"`
}

func (s *Service) Drop(_ context.Context, in DropRequest) (DropOutput, error) {
	if s.lake == nil {
		return DropOutput{}, errUnavailable
	}
	name, err := required(in.Name, "name")
	if err != nil {
		return DropOutput{}, err
	}
	if err := s.lake.DropCollection(name); err != nil {
		switch {
		case errors.Is(err, datalake.ErrSystem):
			return DropOutput{}, errors.New("built-in collection cannot be dropped: " + name)
		case errors.Is(err, datalake.ErrNotFound):
			return DropOutput{}, errors.New("no such collection: " + name)
		default:
			return DropOutput{}, err
		}
	}
	return DropOutput{Dropped: name}, nil
}

func properties(names ...string) json.RawMessage {
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, `"`+n+`":{}`)
	}
	return json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` + strings.Join(parts, ",") + `}}`)
}

// Operations declares the two read-only views and the five audited edits, all
// operator-only; the views and the record edits are on their Web UI routes.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("data lake provider required")
	}
	defs := []struct {
		name, method, path string
		read               bool
		output             reflect.Type
		input              []string
		build              func(opapi.Spec) (app.Operation, error)
	}{
		{"data_collections", "GET", "/api/data/collections", true, reflect.TypeFor[CollectionsOutput](), nil, func(s opapi.Spec) (app.Operation, error) {
			return app.NewOperation(s, func(ctx context.Context, in CollectionsRequest) (CollectionsOutput, error) {
				return provider(ctx).Collections(ctx, in)
			})
		}},
		{"data_records", "GET", "/api/data/records", true, reflect.TypeFor[RecordsOutput](), []string{"collection", "search", "sort", "desc", "limit", "offset"}, func(s opapi.Spec) (app.Operation, error) {
			return app.NewOperation(s, func(ctx context.Context, in RecordsRequest) (RecordsOutput, error) {
				return provider(ctx).Records(ctx, in)
			})
		}},
		{"data_insert", "POST", "/api/data/insert", false, reflect.TypeFor[RecordOutput](), []string{"collection", "record"}, func(s opapi.Spec) (app.Operation, error) {
			return app.NewOperation(s, func(ctx context.Context, in InsertRequest) (RecordOutput, error) {
				return provider(ctx).Insert(ctx, in)
			})
		}},
		{"data_update", "POST", "/api/data/update", false, reflect.TypeFor[RecordOutput](), []string{"collection", "id", "record"}, func(s opapi.Spec) (app.Operation, error) {
			return app.NewOperation(s, func(ctx context.Context, in UpdateRequest) (RecordOutput, error) {
				return provider(ctx).Update(ctx, in)
			})
		}},
		{"data_delete", "POST", "/api/data/delete", false, reflect.TypeFor[DeleteOutput](), []string{"collection", "id"}, func(s opapi.Spec) (app.Operation, error) {
			return app.NewOperation(s, func(ctx context.Context, in DeleteRequest) (DeleteOutput, error) {
				return provider(ctx).Delete(ctx, in)
			})
		}},
		{"data_create_collection", "", "", false, reflect.TypeFor[CreateOutput](), []string{"collection"}, func(s opapi.Spec) (app.Operation, error) {
			return app.NewOperation(s, func(ctx context.Context, in CreateRequest) (CreateOutput, error) {
				return provider(ctx).Create(ctx, in)
			})
		}},
		{"data_drop_collection", "", "", false, reflect.TypeFor[DropOutput](), []string{"name"}, func(s opapi.Spec) (app.Operation, error) {
			return app.NewOperation(s, func(ctx context.Context, in DropRequest) (DropOutput, error) { return provider(ctx).Drop(ctx, in) })
		}},
	}
	ops := make([]app.Operation, 0, len(defs))
	for _, d := range defs {
		out, err := schema.FromType(d.output, false)
		if err != nil {
			return nil, err
		}
		spec := opapi.Spec{Name: d.name, ReadOnly: d.read, OutputSchema: out, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, HTTP: opapi.HTTP{Method: d.method, Path: d.path}}
		if len(d.input) > 0 {
			spec.InputSchema = properties(d.input...)
		}
		op, err := d.build(spec)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", d.name, err)
		}
		ops = append(ops, op)
	}
	return ops, nil
}

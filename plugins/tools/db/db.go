// SPDX-License-Identifier: MIT

// Package db is the in-process `db` tool: it gives the agent a real database to
// work with — the Personal Data Lake (M834). One agent can create a collection
// (table), insert/query/update/delete records, and another agent (or the human
// in chat) reads the same data later. It is the agent-facing front of
// kernel/datalake; the Web UI renders the same collections.
//
// Capability-wise a db op reads or writes structured local state, mapped onto
// the memory capability (the existing "agent's own durable knowledge" axis) so
// it inherits the same allow-by-default posture without a new grant.
package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/datalake"
)

// Store is the slice of *datalake.Lake the tool needs — an interface so the tool
// is decoupled and unit-testable with a fake.
type Store interface {
	ListCollections() []datalake.CollectionInfo
	CreateCollection(sc datalake.Schema, actor string) (datalake.Schema, error)
	DropCollection(name string) error
	Schema(name string) (datalake.Schema, bool)
	Insert(coll string, fields map[string]any, actor string) (datalake.Record, error)
	Get(coll, id string) (datalake.Record, error)
	Update(coll, id string, patch map[string]any, actor string) (datalake.Record, error)
	Delete(coll, id string) error
	Query(coll string, q datalake.Query) ([]datalake.Record, error)
}

// Tool is the `db` implementation of agent.Tool.
type Tool struct {
	lake Store
}

// New returns an empty Tool; call SetStore before use.
func New() *Tool { return &Tool{} }

// SetStore injects the data lake (done by the daemon after the kernel opens).
func (t *Tool) SetStore(s Store) { t.lake = s }

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "db",
		Description: "Your Personal Data Lake — real databases you can build and use. Collections are " +
			"shared with other agents and the human (they can read your data from chat / the Files-style " +
			"Data view). Ops: list_collections; create_collection {name, title?, icon?, view?, fields?}; " +
			"drop_collection {name}; insert {collection, record}; get {collection, id}; " +
			"update {collection, id, record}; delete {collection, id}; " +
			"query {collection, search?, equals?, sort?, desc?, limit?}. Use it to remember structured " +
			"things (expenses, tasks, contacts, notes, bookmarks, …) deterministically instead of free text.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["op"],
  "properties": {
    "op":         {"type":"string", "enum":["list_collections","create_collection","drop_collection","insert","get","update","delete","query"]},
    "collection": {"type":"string", "description":"Target collection name."},
    "name":       {"type":"string", "description":"create_collection/drop_collection: collection name."},
    "title":      {"type":"string", "description":"create_collection: human title."},
    "icon":       {"type":"string", "description":"create_collection: lucide icon name for the UI."},
    "view":       {"type":"string", "description":"create_collection: UI view (table|expense|calendar|tasks|notes|habits|bookmarks|contacts)."},
    "fields":     {"type":"array", "items":{"type":"object"}, "description":"create_collection: field defs {name,type,label}."},
    "id":         {"type":"string", "description":"get/update/delete: record id."},
    "record":     {"type":"object", "description":"insert/update: the field map."},
    "search":     {"type":"string", "description":"query: case-insensitive text match across fields."},
    "equals":     {"type":"object", "description":"query: exact field matches."},
    "sort":       {"type":"string", "description":"query: field to sort by."},
    "desc":       {"type":"boolean", "description":"query: sort descending."},
    "limit":      {"type":"integer", "description":"query: max records (default 50)."}
  }
}`),
	}
}

type input struct {
	Op         string           `json:"op"`
	Collection string           `json:"collection,omitempty"`
	Name       string           `json:"name,omitempty"`
	Title      string           `json:"title,omitempty"`
	Icon       string           `json:"icon,omitempty"`
	View       string           `json:"view,omitempty"`
	Fields     []datalake.Field `json:"fields,omitempty"`
	ID         string           `json:"id,omitempty"`
	Record     map[string]any   `json:"record,omitempty"`
	Search     string           `json:"search,omitempty"`
	Equals     map[string]any   `json:"equals,omitempty"`
	Sort       string           `json:"sort,omitempty"`
	Desc       bool             `json:"desc,omitempty"`
	Limit      int              `json:"limit,omitempty"`
}

const defaultQueryLimit = 50

// Invoke implements agent.Tool.
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("db: parse input: %w", err)
	}
	if t.lake == nil {
		return errResult("data lake unavailable"), nil
	}
	// Provenance: attribute the mutation to the run that caused it (which maps to
	// an agent via the journal). Falls back to a generic tag in a direct unit test.
	actor := agent.CorrelationFromContext(ctx)
	if actor == "" {
		actor = "agent"
	}

	switch strings.ToLower(strings.TrimSpace(in.Op)) {
	case "list_collections":
		return jsonResult(map[string]any{"collections": t.lake.ListCollections()})
	case "create_collection":
		return t.create(in, actor)
	case "drop_collection":
		return t.drop(in)
	case "insert":
		return t.insert(in, actor)
	case "get":
		return t.get(in)
	case "update":
		return t.update(in, actor)
	case "delete":
		return t.del(in)
	case "query":
		return t.query(in)
	default:
		return errResult("op must be one of list_collections, create_collection, drop_collection, insert, get, update, delete, query"), nil
	}
}

func (t *Tool) create(in input, actor string) (agent.Result, error) {
	name := firstNonEmpty(in.Name, in.Collection)
	sc, err := t.lake.CreateCollection(datalake.Schema{
		Name: name, Title: in.Title, Icon: in.Icon, View: in.View, Fields: in.Fields,
	}, actor)
	if err != nil {
		if errors.Is(err, datalake.ErrExists) {
			return errResult(fmt.Sprintf("collection %q already exists", name)), nil
		}
		return errResult(err.Error()), nil
	}
	return jsonResult(map[string]any{"created": true, "collection": sc})
}

func (t *Tool) drop(in input) (agent.Result, error) {
	name := firstNonEmpty(in.Name, in.Collection)
	if err := t.lake.DropCollection(name); err != nil {
		return errResult(dropErr(name, err)), nil
	}
	return jsonResult(map[string]any{"dropped": name})
}

func dropErr(name string, err error) string {
	switch {
	case errors.Is(err, datalake.ErrNotFound):
		return fmt.Sprintf("no such collection %q", name)
	case errors.Is(err, datalake.ErrSystem):
		return fmt.Sprintf("%q is a built-in collection and cannot be dropped", name)
	default:
		return err.Error()
	}
}

func (t *Tool) insert(in input, actor string) (agent.Result, error) {
	if in.Collection == "" {
		return errResult("collection required for insert"), nil
	}
	r, err := t.lake.Insert(in.Collection, in.Record, actor)
	if err != nil {
		return errResult(notFoundOr(in.Collection, err)), nil
	}
	return jsonResult(map[string]any{"inserted": true, "record": r})
}

func (t *Tool) get(in input) (agent.Result, error) {
	if in.Collection == "" || in.ID == "" {
		return errResult("collection and id required for get"), nil
	}
	r, err := t.lake.Get(in.Collection, in.ID)
	if err != nil {
		return errResult(notFoundOr(in.Collection+"/"+in.ID, err)), nil
	}
	return jsonResult(map[string]any{"record": r})
}

func (t *Tool) update(in input, actor string) (agent.Result, error) {
	if in.Collection == "" || in.ID == "" {
		return errResult("collection and id required for update"), nil
	}
	r, err := t.lake.Update(in.Collection, in.ID, in.Record, actor)
	if err != nil {
		return errResult(notFoundOr(in.Collection+"/"+in.ID, err)), nil
	}
	return jsonResult(map[string]any{"updated": true, "record": r})
}

func (t *Tool) del(in input) (agent.Result, error) {
	if in.Collection == "" || in.ID == "" {
		return errResult("collection and id required for delete"), nil
	}
	if err := t.lake.Delete(in.Collection, in.ID); err != nil {
		return errResult(notFoundOr(in.Collection+"/"+in.ID, err)), nil
	}
	return jsonResult(map[string]any{"deleted": in.ID})
}

func (t *Tool) query(in input) (agent.Result, error) {
	if in.Collection == "" {
		return errResult("collection required for query"), nil
	}
	limit := in.Limit
	if limit <= 0 {
		limit = defaultQueryLimit
	}
	recs, err := t.lake.Query(in.Collection, datalake.Query{
		Search: in.Search, Equals: in.Equals, SortBy: in.Sort, Desc: in.Desc, Limit: limit,
	})
	if err != nil {
		return errResult(notFoundOr(in.Collection, err)), nil
	}
	return jsonResult(map[string]any{"count": len(recs), "records": recs})
}

func notFoundOr(what string, err error) string {
	if errors.Is(err, datalake.ErrNotFound) {
		return fmt.Sprintf("no such collection or record: %s", what)
	}
	return err.Error()
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func jsonResult(v any) (agent.Result, error) {
	out, _ := json.MarshalIndent(v, "", "  ")
	return agent.Result{Output: string(out)}, nil
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: "db: " + msg, IsError: true}
}

// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/datalake"
)

// realStore wires the tool to an actual in-memory-backed Lake (temp dir), which
// is the simplest faithful fake — the engine is already unit-tested separately.
func newTool(t *testing.T) *Tool {
	t.Helper()
	var clock int64 = 100
	l, err := datalake.Open(t.TempDir(), func() int64 { clock++; return clock })
	if err != nil {
		t.Fatalf("open lake: %v", err)
	}
	tool := New()
	tool.SetStore(l)
	return tool
}

func call(t *testing.T, tool *Tool, in string) (string, bool) {
	t.Helper()
	r, err := tool.Invoke(context.Background(), json.RawMessage(in))
	if err != nil {
		t.Fatalf("Invoke(%s): %v", in, err)
	}
	return r.Output, r.IsError
}

func TestDB_FullLifecycle(t *testing.T) {
	tool := newTool(t)

	// create
	if out, isErr := call(t, tool, `{"op":"create_collection","name":"expenses","title":"Expenses","view":"expense"}`); isErr {
		t.Fatalf("create: %s", out)
	}
	// list shows it
	out, _ := call(t, tool, `{"op":"list_collections"}`)
	if !strings.Contains(out, "expenses") {
		t.Fatalf("list missing collection: %s", out)
	}
	// insert
	out, isErr := call(t, tool, `{"op":"insert","collection":"expenses","record":{"item":"coffee","amount":5}}`)
	if isErr {
		t.Fatalf("insert: %s", out)
	}
	var ins struct {
		Record datalake.Record `json:"record"`
	}
	_ = json.Unmarshal([]byte(out), &ins)
	if ins.Record.ID == "" {
		t.Fatalf("insert returned no id: %s", out)
	}
	id := ins.Record.ID

	// query (search)
	out, _ = call(t, tool, `{"op":"query","collection":"expenses","search":"coffee"}`)
	if !strings.Contains(out, "coffee") || !strings.Contains(out, `"count": 1`) {
		t.Fatalf("query: %s", out)
	}
	// update
	out, isErr = call(t, tool, `{"op":"update","collection":"expenses","id":"`+id+`","record":{"amount":7}}`)
	if isErr || !strings.Contains(out, `"updated": true`) {
		t.Fatalf("update: %s", out)
	}
	// get reflects the update
	out, _ = call(t, tool, `{"op":"get","collection":"expenses","id":"`+id+`"}`)
	if !strings.Contains(out, `"amount": 7`) {
		t.Fatalf("get after update: %s", out)
	}
	// delete
	if out, isErr := call(t, tool, `{"op":"delete","collection":"expenses","id":"`+id+`"}`); isErr {
		t.Fatalf("delete: %s", out)
	}
	out, _ = call(t, tool, `{"op":"query","collection":"expenses"}`)
	if !strings.Contains(out, `"count": 0`) {
		t.Fatalf("query after delete should be empty: %s", out)
	}
}

func TestDB_Rejections(t *testing.T) {
	tool := newTool(t)
	cases := []string{
		`{"op":"frobnicate"}`,
		`{"op":"insert"}`,                                    // missing collection
		`{"op":"get","collection":"x"}`,                      // missing id
		`{"op":"insert","collection":"missing","record":{}}`, // unknown collection
		`{"op":"query","collection":"missing"}`,
	}
	for _, c := range cases {
		if _, isErr := call(t, tool, c); !isErr {
			t.Errorf("expected error result for %s", c)
		}
	}

	// no store configured
	noStore := New()
	r, _ := noStore.Invoke(context.Background(), json.RawMessage(`{"op":"list_collections"}`))
	if !r.IsError || !strings.Contains(r.Output, "unavailable") {
		t.Errorf("missing store should report unavailable: %s", r.Output)
	}
}

func TestDB_DropSystemRefused(t *testing.T) {
	tool := newTool(t)
	// A normal collection can be dropped.
	_, _ = call(t, tool, `{"op":"create_collection","name":"scratch"}`)
	if _, isErr := call(t, tool, `{"op":"drop_collection","name":"scratch"}`); isErr {
		t.Error("dropping a normal collection should succeed")
	}
	// Dropping a missing collection is a soft error.
	if _, isErr := call(t, tool, `{"op":"drop_collection","name":"ghost"}`); !isErr {
		t.Error("dropping a missing collection should be a soft error")
	}
}

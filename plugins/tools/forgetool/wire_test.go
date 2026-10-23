// SPDX-License-Identifier: MIT

package forgetool

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

// TestTheViewIsTheAgentsWire: the view is what the agent reads — a row it
// cannot find is a field it cannot use. So the key set is pinned: the five
// fields that are always there, and the two that appear only when they carry
// something.
func TestTheViewIsTheAgentsWire(t *testing.T) {
	tool, _ := newBound(t)
	if _, err := tool.Invoke(context.Background(), json.RawMessage(
		`{"op":"draft","name":"echo","description":"echoes","language":"python","code":"print(1)"}`)); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"show","ref":"echo"}`))
	if err != nil {
		t.Fatalf("Invoke show: %v", err)
	}
	if res.IsError {
		t.Fatalf("show errored: %s", res.Output)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(res.Output), &got); err != nil {
		t.Fatalf("the view is not json: %v (%s)", err, res.Output)
	}
	always := []string{"id", "name", "language", "status", "tested_ok"}
	for _, key := range always {
		if _, ok := got[key]; !ok {
			t.Fatalf("the view must always carry %q: %s", key, res.Output)
		}
	}
	// The optional fields: a draft has no callable name, but it does have a
	// description (it was given one).
	if got["callable_as"] != nil {
		t.Fatalf("a draft must not be callable: %s", res.Output)
	}
	if got["description"] != "echoes" {
		t.Fatalf("description = %v", got["description"])
	}
	// show adds the code and the schema.
	if _, ok := got["code"]; !ok {
		t.Fatalf("show must carry the code: %s", res.Output)
	}
}

// TestTheListIsTheAgentsWire: the list carries the count and the rows — the
// agent uses the count to know it has seen everything.
func TestTheListIsTheAgentsWire(t *testing.T) {
	tool, _ := newBound(t)
	for _, name := range []string{"alpha", "beta"} {
		if _, err := tool.Invoke(context.Background(), json.RawMessage(
			`{"op":"draft","name":"`+name+`","description":"d","language":"python","code":"print(1)"}`)); err != nil {
			t.Fatalf("Invoke draft %s: %v", name, err)
		}
	}

	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("Invoke list: %v", err)
	}
	if res.IsError {
		t.Fatalf("list errored: %s", res.Output)
	}
	var got struct {
		Count int `json:"count"`
		Tools []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Language string `json:"language"`
			Status   string `json:"status"`
			TestedOK bool   `json:"tested_ok"`
		} `json:"tools"`
	}
	if err := json.Unmarshal([]byte(res.Output), &got); err != nil {
		t.Fatalf("the list is not json: %v (%s)", err, res.Output)
	}
	if got.Count != 2 || len(got.Tools) != 2 {
		t.Fatalf("the list must carry both tools and their count: %s", res.Output)
	}
	names := []string{got.Tools[0].Name, got.Tools[1].Name}
	sort.Strings(names)
	if names[0] != "alpha" || names[1] != "beta" {
		t.Fatalf("the listed names = %v", names)
	}
	for _, row := range got.Tools {
		if row.ID == "" {
			t.Fatalf("every listed tool must carry its id: %s", res.Output)
		}
		if row.Language != "python" {
			t.Fatalf("every listed tool must carry its language: %s", res.Output)
		}
		if row.Status != "draft" {
			t.Fatalf("a fresh tool is a draft: %s", res.Output)
		}
		if row.TestedOK {
			t.Fatalf("a fresh tool is not tested: %s", res.Output)
		}
	}
}

// TestTheViewCarriesTheTestRecord: the test record is a VALUE the agent acts
// on — a tool that was tested must say so. A view that always reported
// "not tested" would look exactly like a view that carried the field.
func TestTheViewCarriesTheTestRecord(t *testing.T) {
	tool, fk := newBound(t)
	if _, err := tool.Invoke(context.Background(), json.RawMessage(
		`{"op":"draft","name":"echo","description":"echoes","language":"python","code":"print(1)"}`)); err != nil {
		t.Fatalf("Invoke draft: %v", err)
	}
	fk.testOK = true
	if _, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"test","ref":"echo"}`)); err != nil {
		t.Fatalf("Invoke test: %v", err)
	}

	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"show","ref":"echo"}`))
	if err != nil {
		t.Fatalf("Invoke show: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(res.Output), &got); err != nil {
		t.Fatalf("the view is not json: %v (%s)", err, res.Output)
	}
	if got["tested_ok"] != true {
		t.Fatalf("a tested tool must report its test record: %s", res.Output)
	}
}

// TestTheReadOpsNeedTheStorePort: a tool that was given the kernel but no
// store must SAY that — not call into nothing. The read ops are the ones
// that need the store; the write ops go through the kernel.
func TestTheReadOpsNeedTheStorePort(t *testing.T) {
	_, fk := newBound(t)
	// A tool bound to the kernel, but to no store.
	bare := New()
	bare.Bind(fk, nil)

	for _, op := range []string{`{"op":"list"}`, `{"op":"show","ref":"echo"}`} {
		res, err := bare.Invoke(context.Background(), json.RawMessage(op))
		if err != nil {
			t.Fatalf("Invoke %s: %v", op, err)
		}
		if !res.IsError {
			t.Fatalf("%s without a store must report that, got: %s", op, res.Output)
		}
		if !strings.Contains(res.Output, "not available") {
			t.Fatalf("%s without a store must say it is not available: %s", op, res.Output)
		}
	}

	// The write ops still work — they go through the kernel, not the store.
	res, err := bare.Invoke(context.Background(), json.RawMessage(
		`{"op":"draft","name":"echo","description":"d","language":"python","code":"print(1)"}`))
	if err != nil {
		t.Fatalf("Invoke draft: %v", err)
	}
	if res.IsError {
		t.Fatalf("the write op must not need the store: %s", res.Output)
	}
}

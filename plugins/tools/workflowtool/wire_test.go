// SPDX-License-Identifier: MIT

package workflowtool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestTheViewIsTheAgentsWire: the view is what the agent reads — a row it
// cannot find is a field it cannot use. So the key set is pinned: the five
// fields that are always there, and the one that appears only when it carries
// something.
func TestTheViewIsTheAgentsWire(t *testing.T) {
	fk := newFakeKernel(t)
	tool := New()
	tool.Bind(fk, fk.store)

	out := invoke(t, tool, `{"op":"save","workflow":`+graphJSON+`}`)
	if !strings.Contains(out, "created") {
		t.Fatalf("save:\n%s", out)
	}

	out = invoke(t, tool, `{"op":"list"}`)
	if !strings.Contains(out, `"count": 1`) {
		t.Fatalf("list:\n%s", out)
	}
	var listed struct {
		Count int `json:"count"`
		Tools []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
			Nodes   int    `json:"nodes"`
			Edges   int    `json:"edges"`
		} `json:"workflows"`
	}
	if err := json.Unmarshal([]byte(out), &listed); err != nil {
		t.Fatalf("the list is not json: %v (%s)", err, out)
	}
	if listed.Count != 1 || len(listed.Tools) != 1 {
		t.Fatalf("the list must carry the workflow and its count: %s", out)
	}
	row := listed.Tools[0]
	if row.ID == "" || row.Name != "greeter" {
		t.Fatalf("the listed workflow must carry its id and name: %s", out)
	}
	if row.Enabled {
		t.Fatalf("an agent's save arrives disabled: %s", out)
	}
	if row.Nodes != 2 || row.Edges != 1 {
		t.Fatalf("the listed workflow must carry its shape: %s", out)
	}
}

// TestTheReadOpsNeedTheStorePort: a tool that was given the kernel but no
// store must SAY that — not call into nothing. The read ops are the ones
// that need the store; the write ops go through the kernel.
func TestTheReadOpsNeedTheStorePort(t *testing.T) {
	fk := newFakeKernel(t)
	bare := New()
	bare.Bind(fk, nil)

	for _, op := range []string{`{"op":"list"}`, `{"op":"show","ref":"greeter"}`} {
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
		`{"op":"save","workflow":`+graphJSON+`}`))
	if err != nil {
		t.Fatalf("Invoke save: %v", err)
	}
	if res.IsError {
		t.Fatalf("the write op must not need the store: %s", res.Output)
	}
}

// TestTheRunCarriesTheOutcome: op=run reports the run's outcome — the
// executed node ids and the per-node outputs. The row it reports is the
// contract's, and the values must survive the trip.
func TestTheRunCarriesTheOutcome(t *testing.T) {
	fk := newFakeKernel(t)
	tool := New()
	tool.Bind(fk, fk.store)

	out := invoke(t, tool, `{"op":"run","ref":"greeter","payload":{"name":"ersin"}}`)
	var got struct {
		Executed []string       `json:"executed"`
		Outputs  map[string]any `json:"outputs"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("the run output is not json: %v (%s)", err, out)
	}
	if len(got.Executed) != 2 || got.Executed[0] != "start" {
		t.Fatalf("the run must report the executed nodes: %s", out)
	}
	if got.Outputs["greet"] != "hello" {
		t.Fatalf("the run must report the node outputs: %s", out)
	}
}

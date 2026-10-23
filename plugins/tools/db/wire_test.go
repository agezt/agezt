// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestTheCreateOpAnswersAndKeepsTheDeclaration: creating a collection is the
// op the agent uses to declare a table — so the answer must say the
// collection exists (a second create is refused, not silently accepted), and
// the collection that was stored must carry the columns the agent declared.
// A create that dropped the declaration would leave the agent with a table it
// cannot write the right fields into.
func TestTheCreateOpAnswersAndKeepsTheDeclaration(t *testing.T) {
	tool := newTool(t)

	out, isErr := call(t, tool, `{"op":"create_collection","name":"scratch","title":"Scratch","fields":[{"name":"amount","type":"money","label":"Amount"}]}`)
	if isErr {
		t.Fatalf("the first create must succeed: %s", out)
	}
	if !strings.Contains(out, "created") {
		t.Fatalf("the create must answer that it created the collection: %s", out)
	}

	// The declared columns must be in the stored schema.
	out, isErr = call(t, tool, `{"op":"list_collections"}`)
	if isErr {
		t.Fatalf("list_collections: %s", out)
	}
	var listed struct {
		Collections []struct {
			Name   string `json:"name"`
			Title  string `json:"title"`
			Fields []struct {
				Name  string `json:"name"`
				Type  string `json:"type"`
				Label string `json:"label"`
			} `json:"fields"`
		} `json:"collections"`
	}
	if err := json.Unmarshal([]byte(out), &listed); err != nil {
		t.Fatalf("the list is not json: %v (%s)", err, out)
	}
	var found bool
	for _, c := range listed.Collections {
		if c.Name != "scratch" {
			continue
		}
		found = true
		if c.Title != "Scratch" {
			t.Fatalf("the collection must keep its title: %s", out)
		}
		if len(c.Fields) != 1 {
			t.Fatalf("the collection must keep its declared columns: %s", out)
		}
		if c.Fields[0].Name != "amount" || c.Fields[0].Type != "money" || c.Fields[0].Label != "Amount" {
			t.Fatalf("the declared column is wrong: %s", out)
		}
	}
	if !found {
		t.Fatalf("the created collection must be listed: %s", out)
	}

	// A second create with the same name is refused — and the refusal is the
	// tool's OWN "already exists" answer, not the lake's raw error (which
	// would say "datalake: collection already exists" without the name).
	out, isErr = call(t, tool, `{"op":"create_collection","name":"scratch"}`)
	if !isErr {
		t.Fatalf("a second create must be refused: %s", out)
	}
	if !strings.Contains(out, `collection "scratch" already exists`) {
		t.Fatalf("a second create must answer with the collection's name: %s", out)
	}
}

// TestTheToolReportsAMissingStore: a tool that was never given the lake must
// say so — not call into nothing.
func TestTheToolReportsAMissingStore(t *testing.T) {
	tool := New()
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"list_collections"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError {
		t.Fatalf("a tool without a lake must report that, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "unavailable") {
		t.Fatalf("the unwired tool must say it is unavailable: %s", res.Output)
	}
}

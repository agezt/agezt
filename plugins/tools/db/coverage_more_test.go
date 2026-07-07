// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/datalake"
)

func TestDBCoverageDefinitionAndHelpers(t *testing.T) {
	tool := New()
	def := tool.Definition()
	if def.Name != "db" {
		t.Fatalf("Name = %q", def.Name)
	}
	if !strings.Contains(def.Description, "Personal Data Lake") {
		t.Fatalf("description should mention Data Lake, got %q", def.Description)
	}
	if def.Effect.Class != agent.EffectCompensable {
		t.Fatalf("Effect.Class = %v, want %v", def.Effect.Class, agent.EffectCompensable)
	}
	schema := string(def.InputSchema)
	for _, op := range []string{`"list_collections"`, `"create_collection"`, `"insert"`, `"query"`} {
		if !strings.Contains(schema, op) {
			t.Fatalf("schema should include op %q, got %s", op, schema)
		}
	}

	// dataLakeActor branches.
	cases := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{name: "agent+corr", ctx: agent.WithAgent(agent.WithCorrelation(context.Background(), "run-1"), "tester"), want: "tester:run-1"},
		{name: "agent only", ctx: agent.WithAgent(context.Background(), "tester"), want: "tester"},
		{name: "corr only", ctx: agent.WithCorrelation(context.Background(), "run-1"), want: "run-1"},
		{name: "default", ctx: context.Background(), want: "agent"},
	}
	for _, tc := range cases {
		if got := dataLakeActor(tc.ctx); got != tc.want {
			t.Fatalf("dataLakeActor(%s) = %q, want %q", tc.name, got, tc.want)
		}
	}

	if firstNonEmpty(" ", "fallback") != "fallback" {
		t.Fatal("firstNonEmpty whitespace should fall through")
	}
	if firstNonEmpty("primary", "fallback") != "primary" {
		t.Fatal("firstNonEmpty should prefer non-empty value")
	}

	if !strings.Contains(dropErr("c", wrapNotFound("not_found")), "no such collection") {
		t.Fatalf("dropErr not-found message = %q", dropErr("c", wrapNotFound("not_found")))
	}
	if !strings.Contains(dropErr("c", wrapSystem("built-in")), "built-in") {
		t.Fatalf("dropErr system message = %q", dropErr("c", wrapSystem("built-in")))
	}
	if !strings.Contains(dropErr("c", errFake("other")), "other") {
		t.Fatalf("dropErr default should pass through: %q", dropErr("c", errFake("other")))
	}

	if !strings.Contains(notFoundOr("c", wrapNotFound("not_found")), "no such collection") {
		t.Fatalf("notFoundOr not-found = %q", notFoundOr("c", wrapNotFound("not_found")))
	}
	if !strings.Contains(notFoundOr("c", errFake("other")), "other") {
		t.Fatalf("notFoundOr default = %q", notFoundOr("c", errFake("other")))
	}
}

func TestDBCoverageInvokeValidation(t *testing.T) {
	_, err := New().Invoke(context.Background(), json.RawMessage(`{`))
	if err == nil || !strings.Contains(err.Error(), "parse input") {
		t.Fatalf("parse error = %v", err)
	}

	res, err := New().Invoke(context.Background(), json.RawMessage(`{"op":"list_collections"}`))
	if err != nil || !res.IsError || !strings.Contains(res.Output, "unavailable") {
		t.Fatalf("unavailable = %+v err %v", res, err)
	}

	tool := newTool(t)
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"op":"wat"}`))
	if err != nil || !res.IsError || !strings.Contains(res.Output, "op must be one of") {
		t.Fatalf("unknown op = %+v err %v", res, err)
	}
}

func errFake(marker string) error { return &fakeErr{marker: marker} }

type fakeErr struct{ marker string }

func (e *fakeErr) Error() string { return e.marker }

// wrapNotFound returns an error that errors.Is(..., datalake.ErrNotFound)
// matches and that carries marker in its text. We only care about the
// errors.Is branch here, so the marker is purely informational.
func wrapNotFound(marker string) error { return fmt.Errorf("wrap-not-found: %w", datalake.ErrNotFound) }

func wrapSystem(marker string) error { return fmt.Errorf("wrap-system: %w", datalake.ErrSystem) }

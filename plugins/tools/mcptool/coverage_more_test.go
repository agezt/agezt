// SPDX-License-Identifier: MIT

package mcptool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestMCPCoverageDefinitionAndHelpers(t *testing.T) {
	tool := New()
	def := tool.Definition()
	if def.Name != "mcp" {
		t.Fatalf("Name = %q", def.Name)
	}
	if def.Effect.Class != agent.EffectCompensable {
		t.Fatalf("Effect.Class = %v, want %v", def.Effect.Class, agent.EffectCompensable)
	}
	schema := string(def.InputSchema)
	for _, want := range []string{`"add"`, `"attach"`, `"detach"`, `"list"`, `"remove"`, `"op"`, `"name"`} {
		if !strings.Contains(schema, want) {
			t.Fatalf("schema should include %q, got %s", want, schema)
		}
	}
}

func TestMCPCoverageInvokeValidation(t *testing.T) {
	// Parse error → hard.
	_, err := New().Invoke(context.Background(), json.RawMessage(`{`))
	if err == nil || !strings.Contains(err.Error(), "parse input") {
		t.Fatalf("parse error = %v", err)
	}

	// Unbound → soft error.
	tool := New()
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "not available") {
		t.Fatalf("unbound = %+v", res)
	}
}

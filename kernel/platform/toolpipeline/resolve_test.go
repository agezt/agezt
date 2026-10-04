// SPDX-License-Identifier: MIT

package toolpipeline_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	"github.com/agezt/agezt/kernel/platform/toolpipeline"
)

type resolutionTool struct {
	def         toolapi.ToolDef
	definitions int
}

func (p *resolutionTool) Definition() toolapi.ToolDef { p.definitions++; return p.def }
func (*resolutionTool) Invoke(context.Context, json.RawMessage) (toolapi.Result, error) {
	panic("resolution must not execute")
}

func TestResolveRetainsLookupSchemaContract(t *testing.T) {
	for _, tc := range []struct {
		name, input, schema string
		found               bool
	}{
		{"unknown", `{`, `{`, false},
		{"alias", `{"n":1}`, `{"type":"object","properties":{"n":{"type":"integer"}},"required":["n"]}`, true},
		{"invalid-json", `{`, `{"type":"object"}`, true},
		{"required", `{}`, `{"type":"object","properties":{"n":{"type":"integer"}},"required":["n"]}`, true},
		{"wrong-type", `{"n":"one"}`, `{"type":"object","properties":{"n":{"type":"integer"}}}`, true},
		{"malformed-schema", `{}`, `{`, true},
		{"empty-schema", `[1]`, "", true},
		{"empty-schema-invalid-json", `{`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := toolapi.ToolDef{Name: "implementation", Description: "registry alias", InputSchema: json.RawMessage(tc.schema), Capability: toolapi.ToolCapability{Name: "introspect"}, Effect: toolapi.ToolEffect{Class: toolapi.EffectReadOnly}}
			tool := &resolutionTool{def: def}
			call := llm.ToolCall{ID: "call", Name: "alias", Input: json.RawMessage(tc.input)}
			lookups := 0
			got := toolpipeline.Resolve(call, func(name string) (toolapi.Tool, bool) {
				lookups++
				if name != call.Name {
					t.Errorf("lookup name=%q want=%q", name, call.Name)
				}
				return tool, tc.found
			})
			if lookups != 1 || got.Found != tc.found || got.Tool != tool {
				t.Fatalf("resolution=%+v lookups=%d", got, lookups)
			}
			if !tc.found {
				if tool.definitions != 0 || got.InputError != nil || !reflect.DeepEqual(got.Definition, toolapi.ToolDef{}) {
					t.Fatalf("unavailable resolution=%+v definitions=%d", got, tool.definitions)
				}
				return
			}
			if tool.definitions != 1 || !reflect.DeepEqual(got.Definition, def) {
				t.Fatalf("metadata=%+v definitions=%d", got.Definition, tool.definitions)
			}
			wantErr := schema.ValidateToolInput(def, call.Input)
			if wantErr == nil {
				if got.InputError != nil {
					t.Fatalf("unexpected schema error: %v", got.InputError)
				}
			} else if got.InputError == nil || got.InputError.Error() != wantErr.Error() {
				t.Fatalf("schema error=%v want=%v", got.InputError, wantErr)
			}
			if string(call.Input) != tc.input {
				t.Fatal("input was modified")
			}
		})
	}
}

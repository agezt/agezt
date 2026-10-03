// SPDX-License-Identifier: MIT

package schema_test

import (
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

func TestToolInputPreservesValidation(t *testing.T) {
	for _, tc := range []struct{ name, schema, input, want string }{
		{"empty-schema", "", `{"anything":true}`, ""},
		{"invalid-json", "", `{`, "input is not valid JSON"},
		{"required", `{"type":"object","required":["value"]}`, `{}`, "$.value is required"},
		{"implicit-additional-denial", `{"type":"object","properties":{"value":{"type":"string"}}}`, `{"other":1}`, "$.other is not allowed by schema"},
		{"enum", `{"type":"object","properties":{"opts":{"type":"object","properties":{"mode":{"enum":["read"]}}}}}`, `{"opts":{"mode":"write"}}`, "$.opts.mode must be one of the declared enum values"},
		{"integer", `{"type":"object","properties":{"count":{"type":"integer"}}}`, `{"count":1.5}`, "$.count has wrong type: got number, want integer"},
		{"array-items", `{"type":"object","properties":{"values":{"type":"array","items":{"type":"integer"}}}}`, `{"values":["bad"]}`, "$.values[0] has wrong type: got string, want integer"},
		{"union", `{"type":["null","string"]}`, `null`, ""},
		{"additional-schema", `{"type":"object","additionalProperties":{"type":"integer"}}`, `{"x":"bad"}`, "$.x has wrong type: got string, want integer"},
		{"future-type-runtime", `{"type":"future"}`, `"ok"`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := toolapi.ToolDef{Name: "probe", InputSchema: json.RawMessage(tc.schema)}
			for name, validate := range map[string]func(toolapi.ToolDef, json.RawMessage) error{"platform": schema.ValidateToolInput, "agent": agent.ValidateToolInput} {
				err := validate(def, json.RawMessage(tc.input))
				got := ""
				if err != nil {
					got = err.Error()
				}
				if got != tc.want {
					t.Errorf("%s got %q want %q", name, got, tc.want)
				}
			}
		})
	}
}

func TestSchemaLintPreservesRegistrationContract(t *testing.T) {
	for _, tc := range []struct{ schema, want string }{
		{"", ""},
		{`{"type":"object"}`, ""},
		{`{"type":"future"}`, `tool "probe" has invalid input schema: $.type "future" is not supported`},
		{`{"type":"object","additionalProperties":3}`, `tool "probe" has invalid input schema: $.additionalProperties must be boolean or object`},
	} {
		for name, lint := range map[string]func(toolapi.ToolDef) error{"platform": schema.LintToolSchema, "agent": agent.LintToolSchema} {
			err := lint(toolapi.ToolDef{Name: "probe", InputSchema: json.RawMessage(tc.schema)})
			got := ""
			if err != nil {
				got = err.Error()
			}
			if got != tc.want {
				t.Errorf("%s got %q want %q", name, got, tc.want)
			}
		}
	}
}

func TestStructuredOutputValidationPreservesContract(t *testing.T) {
	for _, tc := range []struct{ schema, value, want string }{
		{"", `1`, ""},
		{"", `{`, "value is not valid JSON"},
		{`{"type":"integer"}`, `1.5`, "$ has wrong type: got number, want integer"},
	} {
		for name, validate := range map[string]func(json.RawMessage, json.RawMessage) error{"platform": schema.ValidateJSON, "agent": agent.ValidateJSON} {
			err := validate(json.RawMessage(tc.schema), json.RawMessage(tc.value))
			got := ""
			if err != nil {
				got = err.Error()
			}
			if got != tc.want {
				t.Errorf("%s got %q want %q", name, got, tc.want)
			}
		}
	}
}

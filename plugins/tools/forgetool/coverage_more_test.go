// SPDX-License-Identifier: MIT

package forgetool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/toolforge"
)

func TestForgeCoverageDefinition(t *testing.T) {
	tl := New()
	def := tl.Definition()
	if def.Name != "tool_forge" {
		t.Fatalf("Name = %q", def.Name)
	}
	if def.Effect.Class != agent.EffectCompensable {
		t.Fatalf("Effect.Class = %v, want %v", def.Effect.Class, agent.EffectCompensable)
	}
	if !strings.Contains(def.Description, "forge_<name>") {
		t.Fatalf("description should mention forge_<name>, got %q", def.Description)
	}
	for _, want := range []string{`"draft"`, `"update"`, `"test"`, `"request_promotion"`, `"list"`, `"show"`} {
		if !strings.Contains(string(def.InputSchema), want) {
			t.Fatalf("schema should include op %q, got %s", want, def.InputSchema)
		}
	}
}

func TestForgeCoverageViewAndOKJSON(t *testing.T) {
	// view() with empty/active/draft.
	draft := toolforge.ScriptTool{ID: "t1", Name: "fetch_weather", Language: "python", Status: toolforge.StatusDraft}
	v := view(draft)
	if v["id"] != "t1" || v["name"] != "fetch_weather" || v["language"] != "python" || v["status"] != "draft" {
		t.Fatalf("view draft = %+v", v)
	}
	if _, ok := v["callable_as"]; ok {
		t.Fatalf("draft should not have callable_as: %+v", v)
	}
	if _, ok := v["description"]; ok {
		t.Fatalf("empty description should be omitted: %+v", v)
	}

	active := toolforge.ScriptTool{ID: "t2", Name: "t2", Status: toolforge.StatusActive, Description: "do a thing"}
	v2 := view(active)
	if v2["callable_as"] != "forge_t2" || v2["description"] != "do a thing" {
		t.Fatalf("view active = %+v", v2)
	}

	// okJSON happy path.
	r := okJSON(map[string]any{"k": "v"})
	if r.IsError || !strings.Contains(r.Output, `"k": "v"`) {
		t.Fatalf("okJSON = %+v", r)
	}

	// okJSON marshal failure (e.g. unsupported value type) surfaces as errResult.
	r = okJSON(make(chan int))
	if !r.IsError || !strings.HasPrefix(r.Output, "tool_forge: marshal:") {
		t.Fatalf("okJSON marshal error = %+v", r)
	}
}

func TestForgeCoverageInvokeValidation(t *testing.T) {
	// Parse error → hard error.
	_, err := New().Invoke(context.Background(), json.RawMessage(`{`))
	if err == nil || !strings.Contains(err.Error(), "parse input") {
		t.Fatalf("parse error = %v", err)
	}

	// No kernel bound.
	res, err := New().Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil || !res.IsError || !strings.Contains(res.Output, "not available") {
		t.Fatalf("unavailable = %+v err %v", res, err)
	}

	// Empty op on an unbound tool surfaces the "not available" error (the
	// kernel check happens before op routing).
	tool := New()
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{}`))
	if err != nil || !res.IsError || !strings.Contains(res.Output, "not available") {
		t.Fatalf("empty op = %+v err %v", res, err)
	}

	// Unknown op is similarly gated by the kernel check; cover the parse
	// error path separately above. The op routing is exercised via the
	// real-kernel tests in tool_test.go.
}

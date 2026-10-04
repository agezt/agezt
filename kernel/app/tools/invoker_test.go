// SPDX-License-Identifier: MIT

package tools_test

import (
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolphaseapi"
	"github.com/agezt/agezt/kernel/platform/toolpipeline"
)

type unavailableRegistry struct{ names []string }

func (r *unavailableRegistry) LookupTool(name string) (toolapi.Tool, bool) {
	r.names = append(r.names, name)
	return nil, false
}

func TestAppInvokerExposesBoundPhasePort(t *testing.T) {
	registry := &unavailableRegistry{}
	service := tools.NewInvoker(toolpipeline.Dependencies{Tools: registry})
	phases, ok := service.(toolphaseapi.Phases)
	if !ok {
		t.Fatal("app invoker hides the shared phase port")
	}
	resolved := phases.Resolve(llm.ToolCall{Name: "missing", Input: json.RawMessage(`{}`)}, nil)
	if resolved.Found || len(registry.names) != 1 || registry.names[0] != "missing" {
		t.Fatalf("bound lookup: %+v names=%v", resolved, registry.names)
	}
}

// SPDX-License-Identifier: MIT

package toolexec_test

import (
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolphaseapi"
	"github.com/agezt/agezt/kernel/toolexec"
)

func TestLegacyInvokerExposesHostBoundPhases(t *testing.T) {
	tool := &fakeTool{def: toolapi.ToolDef{Name: "probe"}}
	service := toolexec.NewInvoker(toolexec.Dependencies{Tools: mockLookup{"probe": tool}})
	phases, ok := service.(toolphaseapi.Phases)
	if !ok {
		t.Fatal("standalone legacy service hides phase port")
	}
	if got := phases.Resolve(llm.ToolCall{Name: "probe", Input: json.RawMessage(`{}`)}, nil); got.Tool != tool || !got.Found || got.InputError != nil {
		t.Fatalf("legacy resolution=%+v", got)
	}
}

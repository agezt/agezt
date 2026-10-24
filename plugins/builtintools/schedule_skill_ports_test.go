// SPDX-License-Identifier: MIT

package builtintools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/toolreg"
)

// TestTheScheduleAndSkillSpecsStayUnwiredWithoutTheirPorts: without the
// ports the two tools must say they are unavailable — they must not call into
// nothing.
//
// The nil port is the trap the daemon's rule exists for: a port that wraps a
// nil implementation is NOT nil, so the tool would bind a store that is not
// there and fail in a way the operator could not read. The schedule tool
// wants the cadence store, the skill tool wants the forge and its bundle
// store — and a daemon that hands over none of them leaves both tools
// reporting that.
func TestTheScheduleAndSkillSpecsStayUnwiredWithoutTheirPorts(t *testing.T) {
	for _, tc := range []struct {
		name  string
		spec  func() toolreg.Spec
		input string
		want  string
	}{
		{"schedule", specSchedule, `{"op":"in","delay":"30m","intent":"do the thing"}`, "not available"},
		{"skill", specSkill, `{"op":"learn","name":"diagnose-ci","description":"when CI fails","body":"1. read the logs"}`, "not available"},
	} {
		spec := tc.spec()
		built, err := spec.Build(toolreg.BuildDeps{})
		if err != nil {
			t.Fatalf("%s Build: %v", tc.name, err)
		}
		if err := spec.Configure(built.Tool, toolreg.KernelDeps{}); err != nil {
			t.Fatalf("%s Configure: %v", tc.name, err)
		}
		res, err := built.Tool.Invoke(context.Background(), json.RawMessage(tc.input))
		if err != nil {
			t.Fatalf("%s Invoke: %v", tc.name, err)
		}
		if !res.IsError || !strings.Contains(res.Output, tc.want) {
			t.Fatalf("%s must report that it is unavailable, got %+v", tc.name, res)
		}
	}
}

// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestSkillGet_ReturnsBodyForDiff confirms the data path `agt skill diff` (M118)
// relies on: CmdSkillGet returns each skill's body, so the CLI can fetch two and
// diff them. (The diff math itself is unit-tested in cmd/agt.)
func TestSkillGet_ReturnsBodyForDiff(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	a, _, err := k.Forge().Create("seed", skill.CreateSpec{Name: "s", Body: "line one\nline two\n"})
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := k.Forge().Create("seed", skill.CreateSpec{Name: "s2", Body: "line one\nline TWO\nline three\n"})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ id, wantSub string }{
		{a.ID, "line two"},
		{b.ID, "line three"},
	} {
		res, err := c.Call(ctx, controlplane.CmdSkillGet, map[string]any{"id": tc.id})
		if err != nil {
			t.Fatalf("get %s: %v", tc.id, err)
		}
		if found, _ := res["found"].(bool); !found {
			t.Fatalf("skill %s not found", tc.id)
		}
		sk, _ := res["skill"].(map[string]any)
		body, _ := sk["body"].(string)
		if body == "" {
			t.Fatalf("skill %s has empty body (diff needs it)", tc.id)
		}
		if !strings.Contains(body, tc.wantSub) {
			t.Errorf("skill %s body %q missing %q", tc.id, body, tc.wantSub)
		}
	}
}

// SPDX-License-Identifier: MIT

package toolapi

import (
	"context"
	"testing"
)

// The per-run tool allowlist is a CONTRACT value: the application decides
// which tools a run may call, and the host that starts the run states that
// decision without naming the runtime's module.
//
// The distinction the tests hold is the one the runtime's own reader held: an
// UNSET allowlist (no restriction — use all tools) is not the same as an EMPTY
// one (no tools at all).

func TestTheToolAllowlistTravels(t *testing.T) {
	ctx := WithTools(context.Background(), []string{"shell", "file"})
	got, ok := ToolsFrom(ctx)
	if !ok {
		t.Fatal("the allowlist did not travel")
	}
	if len(got) != 2 || got[0] != "shell" || got[1] != "file" {
		t.Errorf("allowlist = %v, want [shell file]", got)
	}
}

func TestAnEmptyAllowlistIsStillARestriction(t *testing.T) {
	ctx := WithTools(context.Background(), []string{})
	got, ok := ToolsFrom(ctx)
	if !ok {
		t.Fatal("an empty allowlist must still count as set — it means NO tools, not all tools")
	}
	if len(got) != 0 {
		t.Errorf("allowlist = %v, want empty", got)
	}
}

func TestAPlainContextCarriesNoAllowlist(t *testing.T) {
	if _, ok := ToolsFrom(context.Background()); ok {
		t.Error("a plain context must carry no allowlist — an unset allowlist means every tool")
	}
}

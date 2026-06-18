// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"testing"
)

func TestAutoApproveCapabilitiesContext(t *testing.T) {
	base := context.Background()

	// No grant → nothing auto-approves.
	if autoApproveCap(base, "tool.forge") {
		t.Fatal("empty context must not auto-approve")
	}

	// Empty set is a no-op (context unchanged, no auto-approve).
	if got := WithAutoApproveCapabilities(base, nil); got != base {
		t.Fatal("nil caps should leave the context unchanged")
	}
	if got := WithAutoApproveCapabilities(base, map[string]bool{}); got != base {
		t.Fatal("empty caps should leave the context unchanged")
	}

	// A grant covers exactly its listed capabilities and rides the context (so it
	// reaches every sub-agent the run spawns, which inherit context values).
	ctx := WithAutoApproveCapabilities(base, map[string]bool{"tool.forge": true, "code.exec": true})
	if !autoApproveCap(ctx, "tool.forge") {
		t.Fatal("granted tool.forge should auto-approve")
	}
	if !autoApproveCap(ctx, "code.exec") {
		t.Fatal("granted code.exec should auto-approve")
	}
	if autoApproveCap(ctx, "shell") {
		t.Fatal("ungranted capability must still require approval")
	}
}

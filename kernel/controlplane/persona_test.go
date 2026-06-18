// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestPersona_GetSet_LiveAndPersisted proves the legacy persona endpoint edits
// the daemon default identity: set applies live (kernel.System reflects it
// immediately) and persists to the config store as AGEZT_SYSTEM_PROMPT; clearing
// removes it.
func TestPersona_GetSet_LiveAndPersisted(t *testing.T) {
	k, _, c, dir := startPair(t, mock.New(mock.FinalText("ok")))

	// Initially no default identity.
	res, err := c.Call(context.Background(), controlplane.CmdPersonaGet, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res["set"] != false || res["system"] != "" {
		t.Fatalf("fresh daemon should have no default identity, got %v", res)
	}

	// Set a default identity.
	defaultIdentity := "You are Jarvis. Be terse and proactive."
	if _, err := c.Call(context.Background(), controlplane.CmdPersonaSet, map[string]any{"system": defaultIdentity}); err != nil {
		t.Fatal(err)
	}

	// Live: the kernel's System reflects it without a restart.
	if got := k.System(); got != defaultIdentity {
		t.Errorf("kernel.System() = %q, want the new default identity applied live", got)
	}
	// Readback through the control plane.
	res, _ = c.Call(context.Background(), controlplane.CmdPersonaGet, nil)
	if res["system"] != defaultIdentity || res["set"] != true {
		t.Errorf("persona_get = %v, want the set default identity", res)
	}
	// Persisted to the config store (survives restart via startup injection).
	store := settings.NewStore(dir)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if v, _ := store.Get("AGEZT_SYSTEM_PROMPT"); v != defaultIdentity {
		t.Errorf("config store AGEZT_SYSTEM_PROMPT = %q, want the default identity", v)
	}

	// Clearing removes it, live and persisted.
	if _, err := c.Call(context.Background(), controlplane.CmdPersonaSet, map[string]any{"system": ""}); err != nil {
		t.Fatal(err)
	}
	if k.System() != "" {
		t.Errorf("clearing should leave no default identity, got %q", k.System())
	}
	store2 := settings.NewStore(dir)
	_ = store2.Load()
	if v, ok := store2.Get("AGEZT_SYSTEM_PROMPT"); ok || v != "" {
		t.Errorf("config store should have no AGEZT_SYSTEM_PROMPT after clear, got %q (ok=%v)", v, ok)
	}
}

// TestPersona_Set_RequiresStringArg guards the arg contract.
func TestPersona_Set_RequiresStringArg(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	_, err := c.Call(context.Background(), controlplane.CmdPersonaSet, map[string]any{"system": 42})
	if err == nil {
		t.Error("persona_set with a non-string system should error")
	}
	if _, err := c.Call(context.Background(), controlplane.CmdPersonaSet, nil); err == nil {
		t.Error("persona_set without args.system should error")
	}
}

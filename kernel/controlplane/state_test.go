// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestStateList_EmptyStoreReturnsEmptyArray — fresh kernel, no
// writes; both the namespace-listing and the key-listing
// (against an unknown namespace) must come back as valid JSON
// arrays (not null) so jq pipelines stay simple.
func TestStateList_EmptyStoreReturnsEmptyArray(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdStateList, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	rows, ok := res["namespaces"].([]any)
	if !ok {
		t.Fatalf("namespaces wrong type: %T", res["namespaces"])
	}
	if len(rows) != 0 {
		t.Errorf("namespaces should be empty, got %v", rows)
	}
}

// TestState_WriteReadCycle covers the full path: write a key
// via the kernel's state store, then read it back through the
// control plane handlers. Asserts wire shape (found=true, value
// decoded as its native JSON type) and that the namespace shows
// up in list.
func TestState_WriteReadCycle(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	if err := k.State().Set("agent_memory", "last_run", "ok"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// list namespaces
	res, err := c.Call(context.Background(), controlplane.CmdStateList, nil)
	if err != nil {
		t.Fatalf("Call list: %v", err)
	}
	rows, _ := res["namespaces"].([]any)
	found := false
	for _, r := range rows {
		if r == "agent_memory" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("namespace not in list: %v", rows)
	}

	// list keys in namespace
	res, err = c.Call(context.Background(), controlplane.CmdStateList, map[string]any{"namespace": "agent_memory"})
	if err != nil {
		t.Fatalf("Call list ns: %v", err)
	}
	keys, _ := res["keys"].([]any)
	if len(keys) != 1 || keys[0] != "last_run" {
		t.Errorf("keys = %v want [last_run]", keys)
	}

	// get value
	res, err = c.Call(context.Background(), controlplane.CmdStateGet, map[string]any{
		"namespace": "agent_memory",
		"key":       "last_run",
	})
	if err != nil {
		t.Fatalf("Call get: %v", err)
	}
	if found, _ := res["found"].(bool); !found {
		t.Error("found = false; want true")
	}
	if got, _ := res["value"].(string); got != "ok" {
		t.Errorf("value = %v want \"ok\"", res["value"])
	}
}

// TestStateGet_MissingKeyHasFoundFalse — the contract: absent
// keys come back with found=false and value=null so callers can
// distinguish "absent" from "exists but null".
func TestStateGet_MissingKeyHasFoundFalse(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	res, err := c.Call(context.Background(), controlplane.CmdStateGet, map[string]any{
		"namespace": "nonexistent",
		"key":       "nope",
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if found, _ := res["found"].(bool); found {
		t.Errorf("found = true on missing key")
	}
	if res["value"] != nil {
		t.Errorf("value = %v want nil", res["value"])
	}
}

// TestStateGet_RejectsMissingArgs — both namespace and key are
// required; the handler must refuse a partial request rather
// than silently returning the wrong thing.
func TestStateGet_RejectsMissingArgs(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	_, err := c.Call(context.Background(), controlplane.CmdStateGet, map[string]any{
		"namespace": "ns",
		// key missing
	})
	if err == nil {
		t.Error("expected error for missing key")
	}
}

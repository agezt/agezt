// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestConfigCenter_SecretValueMaskedOverAPI verifies that secret-rated config
// values are never returned in cleartext over the control-plane API: the set
// echo, single get, and list all reduce the value to a masked fingerprint and
// flag it "masked". Guards the V-013 fix (secrets must not be readable off the
// /api/configcenter/* response body).
func TestConfigCenter_SecretValueMaskedOverAPI(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	const secret = "sk-supersecretvalue-0123456789"
	want := creds.MaskValue(secret)
	if want == secret || strings.Contains(want, "supersecret") {
		t.Fatalf("MaskValue did not mask the sensitive middle: %q", want)
	}

	assertMasked := func(t *testing.T, where string, entry map[string]any) {
		t.Helper()
		if entry == nil {
			t.Fatalf("%s: nil entry", where)
		}
		if got := entry["value"]; got != want {
			t.Fatalf("%s: value = %q, want masked %q (raw secret must not leak)", where, got, want)
		}
		if v, _ := entry["value"].(string); strings.Contains(v, "supersecret") {
			t.Fatalf("%s: raw secret leaked in value %q", where, v)
		}
		if entry["masked"] != true {
			t.Fatalf("%s: masked flag = %v, want true", where, entry["masked"])
		}
		if entry["rating"] != "secret" {
			t.Fatalf("%s: rating = %v, want secret", where, entry["rating"])
		}
	}

	// Set echo.
	res, err := c.Call(ctx, controlplane.CmdConfigCenterSet, map[string]any{
		"key":    "service/api/key",
		"value":  secret,
		"rating": "secret",
	})
	if err != nil {
		t.Fatalf("configcenter set: %v", err)
	}
	setEntry, _ := res["entry"].(map[string]any)
	assertMasked(t, "set", setEntry)

	// List.
	list, err := c.Call(ctx, controlplane.CmdConfigCenterList, nil)
	if err != nil {
		t.Fatalf("configcenter list: %v", err)
	}
	rows, _ := list["entries"].([]any)
	if len(rows) != 1 {
		t.Fatalf("entries len = %d, want 1", len(rows))
	}
	row, _ := rows[0].(map[string]any)
	assertMasked(t, "list", row)

	// Single get over the API is masked too.
	getRes, err := c.Call(ctx, controlplane.CmdConfigCenterGet, map[string]any{"key": "service/api/key"})
	if err != nil {
		t.Fatalf("configcenter get: %v", err)
	}
	getEntry, _ := getRes["entry"].(map[string]any)
	assertMasked(t, "get", getEntry)

	// Masking is display-only: the real value is still intact in storage and
	// retrievable through the access-controlled kernel Get path (not the API echo).
	got, err := k.ConfigCenter().Get(ctx, configcenter.ConfigAccessRequest{
		AgentID: "ops",
		Key:     "service/api/key",
		Reason:  "verify storage intact",
	})
	if err == nil && got != secret {
		t.Fatalf("stored secret corrupted by masking: got %q, want %q", got, secret)
	}
}

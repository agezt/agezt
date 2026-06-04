// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestArtifactGet_RoundTrip stores a blob in the kernel's artifact store, then
// fetches it back over the control plane by ref (M391, SPEC-04 §3.6).
func TestArtifactGet_RoundTrip(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	original := []byte(strings.Repeat("offloaded output ", 1000))
	ref, err := k.Artifacts().Put(original)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	res, err := c.Call(context.Background(), controlplane.CmdArtifactGet, map[string]any{"ref": ref})
	if err != nil {
		t.Fatalf("artifact_get: %v", err)
	}
	if got, _ := res["ref"].(string); got != ref {
		t.Errorf("ref echoed %q, want %q", got, ref)
	}
	if sz, _ := res["size"].(float64); int(sz) != len(original) {
		t.Errorf("size = %v, want %d", res["size"], len(original))
	}
	enc, _ := res["data"].(string)
	data, derr := base64.StdEncoding.DecodeString(enc)
	if derr != nil {
		t.Fatalf("decode data: %v", derr)
	}
	if string(data) != string(original) {
		t.Errorf("fetched bytes != stored bytes (%d vs %d)", len(data), len(original))
	}
}

// TestArtifactGet_Errors covers the operator-facing error paths.
func TestArtifactGet_Errors(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// Missing ref → an error mentioning ref.
	if _, err := c.Call(context.Background(), controlplane.CmdArtifactGet, map[string]any{}); err == nil ||
		!strings.Contains(err.Error(), "ref") {
		t.Errorf("missing ref: err = %v, want a ref-required error", err)
	}
	// Malformed ref (not 64-hex) → a malformed-ref error, never a panic.
	if _, err := c.Call(context.Background(), controlplane.CmdArtifactGet, map[string]any{"ref": "../escape"}); err == nil ||
		!strings.Contains(err.Error(), "malformed") {
		t.Errorf("malformed ref: err = %v, want a malformed-ref error", err)
	}
	// Well-formed but absent ref → not found.
	absent := strings.Repeat("a", 64)
	if _, err := c.Call(context.Background(), controlplane.CmdArtifactGet, map[string]any{"ref": absent}); err == nil ||
		!strings.Contains(err.Error(), "not found") {
		t.Errorf("absent ref: err = %v, want a not-found error", err)
	}
}

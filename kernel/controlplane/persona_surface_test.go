// SPDX-License-Identifier: MIT
package controlplane

import (
	"github.com/agezt/agezt/kernel/app/binding"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The persona SURFACE is the daemon's own — it is PRIMARY only — and it edits
// the daemon's default identity LIVE.
//
// The test drives the surface the way the operator does, over the socket, so
// the whole path is held: the adapter's wiring, the persona family's port and
// the kernel's live value.
func TestThePersonaSurfaceEditsTheDaemonsIdentity(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	k.SetSystem("before")

	server := NewServer(binding.ScopeFromKernel(k, dir))
	server.token = "primary"

	response := callAppHost(t, server, Request{ID: "persona-set", Cmd: CmdPersonaSet, Token: "primary", Args: map[string]any{"system": "after"}})[0]
	if response.Type != RespResult {
		t.Fatalf("the persona surface refused the change: %+v", response)
	}
	if got := k.System(); got != "after" {
		t.Fatalf("the kernel holds the prompt %q, want the one the persona surface set", got)
	}
}

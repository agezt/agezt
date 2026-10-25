// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"io"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolforgeapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// fakeRunner is the script runner the daemon installs as the Verifier's
// executor.
type fakeRunner struct{}

func (fakeRunner) RunScript(_ context.Context, _, _, _ string) (string, bool, error) {
	return "out", false, nil
}

// TestForgeRunnerPortStaysANilPortWithoutAKernel: a daemon with no kernel must
// hand over a NIL port, not a port wrapping a nil implementation. The check
// is on the concrete type on purpose — a nil *runtime.Kernel handed to an
// interface parameter is already a non-nil interface, so the tool_forge tool
// would bind a runner that is not there.
func TestForgeRunnerPortStaysANilPortWithoutAKernel(t *testing.T) {
	if got := forgeRunnerPort(nil); got != nil {
		t.Fatalf("a missing kernel must yield a nil port, got %v", got)
	}
}

// TestTheDaemonSuppliesTheForgeRunner: the tool_forge tool drives the forge's
// lifecycle through the CONTRACT's port (W3.39g) — the registry's row is no
// longer the live kernel, because a plugin may not name a module's type.
//
// The port must carry the LIVE implementation: a draft made through it has
// to show up in the daemon's own script-tool store. A port that handed over
// a copy — or nothing — would leave the agent authoring tools into a void,
// and every test that built the tool by hand would stay green.
func TestTheDaemonSuppliesTheForgeRunner(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	deps := toolKernelDeps(k, t.TempDir(), io.Discard)
	if deps.ForgeRunner == nil {
		t.Fatal("the daemon must supply the forge runner")
	}
	if deps.Forge == nil {
		t.Fatal("the daemon must supply the forge store")
	}

	drafted, err := deps.ForgeRunner.DraftScriptTool("corr-1", toolforgeapi.ScriptTool{
		Name:        "echo",
		Description: "echoes its input",
		Language:    "python",
		Code:        "print(1)",
	})
	if err != nil {
		t.Fatalf("DraftScriptTool through the port: %v", err)
	}
	if drafted.ID == "" {
		t.Fatal("the port must reach the live forge — the drafted tool has no id")
	}

	all := deps.Forge.List()
	if len(all) != 1 {
		t.Fatalf("the store must hold the drafted tool, got %d", len(all))
	}
	if all[0].ID != drafted.ID {
		t.Fatalf("the store holds %q, the port drafted %q", all[0].ID, drafted.ID)
	}
}

// TestTheDaemonSuppliesTheConductorExecRow: the code_exec tool installs
// itself as the Verifier's executor through the registry's row (W3.39g) — a
// plain closure, because a function value names no type.
//
// The effect is observable: the kernel reports that it can execute code
// only after the row has been called. A daemon that dropped the row would
// leave the Verifier always critiquing — on every live box — while every
// test that built the tool by hand stayed green.
func TestTheDaemonSuppliesTheConductorExecRow(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	deps := toolKernelDeps(k, t.TempDir(), io.Discard)
	if deps.ConductorExec == nil {
		t.Fatal("the daemon must supply the conductor-exec row")
	}
	if k.ReasoningDependencies().CodeExecAvailable() {
		t.Fatal("no executor may be installed before the row is called")
	}

	deps.ConductorExec(fakeRunner{})
	if !k.ReasoningDependencies().CodeExecAvailable() {
		t.Fatal("the row must install the executor on the live kernel")
	}
}

// TestTheDaemonResolvesArtifactBytes: the send_media tool resolves an
// artifact ref through the registry's row (W3.39g) — a plain closure the
// daemon builds over the live blob store.
//
// The bytes must come back VERBATIM: the blob store re-verifies the hash on
// the way out, so a row that handed over something else would attach the
// wrong file to the operator's chat — and a row that dropped the check would
// attach a tampered one.
func TestTheDaemonResolvesArtifactBytes(t *testing.T) {
	k, err := runtime.Open(withArtifacts(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	store := k.Artifacts()
	if store == nil {
		t.Fatal("the kernel must own an artifact store")
	}
	ref, err := store.Put([]byte("payload"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	data, err := artifactBytes(k)(ref)
	if err != nil {
		t.Fatalf("resolve through the row: %v", err)
	}
	if string(data) != "payload" {
		t.Fatalf("the row must hand back the stored bytes, got %q", data)
	}
}

// TestTheArtifactBytesRowSurvivesAMissingKernel: the closure must not
// dereference a nil kernel, and must say the store is unavailable.
func TestTheArtifactBytesRowSurvivesAMissingKernel(t *testing.T) {
	_, err := artifactBytes(nil)("ref-1")
	if err == nil {
		t.Fatal("a missing kernel must report that the store is unavailable")
	}
}

// TestTheConductorExecRowSurvivesAMissingKernel: the closure must not
// dereference a nil kernel.
func TestTheConductorExecRowSurvivesAMissingKernel(t *testing.T) {
	row := conductorExecPort(nil)
	if row == nil {
		t.Fatal("the row must exist even without a kernel")
	}
	row(fakeRunner{}) // must not panic
}

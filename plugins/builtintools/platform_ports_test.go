// SPDX-License-Identifier: MIT

package builtintools

import (
	"io"
	"testing"

	"github.com/agezt/agezt/kernel/contract/wardenapi"
	"github.com/agezt/agezt/kernel/toolreg"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/plugins/tools/shell"
)

// TestTheShellSpecCarriesTheDaemonsWardenThroughThePort: the warden reaches
// the shell tool through the CONTRACT's port.
//
// The tool is a plugin, and a plugin may not name the platform package that
// keeps the engine — so the registry's row is the contract's port and the
// daemon hands the live engine over through it. The spec must pass that
// engine on UNCHANGED: a spec that built its own engine would run every
// command outside the daemon's isolation profile, and — worse — with no audit
// event, because the daemon's bus is what the engine reports to.
func TestTheShellSpecCarriesTheDaemonsWardenThroughThePort(t *testing.T) {
	eng := warden.New(nil)

	built, err := specShell().Build(toolreg.BuildDeps{
		BaseDir:       t.TempDir(),
		WorkspaceRoot: t.TempDir(),
		Warden:        eng,
		Stderr:        io.Discard,
		Get:           func(string) string { return "" },
	})
	if err != nil {
		t.Fatalf("Build the shell spec: %v", err)
	}
	sh, ok := built.Tool.(*shell.Tool)
	if !ok {
		t.Fatalf("the shell spec built %T, want the shell tool", built.Tool)
	}
	// The comparison is on the CONVERTED value: the tool's field is the
	// contract's port, the daemon's engine is the platform's — two different
	// interface types, one engine.
	if sh.Warden != wardenapi.Engine(eng) {
		t.Fatalf("the shell tool must carry the daemon's warden engine, got %v", sh.Warden)
	}
}

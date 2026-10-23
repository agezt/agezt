// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/introspecttool"
)

// TestToolKernelDepsSuppliesTheIntrospectPort: the daemon hands the
// introspect tool its live state through the contract's port. The port is
// supplied HERE — at the composition root — so a daemon that dropped it
// would leave the tool answering "not available" on every live box. This
// test is the only one that sees that wiring, so it asserts it directly.
func TestToolKernelDepsSuppliesTheIntrospectPort(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(),
		Tools:    map[string]toolapi.Tool{"probe": nil},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	deps := toolKernelDeps(k, t.TempDir(), io.Discard)
	if deps.Introspect == nil {
		t.Fatal("the daemon must supply the introspect port")
	}

	// The port must answer the LIVE kernel — not an empty shell.
	ov := deps.Introspect.Overview()
	if ov.Daemon == "" {
		t.Fatalf("the port must name the daemon: %+v", ov)
	}
	if len(ov.Tools) == 0 {
		t.Fatalf("the port must list the live kernel's tools: %+v", ov)
	}
}

// TestToolKernelDepsSuppliesTheOverseerPort: the same wiring duty for the
// overseer port — the daemon supplies it, and it answers the live kernel.
func TestToolKernelDepsSuppliesTheOverseerPort(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	deps := toolKernelDeps(k, t.TempDir(), io.Discard)
	if deps.Overseer == nil {
		t.Fatal("the daemon must supply the overseer port")
	}
	// The port must answer the live kernel — a port bound to nothing would
	// still be non-nil.
	if deps.Overseer.IsHalted() {
		t.Error("a freshly opened daemon is not halted")
	}
	if got := deps.Overseer.ActiveRunIDs(); len(got) != 0 {
		t.Errorf("a freshly opened daemon has no active runs, got %v", got)
	}
}

// TestTheIntrospectToolAnswersOnAConfiguredDaemon: the whole path — the
// composition root's port, the plugin's Bind, the tool's Invoke — has to
// work together, or the agent gets a daemon that cannot describe itself.
func TestTheIntrospectToolAnswersOnAConfiguredDaemon(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(),
		Tools:    map[string]toolapi.Tool{"probe": nil},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	deps := toolKernelDeps(k, t.TempDir(), io.Discard)
	tool := introspecttool.New()
	tool.Bind(deps.Introspect)

	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"overview"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("the introspect tool errored on a configured daemon: %s", res.Output)
	}
	if !strings.Contains(res.Output, `"daemon"`) {
		t.Fatalf("the overview must carry the daemon row: %s", res.Output)
	}
}

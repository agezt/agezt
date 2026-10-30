// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"fmt"
	"github.com/agezt/agezt/kernel/app/binding"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolboxapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// toolboxUnavailableMessage is the contract's answer, pinned as a LITERAL: a
// sentinel comparison would move with the message and prove nothing.
const toolboxUnavailableMessage = "toolbox is not available on this daemon"

// toolboxFakeHost is the contract's port with a fixed answer: the operations
// must talk to THIS host — not to the machine the test happens to run on.
type toolboxFakeHost struct {
	inventory toolboxapi.Inventory
	outdated  map[string]bool
	results   map[string]toolboxapi.InstallResult
	installs  []string
	detects   int
}

func (h *toolboxFakeHost) Detect(context.Context) toolboxapi.Inventory {
	h.detects++
	return h.inventory
}

func (h *toolboxFakeHost) Outdated(context.Context) map[string]bool { return h.outdated }

func (h *toolboxFakeHost) Install(_ context.Context, name string) toolboxapi.InstallResult {
	h.installs = append(h.installs, name)
	return h.results[name]
}

// TestToolboxOperationsWithoutAPortSaySoAndJournalNothing: a daemon with no
// toolbox answers "the toolbox is not available on this daemon" — it does not
// answer with an empty inventory, and it does not journal an install it never
// ran.
func TestToolboxOperationsWithoutAPortSaySoAndJournalNothing(t *testing.T) {
	k, s, _, _ := pulseAppFixture(t) // a bare server: no toolbox port
	head, _ := k.Journal().Head()

	for _, cmd := range []string{CmdToolboxDetect, CmdToolboxOutdated} {
		frames := callAppHost(t, s, Request{ID: "owned", Cmd: cmd, Token: "primary"})
		last := frames[len(frames)-1]
		if last.Type != RespError || !strings.Contains(last.Error, toolboxUnavailableMessage) {
			t.Fatalf("EXPECTED:%s says the toolbox is unavailable ACTUAL:%+v", cmd, last)
		}
		if last.Result != nil {
			t.Fatalf("EXPECTED:%s answers no snapshot ACTUAL:%+v", cmd, last.Result)
		}
	}

	frames := callAppHost(t, s, Request{ID: "owned", Cmd: CmdToolboxInstall, Token: "primary", Args: map[string]any{"names": []any{"owned-tool"}}})
	last := frames[len(frames)-1]
	if last.Type != RespError || !strings.Contains(last.Error, toolboxUnavailableMessage) {
		t.Fatalf("EXPECTED:the install refuses too ACTUAL:%+v", last)
	}

	rows, err := k.Journal().Tail(100)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range rows {
		if e.Seq <= head {
			continue
		}
		if e.Kind == event.KindToolboxInstallRequested || e.Kind == event.KindToolboxInstalled {
			t.Fatalf("EXPECTED:nothing was requested ACTUAL:%v", e)
		}
	}
}

// TestToolboxOperationsReachTheContractPort: with the port wired, the
// operations answer with the host's rows — the machine is never consulted.
func TestToolboxOperationsReachTheContractPort(t *testing.T) {
	_, s, _, _ := pulseAppFixture(t)
	host := &toolboxFakeHost{
		inventory: toolboxapi.Inventory{
			OS:             "owned-os",
			Managers:       []string{"owned-manager"},
			Tools:          []toolboxapi.ToolStatus{{Name: "owned-tool", Installed: true, Version: "owned-version"}},
			InstalledCount: 1,
		},
		outdated: map[string]bool{"owned-tool": true},
		results:  map[string]toolboxapi.InstallResult{"owned-tool": {Tool: "owned-tool", OK: true, Version: "owned-version"}},
	}
	s.SetToolbox(host)

	frames := callAppHost(t, s, Request{ID: "owned", Cmd: CmdToolboxDetect, Token: "primary"})
	last := frames[len(frames)-1]
	if last.Type != RespResult || last.Result["os"] != "owned-os" {
		t.Fatalf("EXPECTED:the host's snapshot ACTUAL:%+v", last)
	}
	rows, _ := last.Result["tools"].([]any)
	if len(rows) != 1 {
		t.Fatalf("EXPECTED:the host's tool row ACTUAL:%+v", last.Result)
	}
	if row, _ := rows[0].(map[string]any); row["name"] != "owned-tool" || row["version"] != "owned-version" {
		t.Fatalf("EXPECTED:the host's tool row ACTUAL:%+v", row)
	}

	frames = callAppHost(t, s, Request{ID: "owned", Cmd: CmdToolboxOutdated, Token: "primary"})
	last = frames[len(frames)-1]
	if last.Type != RespResult || fmt.Sprint(last.Result["count"]) != "1" {
		t.Fatalf("EXPECTED:the host's outdated set ACTUAL:%+v", last)
	}

	frames = callAppHost(t, s, Request{ID: "owned", Cmd: CmdToolboxInstall, Token: "primary", Args: map[string]any{"names": []any{"owned-tool"}}})
	last = frames[len(frames)-1]
	if last.Type != RespResult {
		t.Fatalf("EXPECTED:the install summary ACTUAL:%+v", last)
	}
	if len(host.installs) != 1 || host.installs[0] != "owned-tool" {
		t.Fatalf("EXPECTED:the install reached the host ACTUAL:%v", host.installs)
	}
	installed, _ := last.Result["installed"].([]any)
	if len(installed) != 1 || installed[0] != "owned-tool" {
		t.Fatalf("EXPECTED:the host's outcome ACTUAL:%+v", last.Result)
	}
	if host.detects != 1 {
		t.Fatalf("EXPECTED:one detection ACTUAL:%d", host.detects)
	}
}

// TestToolboxDepsCarryTheContractPort: the daemon hands the port over in its
// deps and the operations answer with THAT port's rows — the wiring is
// proven at the daemon's door, not only through a setter.
func TestToolboxDepsCarryTheContractPort(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	host := &toolboxFakeHost{inventory: toolboxapi.Inventory{OS: "owned-deps-os"}}

	s := NewServerWithDeps(binding.ScopeFromKernel(k, dir), Deps{Toolbox: host})
	s.token = "primary"

	frames := callAppHost(t, s, Request{ID: "owned", Cmd: CmdToolboxDetect, Token: "primary"})
	last := frames[len(frames)-1]
	if last.Type != RespResult || last.Result["os"] != "owned-deps-os" {
		t.Fatalf("EXPECTED:the daemon's toolbox port answers ACTUAL:%+v", last)
	}
}

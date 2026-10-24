// SPDX-License-Identifier: MIT

package toolregapi

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/channelapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/types"
)

// TestTheRegistryRowsAreTheContractsRows pins the rows the registry carries
// to the CONTRACT's types.
//
// The registry moved down into this package in W3.39h — the built-in tools
// are a plugin, and a plugin may name the contract layer and nothing else —
// so every row the registry hands a tool has to BE the contract's row, not a
// copy with the same shape. A row that drifted back to a module's type would
// compile here and quietly buy back the reach the move paid for.
//
// The check is on the TYPE, not on an assignment: an assignment would accept
// a copy, while two named types with the same shape are still two types.
func TestTheRegistryRowsAreTheContractsRows(t *testing.T) {
	// A row may not grow a field: every field is a row somebody hands a tool,
	// and a field nobody names is a reach waiting to happen.
	if n := reflect.TypeOf(Spec{}).NumField(); n != 7 {
		t.Errorf("Spec has %d fields, want 7 — a row may not grow a field", n)
	}
	if n := reflect.TypeOf(Built{}).NumField(); n != 6 {
		t.Errorf("Built has %d fields, want 6 — a row may not grow a field", n)
	}

	// The pre-Open hook answers the contract's port.
	specType := reflect.TypeOf(Spec{})
	preOpen, ok := specType.FieldByName("PreOpen")
	if !ok {
		t.Fatal("Spec has no PreOpen field")
	}
	if preOpen.Type.Kind() != reflect.Func {
		t.Fatalf("Spec.PreOpen must be a function, got %v", preOpen.Type.Kind())
	}
	if preOpen.Type.NumIn() != 2 {
		t.Fatalf("Spec.PreOpen must take (tool, port), got %v", preOpen.Type)
	}
	if preOpen.Type.In(0) != reflect.TypeOf((*toolapi.Tool)(nil)).Elem() {
		t.Errorf("Spec.PreOpen must take the contract's tool, got %v", preOpen.Type.In(0))
	}
	if preOpen.Type.In(1) != reflect.TypeOf((*Config)(nil)).Elem() {
		t.Errorf("Spec.PreOpen must take the contract's Config port, got %v", preOpen.Type.In(1))
	}

	// The plugin manifest entries are the contract's row.
	builtType := reflect.TypeOf(Built{})
	infos, ok := builtType.FieldByName("Infos")
	if !ok {
		t.Fatal("Built has no Infos field")
	}
	if infos.Type != reflect.TypeOf([]types.PluginInfo(nil)) {
		t.Errorf("Built.Infos must be []types.PluginInfo, got %v", infos.Type)
	}

	// The media attachment is the contract's row — the channel package keeps
	// it as an alias, so naming the contract names the same type.
	lateType := reflect.TypeOf(LateDeps{})
	sendMedia, ok := lateType.FieldByName("ChannelSendMedia")
	if !ok {
		t.Fatal("LateDeps has no ChannelSendMedia field")
	}
	if sendMedia.Type.Kind() != reflect.Func {
		t.Fatalf("LateDeps.ChannelSendMedia must be a function, got %v", sendMedia.Type.Kind())
	}
	// (ctx, kind, id, text, atts)
	if sendMedia.Type.NumIn() != 5 {
		t.Fatalf("LateDeps.ChannelSendMedia must take five arguments, got %v", sendMedia.Type)
	}
	wantAtts := reflect.TypeOf([]channelapi.Attachment(nil))
	if sendMedia.Type.In(4) != wantAtts {
		t.Errorf("LateDeps.ChannelSendMedia must take []channelapi.Attachment, got %v", sendMedia.Type.In(4))
	}
}

// TestApplyPreOpenHandsThePortToTheHook: the phase must hand the hook the
// port it was given — not a wrapper of its own.
//
// This is what the move changed. The registry used to know the daemon's
// configuration type and wrapped it in an adapter of its own; now it lives in
// the contract layer and may not name that type at all, so the composition
// root wraps the configuration in the port and hands THAT over. If the
// registry wrapped it again, the installation would land in a copy and the
// script-tool forge would keep reporting itself unavailable — a failure with
// no error anywhere.
func TestApplyPreOpenHandsThePortToTheHook(t *testing.T) {
	resetRegistryForTest()
	var got Config
	Register(Spec{
		Name:    "hooked",
		Build:   func(BuildDeps) (Built, error) { return Built{Tool: &fakeTool{name: "hooked"}}, nil },
		PreOpen: func(tool toolapi.Tool, cfg Config) { got = cfg },
	})
	set, err := BuildAll(BuildDeps{})
	if err != nil {
		t.Fatalf("BuildAll: %v", err)
	}
	port := &recordingConfig{}
	set.ApplyPreOpen(port)
	if got != port {
		t.Fatalf("the hook received %v, want the very port the phase was given", got)
	}
}

// recordingConfig is a fake of the pre-Open port: it remembers that it was
// asked to install a runner.
type recordingConfig struct {
	runner ScriptRunner
}

// SetScriptRunner records the runner.
func (c *recordingConfig) SetScriptRunner(r ScriptRunner) { c.runner = r }

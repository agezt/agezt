// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/modules/work/seat"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestSeatsEditThePrimaryStore drives the typed seat operations through the
// native adapter against the primary kernel's seat store.
//
// The store is opened HERE and handed to the kernel as the contract's port:
// the catalog's file is the composition root's business, so the host speaks
// the port instead of the kernel opening a module's implementation.
func TestSeatsEditThePrimaryStore(t *testing.T) {
	base := t.TempDir()
	seats, err := seat.OpenStore(filepath.Join(base, "seats"))
	if err != nil {
		t.Fatal(err)
	}
	k, err := runtime.Open(runtime.Config{BaseDir: base, Provider: mock.New(), Seats: seats})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, t.TempDir())
	s.token = "primary"
	call := func(cmd string, args map[string]any) (string, string) {
		t.Helper()
		resp := callAppHost(t, s, Request{ID: "s", Cmd: cmd, Token: "primary", Args: args})[0]
		raw, _ := json.Marshal(resp.Result)
		return string(raw), resp.Error
	}
	before := len(k.Seats().List())
	if out, msg := call(CmdSeatCreate, map[string]any{"id": "ops", "tools": []any{"shell"}}); msg != "" || !strings.Contains(out, `"restrict_tools":true`) {
		t.Fatal(out, msg)
	}
	if len(k.Seats().List()) != before+1 {
		t.Fatal("the seat reached the primary store")
	}
	if out, msg := call(CmdSeatList, nil); msg != "" || !strings.Contains(out, `"id":"ops"`) {
		t.Fatal(out, msg)
	}
	if out, msg := call(CmdSeatDelete, map[string]any{"id": "ops"}); msg != "" || out != `{"deleted":"ops"}` {
		t.Fatal(out, msg)
	}
	if len(k.Seats().List()) != before {
		t.Fatal("the delete reached the primary store")
	}
	for cmd, read := range map[string]bool{CmdSeatList: true, CmdSeatCreate: false, CmdSeatDelete: false} {
		if wire, exists := commandRegistry[cmd]; !exists || !wire.AppOwned || wire.ReadOnly != read || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
			t.Fatalf("%s native wire %+v", cmd, wire)
		}
	}
}

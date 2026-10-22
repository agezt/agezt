// SPDX-License-Identifier: MIT

package types

import (
	"encoding/json"
	"testing"
)

// TestCouncilMemberWireShape: the seat's wire shape is the contract's
// promise. The application's council row is this type, so the keys are fixed
// here — the operator's request and the operator's answer use the same names.
func TestCouncilMemberWireShape(t *testing.T) {
	raw, err := json.Marshal(CouncilMember{Seat: "Chair", Model: "m1"})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"seat":"Chair","model":"m1"}` {
		t.Fatal(string(raw))
	}
	var back CouncilMember
	if err := json.Unmarshal([]byte(`{"seat":"Chair","model":"m1"}`), &back); err != nil {
		t.Fatal(err)
	}
	if back != (CouncilMember{Seat: "Chair", Model: "m1"}) {
		t.Fatal(back)
	}
	var empty CouncilMember
	if err := json.Unmarshal([]byte(`{}`), &empty); err != nil {
		t.Fatal(err)
	}
	if empty != (CouncilMember{}) {
		t.Fatal(empty)
	}
}

// TestPluginInfoManifest: the manifest carries the six fields an operator
// needs to answer "is my plugin loaded and serving the tools I expected?".
func TestPluginInfoManifest(t *testing.T) {
	info := PluginInfo{Prefix: "px", Path: "/bin/px", Args: []string{"--flag"}, ToolCount: 2, HashPinned: true, AllowedTools: []string{"px.read"}}
	if info.Prefix != "px" || info.Path != "/bin/px" || len(info.Args) != 1 || info.Args[0] != "--flag" || info.ToolCount != 2 || !info.HashPinned || len(info.AllowedTools) != 1 || info.AllowedTools[0] != "px.read" {
		t.Fatal(info)
	}
	var zero PluginInfo
	if zero.Prefix != "" || zero.Path != "" || zero.Args != nil || zero.ToolCount != 0 || zero.HashPinned || zero.AllowedTools != nil {
		t.Fatal("the empty manifest is empty", zero)
	}
}

// TestSubAgentLimitsCeilings: the delegation ceilings (M46–M48) are read-only
// values; zero means unbounded for the fan-out and the spend.
func TestSubAgentLimitsCeilings(t *testing.T) {
	limits := SubAgentLimits{Enabled: true, MaxDepth: 2, MaxFanout: 3, MaxSpendMicrocents: 1500, MaxTotal: 4}
	if !limits.Enabled || limits.MaxDepth != 2 || limits.MaxFanout != 3 || limits.MaxSpendMicrocents != 1500 || limits.MaxTotal != 4 {
		t.Fatal(limits)
	}
	var zero SubAgentLimits
	if zero.Enabled || zero.MaxDepth != 0 || zero.MaxFanout != 0 || zero.MaxSpendMicrocents != 0 || zero.MaxTotal != 0 {
		t.Fatal("the unset ceilings are zero", zero)
	}
}

// TestSubAgentLimitsWireShape: the delegation ceilings are a wire row — the
// status operation and the introspection tool publish them under these names.
func TestSubAgentLimitsWireShape(t *testing.T) {
	raw, err := json.Marshal(SubAgentLimits{Enabled: true, MaxDepth: 2, MaxFanout: 3, MaxSpendMicrocents: 1500, MaxTotal: 4})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"enabled":true,"max_depth":2,"max_fanout":3,"max_spend_microcents":1500,"max_total":4}` {
		t.Fatal(string(raw))
	}
	var back SubAgentLimits
	if err := json.Unmarshal([]byte(`{"enabled":true,"max_depth":2,"max_fanout":3,"max_spend_microcents":1500,"max_total":4}`), &back); err != nil {
		t.Fatal(err)
	}
	if back != (SubAgentLimits{Enabled: true, MaxDepth: 2, MaxFanout: 3, MaxSpendMicrocents: 1500, MaxTotal: 4}) {
		t.Fatal(back)
	}
}

// TestOpinionWireShape: one member's position in one round. The error is
// omitted when the seat's call did not fail.
func TestOpinionWireShape(t *testing.T) {
	raw, err := json.Marshal(Opinion{Seat: "Chair", Model: "m1", Round: 1, Text: "yes"})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"seat":"Chair","model":"m1","round":1,"text":"yes"}` {
		t.Fatal(string(raw))
	}
	raw, err = json.Marshal(Opinion{Seat: "Scribe", Model: "m2", Round: 1, Error: "provider failed"})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"seat":"Scribe","model":"m2","round":1,"text":"","error":"provider failed"}` {
		t.Fatal(string(raw))
	}
	var back Opinion
	if err := json.Unmarshal([]byte(`{"seat":"Chair","model":"m1","round":1,"text":"yes"}`), &back); err != nil {
		t.Fatal(err)
	}
	if back != (Opinion{Seat: "Chair", Model: "m1", Round: 1, Text: "yes"}) {
		t.Fatal(back)
	}
}

// TestTenantInfoWireShape: the tenant listing snapshot. The registry owns the
// row and the tenant operation publishes it under these names.
func TestTenantInfoWireShape(t *testing.T) {
	raw, err := json.Marshal(TenantInfo{ID: "acme", BaseDir: "/tenants/acme", Open: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"id":"acme","base_dir":"/tenants/acme","open":true}` {
		t.Fatal(string(raw))
	}
	var back TenantInfo
	if err := json.Unmarshal([]byte(`{"id":"acme","base_dir":"/tenants/acme","open":true}`), &back); err != nil {
		t.Fatal(err)
	}
	if back != (TenantInfo{ID: "acme", BaseDir: "/tenants/acme", Open: true}) {
		t.Fatal(back)
	}
}

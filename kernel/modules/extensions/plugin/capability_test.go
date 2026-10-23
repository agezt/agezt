// SPDX-License-Identifier: MIT

package plugin

// White-box test for the declared-capability manifest surface (M900).

import "testing"

func TestToolCapabilities_PrefixedDeclaredOnly(t *testing.T) {
	p := &Plugin{tools: []ToolDef{
		{Name: "post", Capability: "http.post"},
		{Name: "fetch"}, // no declaration → absent from the map
		{Name: "wipe", Capability: "file.delete"},
	}}
	got := p.ToolCapabilities("myplug.")
	if len(got) != 2 {
		t.Fatalf("ToolCapabilities = %v, want 2 declared entries", got)
	}
	if got["myplug.post"] != "http.post" {
		t.Errorf("myplug.post = %q, want http.post", got["myplug.post"])
	}
	if got["myplug.wipe"] != "file.delete" {
		t.Errorf("myplug.wipe = %q, want file.delete", got["myplug.wipe"])
	}
	if _, present := got["myplug.fetch"]; present {
		t.Error("undeclared tool must not appear in the capability map")
	}
}

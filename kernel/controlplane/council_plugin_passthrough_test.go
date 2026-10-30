// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"github.com/agezt/agezt/kernel/app/binding"
	"reflect"
	"testing"

	appplugins "github.com/agezt/agezt/kernel/app/plugins"
	"github.com/agezt/agezt/kernel/contract/types"
	"github.com/agezt/agezt/kernel/modules/providers/catalog"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestCouncilAndPluginRowsPassThroughTheHost: the host hands its membership and
// its manifest to the application as the contract types themselves. There is no
// field-by-field projection left in between, so a field cannot be dropped,
// swapped or invented on the way — and the listing must not disturb the
// kernel's own configuration.
func TestCouncilAndPluginRowsPassThroughTheHost(t *testing.T) {
	cat := &catalog.Catalog{Providers: map[string]*catalog.Provider{"mock": {ID: "mock", Models: map[string]*catalog.Model{"model-a": {ID: "model-a"}, "model-b": {ID: "model-b"}}}}}
	manifest := []runtime.PluginInfo{
		{Prefix: "zz", Path: "/bin/zz", Args: []string{"--zz"}, ToolCount: 2, HashPinned: true, AllowedTools: []string{"zz.read"}},
		{Prefix: "aa", Path: "/bin/aa", Args: []string{"--aa"}, ToolCount: 1},
	}
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Catalog: cat, Plugins: manifest})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(binding.ScopeFromKernel(k, t.TempDir()))
	s.token = "primary"
	call := func(cmd string, args map[string]any) string {
		t.Helper()
		resp := callAppHost(t, s, Request{ID: "p", Cmd: cmd, Token: "primary", Args: args})[0]
		if resp.Type != RespResult {
			t.Fatal(cmd, resp.Error)
		}
		raw, _ := json.Marshal(resp.Result)
		return string(raw)
	}

	// The membership the operator sets reaches the kernel unchanged: the seat
	// stays the seat and the model stays the model.
	if got := call(CmdCouncilSet, map[string]any{"members": []any{
		map[string]any{"seat": "Chair", "model": "model-a"},
		map[string]any{"seat": "Scribe", "model": "model-b"},
	}}); got != `{"applied":"live","member_count":2,"saved":true}` {
		t.Fatal(got)
	}
	want := []types.CouncilMember{{Seat: "Chair", Model: "model-a"}, {Seat: "Scribe", Model: "model-b"}}
	if members := k.CouncilDefaultMembers(); !reflect.DeepEqual(members, want) {
		t.Fatal("the membership changed on the way to the kernel", members)
	}
	if got := call(CmdCouncilMembers, nil); got != `{"count":2,"members":[{"model":"model-a","seat":"Chair"},{"model":"model-b","seat":"Scribe"}]}` {
		t.Fatal(got)
	}

	// The manifest reaches the wire unchanged, in the application's own order.
	var listed appplugins.ListOutput
	if err := json.Unmarshal([]byte(call(CmdPluginList, nil)), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Count != 2 || len(listed.Plugins) != 2 {
		t.Fatal(listed)
	}
	first := listed.Plugins[0]
	if first.Prefix != "aa" || first.Path != "/bin/aa" || !reflect.DeepEqual(first.Args, []string{"--aa"}) || first.ToolCount != 1 || first.HashPinned {
		t.Fatal("the first row is not the manifest's", first)
	}
	second := listed.Plugins[1]
	if second.Prefix != "zz" || second.Path != "/bin/zz" || !reflect.DeepEqual(second.Args, []string{"--zz"}) || second.ToolCount != 2 || !second.HashPinned || !reflect.DeepEqual(second.AllowedTools, []string{"zz.read"}) {
		t.Fatal("the second row is not the manifest's", second)
	}
	// Listing is a read: the kernel's own configuration keeps its order and
	// its contents.
	if live := k.Plugins(); !reflect.DeepEqual(live, manifest) {
		t.Fatal("the listing disturbed the kernel's manifest", live)
	}
}

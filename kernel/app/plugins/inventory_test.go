// SPDX-License-Identifier: MIT
package plugins

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

type inventoryProbe struct {
	rows  []Registration
	calls int
}

func (p *inventoryProbe) Plugins() []Registration { p.calls++; return p.rows }
func TestInventoryTypedEmptyRequiredFieldsNilAndRawValues(t *testing.T) {
	for _, rows := range [][]Registration{nil, {}, {{Prefix: "z", Path: " raw path ö ", Args: []string{" raw arg ", "second"}, ToolCount: -2, HashPinned: true, AllowedTools: []string{" raw-tool ", "second"}}, {Prefix: "a", Args: nil, AllowedTools: nil}, {Prefix: "b", Args: []string{}, AllowedTools: []string{}}}} {
		p := &inventoryProbe{rows: rows}
		out, err := New(p).List(context.Background(), ListInput{})
		if err != nil || out.Plugins == nil || out.Count != len(rows) || p.calls != 1 {
			t.Fatal(out, err, p)
		}
		raw, _ := json.Marshal(out)
		var root map[string]json.RawMessage
		json.Unmarshal(raw, &root)
		if len(root) != 2 {
			t.Fatal(string(raw))
		}
		if len(rows) > 0 {
			result := out.Plugins
			if result[0].Prefix != "a" || result[1].Prefix != "b" || result[2].Prefix != "z" || result[2].Path != " raw path ö " || result[2].ToolCount != -2 || !result[2].HashPinned || !reflect.DeepEqual(result[2].Args, []string{" raw arg ", "second"}) || !reflect.DeepEqual(result[2].AllowedTools, []string{" raw-tool ", "second"}) {
				t.Fatal(result)
			}
			var decoded []map[string]json.RawMessage
			json.Unmarshal(root["plugins"], &decoded)
			for _, row := range decoded {
				if len(row) != 6 {
					t.Fatal(row)
				}
			}
			if string(decoded[0]["args"]) != "[]" || string(decoded[1]["args"]) != "[]" || string(decoded[0]["allowed_tools"]) != "null" || string(decoded[1]["allowed_tools"]) != "[]" || string(decoded[0]["hash_pinned"]) != "false" || string(decoded[0]["tool_count"]) != "0" {
				t.Fatal(string(raw))
			}
			result[2].Args[0] = "changed"
			result[2].AllowedTools[0] = "changed"
			result[2].Prefix = "changed"
			if p.rows[0].Prefix != "z" || p.rows[0].Args[0] != " raw arg " || p.rows[0].AllowedTools[0] != " raw-tool " || p.rows[1].Prefix != "a" {
				t.Fatal("manifest aliased or sorted in place", p.rows)
			}
		}
	}
}
func TestInventoryTypedFreshSnapshotsAndLegacyServiceCanceledRead(t *testing.T) {
	p := &inventoryProbe{rows: []Registration{{Prefix: " first "}}}
	svc := New(p)
	first, err := svc.List(context.Background(), ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	p.rows = []Registration{{Prefix: "second", AllowedTools: []string{"owned"}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	second, err := svc.List(ctx, ListInput{})
	if err != nil || p.calls != 2 || first.Plugins[0].Prefix != " first " || second.Plugins[0].Prefix != "second" {
		t.Fatal(first, second, err, p)
	}
}

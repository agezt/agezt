// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"strings"
	"testing"
)

func TestStripJSONCComments(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "leading line comment",
			in:   "// hello\n{\"a\":1}",
			want: "\n{\"a\":1}",
		},
		{
			name: "trailing line comment",
			in:   "{\"a\":1} // note\n",
			want: "{\"a\":1} \n",
		},
		{
			name: "preserve in string",
			in:   "\"http://x\"",
			want: "\"http://x\"",
		},
		{
			name: "escaped quote then slash",
			in:   "\"a\\\"b\" // c",
			want: "\"a\\\"b\" ",
		},
		{
			name: "two slashes inside string",
			in:   "\"//\"",
			want: "\"//\"",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := stripJSONCComments(c.in)
			if got != c.want {
				t.Errorf("stripJSONCComments(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestExportedName(t *testing.T) {
	cases := map[string]string{
		"plugin_id":        "PluginID",
		"protocol_version": "ProtocolVersion",
		"cost_microcents":  "CostMicrocents",
		"display_name":     "DisplayName",
		"http_route":       "HTTPRoute",
		"api_routes":       "APIRoutes",
		"ulid":             "ULID",
		"id":               "ID",
		"jsonrpc":          "Jsonrpc", // single token, no underscore
		"ts_unix_ms":       "TSUnixMS",
		"inline_b64":       "InlineB64",
	}
	for in, want := range cases {
		if got := exportedName(in); got != want {
			t.Errorf("exportedName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRefTypeName(t *testing.T) {
	cases := map[string]string{
		"#/Event":          "Event",
		"#/UnifiedMessage": "UnifiedMessage",
		"PlainName":        "PlainName",
	}
	for in, want := range cases {
		if got := refTypeName(in); got != want {
			t.Errorf("refTypeName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestEndToEnd runs the generator over the real contract and asserts that the
// output compiles (via go/format) and contains the base types we expect.
// This is the contract-conformance gate for M0: if the contract grows in a
// way the generator can't handle, this test breaks loudly.
func TestEndToEndContract(t *testing.T) {
	out := t.TempDir() + "/types.gen.go"
	if err := run("../../.project/agezt-contract.jsonc", out, "gen"); err != nil {
		t.Fatalf("run: %v", err)
	}
	data, err := readFile(out)
	if err != nil {
		t.Fatalf("read out: %v", err)
	}
	s := string(data)
	// Spot-check that base schemas made it through.
	for _, name := range []string{"Event", "RegisterParams", "Capability", "ToolSchema", "ModelInfo"} {
		if !strings.Contains(s, "type "+name+" ") {
			t.Errorf("generated output missing type %s", name)
		}
	}
	if !strings.Contains(s, "DO NOT EDIT") {
		t.Error("missing DO-NOT-EDIT header")
	}
}

func readFile(p string) ([]byte, error) {
	return os.ReadFile(p)
}

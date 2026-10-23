// SPDX-License-Identifier: MIT

package mcpapi

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// rowKeys reads the row's own wire keys from its tags — so the test is
// checked against the DECLARATION and not against a copy of it that could
// drift with it.
func rowKeys(t *testing.T) map[string]bool {
	t.Helper()
	rt := reflect.TypeOf(Server{})
	keys := map[string]bool{}
	for i := 0; i < rt.NumField(); i++ {
		name := strings.Split(rt.Field(i).Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		keys[name] = true
	}
	return keys
}

// TestTheServerWireShape: the registry PERSISTS these rows, and the control
// plane and the Web UI read them back by key. A renamed key is a silent
// break — the old registry file would lose the field, and the UI would read
// nothing — so the exact key set is pinned here.
//
// The row's keys are the models.dev/registry format; they are read by name.
func TestTheServerWireShape(t *testing.T) {
	full := Server{
		ID:          "srv-1",
		Name:        "github",
		Command:     "npx",
		Args:        []string{"-y", "@modelcontextprotocol/server-github"},
		URL:         "https://example.invalid/mcp",
		Enabled:     true,
		Description: "GitHub",
		Env:         map[string]string{"GITHUB_TOKEN": "secret"},
		Headers:     map[string]string{"Authorization": "Bearer secret"},
		Lazy:        true,
		ToolAllow:   []string{"search_issues"},
		CreatedMS:   1700000000000,
		UpdatedMS:   1700000000001,
	}
	raw, err := json.Marshal(full)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	want := []string{
		"id", "name", "command", "args", "url", "enabled", "description",
		"env", "headers", "lazy", "tool_allow", "created_ms", "updated_ms",
	}
	keys := rowKeys(t)
	for _, k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("the wire lost the key %q: %s", k, raw)
		}
		if !keys[k] {
			t.Errorf("the key %q is not a key of the row", k)
		}
	}
	if len(got) != len(want) {
		t.Errorf("the wire grew a key: %s", raw)
	}
	for k := range got {
		if !keys[k] {
			t.Errorf("the wire carries the key %q, which the row does not declare", k)
		}
	}
	if got["tool_allow"] == nil || got["enabled"] != true {
		t.Fatalf("the wire does not carry the row's own values: %s", raw)
	}
}

// TestTheMinimalWireKeepsTheAlwaysFields: the fields without omitempty stay
// on the wire even when they are empty — the registry's reader relies on
// them, and a key that appears and disappears is a key nobody can rely on.
func TestTheMinimalWireKeepsTheAlwaysFields(t *testing.T) {
	raw, err := json.Marshal(Server{ID: "srv-2", Name: "fetch"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := []string{"id", "name", "enabled", "created_ms", "updated_ms"}
	for _, k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("the minimal wire lost the key %q: %s", k, raw)
		}
	}
	if len(got) != len(want) {
		t.Errorf("the minimal wire carries more than the always-fields: %s", raw)
	}
	if got["enabled"] != false {
		t.Errorf("enabled = %v, want false", got["enabled"])
	}
}

// TestValidateHoldsTheRules: the rule travels with the row, so it is
// measured here — the name format, the one-transport rule, the URL's shape
// and the caps.
func TestValidateHoldsTheRules(t *testing.T) {
	ok := Server{Name: "github", Command: "npx", Args: []string{"-y"}}
	if err := Validate(ok); err != nil {
		t.Fatalf("a valid server was rejected: %v", err)
	}
	remote := Server{Name: "remote", URL: "https://example.invalid/mcp"}
	if err := Validate(remote); err != nil {
		t.Fatalf("a valid remote server was rejected: %v", err)
	}

	cases := []struct {
		name string
		srv  Server
		want string
	}{
		{"a bad name", Server{Name: "GitHub", Command: "npx"}, "name must match"},
		{"both transports", Server{Name: "x", Command: "npx", URL: "https://a.invalid"}, "not both"},
		{"no transport", Server{Name: "x"}, "is required"},
		{"a bad scheme", Server{Name: "x", URL: "ftp://a.invalid"}, "must be http or https"},
		{"no host", Server{Name: "x", URL: "https://"}, "must have a host"},
		{"an empty arg", Server{Name: "x", Command: "npx", Args: []string{" "}}, "empty arg"},
		{"a bad env name", Server{Name: "x", Command: "npx", Env: map[string]string{"1BAD": "x"}}, "invalid env var name"},
		{"a bad header name", Server{Name: "x", URL: "https://a.invalid", Headers: map[string]string{"bad header": "x"}}, "invalid header name"},
		{"an empty allowlisted tool", Server{Name: "x", Command: "npx", ToolAllow: []string{" "}}, "empty tool name"},
	}
	for _, tc := range cases {
		err := Validate(tc.srv)
		if err == nil {
			t.Errorf("%s: expected a rejection", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error = %q, want it to mention %q", tc.name, err.Error(), tc.want)
		}
	}

	// The caps are BOUNDARIES, so they are measured at the boundary: the
	// value just under the cap is accepted and the value just over it is
	// rejected. A test that builds Cap+1 items would pass whatever the cap
	// is — the cap is the policy, so the policy is spelled out here instead
	// of being read back from the constant.
	caps := []struct {
		name  string
		max   int
		build func(n int) Server
	}{
		{"args", 32, func(n int) Server {
			s := Server{Name: "x", Command: "npx"}
			for i := 0; i < n; i++ {
				s.Args = append(s.Args, "a")
			}
			return s
		}},
		{"env vars", 32, func(n int) Server {
			s := Server{Name: "x", Command: "npx"}
			s.Env = map[string]string{}
			for i := 0; i < n; i++ {
				s.Env[fmt.Sprintf("K%d", i)] = "v"
			}
			return s
		}},
		{"headers", 32, func(n int) Server {
			s := Server{Name: "x", URL: "https://a.invalid"}
			s.Headers = map[string]string{}
			for i := 0; i < n; i++ {
				s.Headers[fmt.Sprintf("X-%d", i)] = "v"
			}
			return s
		}},
		{"allowlisted tools", 128, func(n int) Server {
			s := Server{Name: "x", Command: "npx"}
			for i := 0; i < n; i++ {
				s.ToolAllow = append(s.ToolAllow, fmt.Sprintf("t%d", i))
			}
			return s
		}},
	}
	for _, c := range caps {
		if err := Validate(c.build(c.max)); err != nil {
			t.Errorf("%s: %d must be accepted: %v", c.name, c.max, err)
		}
		if err := Validate(c.build(c.max + 1)); err == nil || !strings.Contains(err.Error(), "at most") {
			t.Errorf("%s: %d must be rejected: %v", c.name, c.max+1, err)
		}
	}
}

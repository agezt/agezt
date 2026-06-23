// SPDX-License-Identifier: MIT

package openai

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/toolname"
)

// The encoded request carries the conformed name (matching the OpenAI pattern),
// never the raw dotted one. (Unit coverage of the conformance maps themselves
// lives in plugins/providers/internal/toolname.)
func TestEncodeRequestSanitizesToolNames(t *testing.T) {
	tools := []agent.ToolDef{{Name: "browser.read", Description: "read a page"}}
	body, err := encodeRequest("gpt-5.5", "sys", nil, tools, 100, false, agent.Params{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if strings.Contains(s, `"browser.read"`) {
		t.Errorf("request leaks the dotted tool name: %s", s)
	}
	if !strings.Contains(s, `"browser_read"`) {
		t.Errorf("request missing the conformed tool name: %s", s)
	}
}

// Two distinct tool names that conform to the SAME wire name must be sent under
// distinct names (the mapping is injective), so the encoded request never carries
// a duplicate function name — which strict gateways reject and which could
// misroute a tool_call (M415). "browser.read" and "browser_read" both naively →
// "browser_read".
func TestEncodeRequest_CollisionStaysDistinct(t *testing.T) {
	tools := []agent.ToolDef{{Name: "browser.read"}, {Name: "browser_read"}}
	// Sanity: the shared mapping is injective.
	fwd, _ := toolname.Maps(tools)
	if fwd["browser.read"] == fwd["browser_read"] {
		t.Fatalf("collision not broken: both → %q", fwd["browser.read"])
	}
	body, err := encodeRequest("m", "", nil, tools, 0, false, agent.Params{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(body), `"browser_read"`); n != 1 {
		t.Errorf("expected exactly one bare \"browser_read\" wire name, got %d: %s", n, body)
	}
}

// SPDX-License-Identifier: MIT

package edict

import (
	"encoding/json"
	"testing"
)

func TestCapabilityForToolCall(t *testing.T) {
	cases := []struct {
		tool  string
		input string
		want  Capability
	}{
		{"shell", `{"command":"ls"}`, CapShell},
		{"file", `{"op":"read","path":"x"}`, CapFileRead},
		{"file", `{"op":"list"}`, CapFileList},
		{"file", `{"op":"write","path":"x","content":"y"}`, CapFileWrite},
		{"file", `{"op":"append","path":"x","content":"y"}`, CapFileWrite},
		{"file", `{"op":"delete","path":"x"}`, CapFileDelete},
		{"file", `{"op":"stat","path":"x"}`, CapFileRead},
		{"file", `{"op":"search","pattern":"x"}`, CapFileRead},
		{"file", `{"op":"chmod"}`, Capability("file.chmod")},
		{"http", `{"method":"GET","url":"https://x"}`, CapHTTPGet},
		{"http", `{"method":"POST","url":"https://x"}`, CapHTTPPost},
		{"http", `{"method":"  post  ","url":"https://x"}`, CapHTTPPost},
		{"unknown-tool", `{}`, Capability("unknown-tool")},
	}
	for _, c := range cases {
		got := CapabilityForToolCall(c.tool, json.RawMessage(c.input))
		if got != c.want {
			t.Errorf("tool=%s input=%s: got %s, want %s", c.tool, c.input, got, c.want)
		}
	}
}

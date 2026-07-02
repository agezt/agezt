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
		{"file", `{"op":"replace","path":"x","find":"a","replacement":"b"}`, CapFileWrite},
		{"file", `{"op":"delete","path":"x"}`, CapFileDelete},
		{"file", `{"op":"stat","path":"x"}`, CapFileRead},
		{"file", `{"op":"search","pattern":"x"}`, CapFileRead},
		{"file", `{"op":"chmod"}`, Capability("file.chmod")},
		{"http", `{"method":"GET","url":"https://x"}`, CapHTTPGet},
		{"http", `{"method":"POST","url":"https://x"}`, CapHTTPPost},
		{"http", `{"method":"  post  ","url":"https://x"}`, CapHTTPPost},
		{"browser.action", `{"url":"https://x","actions":[]}`, CapBrowserAction},
		{"browser.open", `{"url":"https://x"}`, CapBrowserAction},
		{"browser.snapshot", `{"url":"https://x"}`, CapBrowserAction},
		{"browser.click", `{"url":"https://x","selector":"button"}`, CapBrowserAction},
		{"browser.type", `{"url":"https://x","selector":"input","value":"hi"}`, CapBrowserAction},
		{"browser.wait", `{"url":"https://x","wait_ms":100}`, CapBrowserAction},
		{"browser.screenshot", `{"url":"https://x"}`, CapBrowserAction},
		{"browser.downloads", `{"url":"https://x","selector":"a"}`, CapBrowserAction},
		{"browser.cookies", `{"url":"https://x"}`, CapBrowserAction},
		{"browser.tabs", `{"session_id":"work"}`, CapBrowserAction},
		{"browser.close", `{"session_id":"work"}`, CapBrowserAction},
		// A download (fetch) is a network GET that saves the bytes (M831).
		{"fetch", `{"url":"https://x/cat.png"}`, CapHTTPGet},
		// The artifacts tool (M832): list/read are file-read; delete is file-delete.
		{"artifacts", `{"op":"list"}`, CapFileRead},
		{"artifacts", `{"op":"read","id":"art-x"}`, CapFileRead},
		{"artifacts", `{"op":"  DELETE  ","id":"art-x"}`, CapFileDelete},
		{"artifacts", `{}`, CapFileRead}, // garbled call lands on read (low-risk default)
		// The Personal Data Lake tool (M834) rides the memory capability.
		{"db", `{"op":"query","collection":"expenses"}`, CapMemory},
		{"db", `{"op":"insert","collection":"notes","record":{}}`, CapMemory},
		// The Council of Elders tool (M837) rides the delegate capability.
		{"council", `{"question":"ship?"}`, CapDelegate},
		{"remote_run", `{"task":"x"}`, CapRemoteRun},
		{"notify", `{"text":"hi"}`, CapNotify},
		{"send_media", `{"artifact":"abc"}`, CapNotify},
		{"homeassistant", `{"operation":"get_states","entity_id":"light.x"}`, CapHomeAssistantRead},
		{"homeassistant", `{"operation":"call_service","domain":"light","service":"turn_on"}`, CapHomeAssistantCall},
		{"homeassistant", `{"operation":"  CALL_SERVICE  "}`, CapHomeAssistantCall},
		{"homeassistant", `{}`, CapHomeAssistantRead}, // unparsed/absent op → read (low-risk default)
		{"workboard", `{"op":"create","title":"x"}`, CapWorkboard},
		{"workboard", `{"op":"list"}`, CapWorkboard},
		{"unknown-tool", `{}`, Capability("unknown-tool")},
		// Script-tool forge (M794): every forged forge_<name> call IS a
		// sandboxed code execution; tool_forge authoring is its own grant,
		// except op=test, which runs code.
		{"forge_fetch_weather", `{"city":"izmir"}`, CapCodeExec},
		{"forge_x", `{}`, CapCodeExec},
		{"tool_forge", `{"op":"draft","name":"x"}`, CapToolForge},
		{"tool_forge", `{"op":"list"}`, CapToolForge},
		{"tool_forge", `{"op":"  TEST  ","ref":"x"}`, CapCodeExec},
		{"tool_forge", `{}`, CapToolForge},
		// MCP self-install (M796): every bridged mcp_<server>_<tool> call is
		// external code → mcp.call; the mcp tool's install ops are gated,
		// list reads the daemon's own state.
		{"mcp_fake_greet", `{"name":"x"}`, CapMCP},
		{"mcp_everything_read-file", `{}`, CapMCP},
		{"mcp", `{"op":"add","name":"x"}`, CapMCPInstall},
		{"mcp", `{"op":"attach","ref":"x"}`, CapMCPInstall},
		{"mcp", `{"op":"remove","ref":"x"}`, CapMCPInstall},
		{"mcp", `{"op":"list"}`, CapIntrospect},
		{"mcp", `{}`, CapMCPInstall}, // garbled call lands on the gated axis
	}
	for _, c := range cases {
		got := CapabilityForToolCall(c.tool, json.RawMessage(c.input))
		if got != c.want {
			t.Errorf("tool=%s input=%s: got %s, want %s", c.tool, c.input, got, c.want)
		}
	}
}

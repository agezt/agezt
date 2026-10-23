// SPDX-License-Identifier: MIT

package acpagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func invoke(t *testing.T, tool *Tool, task string) (string, bool) {
	t.Helper()
	in, _ := json.Marshal(map[string]string{"task": task})
	res, err := tool.Invoke(context.Background(), in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	return res.Output, res.IsError
}

func TestACPAgent_RelaysStreamedAnswer(t *testing.T) {
	peer := &wirePeer{chunks: []string{"analy", "sing… ", "done"}}
	tool := &Tool{Cmd: "fake-acp-agent", Cwd: "/workspace", Catalog: fakeCatalog{}, dial: fakePeer(peer)}

	out, isErr := invoke(t, tool, "summarise the repo")
	if isErr {
		t.Fatalf("unexpected error result: %s", out)
	}
	if !strings.Contains(out, "analysing… done") {
		t.Errorf("streamed chunks not relayed, got:\n%s", out)
	}
	if peer.gotTask != "summarise the repo" {
		t.Errorf("external agent saw task %q", peer.gotTask)
	}
	if peer.gotCwd != "/workspace" {
		t.Errorf("external agent saw cwd %q, want /workspace", peer.gotCwd)
	}
}

func TestACPAgent_NonStreamingAnswerRelayed(t *testing.T) {
	// No streamed chunks — the peer answers with the final result only.
	peer := &wirePeer{}
	tool := &Tool{Cmd: "x", Cwd: "/w", Catalog: fakeCatalog{}, dial: fakePeer(peer)}
	out, isErr := invoke(t, tool, "q")
	if isErr {
		t.Fatalf("unexpected error: %s", out)
	}
	if !strings.Contains(out, "no message") {
		t.Errorf("an empty answer should say so, got:\n%s", out)
	}
}

func TestACPAgent_AgentErrorSurfaced(t *testing.T) {
	peer := &wirePeer{promptErr: "model unavailable"}
	tool := &Tool{Cmd: "x", Cwd: "/w", Catalog: fakeCatalog{}, dial: fakePeer(peer)}
	out, isErr := invoke(t, tool, "go")
	if !isErr {
		t.Error("a failing external agent should yield an error result")
	}
	if !strings.Contains(out, "session/prompt failed") {
		t.Errorf("error not surfaced, got:\n%s", out)
	}
}

func TestACPAgent_EmptyTask(t *testing.T) {
	tool := &Tool{Cmd: "x", Cwd: "/w", Catalog: fakeCatalog{}, dial: fakePeer(&wirePeer{})}
	out, isErr := invoke(t, tool, "   ")
	if !isErr || !strings.Contains(out, "task is required") {
		t.Errorf("empty task should error, got:\n%s", out)
	}
}

func TestACPAgent_WithoutACatalogueSaysSo(t *testing.T) {
	// A tool built without the catalogue port has nothing to resolve an agent
	// with — it must say so, not call into nothing.
	tool := &Tool{Cmd: "x", Cwd: "/w", dial: fakePeer(&wirePeer{})}
	out, isErr := invoke(t, tool, "go")
	if !isErr || !strings.Contains(out, "catalogue is not available") {
		t.Errorf("a tool without the catalogue should say so, got:\n%s", out)
	}
}

func TestACPAgent_UnknownAgentListsTheInstalledOnes(t *testing.T) {
	// The catalogue refuses the unknown slug; the tool's answer names the
	// agents that ARE installed, so the operator can pick one.
	catalog := fakeCatalog{installed: []string{"gemini", "codex"}}
	tool := &Tool{Cmd: "x", Cwd: "/w", Catalog: catalog, dial: fakePeer(&wirePeer{})}
	in, _ := json.Marshal(map[string]string{"task": "go", "agent": "nope"})
	res, err := tool.Invoke(context.Background(), in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError {
		t.Error("an unknown agent should yield an error result")
	}
	if !strings.Contains(res.Output, "available installed agents: gemini, codex") {
		t.Errorf("the answer should name the installed agents, got:\n%s", res.Output)
	}
}

func TestACPAgent_ResolvesTheCataloguedCommand(t *testing.T) {
	// The launch command comes from the catalogue — never from the caller's
	// selector, which is prompt-influenceable (CWE-78).
	catalog := fakeCatalog{installed: []string{"gemini"}, commands: map[string]string{"gemini": "gemini --experimental-acp"}}
	peer := &wirePeer{}
	tool := &Tool{Cmd: "fallback-cmd", Cwd: "/w", Catalog: catalog, dial: fakePeer(peer)}
	in, _ := json.Marshal(map[string]string{"task": "go", "agent": "gemini"})
	res, err := tool.Invoke(context.Background(), in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("a catalogued agent should resolve, got:\n%s", res.Output)
	}
}

func TestNew_DisabledWhenNoCmdAndNoInstalledAgent(t *testing.T) {
	// A configured default command always enables the tool.
	if New("codex acp", "/w", fakeCatalog{}) == nil {
		t.Error("New with a cmd should return a tool")
	}
	// With no default command, the tool is enabled iff the catalogue reports
	// an installed agent (discovery makes it usable without configuration).
	if tool := New("", "/w", fakeCatalog{installed: []string{"gemini"}}); tool == nil {
		t.Error("New with no cmd but an installed agent should return a tool")
	}
	if tool := New("", "/w", fakeCatalog{}); tool != nil {
		t.Error("New with no cmd and no installed agent should return nil (disabled)")
	}
	// No catalogue at all and no default command: there is nothing to
	// discover, so the tool stays disabled.
	if tool := New("", "/w", nil); tool != nil {
		t.Error("New with no cmd and no catalogue should return nil (disabled)")
	}
}

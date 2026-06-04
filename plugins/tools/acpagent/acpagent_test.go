// SPDX-License-Identifier: MIT

package acpagent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/acp"
)

// peerRunner is the fake external ACP agent's brain: it streams chunks and
// records the task it was handed. It backs a real acp.Server so the tool talks
// to a genuine ACP peer over pipes (round-trip proof, not a stub).
type peerRunner struct {
	chunks  []string
	answer  string
	err     error
	gotTask string
	gotCwd  string
}

func (p *peerRunner) Prompt(_ context.Context, cwd, intent string, onChunk func(acp.ChunkKind, string)) (string, error) {
	p.gotTask = intent
	p.gotCwd = cwd
	for _, c := range p.chunks {
		onChunk(acp.ChunkMessage, c)
	}
	return p.answer, p.err
}

// fakePeer returns a dialFunc that wires the tool's ACP client to a real
// acp.Server (the fake external agent) over a pair of pipes.
func fakePeer(runner *peerRunner) dialFunc {
	return func(_ context.Context, _, _ string) (*transport, error) {
		toolToPeerR, toolToPeerW := io.Pipe()
		peerToToolR, peerToToolW := io.Pipe()
		srv := acp.New(runner, toolToPeerR, peerToToolW)
		go func() { _ = srv.Serve(context.Background()) }()
		return &transport{
			out:   peerToToolR, // agent stdout → tool reads
			in:    toolToPeerW, // tool writes → agent stdin
			close: func() error { _ = toolToPeerW.Close(); return nil },
		}, nil
	}
}

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
	runner := &peerRunner{chunks: []string{"analy", "sing… ", "done"}, answer: "ignored-when-streamed"}
	tool := &Tool{Cmd: "fake-acp-agent", Cwd: "/workspace", dial: fakePeer(runner)}

	out, isErr := invoke(t, tool, "summarise the repo")
	if isErr {
		t.Fatalf("unexpected error result: %s", out)
	}
	if !strings.Contains(out, "analysing… done") {
		t.Errorf("streamed chunks not relayed, got:\n%s", out)
	}
	if runner.gotTask != "summarise the repo" {
		t.Errorf("external agent saw task %q", runner.gotTask)
	}
	if runner.gotCwd != "/workspace" {
		t.Errorf("external agent saw cwd %q, want /workspace", runner.gotCwd)
	}
}

func TestACPAgent_NonStreamingAnswerRelayed(t *testing.T) {
	// No streamed chunks — the server emits the whole answer as a single chunk.
	runner := &peerRunner{answer: "the complete answer"}
	tool := &Tool{Cmd: "x", Cwd: "/w", dial: fakePeer(runner)}
	out, isErr := invoke(t, tool, "q")
	if isErr {
		t.Fatalf("unexpected error: %s", out)
	}
	if !strings.Contains(out, "the complete answer") {
		t.Errorf("non-streaming answer not relayed, got:\n%s", out)
	}
}

func TestACPAgent_AgentErrorSurfaced(t *testing.T) {
	runner := &peerRunner{err: errors.New("model unavailable")}
	tool := &Tool{Cmd: "x", Cwd: "/w", dial: fakePeer(runner)}
	out, isErr := invoke(t, tool, "go")
	if !isErr {
		t.Error("a failing external agent should yield an error result")
	}
	if !strings.Contains(out, "session/prompt failed") {
		t.Errorf("error not surfaced, got:\n%s", out)
	}
}

func TestACPAgent_EmptyTask(t *testing.T) {
	tool := &Tool{Cmd: "x", Cwd: "/w", dial: fakePeer(&peerRunner{})}
	out, isErr := invoke(t, tool, "   ")
	if !isErr || !strings.Contains(out, "task is required") {
		t.Errorf("empty task should error, got:\n%s", out)
	}
}

func TestNew_DisabledWhenNoCmd(t *testing.T) {
	if New("", "/w") != nil {
		t.Error("New with empty cmd should return nil (tool disabled)")
	}
	if New("  ", "/w") != nil {
		t.Error("New with blank cmd should return nil")
	}
	if New("codex acp", "/w") == nil {
		t.Error("New with a cmd should return a tool")
	}
}

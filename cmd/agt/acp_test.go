// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/agezt/agezt/kernel/acp"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
)

// fakeStreamer records the args of the last Stream call and relays a canned
// answer, standing in for *controlplane.Client without a daemon.
type fakeStreamer struct {
	lastCmd  string
	lastArgs map[string]any
	answer   string
	emit     []*event.Event // streamed to the onEvent callback before returning
}

func (f *fakeStreamer) Stream(_ context.Context, cmd string, args map[string]any, onEvent func(*event.Event)) (map[string]any, error) {
	f.lastCmd = cmd
	f.lastArgs = args
	for _, ev := range f.emit {
		onEvent(ev)
	}
	return map[string]any{"answer": f.answer}, nil
}

// With a tenant set, the ACP runner must forward it as the `tenant` run arg so
// the daemon routes the prompt to that tenant's isolated kernel (M14 Phase 6).
func TestACPRunner_ForwardsTenant(t *testing.T) {
	fs := &fakeStreamer{answer: "ok"}
	r := controlPlaneRunner{c: fs, tenant: "alpha"}

	ans, err := r.Prompt(context.Background(), "/cwd", "do a thing", func(acp.ChunkKind, string) {})
	if err != nil {
		t.Fatal(err)
	}
	if ans != "ok" {
		t.Errorf("answer = %q, want ok", ans)
	}
	if fs.lastCmd != controlplane.CmdRun {
		t.Errorf("cmd = %q, want %q", fs.lastCmd, controlplane.CmdRun)
	}
	if fs.lastArgs["intent"] != "do a thing" {
		t.Errorf("intent = %v", fs.lastArgs["intent"])
	}
	if fs.lastArgs["tenant"] != "alpha" {
		t.Errorf("tenant = %v, want alpha", fs.lastArgs["tenant"])
	}
}

// Without a tenant, the `tenant` key must be absent entirely (byte-for-byte the
// prior single-tenant request — not an empty string the daemon would have to
// special-case).
func TestACPRunner_OmitsTenantWhenUnset(t *testing.T) {
	fs := &fakeStreamer{answer: "ok"}
	r := controlPlaneRunner{c: fs} // no tenant

	if _, err := r.Prompt(context.Background(), "/cwd", "hi", func(acp.ChunkKind, string) {}); err != nil {
		t.Fatal(err)
	}
	if _, present := fs.lastArgs["tenant"]; present {
		t.Errorf("tenant key must be absent when unset, got args=%v", fs.lastArgs)
	}
}

// The ACP runner must relay llm.token events as message chunks and llm.reasoning
// events as thought chunks (M322), tagging each with the right ChunkKind so the
// server maps it to agent_message_chunk vs agent_thought_chunk. Other event kinds
// are dropped.
func TestACPRunner_RelaysReasoningAsThought(t *testing.T) {
	mk := func(kind event.Kind, text string) *event.Event {
		return &event.Event{Kind: kind, Payload: json.RawMessage(`{"text":` + strconv.Quote(text) + `}`)}
	}
	fs := &fakeStreamer{answer: "done", emit: []*event.Event{
		mk(event.KindLLMReasoning, "let me think"),
		mk(event.KindLLMToken, "the answer"),
		mk(event.KindToolResult, "ignored"), // unrelated kind → dropped
	}}
	r := controlPlaneRunner{c: fs}

	type chunk struct {
		kind acp.ChunkKind
		text string
	}
	var got []chunk
	if _, err := r.Prompt(context.Background(), "/cwd", "go", func(k acp.ChunkKind, s string) {
		got = append(got, chunk{k, s})
	}); err != nil {
		t.Fatal(err)
	}
	want := []chunk{
		{acp.ChunkThought, "let me think"},
		{acp.ChunkMessage, "the answer"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d chunks %+v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("chunk %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

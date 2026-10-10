// SPDX-License-Identifier: MIT

package controlplane

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestChatSuggestionsReadThePrimaryMemory: the suggestions lead with the
// primary kernel's active memory, fill from the tool context and keep the
// session id check.
func TestChatSuggestionsReadThePrimaryMemory(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	if _, _, err := k.Memory().Remember("seed", memory.RememberSpec{Type: memory.TypePreference, Subject: "Code Style", Content: "be blunt", Confidence: 0.9, Force: true}); err != nil {
		t.Fatal(err)
	}
	s := NewServer(k, t.TempDir())
	s.token = "primary"
	// The raw line keeps each suggestion's declared member order.
	client, server := net.Pipe()
	defer client.Close()
	go s.handleConn(context.Background(), server)
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	raw, _ := json.Marshal(Request{ID: "c", Cmd: CmdChatSuggestions, Token: "primary", Args: map[string]any{"session_id": "conv", "tools": "bash"}})
	if _, err := client.Write(append(raw, 10)); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(client).ReadBytes(10)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"c","type":"result","result":{"suggestions":[{"id":"mem-code-style","label":"Apply: Code Style","prompt":"Keep my preference about Code Style in mind (be blunt) and apply it now.","category":"memory","icon":"brain"},{"id":"debug-why","label":"Why did this happen?","prompt":"Explain why this command produced this output","category":"debug","icon":"search"},{"id":"debug-alternatives","label":"Show alternatives","prompt":"What are alternative ways to accomplish the same task?","category":"debug","icon":"git-branch"}]}}`
	if string(bytes.TrimSpace(line)) != want {
		t.Fatal(string(line))
	}
	if resp := callAppHost(t, s, Request{ID: "c", Cmd: CmdChatSuggestions, Token: "primary", Args: map[string]any{"session_id": 5}})[0]; resp.Type != RespError || resp.Error != "args.session_id must be a string" {
		t.Fatal(resp)
	}
	if wire, exists := commandRegistry[CmdChatSuggestions]; !exists || !wire.AppOwned || !wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone {
		t.Fatalf("native wire %+v", wire)
	}
}

// TestChatSummarizeBindsTheKernel: the summarizer calls the primary kernel's
// provider with its default model, live and audited.
func TestChatSummarizeBindsTheKernel(t *testing.T) {
	prov := mock.New(mock.FinalText(" the briefing "))
	var model string
	prov.OnRequest = func(r llm.CompletionRequest) { model = r.Model }
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: prov, Model: "kernel-model"})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, t.TempDir())
	s.token = "primary"
	resp := callAppHost(t, s, Request{ID: "c", Cmd: CmdChatSummarize, Token: "primary", Args: map[string]any{"turns": []any{map[string]any{"role": "user", "text": "hi"}}}})[0]
	if raw, _ := json.Marshal(resp.Result); resp.Type != RespResult || string(raw) != `{"summary":"the briefing","turns":1}` || model != "kernel-model" {
		t.Fatal(resp, model)
	}
	if wire, exists := commandRegistry[CmdChatSummarize]; !exists || !wire.AppOwned || wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamLive {
		t.Fatalf("native wire %+v", wire)
	}
}

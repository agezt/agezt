// SPDX-License-Identifier: MIT

package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type capture struct {
	calls []string // "kind/id:text"
	fail  bool
}

func (c *capture) send(_ context.Context, kind, id, text string) error {
	if c.fail {
		return fmt.Errorf("boom")
	}
	c.calls = append(c.calls, kind+"/"+id+":"+text)
	return nil
}

func TestNotify_New_DisabledWhenNoTargets(t *testing.T) {
	cap := &capture{}
	if tool := New(cap.send, nil); tool != nil {
		t.Error("New with no targets should return nil (tool disabled)")
	}
	// Only-empty kinds are pruned away → still nil.
	if tool := New(cap.send, map[string][]string{"slack": {}}); tool != nil {
		t.Error("New with only-empty targets should return nil")
	}
	if tool := New(nil, map[string][]string{"slack": {"C1"}}); tool != nil {
		t.Error("New with nil sender should return nil")
	}
}

func TestNotify_SendsToAllConfiguredTargets(t *testing.T) {
	cap := &capture{}
	tool := New(cap.send, map[string][]string{
		"slack":   {"C1", "C2"},
		"discord": {"D1"},
	})
	if tool == nil {
		t.Fatal("tool should be enabled")
	}
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"text":"hi team"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", res.Output)
	}
	if len(cap.calls) != 3 {
		t.Fatalf("expected 3 sends (C1,C2,D1), got %d: %v", len(cap.calls), cap.calls)
	}
	for _, c := range cap.calls {
		if !strings.HasSuffix(c, ":hi team") {
			t.Errorf("send carried wrong text: %q", c)
		}
	}
}

func TestNotify_ChannelFilter(t *testing.T) {
	cap := &capture{}
	tool := New(cap.send, map[string][]string{"slack": {"C1"}, "discord": {"D1"}})
	// Restrict to discord only.
	if _, err := tool.Invoke(context.Background(), json.RawMessage(`{"text":"x","channel":"discord"}`)); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(cap.calls) != 1 || !strings.HasPrefix(cap.calls[0], "discord/D1:") {
		t.Errorf("channel filter should send only to discord, got %v", cap.calls)
	}
	// An unconfigured channel kind is an error, no send.
	res, _ := tool.Invoke(context.Background(), json.RawMessage(`{"text":"x","channel":"telegram"}`))
	if !res.IsError {
		t.Error("unconfigured channel should yield an error result")
	}
}

func TestNotify_EmptyTextRejected(t *testing.T) {
	cap := &capture{}
	tool := New(cap.send, map[string][]string{"slack": {"C1"}})
	res, _ := tool.Invoke(context.Background(), json.RawMessage(`{"text":"  "}`))
	if !res.IsError {
		t.Error("empty text should be an error")
	}
	if len(cap.calls) != 0 {
		t.Error("no send should happen for empty text")
	}
}

func TestNotify_AllSendsFailIsError(t *testing.T) {
	cap := &capture{fail: true}
	tool := New(cap.send, map[string][]string{"slack": {"C1"}})
	res, _ := tool.Invoke(context.Background(), json.RawMessage(`{"text":"hi"}`))
	if !res.IsError {
		t.Error("a total delivery failure should be an error result")
	}
}

func TestNotify_DefinitionListsKinds(t *testing.T) {
	tool := New((&capture{}).send, map[string][]string{"slack": {"C1"}, "discord": {"D1"}})
	def := tool.Definition()
	if def.Name != "notify" {
		t.Errorf("name = %q want notify", def.Name)
	}
	if !strings.Contains(def.Description, "discord") || !strings.Contains(def.Description, "slack") {
		t.Errorf("description should list configured kinds; got %q", def.Description)
	}
}

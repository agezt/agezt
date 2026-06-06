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
	calls  []string // "kind/id:text"
	fail   bool     // fail every send
	failID string   // fail only sends to this id (partial failure)
}

func (c *capture) send(_ context.Context, kind, id, text string) error {
	if c.fail || (c.failID != "" && id == c.failID) {
		return fmt.Errorf("boom")
	}
	c.calls = append(c.calls, kind+"/"+id+":"+text)
	return nil
}

// bound is a test helper: a fresh tool wired to cap with the given targets.
func bound(cap *capture, targets map[string][]string) *Tool {
	t := New()
	t.Bind(cap.send, targets)
	return t
}

func TestNotify_UnboundReportsNotConfigured(t *testing.T) {
	tool := New() // never bound
	res, _ := tool.Invoke(context.Background(), json.RawMessage(`{"text":"hi"}`))
	if !res.IsError || !strings.Contains(res.Output, "not configured") {
		t.Errorf("unbound notify should report not configured, got %+v", res)
	}
	// Binding with no targets is still "not configured": Bind prunes every kind
	// whose id list is empty (len(ids) > 0), so a kind with no allowlisted
	// recipients leaves the tool disabled rather than advertised-but-undeliverable.
	// Assert the precise "not configured" outcome (not merely IsError): without the
	// prune the empty kind survives and Invoke instead proceeds to a "notify failed"
	// delivery against zero recipients — a different, wrong result that would still
	// be IsError. Also require no send was attempted.
	cap := &capture{}
	tool.Bind(cap.send, map[string][]string{"slack": {}})
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"text":"hi"}`))
	if !res.IsError || !strings.Contains(res.Output, "not configured") {
		t.Errorf("binding with only-empty targets should stay disabled (not configured), got %+v", res)
	}
	if len(cap.calls) != 0 {
		t.Errorf("an empty-id kind must be pruned, not delivered to; got sends %v", cap.calls)
	}
}

func TestNotify_SendsToAllConfiguredTargets(t *testing.T) {
	cap := &capture{}
	tool := bound(cap, map[string][]string{
		"slack":   {"C1", "C2"},
		"discord": {"D1"},
	})
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
	tool := bound(cap, map[string][]string{"slack": {"C1"}, "discord": {"D1"}})
	if _, err := tool.Invoke(context.Background(), json.RawMessage(`{"text":"x","channel":"discord"}`)); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(cap.calls) != 1 || !strings.HasPrefix(cap.calls[0], "discord/D1:") {
		t.Errorf("channel filter should send only to discord, got %v", cap.calls)
	}
	res, _ := tool.Invoke(context.Background(), json.RawMessage(`{"text":"x","channel":"telegram"}`))
	if !res.IsError {
		t.Error("unconfigured channel should yield an error result")
	}
}

func TestNotify_EmptyTextRejected(t *testing.T) {
	cap := &capture{}
	tool := bound(cap, map[string][]string{"slack": {"C1"}})
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
	tool := bound(cap, map[string][]string{"slack": {"C1"}})
	res, _ := tool.Invoke(context.Background(), json.RawMessage(`{"text":"hi"}`))
	if !res.IsError {
		t.Error("a total delivery failure should be an error result")
	}
}

func TestNotify_PartialFailureIsError(t *testing.T) {
	// Two recipients, one fails: the result must flag IsError so automation
	// doesn't read a half-delivered alert as fully sent.
	cap := &capture{failID: "C2"}
	tool := bound(cap, map[string][]string{"slack": {"C1", "C2"}})
	res, _ := tool.Invoke(context.Background(), json.RawMessage(`{"text":"hi"}`))
	if !res.IsError {
		t.Errorf("partial delivery failure should be IsError; got %+v", res)
	}
	if !strings.Contains(res.Output, "FAILED") || !strings.Contains(res.Output, "C2") {
		t.Errorf("partial failure should name the failed recipient; got %q", res.Output)
	}
	if len(cap.calls) != 1 { // C1 still delivered
		t.Errorf("the succeeding recipient should still be sent; got %v", cap.calls)
	}
}

func TestNotify_DefinitionListsKinds(t *testing.T) {
	tool := bound(&capture{}, map[string][]string{"slack": {"C1"}, "discord": {"D1"}})
	def := tool.Definition()
	if def.Name != "notify" {
		t.Errorf("name = %q want notify", def.Name)
	}
	if !strings.Contains(def.Description, "discord") || !strings.Contains(def.Description, "slack") {
		t.Errorf("description should list configured kinds; got %q", def.Description)
	}
}

// TestNotify_BindIsolatesTargets ensures Bind copies the targets so a later
// mutation of the caller's slice/map can't change what the tool delivers to.
func TestNotify_BindIsolatesTargets(t *testing.T) {
	cap := &capture{}
	ids := []string{"C1"}
	targets := map[string][]string{"slack": ids}
	tool := New()
	tool.Bind(cap.send, targets)
	// Mutate the caller's data after Bind.
	ids[0] = "HACKED"
	targets["discord"] = []string{"D1"}
	if _, err := tool.Invoke(context.Background(), json.RawMessage(`{"text":"hi"}`)); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(cap.calls) != 1 || !strings.HasPrefix(cap.calls[0], "slack/C1:") {
		t.Errorf("Bind should snapshot targets; got %v", cap.calls)
	}
}

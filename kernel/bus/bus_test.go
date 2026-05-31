// SPDX-License-Identifier: MIT

package bus

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/redact"
)

func newTestBus(t *testing.T) *Bus {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { j.Close() })
	b := New(j)
	t.Cleanup(b.Close)
	return b
}

func TestSubscribePublish(t *testing.T) {
	b := newTestBus(t)
	sub, err := b.Subscribe("agent.spawned", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Cancel()

	got, err := b.Publish(event.Spec{Subject: "agent.spawned", Kind: event.KindAgentSpawned, Actor: "kernel"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-sub.C:
		if ev.ID != got.ID {
			t.Errorf("delivered event id=%s, want %s", ev.ID, got.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no delivery within 1s")
	}
}

// With a redactor installed, a secret in a published payload must never reach
// the journal: the persisted event carries the placeholder, the hash is computed
// over the redacted bytes (so it still verifies), and tag values are scrubbed too.
func TestRedactor_ScrubsBeforeJournal(t *testing.T) {
	b := newTestBus(t)
	r := redact.New()
	r.SetSecrets([]string{"literal-tenant-key-xyz"})
	b.SetRedactor(r)

	sub, _ := b.Subscribe("tool.result", 4)
	defer sub.Cancel()

	got, err := b.Publish(event.Spec{
		Subject: "tool.result",
		Kind:    event.KindToolResult,
		Actor:   "agent-1",
		Payload: map[string]any{
			"stdout": "OPENAI_API_KEY=sk-abcdefghijklmnopqrstuvwx and literal-tenant-key-xyz",
			"ok":     true,
		},
		Tags: map[string]string{"note": "key sk-zzzzzzzzzzzzzzzzzzzzzz here"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// The returned (and delivered) event is already redacted.
	if strings.Contains(string(got.Payload), "sk-abcdefghijklmnopqrstuvwx") ||
		strings.Contains(string(got.Payload), "literal-tenant-key-xyz") {
		t.Fatalf("returned payload still has a secret: %s", got.Payload)
	}
	if !strings.Contains(string(got.Payload), redact.Placeholder) {
		t.Errorf("expected placeholder in payload: %s", got.Payload)
	}
	if strings.Contains(got.Tags["note"], "sk-zzzzzzzzzzzzzzzzzzzzzz") {
		t.Errorf("tag value not redacted: %q", got.Tags["note"])
	}

	// The PERSISTED event in the journal is redacted and its hash verifies over
	// the redacted bytes.
	var seen int
	_ = b.j.Range(func(e *event.Event) error {
		if e.Kind != event.KindToolResult {
			return nil
		}
		seen++
		if strings.Contains(string(e.Payload), "sk-abcdefghijklmnopqrstuvwx") ||
			strings.Contains(string(e.Payload), "literal-tenant-key-xyz") {
			t.Errorf("journaled payload leaks a secret: %s", e.Payload)
		}
		if err := e.VerifyHash(); err != nil {
			t.Errorf("redacted event fails hash verify: %v", err)
		}
		return nil
	})
	if seen != 1 {
		t.Fatalf("expected 1 journaled tool.result, got %d", seen)
	}

	// Sanity: the non-secret field is intact.
	var back map[string]any
	if err := json.Unmarshal(got.Payload, &back); err != nil {
		t.Fatalf("redacted payload is invalid JSON: %v", err)
	}
	if back["ok"] != true {
		t.Errorf("non-secret field corrupted: %v", back)
	}
}

// Without a redactor, payloads pass through untouched (default behavior).
func TestRedactor_DisabledByDefault(t *testing.T) {
	b := newTestBus(t)
	got, err := b.Publish(event.Spec{
		Subject: "tool.result", Kind: event.KindToolResult, Actor: "a",
		Payload: map[string]any{"v": "sk-abcdefghijklmnopqrstuvwx"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got.Payload), "sk-abcdefghijklmnopqrstuvwx") {
		t.Errorf("no redactor configured, payload should be verbatim: %s", got.Payload)
	}
}

func TestPatternMatching(t *testing.T) {
	cases := []struct {
		pattern string
		subject string
		want    bool
	}{
		{"agent.spawned", "agent.spawned", true},
		{"agent.spawned", "agent.died", false},
		{"agent.*", "agent.spawned", true},
		{"agent.*", "agent.01H.tool", false},
		{"agent.*.tool", "agent.01H.tool", true},
		{"agent.*.tool", "agent.01H.llm", false},
		{"agent.>", "agent.spawned", true},
		{"agent.>", "agent.01H.tool", true},
		{"agent.>", "agent.01H.tool.invoked", true},
		{"agent.>", "task.received", false},
		{">", "anything.at.all", true},
		{">", "x", true},
	}
	for _, c := range cases {
		tokens, err := parsePattern(c.pattern)
		if err != nil {
			t.Fatalf("parsePattern(%q): %v", c.pattern, err)
		}
		sub := splitSubject(c.subject)
		got := matches(tokens, sub)
		if got != c.want {
			t.Errorf("matches(%q, %q) = %v, want %v", c.pattern, c.subject, got, c.want)
		}
	}
}

func TestParsePattern_BadInputs(t *testing.T) {
	bad := []string{"", "..", "agent..", "agent.>.x"}
	for _, p := range bad {
		if _, err := parsePattern(p); err == nil || !errors.Is(err, ErrPattern) {
			t.Errorf("parsePattern(%q) got err=%v, want ErrPattern", p, err)
		}
	}
}

func TestDurableBeforePublish(t *testing.T) {
	// When a subscriber receives an event, it MUST already be on disk.
	dir := t.TempDir()
	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	b := New(j)
	defer b.Close()

	sub, err := b.Subscribe(">", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Cancel()

	published, err := b.Publish(event.Spec{Subject: "x", Kind: event.KindHalt, Actor: "kernel"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-sub.C:
		// Verify the journal already has it.
		var found bool
		_ = j.Range(func(e *event.Event) error {
			if e.ID == ev.ID {
				found = true
			}
			return nil
		})
		if !found {
			t.Errorf("subscriber received event %s before journal had it", ev.ID)
		}
		if ev.ID != published.ID {
			t.Errorf("delivered id=%s, want %s", ev.ID, published.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no delivery")
	}
}

func TestBackpressure_DropsAndCounts(t *testing.T) {
	b := newTestBus(t)
	// Tiny buffer; do not drain. The 2nd publish must drop.
	sub, err := b.Subscribe(">", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Cancel()

	for range 5 {
		_, err := b.Publish(event.Spec{Subject: "x", Kind: event.KindHalt, Actor: "kernel"})
		if err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}
	// First fills the channel; the remaining 4 are dropped.
	if d := sub.Dropped.Load(); d != 4 {
		t.Errorf("Dropped=%d, want 4", d)
	}
	// The channel still holds the first one.
	if ev := <-sub.C; ev == nil {
		t.Error("expected an event in the buffer")
	}
}

func TestCancel_StopsDelivery(t *testing.T) {
	b := newTestBus(t)
	sub, err := b.Subscribe(">", 4)
	if err != nil {
		t.Fatal(err)
	}
	sub.Cancel()

	_, err = b.Publish(event.Spec{Subject: "x", Kind: event.KindHalt, Actor: "kernel"})
	if err != nil {
		t.Fatal(err)
	}
	// Channel is closed; reading must return zero value with ok=false.
	if ev, ok := <-sub.C; ok {
		t.Errorf("expected closed channel, got ev=%v", ev)
	}
}

func TestCancel_Idempotent(t *testing.T) {
	b := newTestBus(t)
	sub, err := b.Subscribe(">", 0)
	if err != nil {
		t.Fatal(err)
	}
	sub.Cancel()
	sub.Cancel() // must not panic
}

func TestClose_PreventsFurtherPublish(t *testing.T) {
	b := newTestBus(t)
	b.Close()
	_, err := b.Publish(event.Spec{Subject: "x", Kind: event.KindHalt, Actor: "kernel"})
	if !errors.Is(err, ErrClosed) {
		t.Errorf("got err=%v, want ErrClosed", err)
	}
	_, err = b.Subscribe(">", 0)
	if !errors.Is(err, ErrClosed) {
		t.Errorf("Subscribe after Close: got err=%v, want ErrClosed", err)
	}
}

func TestOrderPreserved_UnderConcurrency(t *testing.T) {
	// Publish from N goroutines; each subscriber sees a globally consistent
	// monotonic seq (because Publish is fully serialized).
	b := newTestBus(t)
	sub, err := b.Subscribe(">", 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Cancel()

	const total = 100
	const goroutines = 5
	const per = total / goroutines

	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Go(func() {
			for i := range per {
				_, err := b.Publish(event.Spec{
					Subject: fmt.Sprintf("ev.%d.%d", g, i),
					Kind:    event.KindHalt,
					Actor:   "kernel",
				})
				if err != nil {
					t.Errorf("Publish: %v", err)
					return
				}
			}
		})
	}
	wg.Wait()

	var prevSeq int64 = -1
	for range total {
		select {
		case ev := <-sub.C:
			if ev.Seq <= prevSeq {
				t.Fatalf("seq regressed: prev=%d cur=%d", prevSeq, ev.Seq)
			}
			prevSeq = ev.Seq
		case <-time.After(time.Second):
			t.Fatal("timeout draining subscriber")
		}
	}
}

// splitSubject mirrors strings.Split(subject, ".") without importing it
// just to keep this test file's surface tight.
func splitSubject(s string) []string {
	out := []string{}
	cur := ""
	for i := range len(s) {
		if s[i] == '.' {
			out = append(out, cur)
			cur = ""
		} else {
			cur += string(s[i])
		}
	}
	out = append(out, cur)
	return out
}

func TestPublishStreaming_DoesNotJournal(t *testing.T) {
	// Capture the journal's tail seq before and after PublishStreaming —
	// it must NOT advance. The contract is the whole point of having
	// PublishStreaming exist.
	b := newTestBus(t)
	if err := b.j.Verify(); err != nil {
		t.Fatalf("journal not in valid empty state: %v", err)
	}

	// Subscribe so we can verify fan-out still happens.
	sub, err := b.Subscribe("agent.>", 16)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Cancel()

	for i := range 5 {
		_, err := b.PublishStreaming(event.Spec{
			Subject: "agent.01H.llm",
			Kind:    event.KindLLMToken,
			Actor:   "agent-01H",
			Payload: map[string]any{"text": fmt.Sprintf("chunk-%d", i)},
		})
		if err != nil {
			t.Fatalf("PublishStreaming[%d]: %v", i, err)
		}
	}

	// Subscriber should have received all 5.
	deadline := time.After(time.Second)
	received := 0
	for received < 5 {
		select {
		case ev := <-sub.C:
			if !ev.IsEphemeral() {
				t.Errorf("delivered event reports !IsEphemeral: %+v", ev)
			}
			if ev.Kind != event.KindLLMToken {
				t.Errorf("kind=%q want %q", ev.Kind, event.KindLLMToken)
			}
			received++
		case <-deadline:
			t.Fatalf("only got %d/5 streaming events", received)
		}
	}

	// Critical: journal still verifies cleanly with no new events.
	// We test this by appending one real event afterwards and
	// confirming its seq is 1 (would be 6 if PublishStreaming had
	// journaled the 5 chunks).
	durableSpec := event.Spec{
		Subject: "agent.01H.task",
		Kind:    event.KindTaskReceived,
		Actor:   "agent-01H",
		Payload: map[string]string{"intent": "x"},
	}
	durable, err := b.Publish(durableSpec)
	if err != nil {
		t.Fatalf("Publish (durable): %v", err)
	}
	// First durable event in a fresh journal gets seq=0. If
	// PublishStreaming had leaked, this would be 5.
	if durable.Seq != 0 {
		t.Errorf("durable.Seq=%d, want 0 — PublishStreaming leaked into journal (seq advanced)", durable.Seq)
	}
	if durable.IsEphemeral() {
		t.Error("durable event reports IsEphemeral=true")
	}
	// And another, just to confirm the journal counter is healthy.
	more, err := b.Publish(durableSpec)
	if err != nil {
		t.Fatalf("Publish (second durable): %v", err)
	}
	if more.Seq != 1 {
		t.Errorf("second durable.Seq=%d, want 1", more.Seq)
	}
}

func TestPublishStreaming_RespectsPatternMatching(t *testing.T) {
	// Streaming events MUST match subscribers' patterns the same way
	// durable events do — otherwise the controlplane's per-run
	// subscription would silently drop tokens for that run.
	b := newTestBus(t)
	sub, err := b.Subscribe("agent.01H.>", 4)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Cancel()

	// Different correlation — should NOT be delivered.
	_, _ = b.PublishStreaming(event.Spec{
		Subject: "agent.OTHER.llm",
		Kind:    event.KindLLMToken,
		Actor:   "agent-OTHER",
		Payload: map[string]string{"text": "wrong-sub"},
	})
	// Matching — should be delivered.
	_, _ = b.PublishStreaming(event.Spec{
		Subject: "agent.01H.llm",
		Kind:    event.KindLLMToken,
		Actor:   "agent-01H",
		Payload: map[string]string{"text": "right-sub"},
	})

	select {
	case ev := <-sub.C:
		var p struct{ Text string }
		_ = json.Unmarshal(ev.Payload, &p)
		if p.Text != "right-sub" {
			t.Errorf("got wrong event: %q", p.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("matching event not delivered")
	}
	// And no extras.
	select {
	case ev := <-sub.C:
		t.Errorf("non-matching subject leaked through: %+v", ev)
	case <-time.After(100 * time.Millisecond):
	}
}

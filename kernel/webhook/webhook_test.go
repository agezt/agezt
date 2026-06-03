// SPDX-License-Identifier: MIT

package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

// capture is a thread-safe record of received webhook deliveries.
type capture struct {
	mu       sync.Mutex
	bodies   []string
	headers  []http.Header
	statuses []int // status to return per call (consumed in order; default 200)
	calls    int
}

func (c *capture) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.bodies = append(c.bodies, string(body))
		c.headers = append(c.headers, r.Header.Clone())
		status := http.StatusOK
		if c.calls < len(c.statuses) {
			status = c.statuses[c.calls]
		}
		c.calls++
		c.mu.Unlock()
		w.WriteHeader(status)
	}
}

func (c *capture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.bodies)
}

func newBus(t *testing.T) *bus.Bus {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })
	return b
}

// waitFor polls until cond is true or the deadline elapses.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within deadline")
}

func TestDispatch_DeliversMatchingEventSigned(t *testing.T) {
	cap := &capture{}
	srv := httptest.NewServer(cap.handler())
	defer srv.Close()

	b := newBus(t)
	d := NewDispatcher(b, []Sink{{URL: srv.URL, Subject: "agent.>", Secret: "topsecret"}}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)

	// An event that matches the sink subject.
	if _, err := b.Publish(event.Spec{Subject: "agent.run-1.task", Kind: event.KindTaskCompleted, Actor: "agent-1"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return cap.count() == 1 })

	cap.mu.Lock()
	defer cap.mu.Unlock()
	body := cap.bodies[0]
	hdr := cap.headers[0]
	if !strings.Contains(body, `"kind":"task.completed"`) {
		t.Errorf("body missing event kind: %s", body)
	}
	if hdr.Get("X-Agezt-Event") != "task.completed" {
		t.Errorf("X-Agezt-Event = %q", hdr.Get("X-Agezt-Event"))
	}
	if hdr.Get("X-Agezt-Delivery") == "" {
		t.Error("missing X-Agezt-Delivery id")
	}
	// Verify the HMAC signature over the exact body.
	got := hdr.Get("X-Agezt-Signature")
	mac := hmac.New(sha256.New, []byte("topsecret"))
	mac.Write([]byte(body))
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if got != want {
		t.Errorf("signature = %q, want %q", got, want)
	}
}

func TestDispatch_SubjectFilterExcludesNonMatching(t *testing.T) {
	cap := &capture{}
	srv := httptest.NewServer(cap.handler())
	defer srv.Close()

	b := newBus(t)
	d := NewDispatcher(b, []Sink{{URL: srv.URL, Subject: "edict.>"}}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)

	// Publish a non-matching event, then a matching one.
	_, _ = b.Publish(event.Spec{Subject: "agent.run-1.task", Kind: event.KindTaskCompleted, Actor: "a"})
	_, _ = b.Publish(event.Spec{Subject: "edict.approval.requested", Kind: event.KindPolicyDecision, Actor: "edict"})

	waitFor(t, func() bool { return cap.count() == 1 })
	time.Sleep(30 * time.Millisecond) // give a wrongly-matched delivery a chance to (not) arrive
	if cap.count() != 1 {
		t.Errorf("only the edict.> event should deliver, got %d deliveries", cap.count())
	}
	cap.mu.Lock()
	defer cap.mu.Unlock()
	if !strings.Contains(cap.bodies[0], "edict.approval.requested") {
		t.Errorf("delivered wrong event: %s", cap.bodies[0])
	}
}

func TestDispatch_RetriesThenSucceeds(t *testing.T) {
	cap := &capture{statuses: []int{500, 503, 200}} // fail twice, then succeed
	srv := httptest.NewServer(cap.handler())
	defer srv.Close()

	b := newBus(t)
	audit := newAuditSink(t, b)
	d := NewDispatcher(b, []Sink{{URL: srv.URL, Subject: ">"}}, nil)
	d.Backoff = func(int) time.Duration { return 0 } // no delay in tests
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)

	_, _ = b.Publish(event.Spec{Subject: "agent.x.task", Kind: event.KindTaskCompleted, Actor: "a"})
	waitFor(t, func() bool { return cap.count() == 3 })

	// A webhook.delivered audit event should have been journaled (attempts=3).
	waitFor(t, func() bool { return audit.has(event.KindWebhookDelivered) })
}

func TestDispatch_FailsAfterMaxAttempts(t *testing.T) {
	cap := &capture{statuses: []int{500, 500, 500, 500}}
	srv := httptest.NewServer(cap.handler())
	defer srv.Close()

	b := newBus(t)
	audit := newAuditSink(t, b)
	d := NewDispatcher(b, []Sink{{URL: srv.URL, Subject: ">"}}, nil)
	d.MaxAttempts = 3
	d.Backoff = func(int) time.Duration { return 0 }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)

	_, _ = b.Publish(event.Spec{Subject: "agent.x.task", Kind: event.KindTaskCompleted, Actor: "a"})
	waitFor(t, func() bool { return cap.count() == 3 }) // exactly MaxAttempts
	waitFor(t, func() bool { return audit.has(event.KindWebhookFailed) })
}

func TestDispatch_NeverRedeliversWebhookEvents(t *testing.T) {
	// A sink subscribed to ">" must NOT deliver the webhook.delivered audit
	// events it itself produces — otherwise every delivery loops forever.
	cap := &capture{}
	srv := httptest.NewServer(cap.handler())
	defer srv.Close()

	b := newBus(t)
	d := NewDispatcher(b, []Sink{{URL: srv.URL, Subject: ">"}}, nil)
	d.Backoff = func(int) time.Duration { return 0 }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)

	_, _ = b.Publish(event.Spec{Subject: "agent.x.task", Kind: event.KindTaskCompleted, Actor: "a"})
	// One original event → exactly one delivery. The webhook.delivered audit
	// event it spawns must not produce a second delivery.
	waitFor(t, func() bool { return cap.count() == 1 })
	time.Sleep(50 * time.Millisecond)
	if n := cap.count(); n != 1 {
		t.Errorf("expected exactly 1 delivery (no loop), got %d", n)
	}
}

// auditSink records webhook.* audit events as the dispatcher journals them.
// It must be created BEFORE publishing the triggering event (the bus is
// live-only).
type auditSink struct {
	mu   sync.Mutex
	seen map[event.Kind]bool
}

func newAuditSink(t *testing.T, b *bus.Bus) *auditSink {
	t.Helper()
	sub, err := b.Subscribe("webhook.>", 64)
	if err != nil {
		t.Fatalf("subscribe audit: %v", err)
	}
	a := &auditSink{seen: map[event.Kind]bool{}}
	go func() {
		for ev := range sub.C {
			a.mu.Lock()
			a.seen[ev.Kind] = true
			a.mu.Unlock()
		}
	}()
	t.Cleanup(sub.Cancel)
	return a
}

func (a *auditSink) has(k event.Kind) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.seen[k]
}

func TestParseSinks(t *testing.T) {
	sinks, err := ParseSinks("https://h/a|agent.>|sec, http://h2/b , https://h3/c|edict.>")
	if err != nil {
		t.Fatal(err)
	}
	if len(sinks) != 3 {
		t.Fatalf("got %d sinks", len(sinks))
	}
	if sinks[0].URL != "https://h/a" || sinks[0].Subject != "agent.>" || sinks[0].Secret != "sec" {
		t.Errorf("sink0 = %+v", sinks[0])
	}
	if sinks[1].Subject != ">" || sinks[1].Secret != "" { // defaults
		t.Errorf("sink1 defaults wrong: %+v", sinks[1])
	}
	if sinks[2].Subject != "edict.>" {
		t.Errorf("sink2 = %+v", sinks[2])
	}

	// Invalid URL is a hard error.
	if _, err := ParseSinks("not-a-url|agent.>"); err == nil {
		t.Error("expected error for non-http URL")
	}
	if _, err := ParseSinks("ftp://h/x"); err == nil {
		t.Error("expected error for non-http scheme")
	}
	// Empty spec → no sinks, no error.
	if s, err := ParseSinks("  "); err != nil || s != nil {
		t.Errorf("empty spec = %v, %v", s, err)
	}
}

// TestParseSinks_RejectsBadSubjectFilter ensures a malformed subject filter is a hard
// error at parse time — it would otherwise silently match nothing and deliver nothing
// (M217).
func TestParseSinks_RejectsBadSubjectFilter(t *testing.T) {
	bad := []string{
		"https://h/a|agent..tool", // empty token
		"https://h/a|>.agent",     // '>' not last
		"https://h/a|",            // empty filter is treated as default ">", so this is fine — covered below
	}
	// The first two are malformed; the third (empty after |) falls back to the default
	// ">", which is valid — assert that distinction.
	if _, err := ParseSinks(bad[0]); err == nil {
		t.Errorf("%q: expected error for empty token", bad[0])
	}
	if _, err := ParseSinks(bad[1]); err == nil {
		t.Errorf("%q: expected error for misplaced '>'", bad[1])
	}
	if sinks, err := ParseSinks(bad[2]); err != nil || len(sinks) != 1 || sinks[0].Subject != ">" {
		t.Errorf("%q: empty filter should default to \">\": sinks=%+v err=%v", bad[2], sinks, err)
	}
	// A valid explicit filter still parses.
	if _, err := ParseSinks("https://h/a|agent.*.tool"); err != nil {
		t.Errorf("a valid subject filter should parse: %v", err)
	}
}

// TestParseSinks_SecretWithPipe ensures an HMAC secret that contains '|' is preserved
// intact rather than truncated at the first pipe (M218) — otherwise every signature
// would mismatch at the receiver.
func TestParseSinks_SecretWithPipe(t *testing.T) {
	sinks, err := ParseSinks("https://h/a|agent.>|se|cr|et")
	if err != nil {
		t.Fatal(err)
	}
	if len(sinks) != 1 {
		t.Fatalf("got %d sinks", len(sinks))
	}
	if sinks[0].Secret != "se|cr|et" {
		t.Errorf("secret = %q, want %q (full value after the 2nd pipe)", sinks[0].Secret, "se|cr|et")
	}
	if sinks[0].Subject != "agent.>" {
		t.Errorf("subject = %q, want agent.>", sinks[0].Subject)
	}
}

func TestDescribe_RedactsSecret(t *testing.T) {
	out := Describe([]Sink{{URL: "https://h/a", Subject: "agent.>", Secret: "supersecret"}})
	if strings.Contains(out, "supersecret") {
		t.Errorf("Describe leaked the secret: %s", out)
	}
	if !strings.Contains(out, "(signed)") {
		t.Errorf("signed sink should be marked: %s", out)
	}
}

func TestProbe_Delivers(t *testing.T) {
	cap := &capture{}
	srv := httptest.NewServer(cap.handler())
	defer srv.Close()

	r := Probe(context.Background(), Sink{URL: srv.URL, Secret: "k"}, time.Unix(1700000000, 0), srv.Client())
	if !r.OK() {
		t.Fatalf("probe not OK: status=%d err=%q", r.Status, r.Err)
	}
	if r.Status != http.StatusOK {
		t.Errorf("status = %d want 200", r.Status)
	}
	if !r.Signed {
		t.Errorf("Signed should be true for a sink with a secret")
	}
	if cap.calls != 1 {
		t.Errorf("a probe must POST exactly once (no retry), got %d", cap.calls)
	}
	// Headers + signature mirror a real delivery.
	h := cap.headers[0]
	if got := h.Get("X-Agezt-Event"); got != TestEventKind {
		t.Errorf("X-Agezt-Event = %q want %q", got, TestEventKind)
	}
	if h.Get("X-Agezt-Delivery") == "" {
		t.Errorf("X-Agezt-Delivery should be set")
	}
	mac := hmac.New(sha256.New, []byte("k"))
	mac.Write([]byte(cap.bodies[0]))
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if got := h.Get("X-Agezt-Signature"); got != want {
		t.Errorf("signature = %q want %q", got, want)
	}
}

func TestProbe_Non2xxNoRetry(t *testing.T) {
	cap := &capture{statuses: []int{http.StatusInternalServerError, http.StatusInternalServerError}}
	srv := httptest.NewServer(cap.handler())
	defer srv.Close()

	r := Probe(context.Background(), Sink{URL: srv.URL}, time.Unix(1700000000, 0), srv.Client())
	if r.OK() {
		t.Errorf("500 should not be OK")
	}
	if r.Status != http.StatusInternalServerError {
		t.Errorf("status = %d want 500", r.Status)
	}
	if cap.calls != 1 {
		t.Errorf("probe must not retry, got %d calls", cap.calls)
	}
}

func TestProbe_ConnError(t *testing.T) {
	// Nothing listening on this port → transport error, reported in Err.
	r := Probe(context.Background(), Sink{URL: "http://127.0.0.1:1/hook"}, time.Unix(1700000000, 0), nil)
	if r.OK() || r.Err == "" {
		t.Errorf("connection failure should set Err and not be OK: %+v", r)
	}
}

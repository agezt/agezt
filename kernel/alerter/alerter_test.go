// SPDX-License-Identifier: MIT

package alerter

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/pulse"
)

// captureSink records delivered briefs, safely across goroutines.
type captureSink struct {
	mu     sync.Mutex
	briefs []pulse.Brief
}

func (c *captureSink) Deliver(b pulse.Brief) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.briefs = append(c.briefs, b)
	return nil
}

func (c *captureSink) all() []pulse.Brief {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]pulse.Brief(nil), c.briefs...)
}

func ev(kind event.Kind, corr string, payload map[string]any) *event.Event {
	e := &event.Event{Kind: kind, CorrelationID: corr}
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			panic(err)
		}
		e.Payload = raw
	}
	return e
}

func mustJSON(v map[string]any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}

// TestClassify_MirrorsConsoleAlertRules: the five notify-worthy kinds map to
// the console's titles/levels/sources; everything else is not an alert.
func TestClassify_MirrorsConsoleAlertRules(t *testing.T) {
	cases := []struct {
		name   string
		ev     *event.Event
		want   Alert
		wantOK bool
	}{
		{"task.failed", ev(event.KindTaskFailed, "c1", map[string]any{"reason": "boom"}),
			Alert{Kind: event.KindTaskFailed, Level: LevelWarning, Title: "run failed", Detail: "boom", Source: "run"}, true},
		{"task.failed error fallback", ev(event.KindTaskFailed, "c1", map[string]any{"error": "exploded"}),
			Alert{Kind: event.KindTaskFailed, Level: LevelWarning, Title: "run failed", Detail: "exploded", Source: "run"}, true},
		{"netguard.blocked", ev(event.KindNetguardBlocked, "c2", map[string]any{"ip": "10.0.0.1", "reason": "private", "tool": "http"}),
			Alert{Kind: event.KindNetguardBlocked, Level: LevelWarning, Title: "egress blocked", Detail: "10.0.0.1 — private", Source: "http"}, true},
		{"netguard.blocked no tool", ev(event.KindNetguardBlocked, "", nil),
			Alert{Kind: event.KindNetguardBlocked, Level: LevelWarning, Title: "egress blocked", Source: "egress"}, true},
		{"budget.exceeded", ev(event.KindBudgetExceeded, "c3", nil),
			Alert{Kind: event.KindBudgetExceeded, Level: LevelCritical, Title: "budget ceiling exceeded", Source: "budget"}, true},
		{"rate.limited", ev(event.KindRateLimited, "", map[string]any{"provider": "openai"}),
			Alert{Kind: event.KindRateLimited, Level: LevelWarning, Title: "provider rate-limited", Detail: "openai", Source: "provider"}, true},
		{"halt", ev(event.KindHalt, "", map[string]any{"reason": "operator"}),
			Alert{Kind: event.KindHalt, Level: LevelCritical, Title: "daemon halted", Detail: "operator", Source: "kernel"}, true},
		{"approval.requested", ev(event.KindApprovalRequested, "c4", map[string]any{"capability": "shell.exec", "reason": "install deps"}),
			Alert{Kind: event.KindApprovalRequested, Level: LevelWarning, Title: "approval needed", Detail: "shell.exec — install deps", Source: "approval"}, true},
		{"approval.requested tool fallback", ev(event.KindApprovalRequested, "c4", map[string]any{"tool_name": "browser"}),
			Alert{Kind: event.KindApprovalRequested, Level: LevelWarning, Title: "approval needed", Detail: "browser", Source: "approval"}, true},
		{"doctor.auto_repair failed degraded", &event.Event{
			Kind:    event.KindInfo,
			Subject: "doctor.auto_repair",
			Payload: mustJSON(map[string]any{"agent": "builder", "mode": "degraded", "phase": "failed", "error": "provider timeout"}),
		}, Alert{Kind: event.KindInfo, Level: LevelWarning, Title: "doctor run failed", Detail: "builder — provider timeout", Source: "doctor"}, true},
		{"doctor.auto_repair forced chain exhausted", &event.Event{
			Kind:    event.KindInfo,
			Subject: "doctor.auto_repair",
			Payload: mustJSON(map[string]any{
				"agent":        "builder",
				"mode":         "routing_forced_exhausted",
				"phase":        "routing_force_exhausted_detected",
				"reason":       "forced chain stayed under fallback pressure after generation 2",
				"root_agent":   "builder",
				"chain_depth":  1,
				"target_agent": "lead",
			}),
		}, Alert{Kind: event.KindInfo, Level: LevelWarning, Title: "forced chain exhausted", Detail: "builder — forced chain stayed under fallback pressure after generation 2 · root builder · hop 1 · next owner lead", Source: "doctor"}, true},
		{"tool.invoked is not an alert", ev(event.KindToolInvoked, "", nil), Alert{}, false},
		{"pulse observer.delta NOT handled here (pulse delivers its own)",
			&event.Event{Kind: event.Kind("observer.delta")}, Alert{}, false},
		{"briefing.sent NOT handled here", &event.Event{Kind: event.Kind("briefing.sent")}, Alert{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Classify(tc.ev)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestHandle_MinLevelGate: MinLevel=critical drops warnings, passes criticals.
func TestHandle_MinLevelGate(t *testing.T) {
	sink := &captureSink{}
	n := New(sink, Config{MinLevel: LevelCritical})
	if n.Handle(ev(event.KindTaskFailed, "c1", nil)) {
		t.Fatal("warning delivered despite MinLevel=critical")
	}
	if !n.Handle(ev(event.KindBudgetExceeded, "c1", nil)) {
		t.Fatal("critical not delivered")
	}
	if got := len(sink.all()); got != 1 {
		t.Fatalf("delivered %d briefs, want 1", got)
	}
}

// TestHandle_DedupCooldown: the same kind+correlation inside the cooldown is
// sent once; a different correlation or an elapsed cooldown sends again.
func TestHandle_DedupCooldown(t *testing.T) {
	sink := &captureSink{}
	n := New(sink, Config{Cooldown: time.Minute})
	clock := time.Unix(1000, 0)
	n.now = func() time.Time { return clock }

	if !n.Handle(ev(event.KindTaskFailed, "run-1", nil)) {
		t.Fatal("first alert not delivered")
	}
	if n.Handle(ev(event.KindTaskFailed, "run-1", nil)) {
		t.Fatal("duplicate inside cooldown delivered")
	}
	if !n.Handle(ev(event.KindTaskFailed, "run-2", nil)) {
		t.Fatal("different correlation suppressed")
	}
	clock = clock.Add(61 * time.Second)
	if !n.Handle(ev(event.KindTaskFailed, "run-1", nil)) {
		t.Fatal("alert after cooldown elapsed suppressed")
	}
	if got := len(sink.all()); got != 3 {
		t.Fatalf("delivered %d briefs, want 3", got)
	}
}

func TestHandle_DoctorFailureDedupesByAgentFingerprint(t *testing.T) {
	sink := &captureSink{}
	n := New(sink, Config{Cooldown: time.Minute})
	clock := time.Unix(1000, 0)
	n.now = func() time.Time { return clock }

	docFail := func(agent, fp string) *event.Event {
		return &event.Event{
			Kind:    event.KindInfo,
			Subject: "doctor.auto_repair",
			Payload: mustJSON(map[string]any{
				"agent": agent, "mode": "degraded", "phase": "failed", "fingerprint": fp, "error": "provider timeout",
			}),
		}
	}

	if !n.Handle(docFail("builder", "fp-1")) {
		t.Fatal("first doctor failure not delivered")
	}
	if n.Handle(docFail("builder", "fp-1")) {
		t.Fatal("same agent+fingerprint inside cooldown delivered")
	}
	if !n.Handle(docFail("writer", "fp-1")) {
		t.Fatal("different agent should not collide on dedupe")
	}
	if got := len(sink.all()); got != 2 {
		t.Fatalf("delivered %d briefs, want 2", got)
	}
}

// TestHandle_RateCap: a flood of DISTINCT alerts is capped per window, and the
// cap frees up once the window slides past the burst.
func TestHandle_RateCap(t *testing.T) {
	sink := &captureSink{}
	n := New(sink, Config{MaxPerWindow: 3, Window: 10 * time.Minute})
	clock := time.Unix(1000, 0)
	n.now = func() time.Time { return clock }

	delivered := 0
	for i := range 8 {
		clock = clock.Add(time.Second)
		if n.Handle(ev(event.KindTaskFailed, "run-"+string(rune('a'+i)), nil)) {
			delivered++
		}
	}
	if delivered != 3 {
		t.Fatalf("delivered %d, want exactly the cap 3", delivered)
	}
	clock = clock.Add(11 * time.Minute) // burst slides out of the window
	if !n.Handle(ev(event.KindTaskFailed, "run-z", nil)) {
		t.Fatal("delivery after the window cleared was suppressed")
	}
}

// TestBrief_RendersSeverityAndDisposition: criticals get the siren, warnings
// the caution sign; every brief is DispAlert (breaks through digests/quiet
// hours) with the correlation threaded for `agt why`.
func TestBrief_RendersSeverityAndDisposition(t *testing.T) {
	sink := &captureSink{}
	n := New(sink, Config{})
	n.Handle(ev(event.KindHalt, "corr-7", map[string]any{"reason": "operator"}))
	n.Handle(ev(event.KindTaskFailed, "corr-8", map[string]any{"reason": "boom"}))
	briefs := sink.all()
	if len(briefs) != 2 {
		t.Fatalf("delivered %d briefs, want 2", len(briefs))
	}
	crit, warn := briefs[0], briefs[1]
	if crit.Title != "🚨 daemon halted" {
		t.Fatalf("critical title = %q", crit.Title)
	}
	if warn.Title != "⚠ run failed" {
		t.Fatalf("warning title = %q", warn.Title)
	}
	for _, b := range briefs {
		if b.Disposition != pulse.DispAlert {
			t.Fatalf("disposition = %q, want %q", b.Disposition, pulse.DispAlert)
		}
	}
	if crit.CorrelationID != "corr-7" || crit.IssueKey != "alert/halt" {
		t.Fatalf("correlation/issue = %q/%q", crit.CorrelationID, crit.IssueKey)
	}
	if crit.Body != "operator\nsource: kernel" {
		t.Fatalf("critical body = %q", crit.Body)
	}
}

func newBus(t *testing.T) *bus.Bus {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { _ = j.Close() })
	return bus.New(j)
}

// TestStart_DeliversFromRealBus: end to end on a real bus — a published
// task.failed reaches the sink; non-alert events do not.
func TestStart_DeliversFromRealBus(t *testing.T) {
	b := newBus(t)
	sink := &captureSink{}
	if !Start(t.Context(), b, sink, Config{}) {
		t.Fatal("Start returned false")
	}

	if _, err := b.Publish(event.Spec{Subject: "agent.run-1.task", Kind: event.KindToolInvoked,
		Actor: "agent", Payload: map[string]any{"name": "shell"}}); err != nil {
		t.Fatalf("publish tool.invoked: %v", err)
	}
	if _, err := b.Publish(event.Spec{Subject: "agent.run-1.task", Kind: event.KindTaskFailed,
		Actor: "agent", CorrelationID: "run-1", Payload: map[string]any{"reason": "boom"}}); err != nil {
		t.Fatalf("publish task.failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if briefs := sink.all(); len(briefs) >= 1 {
			if briefs[0].Title != "⚠ run failed" || briefs[0].CorrelationID != "run-1" {
				t.Fatalf("unexpected brief: %+v", briefs[0])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("alert never reached the sink")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := len(sink.all()); got != 1 {
		t.Fatalf("delivered %d briefs, want 1 (tool.invoked must not notify)", got)
	}
}

// TestStart_NilGuards: nothing starts without a bus or a sink.
func TestStart_NilGuards(t *testing.T) {
	if Start(context.Background(), nil, &captureSink{}, Config{}) {
		t.Fatal("started without a bus")
	}
	if Start(context.Background(), newBus(t), nil, Config{}) {
		t.Fatal("started without a sink")
	}
}

// TestHandle_MuteWindow (M815): warnings are held during the mute window;
// criticals still break through; outside the window everything flows.
func TestHandle_MuteWindow(t *testing.T) {
	sink := &captureSink{}
	// Mute 0..7 (e.g. overnight).
	n := New(sink, Config{Mute: pulse.ParseQuietHours("0-7")})

	at := func(hour int) time.Time { return time.Date(2026, 6, 10, hour, 0, 0, 0, time.UTC) }

	// 03:00, inside the window: a warning is muted, a critical breaks through.
	n.now = func() time.Time { return at(3) }
	if n.Handle(ev(event.KindTaskFailed, "c1", nil)) {
		t.Fatal("warning delivered inside the mute window")
	}
	if !n.Handle(ev(event.KindHalt, "c2", nil)) {
		t.Fatal("critical suppressed by the mute window — criticals must break through")
	}

	// 09:00, outside the window: the warning flows.
	n.now = func() time.Time { return at(9) }
	if !n.Handle(ev(event.KindTaskFailed, "c3", nil)) {
		t.Fatal("warning suppressed outside the mute window")
	}
	if got := len(sink.all()); got != 2 {
		t.Fatalf("delivered %d, want 2 (critical + post-window warning)", got)
	}
}

// TestHandle_MuteSources (M815): a muted category never notifies, at any
// level; unmuted categories still flow.
func TestHandle_MuteSources(t *testing.T) {
	sink := &captureSink{}
	n := New(sink, Config{MuteSources: ParseMuteSources("provider, kernel")})

	// provider (rate-limit, warning) and kernel (halt, CRITICAL) are muted.
	if n.Handle(ev(event.KindRateLimited, "c1", map[string]any{"provider": "x"})) {
		t.Fatal("muted provider source delivered")
	}
	if n.Handle(ev(event.KindHalt, "c2", nil)) {
		t.Fatal("muted kernel source delivered even at critical")
	}
	// budget (critical) and run (warning) are NOT muted.
	if !n.Handle(ev(event.KindBudgetExceeded, "c3", nil)) {
		t.Fatal("unmuted budget source suppressed")
	}
	if !n.Handle(ev(event.KindTaskFailed, "c4", nil)) {
		t.Fatal("unmuted run source suppressed")
	}
	if got := len(sink.all()); got != 2 {
		t.Fatalf("delivered %d, want 2", got)
	}
}

// TestParseMuteSources: comma/space tolerant, lowercased, empty → nil.
func TestParseMuteSources(t *testing.T) {
	got := ParseMuteSources(" Provider, Kernel  egress ")
	for _, want := range []string{"provider", "kernel", "egress"} {
		if !got[want] {
			t.Fatalf("missing %q in %v", want, got)
		}
	}
	if ParseMuteSources("") != nil || ParseMuteSources("  ,  ") != nil {
		t.Fatal("empty input must yield nil")
	}
}

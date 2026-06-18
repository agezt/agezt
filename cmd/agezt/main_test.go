// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestDeliverScheduled — a scheduled run's answer is broadcast to every configured
// channel recipient, prefixed with the schedule id; empty answers are skipped (M152).
func TestDeliverScheduled(t *testing.T) {
	var calls []string // "kind/id:text"
	send := func(_ context.Context, kind, id, text string) error {
		calls = append(calls, kind+"/"+id+":"+text)
		return nil
	}
	targets := map[string][]string{"slack": {"C1", "C2"}, "discord": {"D1"}}

	n := deliverScheduled(context.Background(), send, targets, "morning-digest", "Here is your summary.")
	if n != 3 {
		t.Fatalf("delivered to %d recipients, want 3", n)
	}
	for _, c := range calls {
		if !strings.Contains(c, "[scheduled: morning-digest]") || !strings.Contains(c, "Here is your summary.") {
			t.Errorf("delivery missing id prefix or answer: %q", c)
		}
	}

	// Empty answer → no delivery.
	calls = nil
	if n := deliverScheduled(context.Background(), send, targets, "x", "   "); n != 0 || len(calls) != 0 {
		t.Errorf("empty answer should not deliver; n=%d calls=%v", n, calls)
	}
	// Nil sender → no panic, no delivery.
	if n := deliverScheduled(context.Background(), nil, targets, "x", "hi"); n != 0 {
		t.Errorf("nil sender should deliver nothing, got %d", n)
	}
}

// TestCollectChannels — env-driven channel inventory for `agt status` (M141):
// only token-set channels appear, and Inbound reflects whether a webhook channel
// is fully configured (addr + secret/public key) vs. half-configured.
func TestCollectChannels(t *testing.T) {
	// No tokens → empty.
	if got := collectChannels(); len(got) != 0 {
		t.Fatalf("no tokens should yield 0 channels, got %d", len(got))
	}

	t.Setenv(brand.EnvPrefix+"TELEGRAM_TOKEN", "tg")
	t.Setenv(brand.EnvPrefix+"TELEGRAM_CHAT_ID", "111,222")
	t.Setenv(brand.EnvPrefix+"SLACK_TOKEN", "xoxb")
	t.Setenv(brand.EnvPrefix+"SLACK_ADDR", "127.0.0.1:8840")
	// SLACK_SIGNING_SECRET intentionally unset → Slack inbound half-configured.
	t.Setenv(brand.EnvPrefix+"SLACK_CHANNELS", "C1")
	t.Setenv(brand.EnvPrefix+"DISCORD_TOKEN", "bot")
	t.Setenv(brand.EnvPrefix+"DISCORD_ADDR", "127.0.0.1:8850")
	t.Setenv(brand.EnvPrefix+"DISCORD_PUBLIC_KEY", "deadbeef")
	t.Setenv(brand.EnvPrefix+"DISCORD_CHANNELS", "D1,D2,D3")

	got := collectChannels()
	if len(got) != 3 {
		t.Fatalf("expected 3 channels, got %d: %+v", len(got), got)
	}
	type info struct {
		inbound   bool
		addr      string
		allowlist int
	}
	by := map[string]info{}
	for _, c := range got {
		by[c.Kind] = info{c.Inbound, c.Addr, c.Allowlist}
	}
	if tg := by["telegram"]; !tg.inbound || tg.allowlist != 2 {
		t.Errorf("telegram = %+v want inbound, allow 2", tg)
	}
	if sl := by["slack"]; sl.inbound || sl.addr != "127.0.0.1:8840" || sl.allowlist != 1 {
		t.Errorf("slack = %+v want NOT inbound (no signing secret), addr set, allow 1", sl)
	}
	if dc := by["discord"]; !dc.inbound || dc.addr != "127.0.0.1:8850" || dc.allowlist != 3 {
		t.Errorf("discord = %+v want inbound, addr set, allow 3", dc)
	}
}

func TestMakeChannelHandlerRunsGovernedAgentUnderChannelCorrelation(t *testing.T) {
	k, err := kernelruntime.Open(kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("channel handled")),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	reply, err := makeChannelHandler(k)(context.Background(), channel.UnifiedMessage{
		ChannelKind: "webhook",
		ChannelID:   "room-1",
		Sender:      "ersin",
		Text:        "check the mailbox",
	}, "chan-corr-1")
	if err != nil {
		t.Fatalf("channel handler: %v", err)
	}
	if reply != "channel handled" {
		t.Fatalf("reply = %q, want channel handled", reply)
	}

	var sawReceived, sawCompleted bool
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.CorrelationID != "chan-corr-1" {
			return nil
		}
		switch e.Kind {
		case event.KindTaskReceived:
			sawReceived = true
			var payload map[string]any
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				t.Fatalf("task.received payload: %v", err)
			}
			if payload["intent"] != "check the mailbox" {
				t.Fatalf("channel task intent = %v, want check the mailbox", payload["intent"])
			}
		case event.KindTaskCompleted:
			sawCompleted = true
		}
		return nil
	})
	if !sawReceived || !sawCompleted {
		t.Fatalf("channel handler did not journal governed run lifecycle: received=%v completed=%v", sawReceived, sawCompleted)
	}
}

// TestDelegationBanner — the boot banner reflects the active delegation ceilings
// (M58): "off" when disabled, the effective caps otherwise (M49 source).
func TestDelegationBanner(t *testing.T) {
	open := func(t *testing.T, cfg kernelruntime.Config) *kernelruntime.Kernel {
		cfg.BaseDir = t.TempDir()
		cfg.Provider = mock.New(mock.FinalText("ok"))
		k, err := kernelruntime.Open(cfg)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(func() { k.Close() })
		return k
	}

	off := open(t, kernelruntime.Config{}) // SubAgentTool false
	if got := delegationBanner(off); !strings.HasPrefix(got, "off") {
		t.Errorf("disabled banner = %q, want off…", got)
	}

	capped := open(t, kernelruntime.Config{
		Tools:                      map[string]agent.Tool{},
		SubAgentTool:               true,
		SubAgentMaxFanout:          3,
		SubAgentMaxSpendMicrocents: 500_000_000, // $0.50
	})
	got := delegationBanner(capped)
	for _, want := range []string{"depth≤1", "fan-out ≤3", "spend $0.5000"} {
		if !strings.Contains(got, want) {
			t.Errorf("capped banner = %q, missing %q", got, want)
		}
	}

	unbounded := open(t, kernelruntime.Config{SubAgentTool: true})
	if got := delegationBanner(unbounded); !strings.Contains(got, "fan-out unbounded") || !strings.Contains(got, "spend unbounded") {
		t.Errorf("unbounded banner = %q, want unbounded fan-out + spend", got)
	}
}

func TestRunVersion(t *testing.T) {
	for _, flag := range []string{"-v", "--version", "version"} {
		var out, errOut bytes.Buffer
		code := run([]string{flag}, &out, &errOut)
		if code != 0 {
			t.Errorf("%s: exit=%d want 0; stderr=%q", flag, code, errOut.String())
		}
		if !strings.Contains(out.String(), brand.Version) {
			t.Errorf("%s: stdout missing version %q; got %q", flag, brand.Version, out.String())
		}
		if !strings.Contains(out.String(), brand.Binary) {
			t.Errorf("%s: stdout missing binary name %q; got %q", flag, brand.Binary, out.String())
		}
	}
}

func TestRunHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "usage:") {
		t.Errorf("help missing 'usage:'; got %q", out.String())
	}
	if !strings.Contains(out.String(), "ANTHROPIC_API_KEY") {
		t.Errorf("help missing ANTHROPIC_API_KEY note; got %q", out.String())
	}
}

func TestRunUnknown(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"bogus"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unknown command") {
		t.Errorf("stderr missing error; got %q", errOut.String())
	}
}

// Note: runDaemon needs a real ANTHROPIC_API_KEY to start, so we don't
// exercise it here. The end-to-end test under kernel/controlplane covers
// the same wire format with a mock provider.

func TestModelAdvisory(t *testing.T) {
	cat := catalog.NewEmpty()
	cat.Providers["acme"] = &catalog.Provider{
		ID: "acme", NPM: "@ai-sdk/openai-compatible",
		Models: map[string]*catalog.Model{
			"mini":  {ID: "mini", ToolCall: false, Limit: catalog.Limit{Context: 32768}},
			"large": {ID: "large", ToolCall: true, Limit: catalog.Limit{Context: 200000}},
		},
	}
	// Tool-less model → advisory mentions tool-use.
	if adv := modelAdvisory(cat, "mini"); !strings.Contains(adv, "tool-use") {
		t.Errorf("mini advisory should mention tool-use; got %q", adv)
	}
	// Tool-capable model → no advisory.
	if adv := modelAdvisory(cat, "large"); adv != "" {
		t.Errorf("large advisory should be empty; got %q", adv)
	}
	// Unknown model / mock / empty → no false alarm.
	for _, m := range []string{"", "mock", "not-in-catalog"} {
		if adv := modelAdvisory(cat, m); adv != "" {
			t.Errorf("modelAdvisory(%q) should be empty; got %q", m, adv)
		}
	}
	if adv := modelAdvisory(nil, "mini"); adv != "" {
		t.Errorf("nil catalog should yield no advisory; got %q", adv)
	}
}

// TestBuildGovernor_UnconfiguredWhenNoProvider: with no AGEZT_PROVIDER and no
// credentialed catalog, the daemon must boot the "unconfigured" sentinel — NOT a
// mock and NOT an auto-picked provider — and must surface no default run model.
// A run then fails fast with the actionable "no LLM provider configured" error
// rather than returning a silent mock answer. This pins the owner's
// "hiçbir default provider/model" rule at the boot layer.
func TestBuildGovernor_UnconfiguredWhenNoProvider(t *testing.T) {
	t.Setenv(brand.EnvPrefix+"PROVIDER", "")
	t.Setenv(brand.EnvPrefix+"MODEL", "")

	gov, desc, model, err := buildGovernor(catalog.NewEmpty(), func(string) string { return "" })
	if err != nil {
		t.Fatalf("buildGovernor: %v", err)
	}
	if model != "" {
		t.Errorf("run model = %q, want empty (no built-in default model)", model)
	}
	if !strings.Contains(desc, "unconfigured") {
		t.Errorf("banner desc = %q, want it to mention unconfigured", desc)
	}

	// A run with a model still fails — there is no provider behind it, and no mock
	// fallback to silently answer.
	_, rerr := gov.Complete(context.Background(), agent.CompletionRequest{
		Model:    "anything",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	})
	if rerr == nil {
		t.Fatal("unconfigured daemon answered a run; want a hard error")
	}
	if !strings.Contains(rerr.Error(), "no LLM provider configured") {
		t.Errorf("err = %v, want it to mention 'no LLM provider configured'", rerr)
	}
}

// TestSelectPrimary_UnknownProviderIsHardError: an explicit but unknown
// AGEZT_PROVIDER is a loud error, never a silent degrade to mock.
func TestSelectPrimary_UnknownProviderIsHardError(t *testing.T) {
	t.Setenv(brand.EnvPrefix+"PROVIDER", "does-not-exist")
	t.Setenv(brand.EnvPrefix+"MODEL", "")
	if _, _, _, _, err := selectPrimary(catalog.NewEmpty(), func(string) string { return "" }); err == nil {
		t.Fatal("unknown provider id should be a hard error")
	}
}

func TestRunScan_Orphans(t *testing.T) {
	ev := func(kind event.Kind, corr string, ts int64, intent string) *event.Event {
		e := &event.Event{Kind: kind, CorrelationID: corr, TSUnixMS: ts}
		if intent != "" {
			e.Payload = []byte(`{"intent":"` + intent + `"}`)
		}
		return e
	}
	s := newRunScan()
	// run A: received + completed → NOT orphaned.
	s.observe(ev(event.KindTaskReceived, "A", 100, "alpha"))
	s.observe(ev(event.KindTaskCompleted, "A", 200, ""))
	// run B: received only → orphaned.
	s.observe(ev(event.KindTaskReceived, "B", 150, "beta"))
	// run C: received + abandoned (already reconciled) → NOT orphaned (idempotent).
	s.observe(ev(event.KindTaskReceived, "C", 120, "gamma"))
	s.observe(ev(event.KindTaskAbandoned, "C", 300, ""))
	// run D: received only, earlier than B → orphaned, sorts first.
	s.observe(ev(event.KindTaskReceived, "D", 110, "delta"))
	// run E: received + failed (errored out live, M30) → terminal, NOT
	// orphaned (the run already has a terminal event; abandoning it would
	// double-mark it).
	s.observe(ev(event.KindTaskReceived, "E", 130, "epsilon"))
	s.observe(ev(event.KindTaskFailed, "E", 140, ""))

	orphans := s.orphans()
	if len(orphans) != 2 {
		t.Fatalf("got %d orphans, want 2 (B,D): %+v", len(orphans), orphans)
	}
	// Sorted by StartedMS: D(110) before B(150).
	if orphans[0].Corr != "D" || orphans[1].Corr != "B" {
		t.Errorf("orphan order = %s,%s want D,B", orphans[0].Corr, orphans[1].Corr)
	}
	if orphans[0].Intent != "delta" {
		t.Errorf("orphan intent = %q want delta", orphans[0].Intent)
	}
	// Explicit: the failed run E must never appear as an orphan.
	for _, o := range orphans {
		if o.Corr == "E" {
			t.Errorf("failed run E was abandoned; task.failed must be terminal")
		}
	}
}

func TestRunScan_Empty(t *testing.T) {
	if got := newRunScan().orphans(); len(got) != 0 {
		t.Errorf("empty scan should yield no orphans; got %v", got)
	}
}

// TestDrainWait covers the graceful-shutdown drain helper (M136): it returns
// promptly when nothing is in flight, waits and succeeds when runs finish, and
// times out (false) when they don't.
func TestDrainWait(t *testing.T) {
	// Nothing in flight → drained immediately.
	if !drainWait(func() int { return 0 }, time.Second) {
		t.Errorf("drainWait with 0 active should return true")
	}

	// Active decrements to 0 across polls → drained true.
	n := 3
	if !drainWait(func() int {
		if n > 0 {
			n--
		}
		return n
	}, 2*time.Second) {
		t.Errorf("drainWait should return true once active reaches 0")
	}

	// Always busy + tiny timeout → drain times out (false).
	if drainWait(func() int { return 5 }, 150*time.Millisecond) {
		t.Errorf("drainWait should time out (false) when runs never finish")
	}

	// timeout<=0 means don't wait: false while busy, true when idle.
	if drainWait(func() int { return 2 }, 0) {
		t.Errorf("drainWait(_, 0) should be false while busy")
	}
	if !drainWait(func() int { return 0 }, 0) {
		t.Errorf("drainWait(_, 0) should be true when idle")
	}
}

func TestIsLoopback_ClassifiesExposureCorrectly(t *testing.T) {
	// isLoopback drives the "reachable beyond localhost" exposure warning shown
	// when the web UI / control plane / REST API binds to a public address. A
	// regression that classified 0.0.0.0 or an empty host as loopback would
	// silently suppress the warning and let an operator expose the daemon. Pin the
	// security-critical cases.
	loopback := []string{
		"127.0.0.1:8800", "localhost:8800", "[::1]:8800",
		"127.0.0.1", "127.0.0.53:8800", "::1",
	}
	exposed := []string{
		"0.0.0.0:8800", // binds every interface — the classic mistake
		":8800",        // empty host = every interface
		"0.0.0.0",
		"192.168.1.5:8800", // LAN
		"10.0.0.1:8800",    // private
		"203.0.113.7:8800", // public
		"example.com:8800", // hostname (conservatively not loopback)
		"",
	}
	for _, a := range loopback {
		if !isLoopback(a) {
			t.Errorf("isLoopback(%q) = false, want true (loopback-only bind)", a)
		}
	}
	for _, a := range exposed {
		if isLoopback(a) {
			t.Errorf("isLoopback(%q) = true, want false (reachable beyond localhost)", a)
		}
	}
}

func TestBoardSubjectSlug(t *testing.T) {
	cases := map[string]string{
		"handoff":          "handoff",
		"Acil Müdahale!":   "acil-m-dahale", // non-ascii + symbols collapse to dashes
		"gunluk-brifing":   "gunluk-brifing",
		"  spaced  topic ": "spaced-topic",
		"a.b.c":            "a-b-c", // dots can't appear inside a subject segment
		"":                 "untopiced",
		"!!!":              "untopiced",
	}
	for in, want := range cases {
		if got := boardSubjectSlug(in); got != want {
			t.Errorf("boardSubjectSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

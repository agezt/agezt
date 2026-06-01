// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/event"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

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

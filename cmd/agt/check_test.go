// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/catalog"
)

func TestParseCheckFlags(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		want    checkFlags
		wantErr bool
	}{
		{name: "empty", args: nil, want: checkFlags{}},
		{name: "id only", args: []string{"anthropic"}, want: checkFlags{providerID: "anthropic"}},
		{name: "lowercases id", args: []string{"Anthropic"}, want: checkFlags{providerID: "anthropic"}},
		{name: "--all", args: []string{"--all"}, want: checkFlags{all: true}},
		{name: "-a short", args: []string{"-a"}, want: checkFlags{all: true}},
		{name: "--json", args: []string{"--json"}, want: checkFlags{jsonOut: true}},
		{name: "-j short", args: []string{"-j"}, want: checkFlags{jsonOut: true}},
		{name: "--stream", args: []string{"--stream"}, want: checkFlags{stream: true}},
		{name: "-s short", args: []string{"-s"}, want: checkFlags{stream: true}},
		{name: "--bench 5", args: []string{"--bench", "5"}, want: checkFlags{bench: 5}},
		{name: "--bench=5 equals form", args: []string{"--bench=5"}, want: checkFlags{bench: 5}},
		{name: "combined --all --json --bench 3",
			args: []string{"--all", "--json", "--bench", "3"},
			want: checkFlags{all: true, jsonOut: true, bench: 3}},
		{name: "id + flags",
			args: []string{"openai", "--bench", "4", "--json"},
			want: checkFlags{providerID: "openai", bench: 4, jsonOut: true}},

		{name: "--bench missing arg", args: []string{"--bench"}, wantErr: true},
		{name: "--bench non-int", args: []string{"--bench", "abc"}, wantErr: true},
		{name: "--bench 1 too small", args: []string{"--bench", "1"}, wantErr: true},
		{name: "--bench=0 too small", args: []string{"--bench=0"}, wantErr: true},
		{name: "unknown flag", args: []string{"--unknown"}, wantErr: true},
		{name: "two positional args", args: []string{"a", "b"}, wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseCheckFlags(c.args)
			if (err != nil) != c.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, c.wantErr)
			}
			if c.wantErr {
				return
			}
			if got != c.want {
				t.Errorf("got %+v want %+v", got, c.want)
			}
		})
	}
}

func TestComputeLatencyStats(t *testing.T) {
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	cases := []struct {
		name  string
		input []time.Duration
		want  latencyStats
	}{
		{
			name: "empty -> zero",
			want: latencyStats{},
		},
		{
			name:  "single value",
			input: []time.Duration{ms(100)},
			want:  latencyStats{min: ms(100), p50: ms(100), p95: ms(100), max: ms(100)},
		},
		{
			// 10 values: 10,20,30,40,50,60,70,80,90,100
			// nearest-rank p50: index ceil(0.5*10)=5 → sorted[4] = 50ms
			// nearest-rank p95: index ceil(0.95*10)=10 → sorted[9] = 100ms
			name:  "ten ascending",
			input: []time.Duration{ms(10), ms(20), ms(30), ms(40), ms(50), ms(60), ms(70), ms(80), ms(90), ms(100)},
			want:  latencyStats{min: ms(10), p50: ms(50), p95: ms(100), max: ms(100)},
		},
		{
			// Pre-shuffled input must yield the same stats — proves the
			// sort happens.
			name:  "unsorted input",
			input: []time.Duration{ms(100), ms(10), ms(50), ms(20), ms(80)},
			want:  latencyStats{min: ms(10), p50: ms(50), p95: ms(100), max: ms(100)},
		},
		{
			name:  "three values",
			input: []time.Duration{ms(30), ms(10), ms(20)},
			want:  latencyStats{min: ms(10), p50: ms(20), p95: ms(30), max: ms(30)},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := computeLatencyStats(c.input)
			if got != c.want {
				t.Errorf("got %+v want %+v", got, c.want)
			}
		})
	}
}

func TestProbeToJSON(t *testing.T) {
	entry := &catalog.Provider{
		ID:  "anthropic",
		NPM: "@ai-sdk/anthropic",
		Models: map[string]*catalog.Model{
			"claude-3-5-haiku-20241022": {
				ID:   "claude-3-5-haiku-20241022",
				Cost: &catalog.Cost{Input: 0.80, Output: 4.00},
			},
		},
	}
	res := probeResult{
		modelID:        "claude-3-5-haiku-20241022",
		model:          entry.Models["claude-3-5-haiku-20241022"],
		reply:          "pong",
		latency:        53 * time.Millisecond,
		stopReason:     "end_turn",
		usage:          agent.Usage{InputTokens: 12, OutputTokens: 3},
		costMicrocents: 21600,
	}
	jp := probeToJSON(entry, res)
	if !jp.OK {
		t.Error("expected OK=true")
	}
	if jp.Provider != "anthropic" || jp.Model != "claude-3-5-haiku-20241022" {
		t.Errorf("provider/model wrong: %+v", jp)
	}
	if jp.Family != "anthropic" {
		t.Errorf("family=%q want 'anthropic'", jp.Family)
	}
	if jp.LatencyMS != 53 {
		t.Errorf("LatencyMS=%d want 53", jp.LatencyMS)
	}
	if jp.CostMicrocents != 21600 {
		t.Errorf("CostMicrocents=%d want 21600", jp.CostMicrocents)
	}
	if jp.InputTokens != 12 || jp.OutputTokens != 3 {
		t.Errorf("tokens wrong: in=%d out=%d", jp.InputTokens, jp.OutputTokens)
	}
	if jp.Reply != "pong" || jp.StopReason != "end_turn" {
		t.Errorf("reply/stop wrong: %q %q", jp.Reply, jp.StopReason)
	}
	if jp.Error != "" {
		t.Errorf("expected no error, got %q", jp.Error)
	}
}

func TestProbeToJSON_Failure(t *testing.T) {
	entry := &catalog.Provider{ID: "groq", NPM: "@ai-sdk/groq"}
	res := probeResult{
		modelID: "llama-3.3-70b",
		latency: 312 * time.Millisecond,
		err:     errFixed("401 Unauthorized"),
	}
	jp := probeToJSON(entry, res)
	if jp.OK {
		t.Error("expected OK=false")
	}
	if jp.Error != "401 Unauthorized" {
		t.Errorf("Error=%q want '401 Unauthorized'", jp.Error)
	}
	if jp.Reply != "" || jp.StopReason != "" || jp.InputTokens != 0 {
		t.Errorf("failure record should not carry response fields: %+v", jp)
	}
}

func TestEmitJSON_RoundTrip(t *testing.T) {
	probes := []jsonProbe{
		{
			Provider: "anthropic", Family: "anthropic",
			Model: "claude-3-5-haiku-20241022", OK: true,
			Reply: "pong", StopReason: "end_turn",
			InputTokens: 12, OutputTokens: 3,
			LatencyMS: 53, CostMicrocents: 21600,
		},
	}
	sum := jsonSummary{Total: 1, OK: 1}
	var buf bytes.Buffer
	if rc := emitJSON(probes, sum, &buf); rc != 0 {
		t.Fatalf("emitJSON rc=%d", rc)
	}
	var got jsonOutput
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\noutput:\n%s", err, buf.String())
	}
	if len(got.Probes) != 1 || got.Probes[0].Provider != "anthropic" {
		t.Errorf("probes round-trip wrong: %+v", got)
	}
	if got.Summary.Total != 1 || got.Summary.OK != 1 || got.Summary.Failed != 0 {
		t.Errorf("summary round-trip wrong: %+v", got.Summary)
	}
	// Schema guard: presence of headline keys protects scripted CI gates.
	for _, key := range []string{`"probes"`, `"summary"`, `"latency_ms"`, `"cost_microcents"`} {
		if !strings.Contains(buf.String(), key) {
			t.Errorf("JSON output missing key %s\n%s", key, buf.String())
		}
	}
}

func TestBenchToJSON(t *testing.T) {
	entry := &catalog.Provider{ID: "openai", NPM: "@ai-sdk/openai"}
	b := benchResult{
		entry:      entry,
		modelID:    "gpt-4o-mini",
		iterations: 5,
		successes:  5,
		failures:   0,
		totalCost:  22500, // 5 × 4500 mc
		stats: latencyStats{
			min: 80 * time.Millisecond,
			p50: 120 * time.Millisecond,
			p95: 200 * time.Millisecond,
			max: 250 * time.Millisecond,
		},
	}
	jp := b.toJSON()
	if jp.Bench == nil {
		t.Fatal("expected Bench block, got nil")
	}
	if jp.Bench.Iterations != 5 || jp.Bench.Successes != 5 || jp.Bench.Failures != 0 {
		t.Errorf("counts wrong: %+v", jp.Bench)
	}
	if jp.Bench.MinMS != 80 || jp.Bench.P50MS != 120 || jp.Bench.P95MS != 200 || jp.Bench.MaxMS != 250 {
		t.Errorf("latency stats wrong: %+v", jp.Bench)
	}
	if jp.LatencyMS != 120 {
		t.Errorf("headline LatencyMS=%d want 120 (p50)", jp.LatencyMS)
	}
	if jp.CostMicrocents != 22500 {
		t.Errorf("CostMicrocents=%d want 22500", jp.CostMicrocents)
	}
}

// errFixed is a helper for tests that want a stable error string.
type errFixed string

func (e errFixed) Error() string { return string(e) }

func TestComputeCostMicrocents(t *testing.T) {
	cases := []struct {
		name  string
		model *catalog.Model
		usage agent.Usage
		want  int64
	}{
		{
			name:  "nil model -> 0",
			model: nil,
			usage: agent.Usage{InputTokens: 100, OutputTokens: 50},
			want:  0,
		},
		{
			name:  "model with no cost -> 0",
			model: &catalog.Model{ID: "free", Cost: nil},
			usage: agent.Usage{InputTokens: 100, OutputTokens: 50},
			want:  0,
		},
		{
			name:  "claude-opus-4-7 small call ($5/$25 per MTok)",
			model: &catalog.Model{ID: "claude-opus-4-7", Cost: &catalog.Cost{Input: 5, Output: 25}},
			// 1000 in tokens × 5_000_000_000 mc/MTok / 1_000_000 = 5_000_000 mc
			// 500  out tokens × 25_000_000_000 / 1_000_000 = 12_500_000 mc
			// total = 17_500_000 microcents = $0.0175
			usage: agent.Usage{InputTokens: 1000, OutputTokens: 500},
			want:  17_500_000,
		},
		{
			name:  "gpt-4o-mini tiny call ($0.15/$0.60 per MTok)",
			model: &catalog.Model{ID: "gpt-4o-mini", Cost: &catalog.Cost{Input: 0.15, Output: 0.60}},
			// 10 in × 150_000_000 / 1_000_000 = 1500 mc
			// 5 out × 600_000_000 / 1_000_000 = 3000 mc
			// total = 4500 mc = $0.0000045
			usage: agent.Usage{InputTokens: 10, OutputTokens: 5},
			want:  4500,
		},
		{
			name:  "zero tokens -> 0",
			model: &catalog.Model{ID: "x", Cost: &catalog.Cost{Input: 5, Output: 25}},
			usage: agent.Usage{},
			want:  0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := computeCostMicrocents(c.model, c.usage); got != c.want {
				t.Errorf("got %d microcents, want %d", got, c.want)
			}
		})
	}
}

func TestFormatMicrocentsUSD(t *testing.T) {
	cases := map[int64]string{
		0:             "0.00",
		1_000_000_000: "1.00",      // exactly $1
		17_500_000:    "0.0175",    // claude small call
		4500:          "0.0000045", // gpt-4o-mini tiny call (full microcent precision)
		1_000_000:     "0.001",     // one cent
		500_000:       "0.0005",
		999_999_999:   "0.999999999", // just under $1, all 9 sub-dollar digits
		2_500_000_000: "2.50",
		1:             "0.000000001", // one microcent ($1e-9)
	}
	for in, want := range cases {
		if got := formatMicrocentsUSD(in); got != want {
			t.Errorf("formatMicrocentsUSD(%d)=%q want %q", in, got, want)
		}
	}
	// Negative microcents are unusual but the renderer should handle
	// them rather than emit garbage.
	if got := formatMicrocentsUSD(-1_000_000_000); got != "-1.00" {
		t.Errorf("formatMicrocentsUSD(-1B)=%q want '-1.00'", got)
	}
}

func TestRenderCheckAllTable(t *testing.T) {
	rows := []checkRow{
		{
			id:      "anthropic",
			family:  "anthropic",
			model:   "claude-3-5-haiku-20241022",
			latency: 53 * time.Millisecond,
			cost:    21_600,
			ok:      true,
		},
		{
			id:      "openai",
			family:  "openai",
			model:   "gpt-4o-mini",
			latency: 120 * time.Millisecond,
			cost:    4500,
			ok:      true,
		},
		{
			id:      "groq",
			family:  "openai-compatible",
			model:   "llama-3.3-70b-versatile",
			latency: 0,
			cost:    0,
			ok:      false,
			err:     "build groq: compat: no credentials available",
		},
	}
	out := renderCheckAllTable(rows)
	// Header line and a body line for each provider must be present.
	for _, want := range []string{
		"STATUS", "PROVIDER", "FAMILY", "MODEL", "LATENCY", "COST",
		"OK", "anthropic", "claude-3-5-haiku-20241022", "$0.0000216",
		"openai", "gpt-4o-mini", "$0.0000045",
		"FAIL", "groq", "llama-3.3-70b-versatile",
		"! groq: build groq: compat: no credentials available",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q\n--- output ---\n%s", want, out)
		}
	}
	// Cost column for the failure row should be "-", not "$0".
	lines := strings.Split(out, "\n")
	var failLine string
	for _, l := range lines {
		if strings.Contains(l, "FAIL") && strings.Contains(l, "groq") {
			failLine = l
			break
		}
	}
	if failLine == "" {
		t.Fatalf("could not find FAIL row in output:\n%s", out)
	}
	if !strings.Contains(failLine, "-") {
		t.Errorf("FAIL row should render cost as '-', got %q", failLine)
	}
}

func TestRenderCheckAllTable_EmptyCost(t *testing.T) {
	// Provider with no pricing data in the catalog (e.g. Ollama) shows
	// "(no price)" rather than "$0.00", which would mislead the operator.
	rows := []checkRow{
		{
			id:      "ollama",
			family:  "ollama",
			model:   "llama3.2",
			latency: 200 * time.Millisecond,
			cost:    0,
			ok:      true,
		},
	}
	out := renderCheckAllTable(rows)
	if !strings.Contains(out, "(no price)") {
		t.Errorf("expected '(no price)' for ok-but-zero-cost row, got:\n%s", out)
	}
}

func TestTruncate(t *testing.T) {
	cases := map[string]struct {
		n    int
		want string
	}{
		"":                                      {n: 10, want: ""},
		"short":                                 {n: 10, want: "short"},
		"abcdefghij":                            {n: 10, want: "abcdefghij"}, // exactly 10 chars
		"definitely longer than ten characters": {n: 10, want: "definitely…"},
	}
	for in, c := range cases {
		if got := truncate(in, c.n); got != c.want {
			t.Errorf("truncate(%q,%d)=%q want %q", in, c.n, got, c.want)
		}
	}
}

func TestParseCheckFlags_Caps(t *testing.T) {
	got, err := parseCheckFlags([]string{"openai", "--caps"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !got.caps || got.providerID != "openai" {
		t.Errorf("got %+v want caps=true provider=openai", got)
	}
	if alias, _ := parseCheckFlags([]string{"--capabilities"}); !alias.caps {
		t.Error("--capabilities alias should set caps")
	}
}

// capsCatalog builds a tiny in-memory catalog with one provider whose
// single model lacks tool-use, for the --caps rendering tests.
func capsCatalog() *catalog.Catalog {
	c := catalog.NewEmpty()
	c.Providers["acme"] = &catalog.Provider{
		ID:  "acme",
		NPM: "@ai-sdk/openai-compatible",
		API: "https://api.acme.test/v1",
		Models: map[string]*catalog.Model{
			"acme-mini": {
				ID:         "acme-mini",
				ToolCall:   false, // ← the agent-readiness gap
				Modalities: catalog.Modalities{Input: []string{"text", "image"}, Output: []string{"text"}},
				Limit:      catalog.Limit{Context: 32768, Output: 4096},
				Knowledge:  "2024-10",
			},
		},
	}
	return c
}

func TestRunCheckCaps_WarnsNoToolUse(t *testing.T) {
	cat := capsCatalog()
	var out, errOut bytes.Buffer
	code := runCheckCaps(cat, checkFlags{providerID: "acme"}, &out, &errOut)
	if code != 3 {
		t.Errorf("exit=%d want 3 (warnings present)", code)
	}
	s := out.String()
	if !strings.Contains(s, "tool-use        : no") {
		t.Errorf("output missing tool-use=no; got:\n%s", s)
	}
	if !strings.Contains(s, "vision (image)  : yes") {
		t.Errorf("output should report vision yes; got:\n%s", s)
	}
	if !strings.Contains(s, "⚠") || !strings.Contains(s, "tool-use") {
		t.Errorf("output should warn about tool-use; got:\n%s", s)
	}
}

func TestRunCheckCaps_JSON(t *testing.T) {
	cat := capsCatalog()
	var out, errOut bytes.Buffer
	code := runCheckCaps(cat, checkFlags{providerID: "acme", jsonOut: true}, &out, &errOut)
	if code != 3 {
		t.Errorf("exit=%d want 3", code)
	}
	var caps jsonCaps
	if err := json.Unmarshal(out.Bytes(), &caps); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if caps.Provider != "acme" || caps.Model != "acme-mini" {
		t.Errorf("ids wrong: %+v", caps)
	}
	if caps.ToolCall {
		t.Error("tool_call should be false")
	}
	if !caps.Vision {
		t.Error("vision should be true (image input)")
	}
	if len(caps.Warnings) == 0 {
		t.Error("warnings should be present")
	}
}

// TestRunCheckCaps_PromptCacheAdvertised (M376, SPEC-15 §1.2): a model with a
// cache-read price advertises prompt caching in `agt provider check --caps`; a
// model without one does not — so the capability is visible, not just billed.
func TestRunCheckCaps_PromptCacheAdvertised(t *testing.T) {
	c := catalog.NewEmpty()
	c.Providers["acme"] = &catalog.Provider{
		ID: "acme", NPM: "@ai-sdk/anthropic", API: "https://a.test",
		Models: map[string]*catalog.Model{
			"cached": {ID: "cached", ToolCall: true,
				Cost:  &catalog.Cost{Input: 3.0, Output: 15.0, CacheRead: 0.30},
				Limit: catalog.Limit{Context: 200000}},
			"plain": {ID: "plain", ToolCall: true,
				Cost:  &catalog.Cost{Input: 3.0, Output: 15.0},
				Limit: catalog.Limit{Context: 200000}},
		},
	}

	get := func(model string) jsonCaps {
		t.Helper()
		t.Setenv("AGEZT_MODEL", model)
		var out, errOut bytes.Buffer
		runCheckCaps(c, checkFlags{providerID: "acme", jsonOut: true}, &out, &errOut)
		var caps jsonCaps
		if err := json.Unmarshal(out.Bytes(), &caps); err != nil {
			t.Fatalf("json: %v\n%s", err, out.String())
		}
		return caps
	}

	if !get("cached").PromptCache {
		t.Error("a model with a cache_read price must advertise prompt_cache=true")
	}
	if get("plain").PromptCache {
		t.Error("a model without a cache_read price must report prompt_cache=false")
	}

	// Human output surfaces it too.
	t.Setenv("AGEZT_MODEL", "cached")
	var human, herr bytes.Buffer
	runCheckCaps(c, checkFlags{providerID: "acme"}, &human, &herr)
	if !strings.Contains(human.String(), "prompt caching  : yes") {
		t.Errorf("human caps output missing 'prompt caching : yes':\n%s", human.String())
	}
}

func TestRunCheckCaps_UnknownProvider(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runCheckCaps(capsCatalog(), checkFlags{providerID: "nope"}, &out, &errOut); code != 1 {
		t.Errorf("exit=%d want 1 for unknown provider", code)
	}
}

// capsMultiCatalog builds a catalog with two supported providers (one
// tool-less, one tool-capable) and one unsupported-family provider that
// the matrix must skip.
func capsMultiCatalog() *catalog.Catalog {
	c := catalog.NewEmpty()
	c.Providers["acme"] = &catalog.Provider{
		ID: "acme", NPM: "@ai-sdk/openai-compatible", API: "https://a.test/v1",
		Models: map[string]*catalog.Model{
			"acme-mini": {ID: "acme-mini", ToolCall: false, Limit: catalog.Limit{Context: 32768}},
		},
	}
	c.Providers["good"] = &catalog.Provider{
		ID: "good", NPM: "@ai-sdk/anthropic", API: "https://g.test",
		Models: map[string]*catalog.Model{
			"good-x": {ID: "good-x", ToolCall: true, Reasoning: true,
				Modalities: catalog.Modalities{Input: []string{"text", "image"}},
				Limit:      catalog.Limit{Context: 200000}},
		},
	}
	c.Providers["weird"] = &catalog.Provider{
		ID: "weird", NPM: "@ai-sdk/unheard-of", // FamilyUnknown → skipped
		Models: map[string]*catalog.Model{"w": {ID: "w", ToolCall: true}},
	}
	return c
}

func TestRunCheckCapsAll_Human(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runCheckCapsAll(capsMultiCatalog(), checkFlags{caps: true, all: true}, &out)
	_ = errOut
	if code != 0 {
		t.Errorf("exit=%d want 0 (survey)", code)
	}
	s := out.String()
	// Both supported providers present, unsupported one skipped.
	if !strings.Contains(s, "acme") || !strings.Contains(s, "good") {
		t.Errorf("matrix missing a supported provider:\n%s", s)
	}
	if strings.Contains(s, "weird") {
		t.Errorf("unsupported family should be skipped:\n%s", s)
	}
	// 2 providers, 1 agent-ready (good has tools, acme doesn't).
	if !strings.Contains(s, "2 providers, 1 agent-ready") {
		t.Errorf("summary wrong:\n%s", s)
	}
}

func TestRunCheckCapsAll_JSON(t *testing.T) {
	var out bytes.Buffer
	if code := runCheckCapsAll(capsMultiCatalog(), checkFlags{caps: true, all: true, jsonOut: true}, &out); code != 0 {
		t.Errorf("exit=%d want 0", code)
	}
	var rows []jsonCaps
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (supported only): %+v", len(rows), rows)
	}
	byID := map[string]jsonCaps{}
	for _, r := range rows {
		byID[r.Provider] = r
	}
	if byID["acme"].ToolCall || len(byID["acme"].Warnings) == 0 {
		t.Errorf("acme should be tool-less + warned: %+v", byID["acme"])
	}
	if !byID["good"].ToolCall || !byID["good"].Vision || len(byID["good"].Warnings) != 0 {
		t.Errorf("good should be tool+vision, no warnings: %+v", byID["good"])
	}
}

func TestTruncate_RuneSafeCLIHelper(t *testing.T) {
	// The cmd/agt truncate helper (used for intent/task/pulse/plan-node display)
	// must be rune-safe: a CLI run list of a Turkish intent must never show a
	// split ç/ş/ğ. 40 'ş' (80 bytes); cap 7 lands mid-rune.
	got := truncate(strings.Repeat("ş", 40), 7)
	if !utf8.ValidString(got) {
		t.Fatalf("truncate produced invalid UTF-8: %q", got)
	}
	if strings.ContainsRune(got, '�') {
		t.Errorf("truncate split a rune (replacement char present): %q", got)
	}
	// Under-cap ASCII is returned unchanged (no marker).
	if got := truncate("short", 20); got != "short" {
		t.Errorf("under-cap = %q, want unchanged", got)
	}
}

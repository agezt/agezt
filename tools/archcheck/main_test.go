// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const mod = "github.com/agezt/agezt"

func testConfig(t *testing.T) *Config {
	t.Helper()
	cfg := &Config{ModulePath: mod, Rules: []Rule{
		{Pattern: "kernel/modules/*/...", Layer: LayerModule, Module: "@1"},
		{Pattern: "internal/...", Layer: LayerFoundation},
		{Pattern: "kernel/contract/...", Layer: LayerContract},
		{Pattern: "kernel/platform/...", Layer: LayerPlatform},
		{Pattern: "kernel/app", Layer: LayerApp},
		{Pattern: "kernel/adapters/...", Layer: LayerAdapter},
		{Pattern: "plugins/...", Layer: LayerPlugin},
		{Pattern: "cmd/...", Layer: LayerRoot},
	}, Calls: allCallPolicies()}
	if err := cfg.validate(); err != nil {
		t.Fatalf("test config invalid: %v", err)
	}
	return cfg
}

func allCallPolicies() []CallPolicy {
	return []CallPolicy{
		{Rule: CallExec, Allow: []string{"kernel/platform/sandbox/..."}},
		{Rule: CallHTTPClient, Allow: []string{"kernel/platform/netout/..."}},
		{Rule: CallRawWrite, Allow: []string{"kernel/platform/store/..."}},
	}
}

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern, rel string
		ok           bool
		capture      string
	}{
		{"kernel/agent", "kernel/agent", true, ""},
		{"kernel/agent", "kernel/agent/sub", false, ""},
		{"kernel/runtime/...", "kernel/runtime", true, ""},
		{"kernel/runtime/...", "kernel/runtime/runexec", true, ""},
		{"kernel/runtime/...", "kernel/runtimex", false, ""},
		{"kernel/modules/*/...", "kernel/modules/knowledge", true, "knowledge"},
		{"kernel/modules/*/...", "kernel/modules/knowledge/internal/store", true, "knowledge"},
		{"kernel/modules/*/...", "kernel/modules", false, ""},
		{"kernel/modules/*", "kernel/modules/runs/api", false, ""},
	}
	for _, c := range cases {
		capture, ok := match(c.pattern, c.rel)
		if ok != c.ok || capture != c.capture {
			t.Errorf("match(%q, %q) = (%q, %v), want (%q, %v)", c.pattern, c.rel, capture, ok, c.capture, c.ok)
		}
	}
}

func TestClassifyFirstMatchWinsAndCapturesModule(t *testing.T) {
	cfg := &Config{ModulePath: mod, Calls: allCallPolicies(), Rules: []Rule{
		{Pattern: "kernel/internal/testfixtures", Layer: LayerRoot},
		{Pattern: "kernel/...", Layer: LayerPlatform},
		{Pattern: "kernel/modules/*/...", Layer: LayerModule, Module: "@1"},
	}}
	if cl, _ := cfg.classify("kernel/internal/testfixtures"); cl.Layer != LayerRoot {
		t.Fatalf("specific rule must win over the broader one listed after it, got %+v", cl)
	}
	// The broad kernel/... rule precedes the modules rule, so it wins: order matters.
	if cl, _ := cfg.classify("kernel/modules/runs"); cl.Layer != LayerPlatform {
		t.Fatalf("first matching rule must win, got %+v", cl)
	}
	cfg2 := testConfig(t)
	cl, ok := cfg2.classify("kernel/modules/knowledge/internal/store")
	if !ok || cl.Layer != LayerModule || cl.Module != "knowledge" {
		t.Fatalf("module capture: got %+v ok=%v", cl, ok)
	}
	if _, ok := cfg2.classify("kernel/brandnew"); ok {
		t.Fatal("an unmatched package must not classify (fail closed)")
	}
}

func TestValidateRejectsBadRules(t *testing.T) {
	bad := []Config{
		{ModulePath: "", Rules: []Rule{{Pattern: "a", Layer: 0}}},
		{ModulePath: mod},
		{ModulePath: mod, Rules: []Rule{{Pattern: "", Layer: 0}}},
		{ModulePath: mod, Rules: []Rule{{Pattern: "a", Layer: 9}}},
		{ModulePath: mod, Rules: []Rule{{Pattern: "a", Layer: LayerModule}}},
		{ModulePath: mod, Rules: []Rule{{Pattern: "a", Layer: LayerPlatform, Module: "x"}}},
		{ModulePath: mod, Rules: []Rule{{Pattern: "a", Layer: LayerModule, Module: "@1"}}},
		{ModulePath: mod, Rules: []Rule{{Pattern: "a", Layer: 0}, {Pattern: "a", Layer: 1}}},
	}
	for i := range bad {
		if bad[i].Calls == nil {
			bad[i].Calls = allCallPolicies()
		}
	}
	badCalls := [][]CallPolicy{
		nil, // every rule needs a policy entry
		{{Rule: "teleport"}, {Rule: CallExec}, {Rule: CallHTTPClient}, {Rule: CallRawWrite}},
		{{Rule: CallExec}, {Rule: CallExec}, {Rule: CallHTTPClient}, {Rule: CallRawWrite}},
	}
	for _, bc := range badCalls {
		bad = append(bad, Config{ModulePath: mod, Rules: []Rule{{Pattern: "a", Layer: 0}}, Calls: bc})
	}
	for i, c := range bad {
		if err := c.validate(); err == nil {
			t.Errorf("case %d: validate accepted an invalid config: %+v", i, c)
		}
	}
}

func TestEdgeKind(t *testing.T) {
	m := func(name string) Class { return Class{Layer: LayerModule, Module: name} }
	l := func(layer int) Class { return Class{Layer: layer} }
	cases := []struct {
		name     string
		from, to Class
		toRel    string
		want     string
	}{
		{"platform imports contract", l(LayerPlatform), l(LayerContract), "kernel/contract/llm", ""},
		{"contract imports platform", l(LayerContract), l(LayerPlatform), "kernel/platform/bus", KindUpward},
		{"kernel imports plugin", l(LayerPlatform), l(LayerPlugin), "plugins/tools/x", KindUpward},
		{"module imports app", m("runs"), l(LayerApp), "kernel/app", KindUpward},
		{"module imports own module", m("runs"), m("runs"), "kernel/modules/runs/internal", ""},
		{"module imports other internals", m("runs"), m("knowledge"), "kernel/modules/knowledge/internal", KindCrossModule},
		{"module imports other api", m("runs"), m("knowledge"), "kernel/modules/knowledge/api", ""},
		{"adapter imports app", l(LayerAdapter), l(LayerApp), "kernel/app", ""},
		{"adapter imports module", l(LayerAdapter), m("runs"), "kernel/modules/runs/api", KindAdapterBypass},
		{"adapter imports platform", l(LayerAdapter), l(LayerPlatform), "kernel/platform/httpx", ""},
		{"plugin imports contract", l(LayerPlugin), l(LayerContract), "kernel/contract/tool", ""},
		{"plugin imports plugin", l(LayerPlugin), l(LayerPlugin), "plugins/x", ""},
		{"plugin imports module", l(LayerPlugin), m("runs"), "kernel/runtime", KindPluginReach},
		{"plugin imports adapter", l(LayerPlugin), l(LayerAdapter), "kernel/controlplane", KindPluginReach},
		{"root imports anything", l(LayerRoot), m("runs"), "kernel/runtime", ""},
	}
	for _, c := range cases {
		if got := edgeKind(c.from, c.to, c.toRel); got != c.want {
			t.Errorf("%s: edgeKind = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestEvaluateIgnoresExternalAndDedupes(t *testing.T) {
	cfg := testConfig(t)
	pkgs := []Pkg{
		{ImportPath: mod + "/kernel/contract/llm", Imports: []string{"fmt", mod + "/kernel/platform/bus", mod + "/kernel/platform/bus"}},
		{ImportPath: mod + "/kernel/platform/bus", Imports: []string{"lukechampine.com/blake3"}},
		{ImportPath: mod + "/kernel/mystery", Imports: nil},
		{ImportPath: "example.com/other", Imports: []string{mod + "/kernel/platform/bus"}},
	}
	rep := evaluate(cfg, pkgs)
	if rep.Packages != 3 {
		t.Fatalf("packages = %d, want 3 (external package excluded)", rep.Packages)
	}
	if len(rep.Violations) != 1 || rep.Violations[0].Key() != "upward kernel/contract/llm -> kernel/platform/bus" {
		t.Fatalf("violations = %+v, want one deduplicated upward edge", rep.Violations)
	}
	if len(rep.Unmapped) != 1 || rep.Unmapped[0] != "kernel/mystery" {
		t.Fatalf("unmapped = %v", rep.Unmapped)
	}
}

func TestDecodePackages(t *testing.T) {
	in := `{"ImportPath":"a","Imports":["b"]}
{"ImportPath":"b"}`
	pkgs, err := decodePackages(strings.NewReader(in))
	if err != nil || len(pkgs) != 2 || pkgs[0].Imports[0] != "b" {
		t.Fatalf("decode: %v %+v", err, pkgs)
	}
	if _, err := decodePackages(strings.NewReader("{")); err == nil {
		t.Fatal("truncated JSON must error")
	}
}

// runFixture writes a config + allowlist into a temp dir, stubs `go list`,
// and runs the command.
func runFixture(t *testing.T, pkgs []Pkg, allow string, args ...string) (int, string, string, string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "layers.json")
	cfgJSON := `{"module_path":"` + mod + `","rules":[
		{"pattern":"kernel/contract/...","layer":1},
		{"pattern":"kernel/platform/...","layer":2},
		{"pattern":"plugins/...","layer":6}],
		"calls":[{"rule":"exec","allow":[]},{"rule":"http-client","allow":[]},{"rule":"raw-write","allow":[]}]}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	allowPath := filepath.Join(dir, "allowlist.txt")
	callsPath := filepath.Join(dir, "calls-allowlist.txt")
	if allow != "" {
		if err := os.WriteFile(allowPath, []byte(allow), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := listPackages
	listPackages = func() ([]Pkg, error) { return pkgs, nil }
	t.Cleanup(func() { listPackages = old })

	var stdout, stderr bytes.Buffer
	code := run(append([]string{"-config", cfgPath, "-allowlist", allowPath, "-calls-allowlist", callsPath}, args...), &stdout, &stderr)
	written, _ := os.ReadFile(allowPath)
	return code, stdout.String(), stderr.String(), string(written)
}

var badGraph = []Pkg{
	{ImportPath: mod + "/kernel/contract/llm", Imports: []string{mod + "/kernel/platform/bus"}},
	{ImportPath: mod + "/kernel/platform/bus"},
}

const badEdge = "upward kernel/contract/llm -> kernel/platform/bus"

func TestRunFailsOnNewViolation(t *testing.T) {
	code, _, stderr, _ := runFixture(t, badGraph, "")
	if code != 1 || !strings.Contains(stderr, badEdge) {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
}

func TestRunPassesWhenAllowlisted(t *testing.T) {
	code, stdout, stderr, _ := runFixture(t, badGraph, "# header\n"+badEdge+"\n")
	if code != 0 || !strings.Contains(stdout, "1 allowlisted import violations") {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestRunFailsOnStaleEntry(t *testing.T) {
	clean := []Pkg{{ImportPath: mod + "/kernel/contract/llm"}, {ImportPath: mod + "/kernel/platform/bus"}}
	code, _, stderr, _ := runFixture(t, clean, badEdge+"\n")
	if code != 1 || !strings.Contains(stderr, "no longer occur") {
		t.Fatalf("a fixed edge left in the allowlist must fail the ratchet; code=%d stderr=%s", code, stderr)
	}
}

func TestRunFailsOnUnmappedPackage(t *testing.T) {
	pkgs := append([]Pkg{{ImportPath: mod + "/kernel/brandnew"}}, badGraph...)
	code, _, stderr, _ := runFixture(t, pkgs, badEdge+"\n")
	if code != 1 || !strings.Contains(stderr, "kernel/brandnew") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
}

func TestUpdateDropsFixedButRefusesNew(t *testing.T) {
	clean := []Pkg{{ImportPath: mod + "/kernel/contract/llm"}, {ImportPath: mod + "/kernel/platform/bus"}}
	code, _, _, written := runFixture(t, clean, badEdge+"\n", "-update")
	if code != 0 || strings.Contains(written, badEdge) {
		t.Fatalf("-update must drop the fixed entry; code=%d allowlist=%q", code, written)
	}

	code, _, stderr, written := runFixture(t, badGraph, "", "-update")
	if code != 1 || strings.Contains(written, badEdge) || !strings.Contains(stderr, badEdge) {
		t.Fatalf("-update without -allow-new must not accept new debt; code=%d allowlist=%q", code, written)
	}

	code, _, _, written = runFixture(t, badGraph, "", "-update", "-allow-new")
	if code != 0 || !strings.Contains(written, badEdge) {
		t.Fatalf("-update -allow-new must record the edge; code=%d allowlist=%q", code, written)
	}
}

func TestAllowNewRequiresUpdate(t *testing.T) {
	if code, _, _, _ := runFixture(t, badGraph, "", "-allow-new"); code != 2 {
		t.Fatalf("code=%d, want usage error", code)
	}
}

// TestRepoLayerMapIsValid pins the committed layers.json: it must parse and
// pass validation (the command runs it at every invocation; this makes a
// broken edit fail in `go test` too, not only in the CI step).
func TestRepoLayerMapIsValid(t *testing.T) {
	cfg, err := loadConfig("layers.json")
	if err != nil {
		t.Fatalf("committed layers.json: %v", err)
	}
	if cfg.ModulePath != mod {
		t.Fatalf("module_path = %q", cfg.ModulePath)
	}
	// Spot-check placements the roadmap depends on.
	want := map[string]Class{
		"kernel/agent":               {Layer: LayerModule, Module: "runs"},
		"kernel/runtime/runexec":     {Layer: LayerModule, Module: "runs"},
		"kernel/memory":              {Layer: LayerModule, Module: "knowledge"},
		"kernel/controlplane":        {Layer: LayerAdapter},
		"kernel/governor":            {Layer: LayerPlatform},
		"kernel/event":               {Layer: LayerContract},
		"kernel/modules/runs/api":    {Layer: LayerModule, Module: "runs"},
		"plugins/tools/overseertool": {Layer: LayerPlugin},
		"internal/atomicfile":        {Layer: LayerFoundation},
	}
	for rel, w := range want {
		got, ok := cfg.classify(rel)
		if !ok || got.Layer != w.Layer || got.Module != w.Module {
			t.Errorf("classify(%q) = %+v ok=%v, want layer %d module %q", rel, got, ok, w.Layer, w.Module)
		}
	}
}

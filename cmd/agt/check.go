// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/plugins/providers/compat"
)

// checkFlags is the parsed argv state for `agt provider check ...`.
type checkFlags struct {
	all        bool
	jsonOut    bool
	stream     bool   // SSE roundtrip when the provider implements it
	caps       bool   // report model capabilities from the catalog; no network
	bench      int    // 0 = single-shot; ≥2 = run N probes and report stats
	providerID string // positional arg, empty for auto-pick
}

// parseCheckFlags accepts argv after "check". Recognised flags:
//
//	--all, -a       iterate every credentialed provider
//	--json, -j      machine-readable output (single or all)
//	--bench N       run N probes, report p50/p95 latencies
//	<provider-id>   positional, single-provider mode only
//
// Unrecognised flags return an error so typos surface immediately
// rather than getting silently treated as provider ids.
func parseCheckFlags(args []string) (checkFlags, error) {
	f := checkFlags{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--all" || a == "-a":
			f.all = true
		case a == "--json" || a == "-j":
			f.jsonOut = true
		case a == "--stream" || a == "-s":
			f.stream = true
		case a == "--caps" || a == "--capabilities":
			f.caps = true
		case a == "--bench":
			if i+1 >= len(args) {
				return f, fmt.Errorf("--bench requires an integer count")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 2 {
				return f, fmt.Errorf("--bench requires N ≥ 2, got %q", args[i+1])
			}
			f.bench = n
			i++
		case strings.HasPrefix(a, "--bench="):
			n, err := strconv.Atoi(strings.TrimPrefix(a, "--bench="))
			if err != nil || n < 2 {
				return f, fmt.Errorf("--bench requires N ≥ 2")
			}
			f.bench = n
		case strings.HasPrefix(a, "-"):
			return f, fmt.Errorf("unknown flag %q", a)
		default:
			if f.providerID != "" {
				return f, fmt.Errorf("unexpected extra arg %q (provider id already set to %q)", a, f.providerID)
			}
			f.providerID = strings.ToLower(strings.TrimSpace(a))
		}
	}
	return f, nil
}

// cmdProviderCheck dispatches `agt provider check`. Modes:
//
//	agt provider check                          # single, auto-pick, human
//	agt provider check <id>                     # single, explicit, human
//	agt provider check --all                    # all-credentialed, human table
//	agt provider check --json [<id>]            # single, machine-readable
//	agt provider check --all --json             # all, machine-readable
//	agt provider check --bench 5 [<id>]         # single, 5 probes, p50/p95
//	agt provider check --all --bench 3          # all, 3 probes each
//	agt provider check --caps [<id>]            # model capabilities; no network
//	agt provider check --caps --json [<id>]     # capabilities, machine-readable
//	agt provider check --caps --all             # capability matrix, all providers
//
// Probe prompt is tiny ("Say 'pong' in one word.", MaxTokens=16) so
// even --bench 20 runs near-free on most providers.
func cmdProviderCheck(args []string, stdout, stderr io.Writer) int {
	flags, err := parseCheckFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 2
	}

	cat, err := loadCatalogIfAny(stderr)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return 1
	}
	if cat == nil {
		fmt.Fprintf(stderr, "%s: catalog is empty — run `agt catalog sync` first\n", brand.CLI)
		return 1
	}

	// --caps reports static model capabilities from the catalog: no
	// network, no credentials, no live probe. It answers "can the model I'm
	// about to use actually call tools / see images?" before a run, and
	// warns when the agent loop's prerequisite (tool-use) is missing.
	if flags.caps {
		if flags.bench >= 2 || flags.stream {
			fmt.Fprintf(stderr, "%s: --caps cannot combine with --bench/--stream (it makes no live call)\n", brand.CLI)
			return 2
		}
		if flags.all {
			return runCheckCapsAll(cat, flags, stdout)
		}
		return runCheckCaps(cat, flags, stdout, stderr)
	}

	credStore, err := openCredsStore(stderr)
	if err != nil {
		return 1
	}
	lookup := creds.ChainLookup(credStore.Lookup, os.Getenv)

	// --stream only applies to single-provider, non-bench mode for v1.
	// Bench would re-stream the same tokens N times (visually noisy);
	// --all would interleave streams across providers. Reject these
	// combinations explicitly so the operator hears the constraint.
	if flags.stream && (flags.all || flags.bench >= 2) {
		fmt.Fprintf(stderr, "%s: --stream is not yet supported with --all or --bench\n", brand.CLI)
		return 2
	}

	if flags.all {
		return runCheckAll(cat, lookup, flags, stdout, stderr)
	}
	return runCheckSingle(cat, lookup, flags, stdout, stderr)
}

// runCheckSingle handles all single-provider modes (with/without
// --json, with/without --bench).
func runCheckSingle(cat *catalog.Catalog, lookup func(string) string, flags checkFlags, stdout, stderr io.Writer) int {
	wantID := flags.providerID
	if wantID == "" {
		wantID = strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "PROVIDER")))
	}
	if wantID == "mock" {
		fmt.Fprintf(stderr, "%s: provider check doesn't apply to the offline mock\n", brand.CLI)
		return 2
	}

	var entry *catalog.Provider
	if wantID != "" {
		var ok bool
		entry, ok = cat.Providers[wantID]
		if !ok {
			fmt.Fprintf(stderr, "%s: provider %q not in catalog (try `agt catalog list`)\n", brand.CLI, wantID)
			return 1
		}
	} else {
		entry = autoPickFromCatalog(cat, lookup)
		if entry == nil {
			fmt.Fprintf(stderr, "%s: no catalog provider has credentials + a wired family — set creds via `agt provider creds set`\n", brand.CLI)
			return 1
		}
	}

	if flags.bench >= 2 {
		bench := runBench(entry, lookup, flags.bench, stdout, !flags.jsonOut)
		if flags.jsonOut {
			return emitJSON([]jsonProbe{bench.toJSON()}, summaryFromProbes(bench.toJSON()), stdout)
		}
		emitBenchHuman(bench, stdout)
		if bench.failures == bench.iterations {
			return 1
		}
		return 0
	}

	if flags.stream {
		return runStreamProbe(entry, lookup, stdout, stderr)
	}

	// Single-shot mode.
	res := runProbe(entry, lookup)
	if flags.jsonOut {
		jp := probeToJSON(entry, res)
		return emitJSON([]jsonProbe{jp}, summaryFromProbes(jp), stdout)
	}
	emitSingleHuman(entry, res, stdout, stderr)
	if res.err != nil {
		return 1
	}
	return 0
}

// jsonCaps is the machine-readable capability record emitted by
// `agt provider check --caps --json`. Stable contract.
type jsonCaps struct {
	Provider     string   `json:"provider"`
	Family       string   `json:"family"`
	Model        string   `json:"model"`
	ToolCall     bool     `json:"tool_call"`
	Reasoning    bool     `json:"reasoning"`
	Vision       bool     `json:"vision"`
	Attachment   bool     `json:"attachment"`
	JSONMode     bool     `json:"json_mode"`
	InputModes   []string `json:"input_modalities,omitempty"`
	OutputModes  []string `json:"output_modalities,omitempty"`
	ContextLimit int      `json:"context_limit,omitempty"`
	OutputLimit  int      `json:"output_limit,omitempty"`
	Knowledge    string   `json:"knowledge,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
}

// runCheckCaps resolves a provider/model from the catalog (explicit id,
// $AGEZT_PROVIDER, or first supported family — no credential requirement,
// since capabilities are static) and reports the model's capabilities and
// any agent-readiness warnings. Exit 3 when there are warnings so CI can
// gate "is this model agent-ready?" without parsing text.
func runCheckCaps(cat *catalog.Catalog, flags checkFlags, stdout, stderr io.Writer) int {
	wantID := flags.providerID
	if wantID == "" {
		wantID = strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "PROVIDER")))
	}
	if wantID == "mock" {
		fmt.Fprintf(stderr, "%s: capabilities don't apply to the offline mock\n", brand.CLI)
		return 2
	}

	var entry *catalog.Provider
	if wantID != "" {
		var ok bool
		if entry, ok = cat.Providers[wantID]; !ok {
			fmt.Fprintf(stderr, "%s: provider %q not in catalog (try `agt catalog list`)\n", brand.CLI, wantID)
			return 1
		}
	} else {
		// No credential filter: capabilities are static facts.
		for _, e := range cat.ProviderList() {
			if compat.IsSupportedFamily(e.Family()) {
				entry = e
				break
			}
		}
		if entry == nil {
			fmt.Fprintf(stderr, "%s: no supported provider in the catalog\n", brand.CLI)
			return 1
		}
	}

	modelID := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "MODEL"))
	if modelID == "" {
		modelID = compat.FirstModelID(entry)
	}
	model := entry.Models[modelID]
	if model == nil {
		fmt.Fprintf(stderr, "%s: model %q not found for provider %q (try `agt catalog list`)\n",
			brand.CLI, modelID, entry.ID)
		return 1
	}

	caps := jsonCaps{
		Provider:     entry.ID,
		Family:       string(entry.Family()),
		Model:        modelID,
		ToolCall:     model.ToolCall,
		Reasoning:    model.Reasoning,
		Vision:       model.SupportsVision(),
		Attachment:   model.Attachment,
		JSONMode:     catalog.FamilySupportsNativeJSONMode(entry.Family()),
		InputModes:   model.Modalities.Input,
		OutputModes:  model.Modalities.Output,
		ContextLimit: model.Limit.Context,
		OutputLimit:  model.Limit.Output,
		Knowledge:    model.Knowledge,
		Warnings:     model.AgentWarnings(),
	}

	if flags.jsonOut {
		buf, _ := json.MarshalIndent(caps, "", "  ")
		fmt.Fprintln(stdout, string(buf))
	} else {
		emitCapsHuman(caps, stdout)
	}
	if len(caps.Warnings) > 0 {
		return 3
	}
	return 0
}

// emitCapsHuman renders the capability record as an aligned block, with
// agent-readiness warnings called out under a ⚠ marker.
func emitCapsHuman(c jsonCaps, stdout io.Writer) {
	yn := func(b bool) string {
		if b {
			return "yes"
		}
		return "no"
	}
	fmt.Fprintf(stdout, "capabilities  provider=%s model=%s family=%s\n\n", c.Provider, c.Model, c.Family)
	fmt.Fprintf(stdout, "  tool-use        : %s\n", yn(c.ToolCall))
	fmt.Fprintf(stdout, "  reasoning       : %s\n", yn(c.Reasoning))
	fmt.Fprintf(stdout, "  vision (image)  : %s\n", yn(c.Vision))
	fmt.Fprintf(stdout, "  attachments     : %s\n", yn(c.Attachment))
	fmt.Fprintf(stdout, "  json mode       : %s\n", yn(c.JSONMode))
	if len(c.InputModes) > 0 {
		fmt.Fprintf(stdout, "  input modes     : %s\n", strings.Join(c.InputModes, ", "))
	}
	if len(c.OutputModes) > 0 {
		fmt.Fprintf(stdout, "  output modes    : %s\n", strings.Join(c.OutputModes, ", "))
	}
	if c.ContextLimit > 0 {
		fmt.Fprintf(stdout, "  context window  : %d tokens\n", c.ContextLimit)
	}
	if c.OutputLimit > 0 {
		fmt.Fprintf(stdout, "  max output      : %d tokens\n", c.OutputLimit)
	}
	if c.Knowledge != "" {
		fmt.Fprintf(stdout, "  knowledge cutoff: %s\n", c.Knowledge)
	}
	if len(c.Warnings) > 0 {
		fmt.Fprintln(stdout)
		for _, w := range c.Warnings {
			fmt.Fprintf(stdout, "  ⚠ %s\n", w)
		}
	} else {
		fmt.Fprintf(stdout, "\n  ✓ agent-ready (advertises tool-use)\n")
	}
}

// runCheckCapsAll renders a capability matrix: one row per supported
// catalog provider (its selected model), network-free and credential-free,
// so an operator can compare models by capability at a glance. Always exit
// 0 — it's a survey, not a gate (the single-provider --caps gates with
// exit 3 for CI). $AGEZT_MODEL, when set, selects that model for every
// provider that serves it; otherwise each provider's first model is shown.
func runCheckCapsAll(cat *catalog.Catalog, flags checkFlags, stdout io.Writer) int {
	modelOverride := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "MODEL"))
	var rows []jsonCaps
	for _, entry := range cat.ProviderList() {
		if !compat.IsSupportedFamily(entry.Family()) {
			continue
		}
		modelID := modelOverride
		if _, ok := entry.Models[modelID]; !ok {
			modelID = compat.FirstModelID(entry)
		}
		model := entry.Models[modelID]
		if model == nil {
			continue // provider with no models — nothing to report
		}
		rows = append(rows, jsonCaps{
			Provider:     entry.ID,
			Family:       string(entry.Family()),
			Model:        modelID,
			ToolCall:     model.ToolCall,
			Reasoning:    model.Reasoning,
			Vision:       model.SupportsVision(),
			Attachment:   model.Attachment,
			JSONMode:     catalog.FamilySupportsNativeJSONMode(entry.Family()),
			ContextLimit: model.Limit.Context,
			Warnings:     model.AgentWarnings(),
		})
	}

	if flags.jsonOut {
		buf, _ := json.MarshalIndent(rows, "", "  ")
		fmt.Fprintln(stdout, string(buf))
		return 0
	}
	if len(rows) == 0 {
		fmt.Fprintln(stdout, "no supported providers in the catalog")
		return 0
	}
	fmt.Fprintln(stdout, renderCapsTable(rows))
	ready := 0
	for _, r := range rows {
		if len(r.Warnings) == 0 {
			ready++
		}
	}
	fmt.Fprintf(stdout, "\n%d providers, %d agent-ready (advertise tool-use)\n", len(rows), ready)
	return 0
}

// renderCapsTable lays out the capability matrix. A leading ✓/⚠ marks
// agent-readiness (tool-use) so the eye lands on the ready ones first.
func renderCapsTable(rows []jsonCaps) string {
	yn := func(b bool) string {
		if b {
			return "yes"
		}
		return "-"
	}
	headers := []string{"", "PROVIDER", "MODEL", "TOOLS", "VISION", "REASON", "CONTEXT"}
	cells := make([][]string, 0, len(rows))
	for _, r := range rows {
		mark := "✓"
		if len(r.Warnings) > 0 {
			mark = "⚠"
		}
		ctx := "-"
		if r.ContextLimit > 0 {
			ctx = fmt.Sprintf("%d", r.ContextLimit)
		}
		cells = append(cells, []string{
			mark, r.Provider, r.Model, yn(r.ToolCall), yn(r.Vision), yn(r.Reasoning), ctx,
		})
	}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, c := range cells {
		for i, v := range c {
			// Width by rune count so the ✓/⚠ column doesn't over-pad.
			if n := len([]rune(v)); n > widths[i] {
				widths[i] = n
			}
		}
	}
	var b strings.Builder
	writeRow := func(vals []string) {
		for i, v := range vals {
			if i > 0 {
				b.WriteString("  ")
			}
			pad := widths[i] - len([]rune(v))
			b.WriteString(v)
			if pad > 0 {
				b.WriteString(strings.Repeat(" ", pad))
			}
		}
		b.WriteByte('\n')
	}
	writeRow(headers)
	for _, c := range cells {
		writeRow(c)
	}
	return strings.TrimRight(b.String(), "\n")
}

// runCheckAll iterates credentialed providers. Each provider runs
// either a single probe or a benchmark, depending on flags.bench.
func runCheckAll(cat *catalog.Catalog, lookup func(string) string, flags checkFlags, stdout, stderr io.Writer) int {
	var (
		rows    []checkRow
		benches []benchResult
		jsonOut []jsonProbe
		skipped int
	)
	for _, entry := range cat.ProviderList() {
		if !compat.IsSupportedFamily(entry.Family()) {
			skipped++
			continue
		}
		if !entry.HasCredentials(lookup) {
			skipped++
			continue
		}
		if !flags.jsonOut {
			fmt.Fprintf(stdout, "checking %-20s …\n", entry.ID)
		}
		if flags.bench >= 2 {
			b := runBench(entry, lookup, flags.bench, stdout, false)
			benches = append(benches, b)
			rows = append(rows, b.toCheckRow())
			if flags.jsonOut {
				jsonOut = append(jsonOut, b.toJSON())
			}
		} else {
			res := runProbe(entry, lookup)
			r := checkRow{
				id:      entry.ID,
				family:  string(entry.Family()),
				model:   res.modelID,
				latency: res.latency,
				cost:    res.costMicrocents,
				ok:      res.err == nil,
			}
			if res.err != nil {
				r.err = truncate(res.err.Error(), 60)
			}
			rows = append(rows, r)
			if flags.jsonOut {
				jsonOut = append(jsonOut, probeToJSON(entry, res))
			}
		}
	}

	if len(rows) == 0 {
		if flags.jsonOut {
			return emitJSON([]jsonProbe{}, jsonSummary{Skipped: skipped}, stdout)
		}
		fmt.Fprintf(stderr, "%s: no catalog provider has credentials + a wired family — set creds via `agt provider creds set`\n", brand.CLI)
		return 1
	}

	pass, fail := 0, 0
	for _, r := range rows {
		if r.ok {
			pass++
		} else {
			fail++
		}
	}

	if flags.jsonOut {
		sum := jsonSummary{Total: len(rows), OK: pass, Failed: fail, Skipped: skipped}
		exit := 0
		if fail > 0 {
			exit = 1
		}
		if rc := emitJSON(jsonOut, sum, stdout); rc != 0 {
			return rc
		}
		return exit
	}

	fmt.Fprintln(stdout)
	if flags.bench >= 2 {
		fmt.Fprintln(stdout, renderBenchAllTable(benches))
	} else {
		fmt.Fprintln(stdout, renderCheckAllTable(rows))
	}
	fmt.Fprintf(stdout, "\n%d checked: %d ok, %d failed (skipped %d uncredentialed/unsupported)\n",
		len(rows), pass, fail, skipped)
	if fail > 0 {
		return 1
	}
	return 0
}

// probeResult is the structured outcome of one runProbe call. Used by
// both the single-provider and --all paths so their reported numbers
// can't drift.
type probeResult struct {
	modelID        string
	model          *catalog.Model
	reply          string
	latency        time.Duration
	stopReason     string
	usage          agent.Usage
	costMicrocents int64
	err            error
}

// runProbe resolves the model, builds the provider, and issues one
// "say pong" Complete call. Returns the structured result rather than
// printing — the caller decides between single-provider detail and
// the --all summary row.
func runProbe(entry *catalog.Provider, lookup func(string) string) probeResult {
	modelID := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "MODEL"))
	if modelID == "" {
		modelID = compat.FirstModelID(entry)
	}
	if modelID == "" {
		return probeResult{err: fmt.Errorf("provider %q has no models; set %sMODEL or sync the catalog",
			entry.ID, brand.EnvPrefix)}
	}

	prov, _, err := compat.Build(entry, modelID, lookup)
	if err != nil {
		return probeResult{modelID: modelID, err: fmt.Errorf("build %s: %w", entry.ID, err)}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	start := time.Now()
	resp, err := prov.Complete(ctx, agent.CompletionRequest{
		Model:     modelID,
		System:    "Be terse.",
		Messages:  []agent.Message{{Role: agent.RoleUser, Content: "Say 'pong' in one word."}},
		MaxTokens: 16,
	})
	latency := time.Since(start)
	if err != nil {
		return probeResult{modelID: modelID, latency: latency, err: err}
	}

	model := entry.Models[modelID]
	return probeResult{
		modelID:        modelID,
		model:          model,
		reply:          resp.Message.Content,
		latency:        latency,
		stopReason:     string(resp.StopReason),
		usage:          resp.Usage,
		costMicrocents: computeCostMicrocents(model, resp.Usage),
	}
}

// streamingUnsupportedMessage explains that a resolved provider's adapter does
// not implement streaming, so the operator can fall back to the plain check.
// Every first-party provider family (anthropic, openai, google, bedrock,
// vertex, cohere, ollama, and openai-compatible vendors) now streams, so this
// is reached only by an adapter that genuinely lacks a streaming path — not the
// stale "only anthropic is wired" state it once described.
func streamingUnsupportedMessage(family string) string {
	return fmt.Sprintf("%s: provider family %q does not implement streaming in this build — re-run `%s provider check` without --stream",
		brand.CLI, family, brand.CLI)
}

// runStreamProbe issues the probe via the provider's streaming path
// (agent.StreamingProvider) and renders incoming text chunks inline.
// Errors out cleanly if the resolved provider doesn't implement
// streaming — that's not a failure of the provider, it's a
// not-yet-wired adapter, and the operator should know so they can
// fall back to the regular check.
func runStreamProbe(entry *catalog.Provider, lookup func(string) string, stdout, stderr io.Writer) int {
	modelID := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "MODEL"))
	if modelID == "" {
		modelID = compat.FirstModelID(entry)
	}
	if modelID == "" {
		fmt.Fprintf(stderr, "%s: provider %q has no models\n", brand.CLI, entry.ID)
		return 1
	}

	prov, _, err := compat.Build(entry, modelID, lookup)
	if err != nil {
		fmt.Fprintf(stderr, "%s: build %s: %v\n", brand.CLI, entry.ID, err)
		return 1
	}

	sp, ok := prov.(agent.StreamingProvider)
	if !ok {
		fmt.Fprintf(stderr, "%s\n", streamingUnsupportedMessage(string(entry.Family())))
		return 2
	}

	fmt.Fprintf(stdout, "streaming provider=%s model=%s family=%s …\n\n",
		entry.ID, modelID, entry.Family())

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	start := time.Now()
	resp, err := sp.CompleteStream(ctx, agent.CompletionRequest{
		Model:    modelID,
		System:   "Be terse.",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "Say 'pong' in one word."}},
		// Bump MaxTokens above the regular probe so streaming has
		// room to be visibly progressive on chatty models. Still
		// trivially cheap.
		MaxTokens: 64,
	}, func(c agent.Chunk) error {
		if c.TextDelta != "" {
			fmt.Fprint(stdout, c.TextDelta)
		}
		if c.ToolUseStart != nil {
			fmt.Fprintf(stdout, "\n→ tool_use_start: %s (id=%s)\n", c.ToolUseStart.Name, c.ToolUseStart.ID)
		}
		if c.ToolInputJSONDelta != "" {
			// Tool input JSON is rendered raw so operators can see
			// how Anthropic streams it (one of the more surprising
			// parts of the API).
			fmt.Fprint(stdout, c.ToolInputJSONDelta)
		}
		if c.ToolUseStop != "" {
			fmt.Fprintf(stdout, "\n← tool_use_stop: %s\n", c.ToolUseStop)
		}
		return nil
	})
	latency := time.Since(start)
	fmt.Fprintln(stdout)

	if err != nil {
		fmt.Fprintf(stderr, "\nFAIL: %v (after %s)\n", err, latency.Truncate(time.Millisecond))
		return 1
	}

	model := entry.Models[modelID]
	cost := computeCostMicrocents(model, resp.Usage)
	fmt.Fprintf(stdout, "\nOK\n")
	fmt.Fprintf(stdout, "  total latency   : %s (wall-clock for the full stream)\n", latency.Truncate(time.Millisecond))
	fmt.Fprintf(stdout, "  stop_reason     : %s\n", resp.StopReason)
	fmt.Fprintf(stdout, "  tokens in / out : %d / %d\n", resp.Usage.InputTokens, resp.Usage.OutputTokens)
	if cost > 0 {
		fmt.Fprintf(stdout, "  this call cost  : $%s (%d microcents)\n",
			formatMicrocentsUSD(cost), cost)
	}
	return 0
}

// benchResult aggregates N runProbe calls for one provider.
type benchResult struct {
	entry      *catalog.Provider
	modelID    string
	iterations int
	successes  int
	failures   int
	latencies  []time.Duration // successful probes only — failures don't time meaningfully
	totalCost  int64
	lastErr    error
	stats      latencyStats // computed once via finalise
}

// latencyStats holds min/p50/p95/max for a successful-probe batch.
type latencyStats struct {
	min, p50, p95, max time.Duration
}

// runBench executes N probes against one provider. Sequential by
// design — concurrent probes would skew latencies for providers with
// per-key rate limits (Groq, Anthropic). N is small (typically 3–10);
// sequential is fine.
//
// When progress != nil and showProgress is true, prints one dot per
// completed probe so operators see progress on slow providers.
func runBench(entry *catalog.Provider, lookup func(string) string, n int, progress io.Writer, showProgress bool) benchResult {
	b := benchResult{entry: entry, iterations: n}
	if showProgress && progress != nil {
		fmt.Fprintf(progress, "  bench %s ×%d: ", entry.ID, n)
	}
	for range n {
		res := runProbe(entry, lookup)
		if b.modelID == "" {
			b.modelID = res.modelID
		}
		if res.err != nil {
			b.failures++
			b.lastErr = res.err
			if showProgress && progress != nil {
				fmt.Fprint(progress, "x")
			}
			continue
		}
		b.successes++
		b.latencies = append(b.latencies, res.latency)
		b.totalCost += res.costMicrocents
		if showProgress && progress != nil {
			fmt.Fprint(progress, ".")
		}
	}
	if showProgress && progress != nil {
		fmt.Fprintln(progress)
	}
	b.stats = computeLatencyStats(b.latencies)
	return b
}

// computeLatencyStats returns min/p50/p95/max for a slice of
// durations. Uses nearest-rank percentile (sort ascending, index
// ⌈p·n⌉) — exact median for odd n, lower-median for even, and
// matches `numpy.percentile(method="nearest")`. Sufficient for the
// small N (≤ ~30) operators will actually run.
func computeLatencyStats(lats []time.Duration) latencyStats {
	if len(lats) == 0 {
		return latencyStats{}
	}
	sorted := make([]time.Duration, len(lats))
	copy(sorted, lats)
	slices.Sort(sorted)
	pick := func(p float64) time.Duration {
		// nearest-rank: index = ⌈p·n⌉ − 1, clamped to [0, n-1]
		rank := int(float64(len(sorted))*p + 0.5)
		rank = max(rank, 1)
		rank = min(rank, len(sorted))
		return sorted[rank-1]
	}
	return latencyStats{
		min: sorted[0],
		p50: pick(0.50),
		p95: pick(0.95),
		max: sorted[len(sorted)-1],
	}
}

// toCheckRow projects a benchmark into a row the --all table can
// render. Uses p50 as the headline latency and total accumulated
// cost across all iterations.
func (b benchResult) toCheckRow() checkRow {
	r := checkRow{
		id:      b.entry.ID,
		family:  string(b.entry.Family()),
		model:   b.modelID,
		latency: b.stats.p50,
		cost:    b.totalCost,
		ok:      b.failures == 0,
	}
	if b.lastErr != nil {
		r.err = truncate(fmt.Sprintf("%d/%d failed: %v", b.failures, b.iterations, b.lastErr), 80)
	}
	return r
}

// --- human renderers ----------------------------------------------------

func emitSingleHuman(entry *catalog.Provider, res probeResult, stdout, stderr io.Writer) {
	fmt.Fprintf(stdout, "checking provider=%s model=%s family=%s …\n",
		entry.ID, res.modelID, entry.Family())
	if res.err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", res.err)
		if res.latency > 0 {
			fmt.Fprintf(stderr, "(latency: %s — failure before/during HTTP)\n", res.latency.Truncate(time.Millisecond))
		}
		return
	}
	fmt.Fprintf(stdout, "\nOK\n")
	fmt.Fprintf(stdout, "  reply           : %q\n", truncate(strings.TrimSpace(res.reply), 80))
	fmt.Fprintf(stdout, "  latency         : %s\n", res.latency.Truncate(time.Millisecond))
	fmt.Fprintf(stdout, "  stop_reason     : %s\n", res.stopReason)
	fmt.Fprintf(stdout, "  tokens in / out : %d / %d\n", res.usage.InputTokens, res.usage.OutputTokens)
	if res.model != nil && res.model.Cost != nil {
		fmt.Fprintf(stdout, "  model pricing   : $%.2f in / $%.2f out per MTok\n",
			res.model.Cost.Input, res.model.Cost.Output)
	}
	if res.costMicrocents > 0 {
		fmt.Fprintf(stdout, "  this call cost  : $%s (%d microcents)\n",
			formatMicrocentsUSD(res.costMicrocents), res.costMicrocents)
	} else if res.model == nil || res.model.Cost == nil {
		fmt.Fprintf(stdout, "  this call cost  : (no pricing in catalog for this model)\n")
	}
}

func emitBenchHuman(b benchResult, stdout io.Writer) {
	fmt.Fprintf(stdout, "\nbench result for %s (model=%s)\n", b.entry.ID, b.modelID)
	fmt.Fprintf(stdout, "  iterations      : %d (%d ok, %d failed)\n", b.iterations, b.successes, b.failures)
	if b.successes > 0 {
		fmt.Fprintf(stdout, "  min / p50 / p95 / max : %s / %s / %s / %s\n",
			b.stats.min.Truncate(time.Millisecond),
			b.stats.p50.Truncate(time.Millisecond),
			b.stats.p95.Truncate(time.Millisecond),
			b.stats.max.Truncate(time.Millisecond))
	}
	if b.totalCost > 0 {
		fmt.Fprintf(stdout, "  total cost      : $%s (%d microcents across %d successful probes)\n",
			formatMicrocentsUSD(b.totalCost), b.totalCost, b.successes)
	}
	if b.lastErr != nil {
		fmt.Fprintf(stdout, "  last error      : %s\n", truncate(b.lastErr.Error(), 100))
	}
}

// checkRow is one provider's outcome in the --all summary table.
type checkRow struct {
	id      string
	family  string
	model   string
	latency time.Duration
	cost    int64
	ok      bool
	err     string
}

// renderCheckAllTable lays out one row per probed provider. Columns
// are sized to the widest cell so misaligned terminals still scan
// cleanly. Cost is rendered via formatMicrocentsUSD for parity with
// the single-provider view.
func renderCheckAllTable(rows []checkRow) string {
	headers := []string{"STATUS", "PROVIDER", "FAMILY", "MODEL", "LATENCY", "COST"}
	cells := make([][]string, 0, len(rows))
	for _, r := range rows {
		status := "OK"
		if !r.ok {
			status = "FAIL"
		}
		lat := "-"
		if r.latency > 0 {
			lat = r.latency.Truncate(time.Millisecond).String()
		}
		cost := "-"
		if r.cost > 0 {
			cost = "$" + formatMicrocentsUSD(r.cost)
		} else if r.ok {
			cost = "(no price)"
		}
		cells = append(cells, []string{status, r.id, r.family, r.model, lat, cost})
	}
	return renderTable(headers, cells, rows)
}

// renderBenchAllTable lays out per-provider latency stats from a
// multi-probe benchmark. Each row shows MIN/P50/P95/MAX in addition
// to the basic identifying columns.
func renderBenchAllTable(benches []benchResult) string {
	headers := []string{"STATUS", "PROVIDER", "FAMILY", "MODEL", "MIN", "P50", "P95", "MAX", "OK/N", "TOTAL_COST"}
	cells := make([][]string, 0, len(benches))
	rows := make([]checkRow, 0, len(benches))
	for _, b := range benches {
		row := b.toCheckRow()
		rows = append(rows, row)

		status := "OK"
		if b.failures > 0 {
			status = "FAIL"
		}
		fmtLat := func(d time.Duration) string {
			if d == 0 {
				return "-"
			}
			return d.Truncate(time.Millisecond).String()
		}
		cost := "-"
		if b.totalCost > 0 {
			cost = "$" + formatMicrocentsUSD(b.totalCost)
		} else if b.successes > 0 {
			cost = "(no price)"
		}
		cells = append(cells, []string{
			status, b.entry.ID, string(b.entry.Family()), b.modelID,
			fmtLat(b.stats.min), fmtLat(b.stats.p50), fmtLat(b.stats.p95), fmtLat(b.stats.max),
			fmt.Sprintf("%d/%d", b.successes, b.iterations),
			cost,
		})
	}
	return renderTable(headers, cells, rows)
}

// renderTable is the shared column-aligned table renderer. `rows` is
// only used for the error-trailer lines below the table — separating
// it from `cells` lets the bench table reuse the layout without
// duplicating the alignment logic.
func renderTable(headers []string, cells [][]string, rows []checkRow) string {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, c := range cells {
		for i, v := range c {
			if len(v) > widths[i] {
				widths[i] = len(v)
			}
		}
	}
	var b strings.Builder
	writeRow := func(vals []string) {
		for i, v := range vals {
			if i > 0 {
				b.WriteString("  ")
			}
			fmt.Fprintf(&b, "%-*s", widths[i], v)
		}
		b.WriteByte('\n')
	}
	writeRow(headers)
	for _, c := range cells {
		writeRow(c)
	}
	for _, r := range rows {
		if r.err != "" {
			fmt.Fprintf(&b, "  ! %s: %s\n", r.id, r.err)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// --- JSON output --------------------------------------------------------

// jsonProbe is the per-provider JSON record. Field names use
// snake_case to match conventions in the existing controlplane API
// surface. Optional fields omit when zero so a successful probe and
// a failed one don't carry each other's fields.
type jsonProbe struct {
	Provider       string     `json:"provider"`
	Family         string     `json:"family"`
	Model          string     `json:"model"`
	OK             bool       `json:"ok"`
	Reply          string     `json:"reply,omitempty"`
	StopReason     string     `json:"stop_reason,omitempty"`
	InputTokens    int        `json:"input_tokens,omitempty"`
	OutputTokens   int        `json:"output_tokens,omitempty"`
	LatencyMS      int64      `json:"latency_ms"`
	CostMicrocents int64      `json:"cost_microcents"`
	Error          string     `json:"error,omitempty"`
	Bench          *jsonBench `json:"bench,omitempty"`
}

// jsonBench holds the multi-probe stats. Embedded inside jsonProbe
// only when --bench was used.
type jsonBench struct {
	Iterations          int   `json:"iterations"`
	Successes           int   `json:"successes"`
	Failures            int   `json:"failures"`
	MinMS               int64 `json:"min_ms"`
	P50MS               int64 `json:"p50_ms"`
	P95MS               int64 `json:"p95_ms"`
	MaxMS               int64 `json:"max_ms"`
	TotalCostMicrocents int64 `json:"total_cost_microcents"`
}

// jsonSummary appears once per invocation; lets CI scripts gate on
// {failed: 0} without iterating probes.
type jsonSummary struct {
	Total   int `json:"total"`
	OK      int `json:"ok"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped,omitempty"`
}

// jsonOutput is the top-level shape emitted by --json. Stable
// machine-readable contract — see the phase report for the schema.
type jsonOutput struct {
	Probes  []jsonProbe `json:"probes"`
	Summary jsonSummary `json:"summary"`
}

func probeToJSON(entry *catalog.Provider, res probeResult) jsonProbe {
	jp := jsonProbe{
		Provider:       entry.ID,
		Family:         string(entry.Family()),
		Model:          res.modelID,
		OK:             res.err == nil,
		LatencyMS:      res.latency.Milliseconds(),
		CostMicrocents: res.costMicrocents,
	}
	if res.err != nil {
		jp.Error = res.err.Error()
		return jp
	}
	jp.Reply = strings.TrimSpace(res.reply)
	jp.StopReason = res.stopReason
	jp.InputTokens = res.usage.InputTokens
	jp.OutputTokens = res.usage.OutputTokens
	return jp
}

func (b benchResult) toJSON() jsonProbe {
	jp := jsonProbe{
		Provider:       b.entry.ID,
		Family:         string(b.entry.Family()),
		Model:          b.modelID,
		OK:             b.failures == 0,
		LatencyMS:      b.stats.p50.Milliseconds(),
		CostMicrocents: b.totalCost,
		Bench: &jsonBench{
			Iterations:          b.iterations,
			Successes:           b.successes,
			Failures:            b.failures,
			MinMS:               b.stats.min.Milliseconds(),
			P50MS:               b.stats.p50.Milliseconds(),
			P95MS:               b.stats.p95.Milliseconds(),
			MaxMS:               b.stats.max.Milliseconds(),
			TotalCostMicrocents: b.totalCost,
		},
	}
	if b.lastErr != nil {
		jp.Error = b.lastErr.Error()
	}
	return jp
}

// summaryFromProbes builds the top-level summary block for the
// single-probe (or single-bench) case. The --all path constructs
// the summary itself so it can include skipped count.
func summaryFromProbes(probes ...jsonProbe) jsonSummary {
	s := jsonSummary{Total: len(probes)}
	for _, p := range probes {
		if p.OK {
			s.OK++
		} else {
			s.Failed++
		}
	}
	return s
}

// emitJSON marshals the output and prints it. Indented for human
// readability when scripts pipe through `jq` or `less`; the
// indentation costs nothing and a one-shot CI gate already invokes
// jq on the bytes anyway.
func emitJSON(probes []jsonProbe, summary jsonSummary, stdout io.Writer) int {
	out := jsonOutput{Probes: probes, Summary: summary}
	buf, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(stdout, `{"error":"marshal: %s"}`, err)
		return 1
	}
	fmt.Fprintln(stdout, string(buf))
	return 0
}

// --- helpers shared with M1.p / M1.p.x ---------------------------------

// autoPickFromCatalog mirrors the daemon's auto-pick logic: first
// supported family + has-credentials wins. Deterministic via the
// catalog's sort order.
func autoPickFromCatalog(cat *catalog.Catalog, lookup func(string) string) *catalog.Provider {
	for _, entry := range cat.ProviderList() {
		if !compat.IsSupportedFamily(entry.Family()) {
			continue
		}
		if !entry.HasCredentials(lookup) {
			continue
		}
		return entry
	}
	return nil
}

// computeCostMicrocents returns the USD-microcents cost of a single
// call given the model's pricing and the actual token usage.
//
//	cost = (in_tokens * input_microcents_per_MTok / 1e6) + (out_tokens * output_microcents_per_MTok / 1e6)
//
// Matches the Governor's pricing arithmetic (kernel/governor/pricing.go)
// so `agt provider check` cost agrees with what the daemon would
// account for the same call.
func computeCostMicrocents(model *catalog.Model, usage agent.Usage) int64 {
	if model == nil || model.Cost == nil {
		return 0
	}
	inPrice := model.Cost.InputMicrocentsPerMTok()
	outPrice := model.Cost.OutputMicrocentsPerMTok()
	const tokensPerMTok = 1_000_000
	return (int64(usage.InputTokens)*inPrice)/tokensPerMTok +
		(int64(usage.OutputTokens)*outPrice)/tokensPerMTok
}

// formatMicrocentsUSD renders microcents as a USD string. 1 USD = 10^9
// microcents. Uses integer arithmetic throughout — float64 can't
// represent sub-cent values precisely (4500 microcents = $0.0000045
// would round to $0.000005 with %.6f).
//
//	0             → "0.00"
//	500_000       → "0.0005"     (half a cent)
//	1_000_000     → "0.001"      (one cent)
//	17_500_000    → "0.0175"     (claude small-call cost)
//	1_000_000_000 → "1.00"
//	999_999_999   → "0.999999999"
func formatMicrocentsUSD(mc int64) string {
	if mc < 0 {
		return "-" + formatMicrocentsUSD(-mc)
	}
	dollars := mc / 1_000_000_000
	sub := mc % 1_000_000_000 // 0 .. 999_999_999 sub-dollar microcents
	subStr := fmt.Sprintf("%09d", sub)
	subStr = strings.TrimRight(subStr, "0")
	if len(subStr) < 2 {
		subStr += strings.Repeat("0", 2-len(subStr))
	}
	return fmt.Sprintf("%d.%s", dollars, subStr)
}

func truncate(s string, n int) string {
	return strutil.Ellipsis(s, n, "…")
}

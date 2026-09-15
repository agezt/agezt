// SPDX-License-Identifier: MIT
//
// cmd/agt provider check entry + flags + single check + caps check.
// Split from check.go during Day 211 god-file refactor (#39).
// Public API unchanged.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/plugins/providers/compat"
)

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
	lookup := catalogCredentialLookup(cat, credStore.Lookup)

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
	Provider       string   `json:"provider"`
	Family         string   `json:"family"`
	Model          string   `json:"model"`
	ToolCall       bool     `json:"tool_call"`
	Reasoning      bool     `json:"reasoning"`
	Vision         bool     `json:"vision"`
	Attachment     bool     `json:"attachment"`
	JSONMode       bool     `json:"json_mode"`
	StrictToolArgs bool     `json:"strict_tool_args"`
	PromptCache    bool     `json:"prompt_cache"`
	InputModes     []string `json:"input_modalities,omitempty"`
	OutputModes    []string `json:"output_modalities,omitempty"`
	ContextLimit   int      `json:"context_limit,omitempty"`
	OutputLimit    int      `json:"output_limit,omitempty"`
	Knowledge      string   `json:"knowledge,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
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
		Provider:       entry.ID,
		Family:         string(entry.Family()),
		Model:          modelID,
		ToolCall:       model.ToolCall,
		Reasoning:      model.Reasoning,
		Vision:         model.SupportsVision(),
		Attachment:     model.Attachment,
		JSONMode:       catalog.FamilySupportsNativeJSONMode(entry.Family()),
		StrictToolArgs: model.SupportsStrictToolArgs(),
		PromptCache:    model.SupportsPromptCache(),
		InputModes:     model.Modalities.Input,
		OutputModes:    model.Modalities.Output,
		ContextLimit:   model.Limit.Context,
		OutputLimit:    model.Limit.Output,
		Knowledge:      model.Knowledge,
		Warnings:       model.AgentWarnings(),
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
	fmt.Fprintf(stdout, "  strict tool args: %s\n", yn(c.StrictToolArgs))
	fmt.Fprintf(stdout, "  prompt caching  : %s\n", yn(c.PromptCache))
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

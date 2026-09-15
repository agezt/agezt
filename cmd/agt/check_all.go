// SPDX-License-Identifier: MIT
//
// cmd/agt provider check all sub-command: iterates credentialed providers,
// runs each through runProbe (or runBench), renders a summary table.
// Split from check.go during Day 211 god-file refactor (#39/#50).
// Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/plugins/providers/compat"
)

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
	if resp == nil {
		// A provider returning (nil, nil) violates the contract; this CLI probe
		// runs against a raw provider (not the daemon governor that normalizes
		// this), so guard the deref here rather than panicking the check.
		return probeResult{modelID: modelID, latency: latency, err: fmt.Errorf("provider returned a nil response without an error")}
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

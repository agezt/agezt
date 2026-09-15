// SPDX-License-Identifier: MIT
//
// cmd/agt provider check --caps (capability matrix) sub-command:
// network-free, credential-free survey of supported providers and their
// models. Extracted from check_all.go during Day 211 god-file refactor (#50).
// Public API unchanged.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/plugins/providers/compat"
)

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
			ContextLimit:   model.Limit.Context,
			Warnings:       model.AgentWarnings(),
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

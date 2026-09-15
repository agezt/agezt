// SPDX-License-Identifier: MIT

// Model-advisory + credential redaction helpers (modelAdvisory, credSecrets,
// extraRedactLiterals). Extracted from main_overlay.go during Day 211
// god-file refactor (#56). Public API unchanged.
package main

import (
	"os"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/creds"
)

func modelAdvisory(cat *catalog.Catalog, model string) string {
	if cat == nil || model == "" {
		return ""
	}
	_, m := cat.FindModel(model)
	if m == nil {
		return ""
	}
	return strings.Join(m.AgentWarnings(), "; ")
}
func credSecrets(store *creds.Store) []string {
	names := store.Names()
	vals := make([]string, 0, len(names))
	for _, n := range names {
		if v := store.Get(n); v != "" {
			vals = append(vals, v)
		}
	}
	vals = append(vals, extraRedactLiterals()...)
	return vals
}
func extraRedactLiterals() []string {
	spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "REDACT_EXTRA"))
	if spec == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(spec, ";") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

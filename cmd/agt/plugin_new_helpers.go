// SPDX-License-Identifier: MIT
//
// cmd/agt `plugin new` shared helpers (pluginNewUsage, sanitizeToolName).
// Extracted from plugin_new.go during Day 211 god-file refactor (#101).
// Public API unchanged.
package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/agezt/agezt/internal/brand"
)

func pluginNewUsage(w io.Writer) {
	fmt.Fprintf(w, "usage: %s plugin new <name> [--dir <path>] [--module <modulepath>]\n", brand.CLI)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "Scaffold a new Go tool plugin built on the agezt SDK.\n")
	fmt.Fprintf(w, "  --dir <path>          target directory (default: ./<name>)\n")
	fmt.Fprintf(w, "  --module <modulepath> go module path (default: agezt-plugin-<name>)\n")
}
func sanitizeToolName(name string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == ' ' || r == '.' || r == '/':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		default:
			// drop other runes entirely
		}
	}
	return strings.Trim(b.String(), "-")
}

// SPDX-License-Identifier: MIT
//
// cmd/agt provider-keys helpers (providerFlag, keyRequest, targetLabel, flagSnippet).
// Extracted from keys.go during Day 211 god-file refactor (#73).
// Public API unchanged.
package keys

import (
	"fmt"
	"io"
	"strings"

	"github.com/agezt/agezt/internal/brand"
)

func providerFlag(args []string, stderr io.Writer) (string, []string, bool) {
	provider := ""
	pos := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--provider" {
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s provider keys: --provider needs a value\n", brand.CLI)
				return "", nil, false
			}
			i++
			provider = args[i]
			continue
		}
		if strings.HasPrefix(a, "--provider=") {
			provider = strings.TrimPrefix(a, "--provider=")
			continue
		}
		pos = append(pos, a)
	}
	return strings.TrimSpace(provider), pos, true
}
func keyRequest(provider, env string) map[string]any {
	out := map[string]any{"env": env}
	if strings.TrimSpace(provider) != "" {
		out["provider"] = strings.TrimSpace(provider)
	}
	return out
}
func targetLabel(provider, env string) string {
	if strings.TrimSpace(provider) == "" {
		return env
	}
	return strings.TrimSpace(provider) + "/" + env
}
func flagSnippet(provider string) string {
	if strings.TrimSpace(provider) == "" {
		return ""
	}
	return "--provider " + strings.TrimSpace(provider) + " "
}

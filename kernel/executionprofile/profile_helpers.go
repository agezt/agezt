// SPDX-License-Identifier: MIT

// Execution profile helpers: toolSet + anyTool + presentTools.
// Code extracted from profile.go during the Day-71 god-file split. Public API unchanged.
package executionprofile


import (
	"sort"
	"strings"
)


func toolSet(tools []string) map[string]bool {
	out := make(map[string]bool, len(tools))
	for _, t := range tools {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			out[t] = true
		}
	}
	return out
}

func anyTool(tools map[string]bool, names ...string) bool {
	for _, n := range names {
		if tools[strings.ToLower(n)] {
			return true
		}
	}
	return false
}

func presentTools(tools map[string]bool, names ...string) []string {
	out := []string{}
	for _, n := range names {
		if tools[strings.ToLower(n)] {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

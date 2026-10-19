// SPDX-License-Identifier: MIT

package controlplane

// Lenient argument readers for the cognition commands: a count that accepts a
// number (truncated) or a string of decimal digits. Anything else reads as the
// zero value rather than an error.
//
// The boolean reader that used to live here moved with the Conductor to
// kernel/app/council (ask.go); the count reader stays until research_ask
// migrates to its own typed operation.

func dlInt(args map[string]any, key string) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		n := 0
		for _, r := range v {
			if r < '0' || r > '9' {
				return 0
			}
			n = n*10 + int(r-'0')
		}
		return n
	default:
		return 0
	}
}

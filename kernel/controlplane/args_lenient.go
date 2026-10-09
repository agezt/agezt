// SPDX-License-Identifier: MIT

package controlplane

// Lenient argument readers for the cognition commands: a flag that accepts a
// JSON boolean or the strings "true" and "1", and a count that accepts a
// number (truncated) or a string of decimal digits. Anything else reads as the
// zero value rather than an error.

func dlBool(args map[string]any, key string) bool {
	switch v := args[key].(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1"
	default:
		return false
	}
}

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

// SPDX-License-Identifier: MIT

package opaudit

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// ValueMax bounds a journaled string: an argument value or an error message.
const ValueMax = 160

// secretArgFragments mark an argument whose value must never be journaled.
// The bus redactor scrubs secrets it already knows, but a key arriving in
// this very request (provider_key_add, an OAuth code) is not known yet.
var secretArgFragments = []string{
	"key", "token", "secret", "password", "passwd", "passphrase", "cred",
	"auth", "cookie", "value", "private", "signature", "code", "state",
}

// IsSecretArg reports whether an argument's name marks its value as one that
// must never reach the journal.
func IsSecretArg(name string) bool {
	n := strings.ToLower(name)
	for _, frag := range secretArgFragments {
		if strings.Contains(n, frag) {
			return true
		}
	}
	return false
}

// TruncateValue bounds a journaled string to ValueMax runes, with an
// ellipsis when it was longer.
func TruncateValue(s string) string {
	if utf8.RuneCountInString(s) > ValueMax {
		return string([]rune(s)[:ValueMax]) + "…"
	}
	return s
}

// SummarizeArgs summarizes request arguments for the journal: scalars
// verbatim (strings truncated), secret-named arguments redacted, and nested
// values reduced to their shape so a secret inside them cannot reach the
// journal.
//
// An empty argument set summarizes to nothing at all (nil), so the journal
// payload carries no empty "args" member.
func SummarizeArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return nil
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		if IsSecretArg(k) {
			out[k] = "[redacted]"
			continue
		}
		switch x := v.(type) {
		case nil:
		case string:
			out[k] = TruncateValue(x)
		case bool, float64, int, int64:
			out[k] = x
		case []any:
			out[k] = "[list]"
		case map[string]any:
			keys := make([]string, 0, len(x))
			for kk := range x {
				keys = append(keys, kk)
			}
			sort.Strings(keys)
			out[k] = "[object: " + strings.Join(keys, ",") + "]"
		default:
			out[k] = "[value]"
		}
	}
	return out
}

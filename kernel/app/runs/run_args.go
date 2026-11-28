// SPDX-License-Identifier: MIT

package runs

import (
	"strings"

	"github.com/agezt/agezt/kernel/contract/opapi"
)

// textList reads a list of texts the way the family reads one: the members are
// trimmed and the blank ones dropped, and a list that was not sent is not a
// list at all.
//
// The members themselves are the row's business — a member that is not a text
// never reaches this reader.
func textList(list opapi.TextArray) ([]string, bool) {
	if !list.Present() {
		return nil, false
	}
	out := make([]string, 0, len(list.Members()))
	for _, member := range list.Members() {
		if member = strings.TrimSpace(member); member != "" {
			out = append(out, member)
		}
	}
	return out, true
}

// approveCaps parses an auto-approve grant — the ONE comma/space-separated
// string a caller sends OR a list of texts — into a capability set.
func approveCaps(list opapi.StrictTextList) map[string]bool {
	if !list.Present() {
		return nil
	}
	parts := list.Members()
	if list.IsText() {
		parts = []string{list.Text()}
	}
	out := map[string]bool{}
	for _, part := range parts {
		for _, field := range strings.FieldsFunc(part, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' }) {
			if field = strings.TrimSpace(field); field != "" {
				out[field] = true
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// truthy reads the trust flag the lenient way the family has always read it:
// a boolean, or the strings "on", "true", "yes" and "1".
//
// The flag's own string is kept as it came, so the family's spelling survives:
// the flag's reading is the family's, not the contract's.
func truthy(flag opapi.BoolFlag) bool {
	if b, ok := flag.Value().(bool); ok && b {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(flag.Text())) {
	case "on", "true", "yes", "1":
		return true
	}
	return false
}

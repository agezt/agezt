// SPDX-License-Identifier: MIT

package skill

import "strings"

// ActivationDirective is an explicit skill request embedded at the top of an
// intent. It is intentionally tiny and deterministic: runtime can strip the
// control line from the user-facing task while Forge receives the requested
// skill names/ids for activation.
type ActivationDirective struct {
	Explicit    bool
	Refs        []string
	CleanIntent string
}

// ParseActivationDirective recognizes leading slash directives:
//
//	/skill diagnose-ci
//	/skills diagnose-ci, webresearch
//
// Only leading lines are control lines; the first ordinary line begins the real
// task. If a directive is present but no task follows, CleanIntent is the
// original input so a bare "/skill foo" does not create an empty run.
func ParseActivationDirective(intent string) ActivationDirective {
	original := intent
	lines := strings.Split(intent, "\n")
	var refs []string
	i := 0
	for ; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" && len(refs) == 0 {
			continue
		}
		if !strings.HasPrefix(line, "/skill ") && !strings.HasPrefix(line, "/skills ") {
			break
		}
		var tail string
		if strings.HasPrefix(line, "/skills ") {
			tail = strings.TrimSpace(strings.TrimPrefix(line, "/skills "))
		} else {
			tail = strings.TrimSpace(strings.TrimPrefix(line, "/skill "))
		}
		refs = appendSkillRefs(refs, tail)
	}
	if len(refs) == 0 {
		return ActivationDirective{CleanIntent: original}
	}
	clean := strings.TrimSpace(strings.Join(lines[i:], "\n"))
	if clean == "" {
		clean = strings.TrimSpace(original)
	}
	return ActivationDirective{
		Explicit:    true,
		Refs:        refs,
		CleanIntent: clean,
	}
}

func appendSkillRefs(dst []string, raw string) []string {
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';'
	}) {
		ref := strings.TrimSpace(part)
		if ref == "" {
			continue
		}
		dup := false
		for _, existing := range dst {
			if strings.EqualFold(existing, ref) {
				dup = true
				break
			}
		}
		if !dup {
			dst = append(dst, ref)
		}
	}
	return dst
}

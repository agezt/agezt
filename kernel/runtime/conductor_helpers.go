// SPDX-License-Identifier: MIT

// Conductor helpers: conductorRoleSystem + conductorThinkerPrompt + conductorWorkerPrompt + parseVerdict + joinReason + parseRoleBriefs + extractRunnableCode + normalizeExecLang.
// Code extracted from conductor.go during the Day-76 god-file split. Public API unchanged.
package runtime


import (
	"fmt"
	"regexp"
	"strings"
)



func conductorRoleSystem(role, brief string) string {
	var base string
	switch role {
	case conductorRoleThinker:
		base = "You are the Thinker. Decompose the task and lay out a clear, concrete approach the Worker can follow. Do not write the full solution — plan it."
	case conductorRoleWorker:
		base = "You are the Worker. Produce the complete solution. When the task is code, write runnable code AND include self-tests (assertions) that fail loudly if the solution is wrong, in a single fenced code block."
	case conductorRoleVerifier:
		base = "You are the Verifier. Judge strictly and concretely whether the answer solves the task."
	}
	if strings.TrimSpace(brief) != "" {
		base = base + " " + strings.TrimSpace(brief)
	}
	return base
}

func conductorThinkerPrompt(task string) string {
	return "Task:\n" + task + "\n\nGive a concrete plan/approach for solving it."
}

func conductorWorkerPrompt(task, plan, feedback string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Task:\n%s\n\nThe Thinker's plan:\n%s\n", task, strings.TrimSpace(orPlaceholder(plan)))
	if strings.TrimSpace(feedback) != "" {
		fmt.Fprintf(&b, "\nThe Verifier rejected your previous attempt. Fix it:\n%s\n", strings.TrimSpace(feedback))
	}
	b.WriteString("\nProduce the complete solution now.")
	return b.String()
}

// parseVerdict reads a "PASS"/"FAIL" verdict from the first non-empty line
// (tolerant of leading markdown markers), keeping any inline remainder of that
// line plus the following lines as the reason. Anything that isn't clearly PASS
// is treated as fail.
func parseVerdict(text string) (verdict, reason string) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	head := ""
	idx := -1
	for i, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		head = strings.TrimLeft(ln, " #*->\t")
		idx = i
		break
	}
	rest := ""
	if idx >= 0 {
		rest = strings.TrimSpace(strings.Join(lines[idx+1:], "\n"))
	}
	lower := strings.ToLower(head)
	if strings.HasPrefix(lower, "pass") {
		r := joinReason(strings.TrimLeft(head[len("pass"):], " :*-\t"), rest)
		if r == "" {
			r = "verifier passed the answer"
		}
		return "pass", r
	}
	inline := head
	if strings.HasPrefix(lower, "fail") {
		inline = strings.TrimLeft(head[len("fail"):], " :*-\t")
	}
	r := joinReason(inline, rest)
	if r == "" {
		r = "verifier rejected the answer"
	}
	return "fail", r
}

// joinReason combines an inline remainder with trailing lines, omitting blanks.
func joinReason(inline, rest string) string {
	inline = strings.TrimSpace(inline)
	switch {
	case inline == "":
		return rest
	case rest == "":
		return inline
	default:
		return inline + "\n" + rest
	}
}

// parseRoleBriefs splits a plan blob into per-role instructions when it uses
// THINKER:/WORKER:/VERIFIER: section labels. Missing sections are simply absent.
func parseRoleBriefs(plan string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(plan) == "" {
		return out
	}
	labels := map[string]string{
		"thinker":  conductorRoleThinker,
		"worker":   conductorRoleWorker,
		"verifier": conductorRoleVerifier,
	}
	var cur string
	var buf []string
	flush := func() {
		if cur != "" {
			out[cur] = strings.TrimSpace(strings.Join(buf, "\n"))
		}
		buf = nil
	}
	for ln := range strings.SplitSeq(plan, "\n") {
		trimmed := strings.TrimLeft(ln, " #*->\t")
		lower := strings.ToLower(trimmed)
		matched := false
		for key, role := range labels {
			if strings.HasPrefix(lower, key+":") {
				flush()
				cur = role
				buf = append(buf, strings.TrimSpace(trimmed[len(key)+1:]))
				matched = true
				break
			}
		}
		if !matched && cur != "" {
			buf = append(buf, ln)
		}
	}
	flush()
	return out
}

// conductorCodeBlock matches a fenced code block, capturing the language tag and
// the body.
var conductorCodeBlock = regexp.MustCompile("(?s)```([a-zA-Z0-9_+-]*)\\s*\\n(.*?)```")

// extractRunnableCode returns the first fenced code block whose language maps to
// a code_exec-supported runtime, normalised to that runtime's language id.
func extractRunnableCode(text string) (lang, code string, ok bool) {
	for _, m := range conductorCodeBlock.FindAllStringSubmatch(text, -1) {
		norm := normalizeExecLang(m[1])
		if norm == "" {
			continue
		}
		body := strings.TrimSpace(m[2])
		if body == "" {
			continue
		}
		return norm, body, true
	}
	return "", "", false
}

// normalizeExecLang maps a fenced-block language tag to a code_exec language id,
// or "" if it isn't a runnable language.
func normalizeExecLang(tag string) string {
	switch strings.ToLower(strings.TrimSpace(tag)) {
	case "python", "py", "python3":
		return "python"
	case "javascript", "js", "node":
		return "javascript"
	case "typescript", "ts", "deno":
		return "typescript"
	default:
		return ""
	}
}

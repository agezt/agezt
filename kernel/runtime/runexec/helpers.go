// SPDX-License-Identifier: MIT

package runexec

import (
	"errors"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/roster"
)

// ErrHalted is the run-engine sentinel returned when a Run call
// hits a halted kernel. Mirrors runtime.ErrHalted for callers
// that want errors.Is from outside the runtime package. The
// canonical definition lives in the runtime package; the runexec
// package re-exports it so a host implementation of KernelAPI
// can use the same sentinel from its RunWith body without
// importing the runtime package (which would create a cycle).
var ErrHalted = errors.New("runtime: kernel is halted")

// shadowEvalLimit bounds how many shadow candidates are judged per
// run, so the extra (opt-in) provider calls stay bounded
// regardless of how many shadow skills match the intent. Moved
// from kernel/runtime/runexec.go on Day 32.
const shadowEvalLimit = 2

// assureVerifyMaxTokens bounds the verifier completion — it only
// emits a tiny JSON verdict, so a small cap keeps the completion
// check cheap. Moved from kernel/runtime/runexec.go on Day 33.
const assureVerifyMaxTokens = 400

// visionDescribeMaxTokens bounds the sidecar caption — a
// description, not an essay. Moved from kernel/runtime/runexec.go
// on Day 33.
const visionDescribeMaxTokens = 1024

// buildTranscript renders a compact, token-cheap summary of a
// run for the distiller: the tools used and the final answer.
// Pure helper — moved from kernel/runtime/runexec.go on Day 32.
func buildTranscript(toolNames []string, answer string) string {
	var b strings.Builder
	if len(toolNames) > 0 {
		b.WriteString("Tools used: ")
		b.WriteString(strings.Join(toolNames, ", "))
		b.WriteString("\n")
	}
	b.WriteString("Final answer:\n")
	b.WriteString(answer)
	return b.String()
}

// truncateHeuristicAnswer clamps a heuristic-bypass answer to a
// bounded size so a long date/time string never floods the
// journal with a megabyte payload. 4096 chars is generous.
// Moved from kernel/runtime/runexec.go on Day 33.
func truncateHeuristicAnswer(s string) string {
	const max = 4096
	if len(s) <= max {
		return s
	}
	return s[:max] + "…[truncated]"
}

// shouldRetireAgentAfterComplete reports whether the agent's
// lifecycle settings retire the profile on a successful run
// (RetireOnComplete flag or LifecycleRetireOnComplete mode).
// Moved from kernel/runtime/runexec.go on Day 33.
func shouldRetireAgentAfterComplete(l roster.AgentLifecycle) bool {
	return l.RetireOnComplete || strings.TrimSpace(l.Mode) == roster.LifecycleRetireOnComplete
}

// deterministicHeuristicBypass fast-paths obvious lookups
// ("what time is it", "today's date", "saat kaç") so the agent
// loop is not invoked for a one-token answer. Returns the canned
// reply and true on hit, or "" + false when no heuristic applies.
// Moved from kernel/runtime/runexec.go on Day 34.
func deterministicHeuristicBypass(intent string, now time.Time) (string, bool) {
	q := strings.ToLower(strings.TrimSpace(strings.Trim(intent, " ?!.\t\r\n")))
	switch q {
	case "time", "current time", "what time is it", "what is the time", "saat kac", "saat kaç":
		return "Current time: " + now.Format(time.RFC3339), true
	case "date", "today", "today's date", "what is today's date", "bugunun tarihi", "bugünün tarihi":
		return "Current date: " + now.Format("2006-01-02"), true
	default:
		return "", false
	}
}

// resetCompletedCycleTasks flips cycle-scoped tasks back to todo
// after a successful cycle, so the next iteration starts clean.
// Idempotent: a non-cycle task or a task that is not "done" is
// left alone. Moved from kernel/runtime/runexec.go on Day 33.
func resetCompletedCycleTasks(tasks []roster.AgentTask) {
	for i := range tasks {
		if strings.TrimSpace(tasks[i].Scope) == "cycle" && strings.TrimSpace(tasks[i].Status) == "done" {
			tasks[i].Status = "todo"
		}
	}
}

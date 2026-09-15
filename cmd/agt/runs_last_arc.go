// SPDX-License-Identifier: MIT
//
// cmd/agt runs last sub-command renderTaskArc helper (the task-arc renderer).
// Extracted from runs_last.go during Day 211 god-file refactor (#63).
// Public API unchanged.
package main

import (
	"fmt"
	"io"
)

func renderTaskArc(w io.Writer, corr string, summary map[string]any, events []map[string]any, outcomes map[string]childOutcome) {
	intent, _ := summary["intent"].(string)
	status, _ := summary["status"].(string)
	iters := intOfStatus(summary["iters"])
	duration := intOfStatus(summary["duration_ms"])

	fmt.Fprintf(w, "correlation: %s\n", corr)
	if intent != "" {
		fmt.Fprintf(w, "intent     : %s\n", intent)
	}
	reason, _ := summary["reason"].(string)
	switch status {
	case "completed":
		fmt.Fprintf(w, "status     : completed (%d iters, %s)\n", iters, fmtDuration(duration))
	case "running":
		fmt.Fprintf(w, "status     : running (no task.completed yet — abandoned?)\n")
	case "failed":
		// A failed run's arc must say WHY (M70): the reason is the first thing an
		// operator wants, and it was silently dropped before. duration shown when
		// the fold could compute it (FailedUnixMS − StartedUnixMS).
		line := "status     : failed"
		if reason != "" {
			line += " (" + reason + ")"
		}
		if duration > 0 {
			line += " after " + fmtDuration(duration)
		}
		fmt.Fprintln(w, line)
	default:
		fmt.Fprintf(w, "status     : %s\n", status)
	}
	// This run's own spend (M50) — shown only when it cost something. For a lead
	// this is its DIRECT spend; each delegation's cost is on its ↳ line below.
	if spent := mcFromAny(summary["spent_mc"]); spent > 0 {
		fmt.Fprintf(w, "spend      : %s\n", fmtUSD(spent))
	}
	fmt.Fprintln(w)

	// Group events into rounds. A "round" starts at llm.request
	// and ends at the next llm.response. Tool events between them
	// belong to that round. Events outside the round structure
	// (task.received, task.completed, policy.decision, etc.)
	// render inline at the position they appear.
	round := 0
	inRound := false
	var finalAnswer string
	// Per-call tool latency (M72): stash each tool.invoked timestamp by call_id
	// so the matching tool.result can show its wall-clock — the same
	// invoked→result span `agt tool log` reports, here inline on the arc.
	invokedAt := map[string]int64{}
	// Arc-footer tallies (M81): a long arc gets a one-line summary at the end so
	// an operator doesn't have to scroll back and count.
	toolCalls := 0
	toolErrors := 0

	for _, e := range events {
		kind, _ := e["kind"].(string)
		payload, _ := e["payload"].(map[string]any)
		seq := intOfStatus(e["seq"])
		ts := intOfStatus(e["ts_unix_ms"])
		switch kind {
		case "task.received":
			// Intent is shown in the header; surface image attachments (M93) as
			// run provenance so a vision run is self-evident in the arc.
			if n := intOfStatus(payload["images"]); n > 0 {
				fmt.Fprintf(w, "inputs     : %d image attachment(s)\n", n)
			}
		case "llm.request":
			round++
			fmt.Fprintf(w, "round %d (seq=%d)\n", round, seq)
			fmt.Fprintf(w, "  llm.request\n")
			inRound = true
		case "llm.response":
			fmt.Fprintf(w, "  llm.response")
			if usage, _ := payload["usage"].(map[string]any); usage != nil {
				in := intOfStatus(usage["input_tokens"])
				out := intOfStatus(usage["output_tokens"])
				fmt.Fprintf(w, "  (input=%d, output=%d tokens)", in, out)
			}
			fmt.Fprintln(w)
			fmt.Fprintln(w)
			inRound = false
		case "tool.invoked":
			tool, _ := payload["tool"].(string)
			if callID, _ := payload["call_id"].(string); callID != "" {
				invokedAt[callID] = ts // M72: anchor for the result's latency
			}
			indent := "  "
			if inRound {
				indent = "    "
			}
			line := fmt.Sprintf("%stool.invoked: %s", indent, tool)
			// Show a compact input excerpt so the arc says WHAT was called, not
			// just that something was (M68). input is journaled as raw JSON.
			if in := arcPreview(payload["input"]); in != "" {
				line += "  " + in
			}
			fmt.Fprintln(w, line)
		case "tool.result":
			indent := "  "
			if inRound {
				indent = "    "
			}
			// The agent journals this flag as "error" (agent.go); reading
			// "is_error" here meant the arc always said "ok", even for a failed
			// tool call (M68 fix). Honour the real field name.
			isErr, _ := payload["error"].(bool)
			toolCalls++
			if isErr {
				toolErrors++
			}
			tag := "ok"
			if isErr {
				tag = "ERROR"
			}
			line := fmt.Sprintf("%stool.result : %s", indent, tag)
			// Per-call latency (M72): the invoked→result span, joined by call_id.
			if callID, _ := payload["call_id"].(string); callID != "" {
				if it, ok := invokedAt[callID]; ok && ts >= it {
					line += fmt.Sprintf(" (%s)", fmtDuration(ts-it))
				}
			}
			if out := arcPreview(payload["output"]); out != "" {
				line += "  " + out
			}
			fmt.Fprintln(w, line)
		case "task.completed":
			// The run's final answer is journaled on task.completed (M51):
			// {iters, chars, stopped, answer}. Prefer it over the last
			// llm.response's content (the older, often-empty path) — it's the
			// authoritative end-of-run text and comes after every llm.response,
			// so it wins. Pre-M51 runs without the field fall back below.
			if a, _ := payload["answer"].(string); a != "" {
				finalAnswer = a
			}
		case "budget.consumed":
			// Per-round cost (M69): the governor stamps {model, cost_microcents,
			// input/output_tokens} on each priced LLM call. The header shows the
			// run's TOTAL spend (M50); this shows where it accrued, round by round.
			// Rendered inside the round's indent when we're mid-round.
			indent := "  "
			if inRound {
				indent = "    "
			}
			model, _ := payload["model"].(string)
			cost := mcFromAny(payload["cost_microcents"])
			line := indent + "budget: "
			if model != "" {
				line += model + " "
			}
			line += fmtUSD(cost)
			in := intOfStatus(payload["input_tokens"])
			out := intOfStatus(payload["output_tokens"])
			if in > 0 || out > 0 {
				line += fmt.Sprintf(" (in=%d, out=%d tokens)", in, out)
			}
			fmt.Fprintln(w, line)
		case "plan.started":
			// Plan-execution runs (M82): `agt runs show <plan-corr>` walks the
			// scheduler's plan.started + node.* events. Render them legibly instead
			// of as generic default-branch lines, so a plan run reads like a task arc.
			name, _ := payload["plan_name"].(string)
			nodeCount := intOfStatus(payload["node_count"])
			if name == "" {
				name = "(unnamed)"
			}
			fmt.Fprintf(w, "plan: %s (%d node(s))\n", name, nodeCount)
		case "node.started":
			nodeID, _ := payload["node_id"].(string)
			nodeKind, _ := payload["node_kind"].(string)
			fmt.Fprintf(w, "  node %s [%s] started\n", nodeID, nodeKind)
		case "node.completed":
			nodeID, _ := payload["node_id"].(string)
			nodeKind, _ := payload["node_kind"].(string)
			line := fmt.Sprintf("  node %s [%s] completed", nodeID, nodeKind)
			if ob := intOfStatus(payload["output_bytes"]); ob > 0 {
				line += fmt.Sprintf(" (%dB)", ob)
			}
			fmt.Fprintln(w, line)
		case "node.failed":
			nodeID, _ := payload["node_id"].(string)
			nodeKind, _ := payload["node_kind"].(string)
			nodeErr, _ := payload["error"].(string)
			line := fmt.Sprintf("  node %s [%s] FAILED", nodeID, nodeKind)
			if nodeErr != "" {
				line += ": " + nodeErr
			}
			fmt.Fprintln(w, line)
		case "task.failed":
			// The terminal failure, inline at the round it occurred (M70). The
			// header summarises it; this marks WHERE in the arc the run died.
			fr, _ := payload["reason"].(string)
			line := "  task.failed"
			if fr != "" {
				line += ": " + fr
			}
			fmt.Fprintln(w, line)
		case "policy.decision":
			// The agent journals {capability, allow, hard_denied, reason} — there
			// is no "decision" string, so the old read rendered a blank verdict on
			// every policy line (M68 fix). Derive the verdict from the real fields.
			capName, _ := payload["capability"].(string)
			allow, _ := payload["allow"].(bool)
			hard, _ := payload["hard_denied"].(bool)
			reason, _ := payload["reason"].(string)
			verdict := "allow"
			if !allow {
				verdict = "DENY"
				if hard {
					verdict = "DENY(hard)"
				}
			}
			line := fmt.Sprintf("  policy: %-10s %s", verdict, capName)
			if reason != "" {
				line += "  (" + reason + ")"
			}
			fmt.Fprintln(w, line)
		case "approval.requested", "approval.granted", "approval.denied", "approval.timeout":
			fmt.Fprintf(w, "  %s\n", kind)
		case "subagent.spawned":
			// Call out the delegation prominently with the child correlation
			// (drill in with `agt runs show <child>`) and the delegated task
			// (M41) — instead of the generic "subagent.spawned (seq=N)" line.
			child, _ := payload["child_correlation"].(string)
			task, _ := payload["task"].(string)
			task = truncate(task, 59)
			fmt.Fprintf(w, "  delegated → %s", child)
			if task != "" {
				fmt.Fprintf(w, "  (task: %s)", task)
			}
			fmt.Fprintln(w)
			// Show the sub-agent's terminal outcome inline (M44), so the
			// lead's arc tells the whole story without `agt runs show <child>`.
			if oc, ok := outcomes[child]; ok && oc.status != "" {
				statusStr := oc.status
				if oc.status == "failed" && oc.reason != "" {
					statusStr = "failed (" + oc.reason + ")"
				}
				durStr := ""
				if oc.status == "completed" || oc.status == "failed" {
					durStr = ", " + fmtDuration(oc.durationMS)
				}
				spendStr := ""
				if oc.spentMC > 0 {
					spendStr = ", " + fmtUSD(oc.spentMC) // M50: what this delegation cost
				}
				fmt.Fprintf(w, "    ↳ %s (%d iters%s%s)", statusStr, oc.iters, durStr, spendStr)
				// One-line preview of what the sub-agent answered (M52), so the
				// lead's arc shows the delegation's RESULT, not just its outcome.
				if oc.answerPreview != "" {
					fmt.Fprintf(w, ": %q", oc.answerPreview)
				}
				fmt.Fprintln(w)
			}
		default:
			// Surface unknown kinds at minimal verbosity so a future
			// kind doesn't silently vanish from the arc view.
			fmt.Fprintf(w, "  %s (seq=%d)\n", kind, seq)
		}

		// Capture the last assistant message content for the final
		// answer line. The agent loop's last llm.response carries
		// it.
		if kind == "llm.response" && payload != nil {
			if msg, _ := payload["message"].(map[string]any); msg != nil {
				if content, _ := msg["content"].(string); content != "" {
					finalAnswer = content
				}
			}
		}
	}

	// Summary footer (M81): collapse the arc into one glanceable line — rounds,
	// tool calls (with error count), spend, duration — so a long arc doesn't need
	// scrolling back to tally. Spend/duration come from the same folded summary
	// the header uses, so the footer never disagrees with it.
	footer := fmt.Sprintf("summary    : %d round(s), %d tool call(s)", round, toolCalls)
	if toolErrors > 0 {
		footer += fmt.Sprintf(" (%d error(s))", toolErrors)
	}
	if spent := mcFromAny(summary["spent_mc"]); spent > 0 {
		footer += ", " + fmtUSD(spent)
	}
	if duration > 0 {
		footer += ", " + fmtDuration(duration)
	}
	fmt.Fprintln(w, footer)

	if finalAnswer != "" {
		fmt.Fprintln(w, "final answer:")
		fmt.Fprintf(w, "  %s\n", finalAnswer)
	}
}

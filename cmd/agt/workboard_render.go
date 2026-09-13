// SPDX-License-Identifier: MIT
//
// cmd/agt workboard helpers: the parseWorkboard* arg parsers +
// workboardFlagValue (the flag walker) + callWorkboard (the dispatcher
// caller) + the renderWorkboard* renderers + workboardWatchTerminal
// (the watch-completion check).
// Extracted from workboard.go during the Day-209 god-file split.
// Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/strutil"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
)

func parseWorkboardIDJSON(args []string, name string, stderr io.Writer) (string, bool, bool) {
	id := ""
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s workboard %s: unexpected flag %q\n", brand.CLI, name, a)
				return "", false, false
			}
			if id != "" {
				fmt.Fprintf(stderr, "%s workboard %s: unexpected arg %q\n", brand.CLI, name, a)
				return "", false, false
			}
			id = a
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s workboard %s: task id required\n", brand.CLI, name)
		return "", false, false
	}
	return id, asJSON, true
}

func parseWorkboardAgentArgs(args []string, name string, stderr io.Writer) (string, string, string, bool, bool) {
	id := ""
	agent := ""
	runID := ""
	asJSON := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			asJSON = true
		case "--agent", "--run":
			val, ok := workboardFlagValue(args, &i, args[i], stderr, name)
			if !ok {
				return "", "", "", false, false
			}
			if args[i-1] == "--agent" {
				agent = val
			} else {
				runID = val
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(stderr, "%s workboard %s: unexpected flag %q\n", brand.CLI, name, args[i])
				return "", "", "", false, false
			}
			if id == "" {
				id = args[i]
				continue
			}
			fmt.Fprintf(stderr, "%s workboard %s: unexpected arg %q\n", brand.CLI, name, args[i])
			return "", "", "", false, false
		}
	}
	if id == "" || agent == "" {
		fmt.Fprintf(stderr, "usage: %s workboard %s <id> --agent A [--run R]\n", brand.CLI, name)
		return "", "", "", false, false
	}
	return id, agent, runID, asJSON, true
}

func parseWorkboardIDActorArgs(args []string, name, actorFlag string, stderr io.Writer) (string, bool, map[string]any, bool) {
	id := ""
	asJSON := false
	callArgs := map[string]any{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			asJSON = true
		case "--actor", "--author", "--body", "--reason", "--type", "--target":
			val, ok := workboardFlagValue(args, &i, a, stderr, name)
			if !ok {
				return "", false, nil, false
			}
			switch a {
			case "--actor", "--author":
				callArgs[actorFlag] = val
			default:
				callArgs[strings.TrimPrefix(a, "--")] = val
			}
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s workboard %s: unexpected flag %q\n", brand.CLI, name, a)
				return "", false, nil, false
			}
			if id == "" {
				id = a
				continue
			}
			fmt.Fprintf(stderr, "%s workboard %s: unexpected arg %q\n", brand.CLI, name, a)
			return "", false, nil, false
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s workboard %s: task id required\n", brand.CLI, name)
		return "", false, nil, false
	}
	return id, asJSON, callArgs, true
}

func workboardFlagValue(args []string, i *int, flag string, stderr io.Writer, cmd string) (string, bool) {
	if *i+1 >= len(args) {
		fmt.Fprintf(stderr, "%s workboard %s: %s needs a value\n", brand.CLI, cmd, flag)
		return "", false
	}
	*i = *i + 1
	return args[*i], true
}

func callWorkboard(cmd string, args map[string]any, stderr io.Writer) (map[string]any, int) {
	c := dialpkg.New(stderr)
	if c == nil {
		return nil, 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, cmd, args)
	if err != nil {
		fmt.Fprintf(stderr, "%s workboard: %v\n", brand.CLI, err)
		return nil, 1
	}
	return res, 0
}

func renderWorkboardMutation(res map[string]any, code int, asJSON bool, stdout io.Writer) int {
	if code != 0 {
		return code
	}
	task := mapAny(res["task"])
	if asJSON {
		return jsonout.Write(stdout, task)
	}
	renderWorkboardTaskLine(stdout, task)
	return 0
}

func renderWorkboardTaskLine(w io.Writer, task map[string]any) {
	assignee := str(task["assignee"])
	if assignee == "" {
		assignee = "-"
	}
	fmt.Fprintf(w, "%-26s %-8s p=%-3d %-16s %s\n",
		shortID(str(task["id"])), str(task["status"]), intNumber(task["priority"]), assignee, str(task["title"]))
}

func renderWorkboardTask(w io.Writer, task map[string]any) {
	fmt.Fprintf(w, "id:       %s\n", str(task["id"]))
	fmt.Fprintf(w, "title:    %s\n", str(task["title"]))
	fmt.Fprintf(w, "status:   %s\n", str(task["status"]))
	if d := str(task["description"]); d != "" {
		fmt.Fprintf(w, "desc:     %s\n", d)
	}
	if assignee := str(task["assignee"]); assignee != "" {
		fmt.Fprintf(w, "assignee: %s\n", assignee)
	}
	if priority := intNumber(task["priority"]); priority != 0 {
		fmt.Fprintf(w, "priority: %d\n", priority)
	}
	if s := str(task["seat"]); s != "" {
		fmt.Fprintf(w, "seat:     %s\n", s)
	}
	if reason := str(task["block_reason"]); reason != "" {
		fmt.Fprintf(w, "blocked:  %s\n", reason)
	}
	if policy := mapAny(task["retry_policy"]); len(policy) > 0 {
		fmt.Fprintf(w, "retry:    max_attempts=%d", intNumber(policy["max_attempts"]))
		if esc := str(policy["escalate_to"]); esc != "" {
			fmt.Fprintf(w, " escalate_to=%s", esc)
		}
		if failures := intNumber(task["failed_attempt_count"]); failures > 0 {
			fmt.Fprintf(w, " failures=%d", failures)
		}
		fmt.Fprintln(w)
	}
	if criteria, _ := task["criteria"].([]any); len(criteria) > 0 {
		met := intNumber(task["criteria_met"])
		verdict := "unproven"
		if proven, _ := task["proven"].(bool); proven {
			verdict = "PROVEN"
		}
		fmt.Fprintf(w, "proof:    %s (%d/%d criteria met)\n", verdict, met, len(criteria))
		for _, raw := range criteria {
			c := mapAny(raw)
			mark := "✗"
			if ok, _ := c["met"].(bool); ok {
				mark = "✓"
			}
			line := fmt.Sprintf("  %s %s", mark, str(c["text"]))
			if note := str(c["note"]); note != "" {
				line += " — " + note
			}
			fmt.Fprintln(w, line)
		}
		if pf := mapAny(task["proof"]); len(pf) > 0 {
			if v := mapAny(pf["verdict"]); len(v) > 0 {
				if gap := str(v["gap"]); gap != "" {
					fmt.Fprintf(w, "  gap: %s\n", gap)
				}
			}
			if ev := mapAny(pf["evidence"]); len(ev) > 0 {
				arts, _ := ev["artifacts"].([]any)
				fmt.Fprintf(w, "  evidence: %d artifact(s), journal #%d–#%d\n",
					len(arts), intNumber(ev["journal_from"]), intNumber(ev["journal_to"]))
			}
		}
	}
	if comments, _ := task["comments"].([]any); len(comments) > 0 {
		fmt.Fprintln(w, "comments:")
		for _, raw := range comments {
			c := mapAny(raw)
			fmt.Fprintf(w, "  - %s: %s\n", str(c["author"]), str(c["body"]))
		}
	}
	if deps, _ := task["dependencies"].([]any); len(deps) > 0 {
		fmt.Fprintln(w, "depends:")
		for _, raw := range deps {
			d := mapAny(raw)
			fmt.Fprintf(w, "  - %s\n", str(d["id"]))
		}
	}
	if links, _ := task["links"].([]any); len(links) > 0 {
		fmt.Fprintln(w, "links:")
		for _, raw := range links {
			l := mapAny(raw)
			fmt.Fprintf(w, "  - %s %s\n", str(l["type"]), str(l["target"]))
		}
	}
}

func renderWorkboardWatch(w io.Writer, res map[string]any) {
	task := mapAny(res["task"])
	renderWorkboardTask(w, task)
	if runID := str(res["run_id"]); runID != "" {
		fmt.Fprintf(w, "run:      %s\n", runID)
	}
	events, _ := res["events"].([]any)
	if len(events) == 0 {
		fmt.Fprintln(w, "events:   none")
	} else {
		fmt.Fprintln(w, "events:")
		for _, raw := range events {
			e := mapAny(raw)
			payload := mapAny(e["payload"])
			detail := firstNonEmptyCLI(str(payload["phase"]), str(payload["action"]), str(payload["status"]), str(payload["error"]))
			if detail != "" {
				detail = " " + detail
			}
			fmt.Fprintf(w, "  #%d %s corr=%s%s\n", intNumber(e["seq"]), str(e["kind"]), shortID(str(e["correlation_id"])), detail)
		}
	}
	if blocked, _ := res["blocked_dependencies"].([]any); len(blocked) > 0 {
		fmt.Fprintln(w, "blocked dependencies:")
		for _, raw := range blocked {
			d := mapAny(raw)
			title := str(d["title"])
			if title != "" {
				title = " " + title
			}
			fmt.Fprintf(w, "  - %s%s (%s)\n", str(d["id"]), title, str(d["status"]))
		}
	}
}

func workboardWatchTerminal(res map[string]any) bool {
	task := mapAny(res["task"])
	switch str(task["status"]) {
	case "blocked", "review", "done", "archived":
		return true
	default:
		return false
	}
}

func firstNonEmptyCLI(items ...string) string { return strutil.FirstNonEmpty(items...) }

func mapAny(v any) map[string]any {
	m, _ := v.(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}

func truthy(v any) bool {
	b, _ := v.(bool)
	return b
}

// SPDX-License-Identifier: MIT

// agt skill view helpers: renderSkillLine + shadowProgress + renderSkillEventDetail + shortID.
// Code extracted from skill.go during the Day-97 god-file split.
// Public API unchanged.
package main


import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func renderSkillLine(sk map[string]any) string {
	id, _ := sk["id"].(string)
	if len(id) > 12 {
		id = id[:12]
	}
	status, _ := sk["status"].(string)
	name, _ := sk["name"].(string)
	desc, _ := sk["description"].(string)
	line := fmt.Sprintf("  %s [%s] %s", id, status, name)
	if desc != "" {
		line += " — " + desc
	}
	// For a shadow skill, surface its evaluation progress toward promotion
	// (SPEC-05 §5.2 / M402): "· shadow <wins>/<evals>".
	if status == "shadow" {
		if ev, wins, ok := shadowProgress(sk); ok {
			line += fmt.Sprintf("  · shadow %d/%d", wins, ev)
		}
	}
	return line
}

// shadowProgress pulls the shadow-evaluation counters from a skill map's metrics,
// reporting (evals, wins, present). Returns ok=false when no evaluations yet.
func shadowProgress(sk map[string]any) (evals, wins int, ok bool) {
	m, _ := sk["metrics"].(map[string]any)
	if m == nil {
		return 0, 0, false
	}
	ev, _ := m["shadow_evals"].(float64)
	w, _ := m["shadow_wins"].(float64)
	if ev == 0 {
		return 0, 0, false
	}
	return int(ev), int(w), true
}

// renderSkillEventDetail summarizes a lifecycle event's payload for `history`.
func renderSkillEventDetail(kind string, p map[string]any) string {
	switch kind {
	case "skill.promoted":
		return fmt.Sprintf("%v -> %v", p["from"], p["to"])
	case "skill.quarantined":
		if r, _ := p["reason"].(string); r != "" {
			return "reason: " + r
		}
		return "(no reason)"
	case "skill.reverted":
		if r, _ := p["restored"].(string); r != "" {
			return "restored " + shortID(r)
		}
		return "archived"
	case "skill.restored":
		return fmt.Sprintf("%v -> %v", p["from"], p["to"])
	case "skill.created":
		return fmt.Sprintf("%v", p["name"])
	case "skill.shared":
		if fa, _ := p["from_agent"].(string); fa != "" {
			return "shared (was private to " + fa + ")"
		}
		return "shared"
	case "skill.reassigned":
		return fmt.Sprintf("%v -> %v", p["from_agent"], p["to_agent"])
	case "skill.activated":
		return fmt.Sprintf("matched %v", p["matched"])
	default:
		return ""
	}
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func cmdSkillHygiene(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	idleDays := 0
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--json":
			asJSON = true
		case args[i] == "--idle-days" && i+1 < len(args):
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 {
				fmt.Fprintf(stderr, "%s skill hygiene: bad --idle-days %q\n", brand.CLI, args[i])
				return 2
			}
			idleDays = n
		case strings.HasPrefix(args[i], "--idle-days="):
			n, err := strconv.Atoi(strings.TrimPrefix(args[i], "--idle-days="))
			if err != nil || n <= 0 {
				fmt.Fprintf(stderr, "%s skill hygiene: bad --idle-days\n", brand.CLI)
				return 2
			}
			idleDays = n
		case strings.HasPrefix(args[i], "--"):
			// unknown flag, skip
		default:
			fmt.Fprintf(stderr, "%s skill hygiene: unexpected arg %q\n", brand.CLI, args[i])
			return 2
		}
	}
	callArgs := map[string]any{}
	if idleDays > 0 {
		callArgs["idle_days"] = idleDays
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdSkillHygiene, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s skill hygiene: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	days := intNumber(res["idle_days"])
	total := intNumber(res["total"])
	active := intNumber(res["active"])
	idleCount := intNumber(res["idle_count"])
	fmt.Fprintf(stdout, "skill hygiene (idle ≥ %d days):\n", days)
	fmt.Fprintf(stdout, "  total:   %d\n", total)
	fmt.Fprintf(stdout, "  active:  %d\n", active)
	fmt.Fprintf(stdout, "  idle:    %d\n", idleCount)
	if idle, _ := res["idle"].([]any); len(idle) > 0 {
		fmt.Fprintf(stdout, "\nidle skills:\n")
		for _, raw := range idle {
			sk, _ := raw.(map[string]any)
			if sk == nil {
				continue
			}
			name := str(sk["name"])
			if name == "" {
				name = str(sk["id"])
			}
			uses := intNumber(sk["uses"])
			lastUsed := intNumber(sk["last_used_ms"])
			detail := "never used"
			if lastUsed > 0 {
				detail = fmt.Sprintf("last used %s", time.UnixMilli(int64(lastUsed)).Format(time.RFC3339))
			}
			fmt.Fprintf(stdout, "  %-30s  %d use(s), %s\n", name, uses, detail)
		}
	} else if idleCount == 0 {
		fmt.Fprintf(stdout, "\nno idle skills — all active skills have been used recently.\n")
	}
	return 0
}

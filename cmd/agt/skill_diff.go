// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// diffOp is one line of a unified-style diff: sign is ' ' (context), '-'
// (removed), or '+' (added).
type diffOp struct {
	sign byte
	text string
}

// lineDiff computes a line-level diff of a→b via the classic LCS, emitting a
// unified-style op sequence (M118). Pure and deterministic, so it's unit-tested
// without a daemon. O(n·m) memory — fine for skill bodies, which are small.
func lineDiff(a, b []string) []diffOp {
	n, m := len(a), len(b)
	// lcs[i][j] = length of the longest common subsequence of a[i:] and b[j:].
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var out []diffOp
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			out = append(out, diffOp{' ', a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, diffOp{'-', a[i]})
			i++
		default:
			out = append(out, diffOp{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, diffOp{'-', a[i]})
	}
	for ; j < m; j++ {
		out = append(out, diffOp{'+', b[j]})
	}
	return out
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}

// fetchSkill returns a skill's body + lineage by id, or (-, -, false) if absent.
func fetchSkill(c *controlplane.Client, ctx context.Context, id string) (body string, lineage []string, found bool, err error) {
	res, cerr := c.Call(ctx, controlplane.CmdSkillGet, map[string]any{"id": id})
	if cerr != nil {
		return "", nil, false, cerr
	}
	if ok, _ := res["found"].(bool); !ok {
		return "", nil, false, nil
	}
	sk, _ := res["skill"].(map[string]any)
	body, _ = sk["body"].(string)
	if raw, ok := sk["lineage"].([]any); ok {
		for _, e := range raw {
			if s, ok := e.(string); ok {
				lineage = append(lineage, s)
			}
		}
	}
	return body, lineage, true, nil
}

// cmdSkillDiff implements `agt skill diff <id> [<id2>]` (M118). With one id it
// diffs the skill against its lineage parent (how it evolved); with two it diffs
// the first (old) against the second (new). Exit 0 = printed a diff (or
// identical), 3 = a skill was not found, 2 = usage.
func cmdSkillDiff(args []string, stdout, stderr io.Writer) int {
	var ids []string
	for _, a := range args {
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s skill diff <id> [<id2>]\n", brand.CLI)
			fmt.Fprintf(stdout, "one id: diff the skill against its lineage parent; two ids: diff old->new\n")
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s skill diff: unexpected flag %q\n", brand.CLI, a)
			return 2
		default:
			ids = append(ids, a)
		}
	}
	if len(ids) == 0 || len(ids) > 2 {
		fmt.Fprintf(stderr, "%s skill diff: need one or two skill ids\n", brand.CLI)
		return 2
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var oldID, newID, oldBody, newBody string
	if len(ids) == 2 {
		oldID, newID = ids[0], ids[1]
		ob, _, found, err := fetchSkill(c, ctx, oldID)
		if err != nil {
			fmt.Fprintf(stderr, "%s skill diff: %v\n", brand.CLI, err)
			return 1
		}
		if !found {
			fmt.Fprintf(stderr, "%s skill diff: %s not found\n", brand.CLI, oldID)
			return 3
		}
		nb, _, found, err := fetchSkill(c, ctx, newID)
		if err != nil {
			fmt.Fprintf(stderr, "%s skill diff: %v\n", brand.CLI, err)
			return 1
		}
		if !found {
			fmt.Fprintf(stderr, "%s skill diff: %s not found\n", brand.CLI, newID)
			return 3
		}
		oldBody, newBody = ob, nb
	} else {
		newID = ids[0]
		nb, lineage, found, err := fetchSkill(c, ctx, newID)
		if err != nil {
			fmt.Fprintf(stderr, "%s skill diff: %v\n", brand.CLI, err)
			return 1
		}
		if !found {
			fmt.Fprintf(stderr, "%s skill diff: %s not found\n", brand.CLI, newID)
			return 3
		}
		if len(lineage) == 0 {
			fmt.Fprintf(stderr, "%s skill diff: %s has no lineage parent — pass a second id to compare\n", brand.CLI, newID)
			return 2
		}
		oldID = lineage[len(lineage)-1] // most recent ancestor
		ob, _, pfound, err := fetchSkill(c, ctx, oldID)
		if err != nil {
			fmt.Fprintf(stderr, "%s skill diff: %v\n", brand.CLI, err)
			return 1
		}
		if !pfound {
			fmt.Fprintf(stderr, "%s skill diff: parent %s not found (archived?)\n", brand.CLI, oldID)
			return 3
		}
		oldBody, newBody = ob, nb
	}

	ops := lineDiff(splitLines(oldBody), splitLines(newBody))
	added, removed := 0, 0
	for _, op := range ops {
		switch op.sign {
		case '+':
			added++
		case '-':
			removed++
		}
	}
	fmt.Fprintf(stdout, "--- %s\n+++ %s\n", oldID, newID)
	if added == 0 && removed == 0 {
		fmt.Fprintln(stdout, "(identical bodies)")
		return 0
	}
	for _, op := range ops {
		fmt.Fprintf(stdout, "%c %s\n", op.sign, op.text)
	}
	fmt.Fprintf(stdout, "\n%d added, %d removed\n", added, removed)
	return 0
}

// SPDX-License-Identifier: MIT

// Package ciguard provides CI workflow linting: it checks that every
// self-hosted job triggered by pull_request has a fork-guard expression,
// preventing arbitrary fork PRs from consuming local runner capacity.
package ciguard

import (
	"regexp"
	"sort"
	"strings"
)

// ForkGuardExpr is the GitHub Actions expression that restricts a job to
// same-repository events (push, or a pull_request whose head is this repo, i.e.
// not a fork). Its presence in a job's `if:` is what this linter requires.
const ForkGuardExpr = "head.repo.full_name == github.repository"

var jobHeaderRE = regexp.MustCompile(`^  ([A-Za-z0-9_.-]+):\s*$`)

// Lint returns the names (sorted) of self-hosted jobs in a workflow that lack
// the fork guard. It returns nil when the workflow does not trigger on
// pull_request (the guard is only needed then) or when every self-hosted job is
// guarded. The input is the raw YAML text of one workflow file.
func Lint(workflow string) []string {
	if !triggersOnPullRequest(workflow) {
		return nil
	}
	var missing []string
	for name, body := range jobBlocks(workflow) {
		if !mentionsSelfHosted(body) {
			continue
		}
		if !strings.Contains(body, ForkGuardExpr) {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

func mentionsSelfHosted(jobBody string) bool {
	for _, ln := range strings.Split(jobBody, "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "runs-on:") && strings.Contains(t, "self-hosted") {
			return true
		}
	}
	return false
}

func jobBlocks(workflow string) map[string]string {
	lines := strings.Split(workflow, "\n")
	start := -1
	for i, ln := range lines {
		if ln == "jobs:" || (strings.HasPrefix(ln, "jobs:") && !strings.HasPrefix(ln, " ")) {
			start = i + 1
			break
		}
	}
	out := map[string]string{}
	if start < 0 {
		return out
	}
	var cur string
	var buf []string
	flush := func() {
		if cur != "" {
			out[cur] = strings.Join(buf, "\n")
		}
		buf = buf[:0]
	}
	for _, ln := range lines[start:] {
		if ln != "" && !strings.HasPrefix(ln, " ") {
			break
		}
		if m := jobHeaderRE.FindStringSubmatch(ln); m != nil {
			flush()
			cur = m[1]
			continue
		}
		if cur != "" {
			buf = append(buf, ln)
		}
	}
	flush()
	return out
}

func triggersOnPullRequest(workflow string) bool {
	lines := strings.Split(workflow, "\n")
	in := false
	for _, ln := range lines {
		isTop := ln != "" && !strings.HasPrefix(ln, " ")
		if isTop && (ln == "on:" || strings.HasPrefix(ln, "on:")) {
			if strings.Contains(ln, "pull_request") {
				return true
			}
			in = true
			continue
		}
		if in {
			if isTop {
				in = false
				continue
			}
			if strings.Contains(ln, "pull_request") {
				return true
			}
		}
	}
	return false
}

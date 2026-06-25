// SPDX-License-Identifier: MIT

// Package ciguard is a tiny, dependency-free linter for the repo's GitHub
// Actions workflows. It enforces one security invariant (V-004): every job that
// runs on a self-hosted runner and can be triggered by a fork pull_request must
// carry the same-repo fork guard, so untrusted fork-PR code never executes on a
// persistent self-hosted runner.
//
// This is a defense for the *convention*, not a replacement for it: the durable
// fix is ephemeral runners + a fork-PR approval policy. But because the guard is
// repeated per job, a future job added without it silently re-opens host-level
// compromise — this linter fails the build (via ciguard_test) the moment that
// happens, in PR review, before it can land on main.
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

// mentionsSelfHosted reports whether a job body targets a self-hosted runner
// (runs-on contains the "self-hosted" label, in either list or scalar form).
func mentionsSelfHosted(jobBody string) bool {
	for _, ln := range strings.Split(jobBody, "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "runs-on:") && strings.Contains(t, "self-hosted") {
			return true
		}
	}
	return false
}

// jobBlocks splits the `jobs:` section into one text block per job, keyed by job
// name. Job keys are the 2-space-indented mapping keys directly under `jobs:`.
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
		// A column-0 (non-space, non-empty) line ends the jobs: section.
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

// triggersOnPullRequest reports whether the workflow's top-level `on:` includes
// pull_request (in block, list, or scalar form).
func triggersOnPullRequest(workflow string) bool {
	lines := strings.Split(workflow, "\n")
	in := false
	for _, ln := range lines {
		isTop := ln != "" && !strings.HasPrefix(ln, " ")
		if isTop && (ln == "on:" || strings.HasPrefix(ln, "on:")) {
			if strings.Contains(ln, "pull_request") { // on: [push, pull_request]
				return true
			}
			in = true
			continue
		}
		if in {
			if isTop { // left the on: block
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

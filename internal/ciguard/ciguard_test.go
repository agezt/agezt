// SPDX-License-Identifier: MIT

package ciguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLint_FlagsUnguardedSelfHostedJob(t *testing.T) {
	wf := `name: x
on:
  pull_request:
jobs:
  guarded:
    if: github.event_name == 'push' || github.event.pull_request.head.repo.full_name == github.repository
    runs-on: [self-hosted, Linux, X64]
    steps: []
  unguarded:
    runs-on: [self-hosted, Linux, X64]
    steps: []
  hosted:
    runs-on: ubuntu-latest
    steps: []
`
	got := Lint(wf)
	if len(got) != 1 || got[0] != "unguarded" {
		t.Fatalf("Lint = %v, want [unguarded] (only the unguarded self-hosted job)", got)
	}
}

func TestLint_NoPullRequestTrigger_NoFindings(t *testing.T) {
	// A push-only workflow can't be triggered by a fork PR, so the guard isn't
	// required even on a self-hosted job.
	wf := `name: x
on:
  push:
    branches: [main]
jobs:
  build:
    runs-on: [self-hosted, Linux, X64]
    steps: []
`
	if got := Lint(wf); got != nil {
		t.Fatalf("Lint = %v, want nil for a push-only workflow", got)
	}
}

func TestLint_HostedJobsIgnored(t *testing.T) {
	// GitHub-hosted runners are ephemeral, so an unguarded hosted job is fine.
	wf := `name: x
on: [pull_request]
jobs:
  a:
    runs-on: ubuntu-latest
    steps: []
`
	if got := Lint(wf); got != nil {
		t.Fatalf("Lint = %v, want nil (hosted jobs need no fork guard)", got)
	}
}

func TestLint_AllGuarded_NoFindings(t *testing.T) {
	wf := `name: x
on:
  pull_request:
jobs:
  a:
    if: github.event.pull_request.head.repo.full_name == github.repository
    runs-on: [self-hosted, Linux, X64]
  b:
    if: github.event_name == 'push' || github.event.pull_request.head.repo.full_name == github.repository
    runs-on: [self-hosted, Linux, X64]
`
	if got := Lint(wf); got != nil {
		t.Fatalf("Lint = %v, want nil when every self-hosted job is guarded", got)
	}
}

// TestRealWorkflowsAreForkGuarded is the actual security guard: every committed
// workflow must pass the lint. If someone adds a self-hosted job triggered by
// pull_request without the fork guard, this fails in CI/PR review (V-004).
func TestRealWorkflowsAreForkGuarded(t *testing.T) {
	workflowFiles(t, func(name, src string) {
		if missing := Lint(src); len(missing) > 0 {
			t.Errorf("%s: self-hosted jobs triggered by pull_request lack the fork guard %q: %v\n"+
				"add `if: github.event_name == 'push' || github.event.pull_request.head.repo.full_name == github.repository` to each",
				name, ForkGuardExpr, missing)
		}
	})
}

func TestRealWorkflowCheckoutsDisableCredentialPersistence(t *testing.T) {
	workflowFiles(t, func(name, src string) {
		lines := strings.Split(src, "\n")
		for i, line := range lines {
			if !strings.Contains(line, "uses: actions/checkout@") {
				continue
			}
			if !nearbyLineContains(lines, i+1, 8, "persist-credentials: false") {
				t.Errorf("%s:%d: actions/checkout must set persist-credentials: false", name, i+1)
			}
		}
	})
}

func TestDependabotCoversSecuritySurfaces(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, ".github", "dependabot.yml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("dependabot config missing: %v", err)
	}
	src := string(b)
	for _, want := range []string{
		"package-ecosystem: gomod",
		"directory: /",
		"package-ecosystem: npm",
		"directory: /frontend",
		"directory: /sdk/typescript",
		"package-ecosystem: github-actions",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("dependabot config must include %q", want)
		}
	}
	if strings.Count(src, "interval: weekly") < 4 {
		t.Fatalf("dependabot config should check all configured ecosystems weekly")
	}
}

func TestEnvExampleIsTrackedAndSanitized(t *testing.T) {
	root := repoRoot(t)
	envExample := filepath.Join(root, ".env.example")
	b, err := os.ReadFile(envExample)
	if err != nil {
		t.Fatalf(".env.example missing: %v", err)
	}
	src := string(b)
	for _, want := range []string{
		"AGEZT_OVERSEER_FLEET_LOCK=",
		"AGEZT_WEB_ALLOWED_HOSTS=",
		"AGEZT_VAULT_PASSPHRASE=",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf(".env.example must document %s", want)
		}
	}
	for n, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || !strings.Contains(trimmed, "=") {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		name := strings.ToUpper(strings.TrimSpace(parts[0]))
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if value == "" {
			continue
		}
		if looksSecretName(name) {
			t.Fatalf(".env.example:%d must not contain a value for secret-like variable %s", n+1, name)
		}
	}

	gitignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(gitignore), "!.env.example") {
		t.Fatalf(".gitignore must unignore .env.example so the safe template can be tracked")
	}
}

func looksSecretName(name string) bool {
	for _, marker := range []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSPHRASE", "CREDENTIAL"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

func TestSetupGoSafeDoesNotUseSharedRunnerFallbacks(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, ".github", "actions", "setup-go-safe", "action.yml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("setup-go-safe action not found (%v)", err)
	}
	src := string(b)
	for _, bad := range []string{
		"actions-runner-2/_work/_tool",
		"RUNNER_TOOL_CACHE:-",
		"RUNNER_NAME:-shared",
	} {
		if strings.Contains(src, bad) {
			t.Fatalf("setup-go-safe action still contains broad/shared fallback %q", bad)
		}
	}
	for _, want := range []string{
		"${RUNNER_TOOL_CACHE:?",
		"${RUNNER_NAME:?",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("setup-go-safe action must fail fast when %s is unset", want)
		}
	}
}

// workflowsDir walks up from the test's working directory to locate
// .github/workflows. Skips (rather than fails) if the layout isn't found, so the
// unit tests above still run in unusual checkouts.
func workflowsDir(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	cand := filepath.Join(root, ".github", "workflows")
	if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
		return cand
	}
	t.Skip(".github/workflows not found walking up from cwd")
	return ""
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Skipf("getwd: %v", err)
	}
	for i := 0; i < 8; i++ {
		cand := filepath.Join(dir, ".github")
		if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Skip(".github not found walking up from cwd")
	return ""
}

func workflowFiles(t *testing.T, fn func(name, src string)) {
	t.Helper()
	dir := workflowsDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("cannot read %s: %v", dir, err)
	}
	checked := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yml") && !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		checked++
		fn(e.Name(), string(b))
	}
	if checked == 0 {
		t.Skip("no workflow files found to check")
	}
}

func TestLint_NoJobsSection_NoFindings(t *testing.T) {
	// A workflow with on: pull_request but no jobs: section should not crash.
	wf := `name: x
on: pull_request
`
	if got := Lint(wf); got != nil {
		t.Fatalf("Lint = %v, want nil when there are no jobs", got)
	}
}

func TestLint_OnSameLinePullRequest(t *testing.T) {
	// The `on: [pull_request, push]` inline form.
	wf := `name: x
on: [pull_request, push]
jobs:
  build:
    runs-on: [self-hosted, Linux, X64]
    steps: []
`
	if got := Lint(wf); len(got) != 1 || got[0] != "build" {
		t.Fatalf("Lint = %v, want [build]", got)
	}
}

func TestLint_TriggerScanningResetsOnOtherTopLevelKeys(t *testing.T) {
	// Simulates a workflow where `on:` has nested content and a later
	// top-level key (e.g. `env:`) appears before pull_request is mentioned.
	wf := `name: x
on:
  push:
    branches: [main]
env:
  FOO: bar
jobs:
  build:
    runs-on: [self-hosted, Linux, X64]
    steps: []
`
	if got := Lint(wf); got != nil {
		t.Fatalf("Lint = %v, want nil (push-only, no PR trigger)", got)
	}
}

func TestTriggersOnPullRequest_InlineTrigger(t *testing.T) {
	if !triggersOnPullRequest("on: [pull_request]\n") {
		t.Fatal("expected pull_request trigger detection")
	}
}

func nearbyLineContains(lines []string, start, maxLines int, needle string) bool {
	limit := start + maxLines
	if limit > len(lines) {
		limit = len(lines)
	}
	for i := start; i < limit; i++ {
		if strings.Contains(lines[i], needle) {
			return true
		}
	}
	return false
}

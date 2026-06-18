package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestAgentUsageDocumentsLifecycleAndManagedSubagents(t *testing.T) {
	var out bytes.Buffer
	if code := agentUsage(&out); code != 0 {
		t.Fatalf("agentUsage code = %d", code)
	}
	text := out.String()
	for _, want := range []string{
		"task|wake|repair|repair-status|pause|resume|retire|revive|remove",
		"show repair history and inflight work",
		"managed sub-agents",
		"woken/repaired through their parent/owner agent",
		"--with-subagents retires dependent sub-agents",
		"does not hard-delete child profiles",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("agent usage missing %q:\n%s", want, text)
		}
	}
}

func TestParseAgentFlagsPolicies(t *testing.T) {
	var stderr bytes.Buffer
	f, rest, ok := parseAgentFlags([]string{
		"--retry-attempts", "3",
		"--retry-backoff", "exponential",
		"--retry-base-sec", "10",
		"--retry-max-sec", "60",
		"--retry-on", "error,timeout",
		"--doctor-agent", "guardian",
		"--health-stale-sec", "3600",
		"--health-window", "900",
		"--health-threshold", "4",
		"--self-repair", "true",
		"--self-repair-attempts", "2",
		"--self-repair-escalate", "lead",
		"--silent-on-success", "true",
		"--disable-memory-writes", "true",
		"--notify-min-severity", "warning",
		"--notify-cooldown-sec", "14400",
		"worker",
	}, &stderr, "add")
	if !ok {
		t.Fatalf("parse failed: %s", stderr.String())
	}
	if !reflect.DeepEqual(rest, []string{"worker"}) {
		t.Fatalf("rest = %v", rest)
	}
	profile := map[string]any{}
	applyAgentPolicyFlags(profile, f)
	retry := profile["retry_policy"].(map[string]any)
	if retry["max_attempts"] != 3 || retry["backoff"] != "exponential" || retry["base_delay_sec"] != 10 || retry["max_delay_sec"] != 60 {
		t.Fatalf("retry policy = %#v", retry)
	}
	if got := retry["retry_on"]; !reflect.DeepEqual(got, []any{"error", "timeout"}) {
		t.Fatalf("retry_on = %#v", got)
	}
	health := profile["health_policy"].(map[string]any)
	if health["doctor_agent"] != "guardian" || health["stale_after_sec"] != 3600 || health["failure_window"] != 900 || health["failure_threshold"] != 4 {
		t.Fatalf("health policy = %#v", health)
	}
	repair := profile["self_repair"].(map[string]any)
	if repair["enabled"] != true || repair["max_attempts"] != 2 || repair["escalate_to"] != "lead" {
		t.Fatalf("self repair policy = %#v", repair)
	}
	noise := profile["noise_policy"].(map[string]any)
	if noise["silent_on_success"] != true || noise["disable_memory_writes"] != true ||
		noise["min_notify_severity"] != "warning" || noise["min_notify_interval_sec"] != 14400 {
		t.Fatalf("noise policy = %#v", noise)
	}
}

func TestParseAgentFlagsAdvancedProfileSettings(t *testing.T) {
	var stderr bytes.Buffer
	f, rest, ok := parseAgentFlags([]string{
		"--instructions", "stay quiet,check mailbox",
		"--tool-allow", "memory,shell",
		"--tool-deny", "browser",
		"--trust-ceiling", "L2",
		"--config", "AGEZT_MAX_ITER=8,AGEZT_DISABLE_HEURISTIC_BYPASS=on",
		"--lifecycle", "cycle",
		"--max-cycles", "5",
		"--cycle-task", "scan inbox,triage alerts",
		"--total-task", "finish migration",
		"ops",
	}, &stderr, "add")
	if !ok {
		t.Fatalf("parse failed: %s", stderr.String())
	}
	if !reflect.DeepEqual(rest, []string{"ops"}) {
		t.Fatalf("rest = %v", rest)
	}
	profile := map[string]any{}
	if err := applyAgentAdvancedFlags(profile, f); err != nil {
		t.Fatalf("apply advanced: %v", err)
	}
	if got := profile["instructions"]; !reflect.DeepEqual(got, []any{"stay quiet", "check mailbox"}) {
		t.Fatalf("instructions = %#v", got)
	}
	if got := profile["tool_allow"]; !reflect.DeepEqual(got, []any{"memory", "shell"}) {
		t.Fatalf("tool_allow = %#v", got)
	}
	if got := profile["tool_deny"]; !reflect.DeepEqual(got, []any{"browser"}) {
		t.Fatalf("tool_deny = %#v", got)
	}
	if profile["trust_ceiling"] != "L2" {
		t.Fatalf("trust_ceiling = %#v", profile["trust_ceiling"])
	}
	cfg := profile["config_overrides"].(map[string]any)
	if cfg["AGEZT_MAX_ITER"] != "8" || cfg["AGEZT_DISABLE_HEURISTIC_BYPASS"] != "on" {
		t.Fatalf("config_overrides = %#v", cfg)
	}
	life := profile["lifecycle"].(map[string]any)
	if life["mode"] != "cycle" || life["max_cycles"] != 5 || life["retire_on_complete"] != false {
		t.Fatalf("lifecycle = %#v", life)
	}
	tasks := profile["tasklist"].([]any)
	if len(tasks) != 3 {
		t.Fatalf("tasklist len = %d want 3: %#v", len(tasks), tasks)
	}
}

func TestApplyAgentAdvancedFlagsTaskScopePatchPreservesOtherScope(t *testing.T) {
	var stderr bytes.Buffer
	f, _, ok := parseAgentFlags([]string{"--cycle-task", "new cycle", "ops"}, &stderr, "set")
	if !ok {
		t.Fatalf("parse failed: %s", stderr.String())
	}
	profile := map[string]any{
		"tasklist": []any{
			map[string]any{"title": "old cycle", "scope": "cycle", "status": "todo"},
			map[string]any{"title": "keep total", "scope": "total", "status": "todo"},
		},
	}
	if err := applyAgentAdvancedFlags(profile, f); err != nil {
		t.Fatalf("apply advanced: %v", err)
	}
	tasks := profile["tasklist"].([]any)
	if len(tasks) != 2 {
		t.Fatalf("tasklist len = %d want 2: %#v", len(tasks), tasks)
	}
	if tasks[0].(map[string]any)["title"] != "keep total" || tasks[1].(map[string]any)["title"] != "new cycle" {
		t.Fatalf("tasklist patch = %#v", tasks)
	}
}

func TestBuildAgentTaskPayload(t *testing.T) {
	var stderr bytes.Buffer
	add, label, ok := buildAgentTaskPayload([]string{
		"add", "ops", "scan inbox", "--scope", "cycle", "--status", "doing", "--desc", "morning pass",
	}, &stderr)
	if !ok {
		t.Fatalf("add parse failed: %s", stderr.String())
	}
	if label != "added" || add["op"] != "add" || add["ref"] != "ops" || add["title"] != "scan inbox" ||
		add["scope"] != "cycle" || add["status"] != "doing" || add["description"] != "morning pass" {
		t.Fatalf("add payload = %#v label=%q", add, label)
	}

	set, label, ok := buildAgentTaskPayload([]string{"set", "ops", "task-1", "--status", "done"}, &stderr)
	if !ok {
		t.Fatalf("set parse failed: %s", stderr.String())
	}
	if label != "updated" || set["op"] != "update" || set["id"] != "task-1" || set["status"] != "done" {
		t.Fatalf("set payload = %#v label=%q", set, label)
	}

	done, label, ok := buildAgentTaskPayload([]string{"blocked", "ops", "task-2"}, &stderr)
	if !ok {
		t.Fatalf("status parse failed: %s", stderr.String())
	}
	if label != "blocked" || done["op"] != "update" || done["status"] != "blocked" {
		t.Fatalf("status payload = %#v label=%q", done, label)
	}

	rm, label, ok := buildAgentTaskPayload([]string{"remove", "ops", "task-3"}, &stderr)
	if !ok {
		t.Fatalf("remove parse failed: %s", stderr.String())
	}
	if label != "removed" || rm["op"] != "remove" || rm["id"] != "task-3" {
		t.Fatalf("remove payload = %#v label=%q", rm, label)
	}
}

func TestBuildAgentTaskPayloadRejectsEmptySet(t *testing.T) {
	var stderr bytes.Buffer
	if _, _, ok := buildAgentTaskPayload([]string{"set", "ops", "task-1"}, &stderr); ok {
		t.Fatalf("empty set accepted")
	}
}

func TestBuildAgentRemovePayloadCascades(t *testing.T) {
	var stderr bytes.Buffer
	ref, cascade, help, ok := buildAgentRemovePayload([]string{"ops", "--with-all"}, &stderr)
	if !ok || help {
		t.Fatalf("remove parse failed help=%v ok=%v stderr=%s", help, ok, stderr.String())
	}
	if ref != "ops" {
		t.Fatalf("ref = %q, want ops", ref)
	}
	for _, key := range []string{"standing", "schedules", "memory", "authored_memory", "skills", "config", "workspace", "subagents"} {
		if cascade[key] != true {
			t.Fatalf("--with-all missing cascade %s in %#v", key, cascade)
		}
	}

	stderr.Reset()
	ref, cascade, help, ok = buildAgentRemovePayload([]string{"ops", "--with-memory", "--with-authored-memory", "--with-workspace"}, &stderr)
	if !ok || help {
		t.Fatalf("remove memory parse failed help=%v ok=%v stderr=%s", help, ok, stderr.String())
	}
	if ref != "ops" || cascade["memory"] != true || cascade["authored_memory"] != true || cascade["workspace"] != true {
		t.Fatalf("memory cascade payload ref=%q cascade=%#v", ref, cascade)
	}
	if cascade["skills"] == true || cascade["subagents"] == true {
		t.Fatalf("specific memory flags should not enable unrelated cleanup: %#v", cascade)
	}
}

func TestAgentRemoveResultSummarySeparatesMemoryClasses(t *testing.T) {
	got := agentRemoveResultSummary(map[string]any{
		"standing_removed":            1,
		"schedules_removed":           2,
		"memories_forgotten":          3,
		"authored_memories_forgotten": 4,
		"skills_archived":             5,
		"configs_deleted":             6,
		"workspaces_deleted":          7,
		"subagents_retired":           8,
		"mailbox_messages_retained":   9,
	})
	want := "1 standing, 2 schedule, 3 private memory, 4 authored shared memory, 5 skill archived, 6 config deleted, 7 workspace deleted, 8 subagent retired, 9 mailbox/audit retained"
	if got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
	if got := agentRemoveResultSummary(map[string]any{"memories_forgotten": 0, "authored_memories_forgotten": 0}); got != "" {
		t.Fatalf("empty summary = %q", got)
	}
}

func TestBuildAgentWakeAndRepairPayloads(t *testing.T) {
	var stderr bytes.Buffer
	wake, ok := buildAgentWakePayload([]string{
		"ops", "check", "mailbox", "--reason", "manual", "--incident", "hop-1", "--root", "root-1", "--parent", "parent-1",
	}, &stderr)
	if !ok {
		t.Fatalf("wake parse failed: %s", stderr.String())
	}
	if wake["ref"] != "ops" || wake["intent"] != "check mailbox" || wake["reason"] != "manual" ||
		wake["incident_id"] != "hop-1" || wake["root_incident_id"] != "root-1" || wake["parent_incident_id"] != "parent-1" {
		t.Fatalf("wake payload = %#v", wake)
	}

	repair, ok := buildAgentRepairPayload([]string{"ops", "routing", "is", "broken", "--incident-id", "repair-1"}, &stderr)
	if !ok {
		t.Fatalf("repair parse failed: %s", stderr.String())
	}
	if repair["ref"] != "ops" || repair["reason"] != "routing is broken" || repair["incident_id"] != "repair-1" {
		t.Fatalf("repair payload = %#v", repair)
	}
}

func TestAgentListStatusSuffixShowsLiveWakeContext(t *testing.T) {
	got := agentListStatusSuffix(map[string]any{
		"status": map[string]any{
			"active_run_count":     float64(1),
			"active_phase":         "using tool",
			"active_tool":          "shell.exec",
			"active_model":         "gpt-5",
			"active_wake_source":   "standing",
			"active_standing_name": "ops events",
			"active_schedule_id":   "sched-ignored",
		},
	})
	if got != "live=1:using tool[shell.exec]#gpt-5@standing(ops events)" {
		t.Fatalf("live suffix = %q", got)
	}

	got = agentListStatusSuffix(map[string]any{
		"status": map[string]any{
			"wake_schedule_count": float64(2),
			"wake_standing_count": float64(1),
			"next_wake_label":     "check disks",
		},
	})
	if got != "wake=2 schedule/1 standing(check disks)" {
		t.Fatalf("wake suffix = %q", got)
	}

	if got := agentListStatusSuffix(map[string]any{}); got != "" {
		t.Fatalf("empty suffix = %q", got)
	}
}

func TestAgentListStateLabelShowsGraveyardSeparately(t *testing.T) {
	if got := agentListStateLabel(map[string]any{"enabled": true}); got != "enabled" {
		t.Fatalf("enabled state = %q", got)
	}
	if got := agentListStateLabel(map[string]any{"enabled": false}); got != "PAUSED" {
		t.Fatalf("paused state = %q", got)
	}
	if got := agentListStateLabel(map[string]any{"enabled": false, "retired": true}); got != "RETIRED" {
		t.Fatalf("retired state = %q", got)
	}
}

func TestPrintAgentImpactSummaryIncludesDependentResources(t *testing.T) {
	var out bytes.Buffer
	if ok := printAgentImpactSummary(&out, map[string]any{
		"standing_orders":                   []any{"watch logs"},
		"schedules":                         []any{"refresh (sch-1)"},
		"memories":                          []any{"ops note (mem-1)"},
		"authored_shared_memories":          []any{"ops shared note (mem-shared-1)"},
		"skills":                            []any{"ops skill (skill-1)"},
		"configs":                           []any{"agent/ops/runtime [internal]"},
		"subagents":                         []any{"ops-worker [parent]"},
		"subagent_standing_orders":          []any{"ops-worker: worker watch"},
		"subagent_schedules":                []any{"ops-worker: worker refresh (sch-2)"},
		"subagent_memories":                 []any{"ops-worker: worker note (mem-2)"},
		"subagent_authored_shared_memories": []any{"ops-worker: worker shared note (mem-shared-2)"},
		"subagent_skills":                   []any{"ops-worker: worker skill (skill-2)"},
		"subagent_configs":                  []any{"ops-worker: agent/ops-worker/runtime [internal]"},
	}); !ok {
		t.Fatalf("impact summary reported empty")
	}
	got := out.String()
	for _, want := range []string{
		"impact:",
		"standing orders (1):",
		"authored shared memory (1):",
		"agent config (1):",
		"dependent sub-agents (1):",
		"sub-agent standing orders (1):",
		"sub-agent schedules (1):",
		"sub-agent private memory (1):",
		"sub-agent authored shared memory (1):",
		"sub-agent skills (1):",
		"sub-agent config (1):",
		"ops shared note (mem-shared-1)",
		"ops-worker: worker skill (skill-2)",
		"ops-worker: agent/ops-worker/runtime [internal]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("impact summary missing %q:\n%s", want, got)
		}
	}
}

func TestPrintAgentImpactSummaryReportsEmpty(t *testing.T) {
	var out bytes.Buffer
	if ok := printAgentImpactSummary(&out, map[string]any{}); ok {
		t.Fatalf("empty impact reported non-empty")
	}
	if out.String() != "" {
		t.Fatalf("empty impact wrote output: %q", out.String())
	}
}

func TestParseAgentFlagsRejectsInvalidPolicyValues(t *testing.T) {
	var stderr bytes.Buffer
	if _, _, ok := parseAgentFlags([]string{"--retry-attempts", "-1", "worker"}, &stderr, "add"); ok {
		t.Fatalf("negative retry attempts accepted")
	}
	stderr.Reset()
	if _, _, ok := parseAgentFlags([]string{"--self-repair", "maybe", "worker"}, &stderr, "add"); ok {
		t.Fatalf("invalid self-repair bool accepted")
	}
	stderr.Reset()
	f, _, ok := parseAgentFlags([]string{"--config", "broken", "worker"}, &stderr, "add")
	if !ok {
		t.Fatalf("parse should accept raw config text for apply validation: %s", stderr.String())
	}
	if err := applyAgentAdvancedFlags(map[string]any{}, f); err == nil {
		t.Fatalf("invalid config accepted")
	}
}

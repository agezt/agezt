// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/internal/brand"
)

const (
	compareTargetAll      = "all"
	compareTargetOpenClaw = "openclaw"
	compareTargetHermes   = "hermes"

	compareStatusSupported = "supported"
	compareStatusPartial   = "partial"
	compareStatusMissing   = "missing"
)

type compareCapability struct {
	ID          string
	Area        string
	Targets     []string
	Status      string
	Expectation string
	Agezt       string
	Evidence    []string
	Next        string
}

type compareEvidence struct {
	Path    string `json:"path"`
	Present bool   `json:"present"`
}

type compareAuditRow struct {
	ID              string            `json:"id"`
	Area            string            `json:"area"`
	Targets         []string          `json:"targets"`
	Status          string            `json:"status"`
	Expectation     string            `json:"expectation"`
	Agezt           string            `json:"agezt"`
	Evidence        []compareEvidence `json:"evidence"`
	EvidenceOK      bool              `json:"evidence_ok"`
	MissingEvidence []string          `json:"missing_evidence,omitempty"`
	Next            string            `json:"next,omitempty"`
}

type compareAudit struct {
	Target          string            `json:"target"`
	Root            string            `json:"root"`
	Total           int               `json:"total"`
	Supported       int               `json:"supported"`
	Partial         int               `json:"partial"`
	Missing         int               `json:"missing"`
	EvidenceOK      int               `json:"evidence_ok"`
	EvidenceMissing int               `json:"evidence_missing"`
	Rows            []compareAuditRow `json:"rows"`
}

// cmdCompare dispatches `agt compare <subcommand>`. It is intentionally
// offline/read-only: the first milestone is a local evidence ledger, not a live
// benchmark that needs a daemon or paid provider.
func cmdCompare(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s compare: subcommand required (audit)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "audit":
		return cmdCompareAudit(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s compare <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  audit [--target openclaw|hermes|all] [--root <repo>] [--json]\n")
		fmt.Fprintf(stdout, "read-only local capability ledger for OpenClaw/Hermes parity claims\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s compare: unknown subcommand %q (audit)\n", brand.CLI, args[0])
		return 2
	}
}

func cmdCompareAudit(args []string, stdout, stderr io.Writer) int {
	target := compareTargetAll
	rootArg := ""
	asJSON := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--target":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s compare audit: --target needs openclaw, hermes, or all\n", brand.CLI)
				return 2
			}
			i++
			target = strings.ToLower(strings.TrimSpace(args[i]))
		case strings.HasPrefix(a, "--target="):
			target = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(a, "--target=")))
		case a == "--root":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s compare audit: --root needs a directory\n", brand.CLI)
				return 2
			}
			i++
			rootArg = args[i]
		case strings.HasPrefix(a, "--root="):
			rootArg = strings.TrimPrefix(a, "--root=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s compare audit [--target openclaw|hermes|all] [--root <repo>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "check AGEZT parity claims against local repository evidence\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s compare audit: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if !validCompareTarget(target) {
		fmt.Fprintf(stderr, "%s compare audit: unknown target %q (want openclaw, hermes, or all)\n", brand.CLI, target)
		return 2
	}
	root, err := resolveCompareRoot(rootArg)
	if err != nil {
		fmt.Fprintf(stderr, "%s compare audit: %v\n", brand.CLI, err)
		return 1
	}
	audit := buildCompareAudit(root, target)
	if asJSON {
		return encodeJSON(stdout, audit)
	}
	renderCompareAudit(stdout, audit)
	return 0
}

func validCompareTarget(target string) bool {
	switch target {
	case compareTargetAll, compareTargetOpenClaw, compareTargetHermes:
		return true
	default:
		return false
	}
}

func resolveCompareRoot(rootArg string) (string, error) {
	if strings.TrimSpace(rootArg) != "" {
		abs, err := filepath.Abs(rootArg)
		if err != nil {
			return "", err
		}
		st, err := os.Stat(abs)
		if err != nil {
			return "", err
		}
		if !st.IsDir() {
			return "", fmt.Errorf("--root %s is not a directory", rootArg)
		}
		return abs, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		if isAgeztRepoRoot(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd, nil
		}
	}
}

func isAgeztRepoRoot(dir string) bool {
	for _, p := range []string{"go.mod", "README.md", filepath.Join("cmd", "agt")} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			return false
		}
	}
	return true
}

func buildCompareAudit(root, target string) compareAudit {
	out := compareAudit{Target: target, Root: root}
	for _, cap := range compareCapabilities() {
		if !compareCapabilityTargets(cap, target) {
			continue
		}
		row := compareAuditRow{
			ID:          cap.ID,
			Area:        cap.Area,
			Targets:     append([]string(nil), cap.Targets...),
			Status:      cap.Status,
			Expectation: cap.Expectation,
			Agezt:       cap.Agezt,
			Next:        cap.Next,
		}
		for _, p := range cap.Evidence {
			ok := compareEvidencePresent(root, p)
			row.Evidence = append(row.Evidence, compareEvidence{Path: filepath.ToSlash(p), Present: ok})
			if !ok {
				row.MissingEvidence = append(row.MissingEvidence, filepath.ToSlash(p))
			}
		}
		row.EvidenceOK = len(row.MissingEvidence) == 0
		out.Rows = append(out.Rows, row)
		out.Total++
		switch row.Status {
		case compareStatusSupported:
			out.Supported++
		case compareStatusPartial:
			out.Partial++
		case compareStatusMissing:
			out.Missing++
		}
		if row.EvidenceOK {
			out.EvidenceOK++
		} else {
			out.EvidenceMissing++
		}
	}
	return out
}

func compareCapabilityTargets(cap compareCapability, target string) bool {
	if target == compareTargetAll {
		return true
	}
	for _, t := range cap.Targets {
		if t == target || t == compareTargetAll {
			return true
		}
	}
	return false
}

func compareEvidencePresent(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

func renderCompareAudit(w io.Writer, audit compareAudit) {
	fmt.Fprintf(w, "AGEZT compare audit (target=%s, root=%s)\n", audit.Target, audit.Root)
	fmt.Fprintf(w, "%d row(s): %d supported, %d partial, %d missing\n", audit.Total, audit.Supported, audit.Partial, audit.Missing)
	fmt.Fprintf(w, "evidence: %d ok, %d missing\n", audit.EvidenceOK, audit.EvidenceMissing)
	for _, row := range audit.Rows {
		present, total := compareEvidenceCounts(row)
		fmt.Fprintf(w, "  [%s] %-24s %d/%d evidence  %s\n", row.Status, row.ID, present, total, row.Area)
		fmt.Fprintf(w, "      %s\n", row.Agezt)
		if len(row.MissingEvidence) > 0 {
			fmt.Fprintf(w, "      missing evidence: %s\n", strings.Join(row.MissingEvidence, ", "))
		}
		if row.Next != "" {
			fmt.Fprintf(w, "      next: %s\n", row.Next)
		}
	}
}

func compareEvidenceCounts(row compareAuditRow) (present, total int) {
	for _, ev := range row.Evidence {
		total++
		if ev.Present {
			present++
		}
	}
	return present, total
}

func compareCapabilities() []compareCapability {
	both := []string{compareTargetOpenClaw, compareTargetHermes}
	return []compareCapability{
		{
			ID:          "channels-gateway",
			Area:        "Messaging gateway",
			Targets:     both,
			Status:      compareStatusSupported,
			Expectation: "Multi-channel inbound/outbound chat gateway with allowlisted users and setup guidance.",
			Agezt:       "AGEZT has broad first-party channel plugins, Connect/Channels documentation and UI, plus a channel probe matrix that reports configured/live accounts and roundtrip readiness through the control plane, CLI, and Web UI.",
			Evidence:    []string{"plugins/channels/slack/slack.go", "plugins/channels/telegram/telegram.go", "plugins/channels/whatsapp/whatsapp.go", "kernel/controlplane/channels.go", "cmd/agt/channel.go", "frontend/src/views/Channels.tsx", "docs/CONNECT.md"},
			Next:        "Add per-adapter setup recipes and optional live roundtrip probes where a safe recipient/test target is configured.",
		},
		{
			ID:          "provider-api-gateway",
			Area:        "Provider and API gateway",
			Targets:     both,
			Status:      compareStatusSupported,
			Expectation: "Multiple model providers plus OpenAI-compatible or gateway APIs.",
			Agezt:       "AGEZT has provider families, OpenAI-compatible API, native REST API, and ACP server surfaces.",
			Evidence:    []string{"plugins/providers/openai/openai.go", "plugins/providers/anthropic/anthropic.go", "kernel/openaiapi/openaiapi.go", "kernel/restapi/restapi.go", "kernel/acp/acp.go"},
			Next:        "Keep SDK parity and model-routing behavior pinned with generated tests.",
		},
		{
			ID:          "durable-agent-identity",
			Area:        "Durable agent identity",
			Targets:     both,
			Status:      compareStatusSupported,
			Expectation: "Agents/profiles survive beyond a single chat run and carry configuration.",
			Agezt:       "AGEZT roster profiles are durable identities with lifecycle, authority, model routing, ownership, memory scope, and wake behavior.",
			Evidence:    []string{"kernel/roster/roster.go", "kernel/runtime/runtime.go", "frontend/src/views/Roster.tsx", "frontend/src/components/AgentDetail.tsx"},
			Next:        "Add importers for OpenClaw/Hermes profile and memory files.",
		},
		{
			ID:          "memory-world",
			Area:        "Memory and world model",
			Targets:     both,
			Status:      compareStatusSupported,
			Expectation: "Long-term memory with searchable/project context and user profile state.",
			Agezt:       "AGEZT separates long-term memory from an entity/relation world model and exposes audit commands.",
			Evidence:    []string{"kernel/memory/memory.go", "kernel/worldmodel/worldmodel.go", "cmd/agt/memory.go", "cmd/agt/world.go"},
			Next:        "Add MEMORY.md/USER.md/SOUL.md import/export and compiled operator-readable views.",
		},
		{
			ID:          "skill-lifecycle-core",
			Area:        "Skill learning core",
			Targets:     both,
			Status:      compareStatusSupported,
			Expectation: "Reusable skills can be created, evolved, activated, and retired.",
			Agezt:       "AGEZT Forge has content-addressed skills, lineage, draft/shadow/active/quarantine/archive states, metrics, and revert paths.",
			Evidence:    []string{"kernel/skill/skill.go", "kernel/skill/forge.go", "kernel/skill/shadoweval_test.go", "cmd/agt/skill.go", "cmd/agt/skill_workshop.go"},
			Next:        "Add richer provenance cards and optional LLM consolidation.",
		},
		{
			ID:          "skill-workshop-ux",
			Area:        "Skill workshop and curator UX",
			Targets:     both,
			Status:      compareStatusPartial,
			Expectation: "Agent/user skill proposals, review UI, security scanner cards, and curator maintenance.",
			Agezt:       "AGEZT now exposes `agt skill workshop list/inspect/scan/diff/curate/apply/reject/quarantine/propose-create/propose-update` over the governed Forge lifecycle, including journaled archive rejects, deterministic scanner output, deterministic stale-skill curation, and Web UI pending-proposal apply/reject with scanner risk chips, parent diff previews, and hash/source/parent chips; optional LLM consolidation is still incomplete.",
			Evidence:    []string{"kernel/skill/forge.go", "frontend/src/views/Skills.tsx", "cmd/agt/skill.go", "cmd/agt/skill_workshop.go"},
			Next:        "Add richer provenance cards and optional LLM consolidation jobs.",
		},
		{
			ID:          "browser-actions",
			Area:        "Interactive browser automation",
			Targets:     both,
			Status:      compareStatusPartial,
			Expectation: "Browser open/snapshot/click/type/screenshot/download flows for JS-rendered websites.",
			Agezt:       "AGEZT has SSRF-guarded `browser.read` plus opt-in first-party `browser.action` and `browser.open/snapshot/click/type/wait/screenshot/downloads/cookies/tabs/close` wrappers with compact snapshots, browser events, screenshot/download artifact registration, cookie inspection on request, download capture, AGEZT-managed `profile=session` cookie/storage carryover, persistent `tab_id` final-URL refs for URL-less follow-up actions, saved snapshot `ref` resolution with missing-ref errors, saved tab-ref list/close lifecycle, and operator-gated isolated/user-attached/remote-cdp profile policy; live browser-process tab lifecycle and DOM-level stale-ref invalidation are not complete.",
			Evidence:    []string{"plugins/tools/browser/browser.go", "plugins/tools/browser/action.go", "plugins/tools/browser/action_verbs.go", "plugins/builtinskills/browseruse/SKILL.md"},
			Next:        "Add live browser-process tab lifecycle, DOM stale-ref invalidation, and full Playwright E2E browser fixtures.",
		},
		{
			ID:          "automation-schedules",
			Area:        "Automation and schedules",
			Targets:     both,
			Status:      compareStatusSupported,
			Expectation: "Cron jobs, standing orders/hooks, and workflow/taskflow automation.",
			Agezt:       "AGEZT has typed schedules, standing orders, workflow canvas/runner, and schedule/standing tools.",
			Evidence:    []string{"kernel/cadence/cadence.go", "kernel/standing/standing.go", "kernel/workflow/workflow.go", "plugins/tools/schedule/schedule.go", "plugins/tools/standingtool/standing.go"},
			Next:        "Add OpenClaw/Hermes cron/standing import dry-runs and parity demos.",
		},
		{
			ID:          "mcp-tool-discovery",
			Area:        "MCP and tool discovery",
			Targets:     both,
			Status:      compareStatusSupported,
			Expectation: "Attach external tools/MCP servers and avoid flooding the model with every schema.",
			Agezt:       "AGEZT has an MCP bridge, MCP registry, tool catalog, toolforge, and runtime tool-selection surfaces.",
			Evidence:    []string{"kernel/mcp/store.go", "plugins/external/mcpbridge/mcp.go", "kernel/runtime/toolsearch.go", "kernel/toolforge/toolforge.go"},
			Next:        "Make policy-aware deferred tool discovery a named product feature with metrics.",
		},
		{
			ID:          "marketplace-registry",
			Area:        "Marketplace and registry",
			Targets:     both,
			Status:      compareStatusSupported,
			Expectation: "Install skills/plugins/packs from a marketplace or hub.",
			Agezt:       "AGEZT has remote market, skill, and plugin registry flows with hash/signature-oriented infrastructure.",
			Evidence:    []string{"kernel/market/market.go", "kernel/market/verify.go", "cmd/agt/market.go", "cmd/agt/plugin.go", "cmd/agt/skill.go"},
			Next:        "Unify trust cards across skills, plugins, MCP, channel adapters, and workflow templates.",
		},
		{
			ID:          "marketplace-trust-ux",
			Area:        "Marketplace trust UX",
			Targets:     both,
			Status:      compareStatusPartial,
			Expectation: "Clear risk cards, scanner findings, publisher identity, permissions, and quarantine/update policy.",
			Agezt:       "AGEZT has verification and governance primitives, but package-level trust cards and scanner UX are not complete.",
			Evidence:    []string{"docs/PLUGIN-SECURITY.md", "kernel/market/verify.go", "frontend/src/views/Market.tsx"},
			Next:        "Add scanner-backed trust cards and package quarantine/update rollback flows.",
		},
		{
			ID:          "checkpoint-rollback",
			Area:        "Checkpoints and rollback",
			Targets:     []string{compareTargetHermes},
			Status:      compareStatusSupported,
			Expectation: "Run/file checkpoints with operator-triggered rollback for mutations.",
			Agezt:       "AGEZT has `agt rollback list/show/dry-run/apply`, `agt rollback list --run <id>`, a local checkpoint catalog, workshop pre-mutation skill.status checkpoints, workflow.snapshot checkpoints for CLI workflow edits, daemon file.snapshot checkpoints for file write/append/replace/delete, config.setting checkpoints for `agt config set`, Web UI run-detail rollback list/apply, rollback_mode labels for irreversible tools, and journaled `skill.restored` / `workflow.restored` rollback paths.",
			Evidence:    []string{"cmd/agt/rollback.go", "cmd/agt/skill_workshop.go", "cmd/agt/workflow.go", "cmd/agt/config.go", "plugins/tools/file/checkpoint.go", "cmd/agezt/main.go", "kernel/webui/rollback.go", "frontend/src/components/RunDetail.tsx", "frontend/src/views/Tools.tsx", "kernel/controlplane/tool.go", "kernel/controlplane/skill.go", "kernel/controlplane/workflow.go", "kernel/skill/forge.go", "kernel/workflow/workflow.go"},
			Next:        "Broaden checkpoint hooks to patch/coding/package-update paths and add agent-detail rollback grouping.",
		},
		{
			ID:          "durable-workboard",
			Area:        "Durable multi-agent work queue",
			Targets:     []string{compareTargetHermes},
			Status:      compareStatusPartial,
			Expectation: "Kanban-like durable tasks with worker lanes, dependencies, comments, heartbeats, and crash recovery.",
			Agezt:       "AGEZT now has a durable typed `kernel/workboard` store with tasks, dependencies, comments, links, claims, heartbeats, status transitions, idempotency keys, retry/escalation policy, persistence, journaled runtime mutations, `agt workboard` CLI commands including lanes/depend/reclaim/sweep/policy/fail/dispatch/watch, an agent-facing `workboard` tool, and a separate Edict capability; dispatch refuses unresolved dependencies, claims assigned roster agents, links run correlations, starts async runs, automatically retries failed Workboard attempts up to task policy, escalates exhausted attempts, moves successful dispatches to review, exposes task/run events, groups tasks by assignee lane in CLI/API/Web UI, provides a dedicated Workboard detail UI for dependencies, attempts, links/artifacts, events, comments, block/fail/unblock/complete/policy/dispatch actions, and can reclaim stale claims on demand or through AGEZT_WORKBOARD_SWEEP_EVERY.",
			Evidence:    []string{"kernel/workboard/workboard.go", "kernel/runtime/workboard.go", "kernel/controlplane/workboard.go", "cmd/agt/workboard.go", "cmd/agt/workboard_test.go", "cmd/agezt/main.go", "kernel/settings/schema.go", "plugins/tools/workboardtool/workboard.go", "kernel/edict/toolmap.go", "kernel/board/board.go", "kernel/workflow/workflow.go", "cmd/agt/conductor.go", "kernel/webui/webui.go", "kernel/webui/webui_test.go", "frontend/src/views/Board.tsx", "frontend/src/views/Board.test.tsx", "frontend/src/views/Workboard.tsx", "frontend/src/views/Workboard.test.tsx"},
			Next:        "Add graph-style dependency visualization, inline artifact/diff preview, and process/delegation heartbeat integration.",
		},
		{
			ID:          "execution-profiles",
			Area:        "Terminal and sandbox backends",
			Targets:     []string{compareTargetHermes},
			Status:      compareStatusPartial,
			Expectation: "Selectable local/docker/ssh/cloud terminal backends with consistent isolation semantics.",
			Agezt:       "AGEZT has warden, netguard, codeexec/shell, worktree coding, browser sessions, docker/ssh skills, remote peers, active K8s pod shell/code_exec, Modal shell/code_exec, and Daytona shell/code_exec adapters, and a unified `kernel/executionprofile` inventory that names local, warden, worktree-coding, browser-session, docker, ssh, remote-agezt, modal, daytona, and k8s profiles with requested/effective isolation, routed tools, filesystem/network/env/secret/limit semantics, structured remote/cloud `AGEZT_EXEC_REMOTE_SECRET_POLICY` reporting, control-plane commands, Web API routes, `agt exec-profile list|show|check`, `agt run --exec-profile local|warden|docker|ssh|k8s|modal|daytona|remote-agezt` per-run routing for shell/code execution when Docker/SSH/K8s/Modal/Daytona backends are explicitly configured, local/SSH/K8s code_exec `.agezt-artifacts/` exports into the durable artifact index, Modal shell/code_exec routing through `modal shell --cmd` with `--add-local` workspace mounts for code_exec and bounded artifact return, Daytona shell/code_exec routing through `daytona exec` with bounded workspace materialization and bounded artifact return, plus whole-run peer delegation when `remote_run` is registered, `agt run --exec-profile remote-agezt --peer <name>` peer pinning for multi-node meshes, live `AGEZT_EXEC_PROFILE_ALLOW` / `AGEZT_EXEC_PROFILE_DENY` run-profile policy, live SSH/K8s/Modal/Daytona backend controls, restart-bound Docker/OCI and peer-mesh controls, live profile-specific env/secret-env passthrough controls and vault-backed temporary secret file mounts for local/warden/docker shell/code_exec child processes, Config Center and Execution Profiles UI controls, Chat UI execution-profile selection, a dedicated Execution Profiles inventory/health UI, optional Docker/OCI warden routing with /workspace mounts and scrubbed env forwarding, SSH shell/code_exec routing with scp-backed remote workspace sync, K8s shell/code_exec routing with scrubbed local kubectl env, remote workspace copy, artifact copy-back, and no daemon secret forwarding into pods, Modal shell/code_exec and Daytona shell/code_exec routing with scrubbed local CLI env and no daemon secret forwarding into cloud sandboxes, remote AGEZT peer routing through policy-gated `Kernel.RunTool` with local task lifecycle events, structured peer correlation metadata, `agt peers run <peer> <corr>` metadata-only remote run drill-down, `agt peers artifacts <peer> <corr>` metadata-only remote artifact drill-down, opt-in metadata/redacted payload peer event mirroring via `AGEZT_REMOTE_EVENT_MIRROR=metadata|redacted`, peer artifact metadata mirroring through `/api/v1/artifacts?corr=...` without bytes, opt-in policy-gated artifact byte transfer via `AGEZT_REMOTE_ARTIFACT_BYTES=allow`, REST `/api/v1/artifacts/{id}/bytes`, and `agt peers artifact-get <peer> <artifact_id> <out_file>`, and Run Detail remote artifact summaries with copyable governed download commands, plus health checks for policy/downgrade/docker/podman/ssh/peer/modal/daytona/kubectl/remote-secret-policy/remote-artifact-bytes readiness. K8s job lifecycle is not complete yet.",
			Evidence:    []string{"kernel/executionprofile/profile.go", "kernel/executionprofile/check.go", "kernel/executionprofile/policy.go", "kernel/executionprofile/env.go", "kernel/executionprofile/secretfiles.go", "kernel/executionprofile/secretpolicy.go", "kernel/executionprofile/ssh.go", "kernel/executionprofile/k8s.go", "kernel/executionprofile/modal.go", "kernel/executionprofile/daytona.go", "kernel/executionprofile/profile_test.go", "kernel/executionprofile/env_test.go", "kernel/executionprofile/secretfiles_test.go", "kernel/controlplane/execution_profiles.go", "kernel/controlplane/execution_profiles_test.go", "kernel/controlplane/run_argvalidation_test.go", "kernel/controlplane/remote_mirror.go", "kernel/controlplane/remote_mirror_test.go", "kernel/controlplane/config.go", "kernel/controlplane/settings.go", "kernel/restapi/artifacts.go", "kernel/restapi/restapi.go", "kernel/restapi/restapi_test.go", "kernel/runtime/toolrun.go", "kernel/settings/schema.go", "kernel/settings/settings_test.go", "cmd/agezt/main.go", "cmd/agezt/main_test.go", "cmd/agt/main.go", "cmd/agt/execution_profile.go", "cmd/agt/execution_profile_test.go", "cmd/agt/peers.go", "cmd/agt/peers_test.go", "cmd/agt/run_test.go", "kernel/webui/webui.go", "kernel/webui/webui_test.go", "kernel/warden/warden.go", "kernel/warden/container.go", "kernel/warden/container_test.go", "kernel/netguard/netguard.go", "frontend/src/lib/conversations.ts", "frontend/src/lib/conversations.test.ts", "frontend/src/lib/chatStore.tsx", "frontend/src/lib/chatStore.cancel.test.tsx", "frontend/src/views/Chat.tsx", "frontend/src/views/Chat.executionProfile.test.tsx", "frontend/src/views/ExecutionProfiles.tsx", "frontend/src/views/ExecutionProfiles.test.tsx", "frontend/src/components/RunDetail.tsx", "frontend/src/components/RunDetail.test.tsx", "plugins/tools/shell/shell.go", "plugins/tools/shell/env.go", "plugins/tools/shell/shell_test.go", "plugins/tools/codeexec/codeexec.go", "plugins/tools/codeexec/artifacts.go", "plugins/tools/codeexec/daytona.go", "plugins/tools/codeexec/packages.go", "plugins/tools/codeexec/codeexec_test.go", "plugins/tools/peer/peer.go", "plugins/tools/peer/peer_test.go", "plugins/tools/coding/coding.go", "plugins/builtinskills/dockerservices/SKILL.md", "plugins/builtinskills/sshremote/SKILL.md", ".env.example"},
			Next:        "Add K8s job lifecycle.",
		},
		{
			ID:          "media-voice",
			Area:        "Media and voice",
			Targets:     both,
			Status:      compareStatusSupported,
			Expectation: "Speech, image/media handling, generated artifacts, and channel media sends where supported.",
			Agezt:       "AGEZT has STT/TTS, voice mode, image provider, artifact store, sendmedia tool, media-capable channel adapters, and a channel media capability matrix for image/voice in/out coverage in API/Web UI.",
			Evidence:    []string{"kernel/stt/stt.go", "kernel/webui/tts.go", "plugins/providers/image/image.go", "kernel/artifact/artifact.go", "plugins/tools/sendmedia/sendmedia.go", "kernel/controlplane/channels.go", "frontend/src/views/Channels.tsx"},
			Next:        "Add artifact-native media review flows in the console.",
		},
		{
			ID:          "device-companion",
			Area:        "Device and companion apps",
			Targets:     []string{compareTargetOpenClaw},
			Status:      compareStatusPartial,
			Expectation: "Local device nodes, companion apps, desktop/browser control, and smart-home/device routing.",
			Agezt:       "AGEZT has Web UI, Home Assistant, tunnel, voice, computer/browser-use skills, peer nodes, `agt peers`, and a node registry API/Web UI surface that lists the local daemon plus reachable AGEZT peers without leaking tokens. Native tray/mobile companion distribution is still missing.",
			Evidence:    []string{"plugins/tools/homeassistant/homeassistant.go", "kernel/tunnel/tunnel.go", "plugins/builtinskills/computeruse/SKILL.md", "cmd/agt/peers.go", "kernel/controlplane/nodes.go", "kernel/webui/webui.go", "frontend/src/views/Connections.tsx"},
			Next:        "Build tray/PWA/mobile companion for approvals, voice, status, and routed capabilities on top of the node registry.",
		},
		{
			ID:          "onboarding-console",
			Area:        "Onboarding and web dashboard",
			Targets:     both,
			Status:      compareStatusSupported,
			Expectation: "Guided first-run setup, provider/channel configuration, and dashboard visibility.",
			Agezt:       "AGEZT has quickstart, Config Center, Setup, Channels, Connect docs, and a broad Web UI console.",
			Evidence:    []string{"cmd/agt/quickstart.go", "frontend/src/views/Setup.tsx", "frontend/src/views/ConfigCenter.tsx", "frontend/src/views/Channels.tsx", "docs/CONSOLE.md"},
			Next:        "Make one-pass provider/channel/MCP/skill/sandbox validation visible from setup.",
		},
		{
			ID:          "audit-policy-safety",
			Area:        "Audit, policy, and safety",
			Targets:     both,
			Status:      compareStatusSupported,
			Expectation: "Tool governance, approval, auditability, and safety controls around autonomous action.",
			Agezt:       "AGEZT's Edict policy, approvals, hash-chain journal, netguard, warden, vault, and `agt why` are core runtime surfaces.",
			Evidence:    []string{"kernel/edict/edict.go", "kernel/approval/approval.go", "kernel/journal/journal.go", "cmd/agt/why.go", "kernel/creds/creds.go", "docs/THREAT-MODEL.md"},
			Next:        "Require every new parity feature to emit policy/effect/provenance evidence.",
		},
		{
			ID:          "native-mobile-tray",
			Area:        "Native companion distribution",
			Targets:     []string{compareTargetOpenClaw},
			Status:      compareStatusMissing,
			Expectation: "Native tray/mobile companion app distribution for operator approvals and local device control.",
			Agezt:       "AGEZT can be operated through Web UI/CLI/channels today, but native tray/mobile companion distribution is not present.",
			Next:        "Build the tray/PWA/mobile companion as the product layer on top of the node registry.",
		},
	}
}

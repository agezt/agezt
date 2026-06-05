// SPDX-License-Identifier: MIT

// Command agezt is the Agezt kernel/daemon binary.
//
// Subcommands:
//
//	agezt                run the daemon (default; foreground)
//	agezt daemon         same as bare invocation, explicit
//	agezt version        print version
//	agezt help           usage
//
// The daemon hosts the kernel runtime (journal + state + bus + agent loop
// + in-process plugins) and the control plane (TCP localhost + token).
// `agt` is a thin client over the control plane.
//
// Provider selection: if $ANTHROPIC_API_KEY is set, the Anthropic provider
// is registered. Otherwise the daemon refuses to start; M0.5 needs a real
// LLM to satisfy the demo gate. Add --offline support later (would require
// a scripted-intent runner).
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/anomaly"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/netguard"
	"github.com/agezt/agezt/kernel/openaiapi"
	"github.com/agezt/agezt/kernel/plugin"
	"github.com/agezt/agezt/kernel/pulse"
	"github.com/agezt/agezt/kernel/redact"
	"github.com/agezt/agezt/kernel/restapi"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/kernel/ulid"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/kernel/webhook"
	"github.com/agezt/agezt/kernel/webui"
	"github.com/agezt/agezt/plugins/channels/discord"
	"github.com/agezt/agezt/plugins/channels/email"
	"github.com/agezt/agezt/plugins/channels/slack"
	"github.com/agezt/agezt/plugins/channels/telegram"
	webhookchan "github.com/agezt/agezt/plugins/channels/webhook"
	"github.com/agezt/agezt/plugins/providers/anthropic"
	"github.com/agezt/agezt/plugins/providers/compat"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/acpagent"
	"github.com/agezt/agezt/plugins/tools/browser"
	"github.com/agezt/agezt/plugins/tools/coding"
	filetool "github.com/agezt/agezt/plugins/tools/file"
	httptool "github.com/agezt/agezt/plugins/tools/http"
	"github.com/agezt/agezt/plugins/tools/notify"
	"github.com/agezt/agezt/plugins/tools/peer"
	"github.com/agezt/agezt/plugins/tools/shell"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return runDaemon(stdout, stderr)
	}
	switch args[0] {
	case "-v", "--version", "version":
		fmt.Fprintf(stdout, "%s %s (protocol v%d)\n", brand.Binary, brand.Version, brand.ProtocolVersion)
		return 0
	case "-h", "--help", "help":
		printHelp(stdout)
		return 0
	case "daemon":
		return runDaemon(stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s: unknown command %q\n", brand.Binary, args[0])
		printHelp(stderr)
		return 2
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintf(w, "usage: %s [command]\n", brand.Binary)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "Commands:\n")
	fmt.Fprintf(w, "  (none)    run the daemon (default)\n")
	fmt.Fprintf(w, "  daemon    run the daemon, explicit\n")
	fmt.Fprintf(w, "  version   show version and exit\n")
	fmt.Fprintf(w, "  help      show this help\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "Environment:\n")
	fmt.Fprintf(w, "  %sHOME             base directory (default: ~/%s)\n", brand.EnvPrefix, brand.ConfigDir)
	fmt.Fprintf(w, "  ANTHROPIC_API_KEY    required to enable the Anthropic provider\n")
	fmt.Fprintf(w, "  %sMODEL            default model (default: %s)\n", brand.EnvPrefix, anthropic.DefaultModel)
	fmt.Fprintf(w, "  %sSYSTEM_PROMPT    system prompt for every run (optional)\n", brand.EnvPrefix)
}

// runDaemon brings up the kernel + control plane, prints connection info
// to stdout, and waits for SIGINT / SIGTERM.
func runDaemon(stdout, stderr io.Writer) int {
	// Honor a cgroup CPU quota (container `--cpus`, constrained host) by lowering
	// GOMAXPROCS to match — the Go runtime is not cgroup-aware and would otherwise
	// over-schedule against a fraction of a core (SPEC-11 §4). No-op off Linux,
	// when no quota is set, or when GOMAXPROCS is explicit.
	if note := applyAutoMaxProcs(); note != "" {
		fmt.Fprintf(stdout, "%s: %s\n", brand.Binary, note)
	}

	baseDir, err := paths.BaseDir()
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.Binary, err)
		return 1
	}

	// Single-instance guard: a second daemon on the same base dir would
	// overwrite the control-plane addr/token files and split clients across
	// two kernels writing the same journal — `agt` would silently reach
	// whichever started last. Refuse if a live daemon already answers.
	// AGEZT_FORCE_START=1 overrides (e.g. to reclaim after a confirmed crash).
	if addr, alive := controlplane.ProbeExisting(baseDir); alive {
		if strings.TrimSpace(os.Getenv(brand.EnvPrefix+"FORCE_START")) != "1" {
			fmt.Fprintf(stderr, "%s: a daemon is already running at %s (base dir %s)\n", brand.Binary, addr, baseDir)
			fmt.Fprintf(stderr, "Hint: stop it with `%s shutdown`, or set %sFORCE_START=1 to override.\n", brand.CLI, brand.EnvPrefix)
			return 1
		}
		fmt.Fprintf(stderr, "%s: warning: %sFORCE_START=1 — starting despite a live daemon at %s\n", brand.Binary, brand.EnvPrefix, addr)
	}

	// Load catalog once; share with buildGovernor + runtime.Config so
	// the daemon and the kernel see the same snapshot. An empty catalog
	// on disk is fine: selectPrimary will fall through to the offline
	// mock and surface a hint in the banner.
	catStore := catalog.NewStore(filepath.Join(baseDir, "catalog"))
	cat, err := catStore.Load()
	if err != nil {
		fmt.Fprintf(stderr, "%s: catalog load: %v\n", brand.Binary, err)
		return 1
	}
	// M93 demo: make the offline "mock" model vision-capable so `agt run --image`
	// passes the M91 gate and exercises the image-input path end-to-end.
	if os.Getenv(brand.EnvPrefix+"DEMO_VISION") == "1" {
		injectDemoVisionModel(cat)
	}

	// Load credentials vault (M1.o). Missing file is a valid first-run
	// state — operators can still rely on env vars. Vault entries take
	// precedence over env in the chained lookup below, so `export FOO=...`
	// can temporarily override a vaulted value in a shell session.
	credStore := creds.NewStore(baseDir)
	if err := credStore.Load(); err != nil {
		fmt.Fprintf(stderr, "%s: creds load: %v\n", brand.Binary, err)
		return 1
	}
	// Credential resolution chain (M1.dd):
	//   1. agezt vault (M1.w) — operator-managed, AES-256-GCM-at-rest
	//   2. process env — `export FOO=...` always wins over file sources
	//   3. ~/.aws/credentials + ~/.aws/config (AWS_PROFILE-aware)
	//   4. EC2 IMDSv2 — instance-role credentials, refreshed on expiry
	// The AWS-specific stages (3-4) answer ONLY the AWS_* names they
	// know about; every other name falls through. Operators on a
	// non-EC2 host pay only a brief, neg-cached IMDS timeout on the
	// first lookup (then nothing for 30s) — the chain remains fast.
	credLookup, awsChainDesc := buildAWSCredChain(credStore.Lookup)
	credCount := len(credStore.Names())
	credDesc := fmt.Sprintf("vault entries=%d at %s — %s", credCount, credStore.Path, awsChainDesc)

	gov, govDesc, model, err := buildGovernor(cat, credLookup)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.Binary, err)
		return 1
	}

	// Warden is constructed before the kernel so tools that close over
	// it (shell) can be built before runtime.Open. Bus is attached
	// post-Open via SetBus, same pattern as the Governor.
	ward := warden.New(nil)
	wardDesc := fmt.Sprintf("requested=namespace, effective=%s (M1.c facade; downgrades journaled)",
		ward.EffectiveProfile(warden.ProfileNamespace))

	// Edict policy mode: AGEZT_APPROVAL_MODE=allow|deny|prompt
	// (M1.a default: allow; M1.d adds prompt for live HITL).
	askPolicy, askPolicyDesc := selectAskPolicy()
	// Operator-extensible hard-deny rules (M17): AGEZT_EDICT_DENY appends
	// site-specific rules to the built-in set (e.g. "git push;shell:/etc/shadow").
	hardDeny := edict.DefaultHardDeny()
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "EDICT_DENY")); spec != "" {
		extra, derr := edict.ParseDenyRules(spec)
		if derr != nil {
			fmt.Fprintf(stderr, "%s: %sEDICT_DENY: %v\n", brand.Binary, brand.EnvPrefix, derr)
			return 1
		}
		hardDeny = append(hardDeny, extra...)
		askPolicyDesc += fmt.Sprintf("; +%d operator deny rule(s)", len(extra))
	}
	edictEng := edict.New(edict.Options{AskPolicy: askPolicy, HardDeny: hardDeny})

	tools, pluginManifest, toolsDesc, err := buildTools(baseDir, stderr, ward)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.Binary, err)
		return 1
	}

	// Proactive-messaging tool (`notify`, M143). Register it here — BEFORE the
	// kernel (and its HTTP servers / channels) start — so the tool map is never
	// written while the agent loop reads it (a fatal concurrent-map race otherwise).
	// The sender needs the live channels (built after the kernel), so the tool is
	// created unbound now and Bind-wired later. Decide registration from env: a
	// channel kind contributes targets only when its token AND a non-empty allowlist
	// are set, so a half-configured channel never advertises a tool that can't send.
	notifyTargets := map[string][]string{}
	for _, c := range []struct{ kind, tokenEnv, idsEnv string }{
		{"telegram", "TELEGRAM_TOKEN", "TELEGRAM_CHAT_ID"},
		{"slack", "SLACK_TOKEN", "SLACK_CHANNELS"},
		{"discord", "DISCORD_TOKEN", "DISCORD_CHANNELS"},
	} {
		if strings.TrimSpace(os.Getenv(brand.EnvPrefix+c.tokenEnv)) == "" {
			continue
		}
		if ids := splitNonEmpty(os.Getenv(brand.EnvPrefix + c.idsEnv)); len(ids) > 0 {
			notifyTargets[c.kind] = ids
		}
	}
	var notifyTool *notify.Tool
	if len(notifyTargets) > 0 {
		notifyTool = notify.New() // unbound; Bind wires the sender once channels exist
		tools["notify"] = notifyTool
	}

	// OnReload is invoked by the control plane's `provider_reload`
	// command (and `agt provider reload`). It re-reads the vault,
	// re-runs primary-provider selection against the freshly-reloaded
	// catalog, and atomically swaps the Governor's primary registry
	// entry. Catalog refresh happens inside Kernel.Reload before this
	// closure is invoked, so `cat` here is stale — we re-pull it
	// inside via `k.Catalog()` once the kernel exists.
	//
	// Note that this captures `gov` (the Governor instance), `catStore`,
	// `credStore`, and rebuilds via the same `selectPrimary` →
	// `buildFromCatalog` path the boot path uses, so the live
	// post-reload registry matches what a fresh boot would have
	// produced for the same on-disk state.
	// Memory-lite (ROADMAP §2.3): on by default. The agent reads recalled
	// records as injected context, can remember/recall/forget via the
	// in-process `memory` tool, and multi-tool runs are auto-distilled into
	// durable facts. Set AGEZT_MEMORY=off to disable the per-run behaviour
	// (the store and `agt memory` CLI stay available either way).
	memOn := !strings.EqualFold(os.Getenv(brand.EnvPrefix+"MEMORY"), "off")
	// World-model per-run behaviour (entity injection + the `world` tool).
	// The graph store and `agt world` CLI always work; this only gates the
	// in-run wiring. AGEZT_WORLDMODEL=off disables it.
	worldOn := !strings.EqualFold(os.Getenv(brand.EnvPrefix+"WORLDMODEL"), "off")
	// Forge / skills (SPEC-05 §4-5). Active skills inject into runs and Forge
	// proposes drafts after complex tasks. Store + `agt skill` CLI stay live
	// regardless. AGEZT_SKILLS=off disables injection; AGEZT_FORGE=off
	// disables post-run proposal.
	skillOn := !strings.EqualFold(os.Getenv(brand.EnvPrefix+"SKILLS"), "off")
	forgeOn := !strings.EqualFold(os.Getenv(brand.EnvPrefix+"FORGE"), "off")
	// Multi-agent delegation (P6-MULTI-01): the `delegate` tool lets a lead
	// agent spawn bounded sub-agents. On by default; AGEZT_SUBAGENT=off disables
	// it, AGEZT_SUBAGENT_DEPTH sets how deep delegation may nest (default 1).
	subAgentOn := !strings.EqualFold(os.Getenv(brand.EnvPrefix+"SUBAGENT"), "off")
	subAgentDepth := 1
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "SUBAGENT_DEPTH")); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d > 0 {
			subAgentDepth = d
		}
	}
	// AGEZT_SUBAGENT_FANOUT bounds how many sub-agents a single run may spawn at
	// its level (M46). 0 / absent = unbounded (the historical default); a
	// positive value refuses the Nth+1 delegate call with a tool error.
	subAgentFanout := 0
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "SUBAGENT_FANOUT")); v != "" {
		if f, err := strconv.Atoi(v); err == nil && f > 0 {
			subAgentFanout = f
		}
	}
	// AGEZT_SUBAGENT_SPEND_CAP caps the total spend a single run's sub-agents
	// may collectively consume (M48), given as a USD amount (matching the
	// AGEZT_*_DAILY_CEILING convention) and stored as microcents. Once a lead's
	// delegations have spent past it, the next delegate is refused. 0 / absent =
	// unbounded; a malformed value is a hard startup error (fast feedback).
	var subAgentSpendCap int64
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "SUBAGENT_SPEND_CAP")); v != "" {
		usd, perr := strconv.ParseFloat(v, 64)
		if perr != nil || usd < 0 {
			fmt.Fprintf(stderr, "%s: %sSUBAGENT_SPEND_CAP: want a non-negative USD amount, got %q\n", brand.Binary, brand.EnvPrefix, v)
			return 1
		}
		subAgentSpendCap = int64(usd * 1e9)
	}

	// Artifact offload threshold (SPEC-04 §3.6): tool outputs larger than this are
	// stored content-addressed and the journal event carries a raw_ref + preview.
	// Unset/invalid → the kernel default (agent.DefaultArtifactThreshold).
	artifactThreshold := 0
	if v := os.Getenv(brand.EnvPrefix + "ARTIFACT_THRESHOLD"); v != "" {
		if n, perr := strconv.Atoi(v); perr == nil && n > 0 {
			artifactThreshold = n
		} else {
			fmt.Fprintf(stderr, "%s: %sARTIFACT_THRESHOLD: want a positive byte count, got %q (using default)\n", brand.Binary, brand.EnvPrefix, v)
		}
	}

	// Context budget (SPEC-04 §3 / SPEC-10 §3): cap the assembled-context chars
	// the loop sends per call; over-budget runs elide their oldest tool outputs.
	// 0 (unset) = full history (current behaviour).
	contextBudget := 0
	contextBudgetAuto := false
	if v := os.Getenv(brand.EnvPrefix + "CONTEXT_BUDGET"); v != "" {
		if strings.EqualFold(v, "auto") {
			contextBudgetAuto = true // derive from the model's catalog context window
		} else if n, perr := strconv.Atoi(v); perr == nil && n > 0 {
			contextBudget = n
		} else {
			fmt.Fprintf(stderr, "%s: %sCONTEXT_BUDGET: want a positive char count or \"auto\", got %q (ignored)\n", brand.Binary, brand.EnvPrefix, v)
		}
	}

	// AGEZT_CONTEXT_PROTECT_FIRST=<n> shields the first n messages from context
	// compaction so the run's original grounding survives even as the oldest
	// middle turns are elided (SPEC-10 §3 / M395). 0 (unset) = oldest-first.
	contextProtectFirst := 0
	if v := os.Getenv(brand.EnvPrefix + "CONTEXT_PROTECT_FIRST"); v != "" {
		if n, perr := strconv.Atoi(v); perr == nil && n >= 0 {
			contextProtectFirst = n
		} else {
			fmt.Fprintf(stderr, "%s: %sCONTEXT_PROTECT_FIRST: want a non-negative count, got %q (ignored)\n", brand.Binary, brand.EnvPrefix, v)
		}
	}

	// AGEZT_CONTEXT_SUMMARIZE=1 replaces the deterministic head-snippet stub of an
	// elided tool output with a one-line abstractive summary from a bounded
	// provider call (SPEC-10 §3 / M398). Off by default — it spends extra (cached,
	// once-per-output) provider calls, so the operator opts in.
	contextSummarize := os.Getenv(brand.EnvPrefix+"CONTEXT_SUMMARIZE") == "1"

	// AGEZT_SKILL_SHADOWEVAL=on judges the shadow skills relevant to a completed
	// run against what actually happened (SPEC-05 §5.2). Off by default — it spends
	// extra provider calls per run, so the operator opts in.
	shadowEval := strings.EqualFold(os.Getenv(brand.EnvPrefix+"SKILL_SHADOWEVAL"), "on")

	cfg := kernelruntime.Config{
		BaseDir:                    baseDir,
		Provider:                   gov, // Governor implements agent.Provider
		Tools:                      tools,
		Plugins:                    pluginManifest,
		Model:                      model,
		System:                     os.Getenv(brand.EnvPrefix + "SYSTEM_PROMPT"),
		Warden:                     ward,
		Edict:                      edictEng,
		Catalog:                    cat,
		MemoryInject:               memOn,
		MemoryTool:                 memOn,
		MemoryDistill:              memOn,
		MemoryTopK:                 5,
		MemoryDistillMinTools:      4,
		WorldInject:                worldOn,
		WorldTool:                  worldOn,
		WorldTopK:                  5,
		SkillInject:                skillOn,
		SkillTopK:                  3,
		SkillForge:                 forgeOn,
		SkillForgeMinTools:         4,
		ArtifactThreshold:          artifactThreshold,
		ContextBudget:              contextBudget,
		ContextBudgetAuto:          contextBudgetAuto,
		ContextProtectFirst:        contextProtectFirst,
		ContextSummarize:           contextSummarize,
		ShadowEval:                 shadowEval,
		SubAgentTool:               subAgentOn,
		SubAgentMaxDepth:           subAgentDepth,
		SubAgentMaxFanout:          subAgentFanout,
		SubAgentMaxSpendMicrocents: subAgentSpendCap,
	}
	// Per-run wall-clock timeout (M31): AGEZT_RUN_TIMEOUT=<duration> caps how
	// long a single run may take inside a live session. Off by default (only
	// MaxIter + explicit halt bound a run); a positive duration arms the cap.
	// A malformed value is a hard startup error (fast feedback over silent
	// misconfig); a non-positive value is treated as "off".
	runTimeoutDesc := "disabled (set " + brand.EnvPrefix + "RUN_TIMEOUT, e.g. 5m)"
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "RUN_TIMEOUT")); spec != "" {
		d, derr := time.ParseDuration(spec)
		if derr != nil {
			fmt.Fprintf(stderr, "%s: %sRUN_TIMEOUT: want a Go duration (e.g. 90s, 5m), got %q\n", brand.Binary, brand.EnvPrefix, spec)
			return 1
		}
		if d > 0 {
			cfg.MaxDuration = d
			runTimeoutDesc = fmt.Sprintf("%s per run (task.failed reason=timeout on overrun)", d)
		}
	}
	// Per-tool-call timeout (M34): AGEZT_TOOL_TIMEOUT=<duration> bounds each
	// individual tool invocation. Unlike the per-run cap, an overrun fails
	// only that tool call (the model gets an error result and can adapt) —
	// the run continues. Off by default; malformed = hard startup error.
	toolTimeoutDesc := "disabled (set " + brand.EnvPrefix + "TOOL_TIMEOUT, e.g. 30s)"
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TOOL_TIMEOUT")); spec != "" {
		d, derr := time.ParseDuration(spec)
		if derr != nil {
			fmt.Fprintf(stderr, "%s: %sTOOL_TIMEOUT: want a Go duration (e.g. 30s, 2m), got %q\n", brand.Binary, brand.EnvPrefix, spec)
			return 1
		}
		if d > 0 {
			cfg.ToolTimeout = d
			toolTimeoutDesc = fmt.Sprintf("%s per tool call (error result on overrun; run continues)", d)
		}
	}
	// HITL approval window (M100): AGEZT_APPROVAL_TIMEOUT=<duration> sets how long
	// a prompt-mode approval blocks waiting for an operator before it auto-denies
	// (DecisionTimeout). Default is approval.DefaultTimeout (5m); right-size it for
	// the deployment — a short window for unattended runs, longer for an operator
	// at the console. Malformed = hard startup error; non-positive = use default.
	approvalTimeoutDesc := "default (5m; set " + brand.EnvPrefix + "APPROVAL_TIMEOUT, e.g. 2m)"
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "APPROVAL_TIMEOUT")); spec != "" {
		d, derr := time.ParseDuration(spec)
		if derr != nil {
			fmt.Fprintf(stderr, "%s: %sAPPROVAL_TIMEOUT: want a Go duration (e.g. 2m, 30s), got %q\n", brand.Binary, brand.EnvPrefix, spec)
			return 1
		}
		if d > 0 {
			cfg.ApprovalTimeout = d
			approvalTimeoutDesc = fmt.Sprintf("%s per HITL approval (auto-deny on overrun)", d)
		}
	}
	// Secret redaction (M15 / SPEC-06): scrub secrets from every durably-published
	// event before it enters the hash-chained (permanent) journal. On by default;
	// AGEZT_REDACT=off disables. Seeded with the configured provider keys (exact
	// literals) plus built-in high-confidence secret patterns.
	var redactor *redact.Redactor
	redactDesc := "disabled (" + brand.EnvPrefix + "REDACT=off)"
	if !strings.EqualFold(os.Getenv(brand.EnvPrefix+"REDACT"), "off") {
		redactor = redact.New()
		lits := credSecrets(credStore)
		redactor.SetSecrets(lits)
		redactDesc = fmt.Sprintf("enabled (%d literal secrets + built-in patterns)", len(lits))
		if n := len(extraRedactLiterals()); n > 0 {
			redactDesc += fmt.Sprintf(", %d via %sREDACT_EXTRA", n, brand.EnvPrefix)
		}
	}

	cfg.OnReload = func() error {
		// Re-load vault (catalog already refreshed by Kernel.Reload).
		if err := credStore.Load(); err != nil {
			return fmt.Errorf("credentials vault: %w", err)
		}
		// Refresh the redactor's literal set so a rotated/added key is scrubbed
		// from here on (the patterns already cover it regardless).
		if redactor != nil {
			redactor.SetSecrets(credSecrets(credStore))
		}
		freshLookup, _ := buildAWSCredChain(credStore.Lookup)

		freshCat := catStore // We hold a *Store reference; pull fresh catalog snapshot
		// catStore stays stable; the catalog data was reloaded by the
		// Kernel — but selectPrimary needs the actual *catalog.Catalog.
		// Re-load locally so we don't depend on Kernel internals.
		c, err := freshCat.Load()
		if err != nil {
			return fmt.Errorf("catalog: %w", err)
		}

		// Re-run the same selection logic the boot path uses. Errors
		// are surfaced to the operator rather than swallowed — a
		// missing credential after rotation should be visible
		// immediately, not next time the daemon happens to dispatch
		// an LLM call.
		prov, _, model2, auth, err := selectPrimary(c, freshLookup)
		if err != nil {
			return fmt.Errorf("select primary: %w", err)
		}
		_ = model2 // Model swap mid-flight would need extra plumbing; defer to M1.r.x.
		if err := gov.Replace(&governor.ProviderInfo{
			Name:     prov.Name(),
			Provider: prov,
			AuthMode: auth,
		}); err != nil {
			return fmt.Errorf("registry replace: %w", err)
		}
		return nil
	}

	k, err := kernelruntime.Open(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "%s: open runtime: %v\n", brand.Binary, err)
		return 1
	}
	defer k.Close()

	// Wire the bus into the Governor and the Warden so their events
	// land in the journal. MUST happen before any Run is dispatched.
	gov.SetBus(k.Bus())
	ward.SetBus(k.Bus())
	// Skill auto-quarantine (SPEC-05 §5): on by default — an active skill that
	// repeatedly fails in production is pulled automatically (journaled, reversible
	// with `agt skill promote`). AGEZT_SKILL_AUTOQUARANTINE=off disables it.
	autoQDesc := fmt.Sprintf("on (pull active skill after ≥%d failures at ≥%.0f%% rate; set %sSKILL_AUTOQUARANTINE=off to disable)",
		skill.DefaultAutoQuarantineMinFailures, skill.DefaultAutoQuarantineRate*100, brand.EnvPrefix)
	if strings.EqualFold(os.Getenv(brand.EnvPrefix+"SKILL_AUTOQUARANTINE"), "off") {
		k.Forge().SetAutoQuarantine(0, 0)
		autoQDesc = "off (set " + brand.EnvPrefix + "SKILL_AUTOQUARANTINE=on to enable)"
	}
	// Skill auto-shadow (SPEC-05 §5.2): off by default — staging a draft toward
	// production is opt-in. When on, a freshly-authored draft that passes the
	// deterministic shadow-test auto-advances to shadow. AGEZT_SKILL_AUTOSHADOW=on.
	autoShadowDesc := "off (set " + brand.EnvPrefix + "SKILL_AUTOSHADOW=on to auto-stage drafts that pass the shadow-test)"
	if strings.EqualFold(os.Getenv(brand.EnvPrefix+"SKILL_AUTOSHADOW"), "on") {
		k.Forge().SetAutoShadow(true)
		autoShadowDesc = "on (auto-advance a well-formed draft to shadow on creation)"
	}
	// Shadow evaluation (SPEC-05 §5.2): off by default — judging shadow skills
	// against completed runs spends extra provider calls, so the operator opts in.
	// The flag is read into kernelruntime.Config above via shadowEval.
	shadowEvalDesc := "off (set " + brand.EnvPrefix + "SKILL_SHADOWEVAL=on to judge shadow skills against completed runs)"
	if shadowEval {
		shadowEvalDesc = "on (judge relevant shadow skills against each completed run)"
	}
	// Shadow→active auto-promotion (SPEC-05 §5.2): on by default, but inert unless
	// shadow evaluation is feeding wins. AGEZT_SKILL_AUTOPROMOTE=off disables it.
	autoPromoteDesc := fmt.Sprintf("on (promote a shadow skill after ≥%d helpful evals at ≥%.0f%% rate; set %sSKILL_AUTOPROMOTE=off to disable)",
		skill.DefaultAutoPromoteMinWins, skill.DefaultAutoPromoteRate*100, brand.EnvPrefix)
	if strings.EqualFold(os.Getenv(brand.EnvPrefix+"SKILL_AUTOPROMOTE"), "off") {
		k.Forge().SetAutoPromote(0, 0)
		autoPromoteDesc = "off (set " + brand.EnvPrefix + "SKILL_AUTOPROMOTE=on to enable)"
	}
	// Egress-block audit (M109): when the http/browser tools' guard refuses a
	// dial, journal a netguard.blocked event so an operator can see attempted
	// SSRF / metadata reads. Wired here because the tools are built before the
	// kernel exists (same ordering as gov.SetBus).
	wireNetguardAudit(tools, k.Bus())
	// Install the secret redactor on the primary bus before any Run, so no
	// event is journaled un-scrubbed.
	if redactor != nil {
		k.Bus().SetRedactor(redactor)
	}

	// Durable runtime policy (M20): runtime deny rules (M18) and trust-level
	// changes (M19) are journaled as policy.changed events. When
	// AGEZT_EDICT_DURABLE=on, replay them at boot onto the freshly-built
	// engine so they survive a restart — the journal is the source of truth,
	// the engine overlay is a projection of it. Opt-in: a level *loosening*
	// that silently persisted across a restart would be a footgun, so the
	// operator asks for it explicitly. MUST run before any Run is dispatched.
	if strings.EqualFold(os.Getenv(brand.EnvPrefix+"EDICT_DURABLE"), "on") {
		overlay, rerr := replayPolicyOverlay(k)
		if rerr != nil {
			fmt.Fprintf(stderr, "%s: replay durable policy: %v\n", brand.Binary, rerr)
			return 1
		}
		nl, nr := k.Edict().ApplyOverlay(overlay)
		restored := fmt.Sprintf("restored %d level(s), %d deny rule(s)", nl, nr)
		if overlay.Mode != nil {
			// A restored mode overrides the boot AskPolicy — call it out so
			// the banner's mode label isn't silently stale.
			restored += "; mode=" + overlay.Mode.String()
		}
		askPolicyDesc += "; durable=on (" + restored + ")"
	}

	// Orphaned-run reconciliation (M28). A run that was in-flight when a
	// prior daemon exited (crash or error) sits in the journal as received
	// with no completion; mark each as abandoned now — before any new run
	// starts — so `agt runs` reflects reality instead of "running" forever.
	recoveryDesc := "clean (no in-flight runs from a prior session)"
	if n, rerr := reconcileOrphanRuns(k); rerr != nil {
		fmt.Fprintf(stderr, "%s: reconcile orphaned runs: %v\n", brand.Binary, rerr)
		return 1
	} else if n > 0 {
		recoveryDesc = fmt.Sprintf("%d run(s) abandoned on restart (were in-flight, never completed)", n)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := controlplane.NewServer(k, baseDir)
	// Cancel-on-disconnect (M35): when AGEZT_CANCEL_ON_DISCONNECT=on, a
	// streaming `agt run` whose client drops (Ctrl-C / killed) cancels its run
	// server-side instead of running on headless. Off by default so a
	// backgrounded `agt run &` (client still alive) is unaffected.
	cancelOnDisconnect := strings.EqualFold(os.Getenv(brand.EnvPrefix+"CANCEL_ON_DISCONNECT"), "on")
	srv.SetCancelOnDisconnect(cancelOnDisconnect)
	// Disk-space health (M131): inject the cross-platform disk-usage probe so the
	// disk handler / doctor check can report free space without controlplane
	// importing kernel/pulse (same decoupling as SetPulse).
	srv.SetDiskFree(pulse.DiskUsage)
	cancelOnDisconnectDesc := "disabled (set " + brand.EnvPrefix + "CANCEL_ON_DISCONNECT=on)"
	if cancelOnDisconnect {
		cancelOnDisconnectDesc = "on (a dropped `agt run` client cancels its run)"
	}
	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(stderr, "%s: start control plane: %v\n", brand.Binary, err)
		return 1
	}
	defer srv.Stop()

	// Multi-tenant registry (ROADMAP P6-MULTI), opt-in via AGEZT_MULTITENANT.
	// Each tenant gets its own isolated base dir under <baseDir>/tenants/<id>
	// and its own kernel — opened with the same provider/tools/model as the
	// primary, but a fresh per-tenant Warden/Edict (so a tenant HALT or policy
	// state is its own) and no reload hook. The primary kernel is unaffected;
	// `agt tenant` manages the registry over the control plane.
	tenantsDesc := "disabled (set " + brand.EnvPrefix + "MULTITENANT=on)"
	var tenantReg *tenant.Registry
	if strings.EqualFold(os.Getenv(brand.EnvPrefix+"MULTITENANT"), "on") {
		// Per-tenant daily spend ceiling (M14 quotas). Each tenant gets its OWN
		// governor (independent ledger) so one tenant exhausting its cap can
		// never block another's runs, while the provider pool stays shared. The
		// ceiling defaults to the primary's; AGEZT_TENANT_DAILY_CEILING (USD)
		// overrides it for every tenant.
		tenantCeiling := gov.DailyCeilingMicrocents()
		ceilingDesc := "inherited"
		if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TENANT_DAILY_CEILING")); spec != "" {
			usd, perr := strconv.ParseFloat(spec, 64)
			if perr != nil || usd < 0 {
				fmt.Fprintf(stderr, "%s: %sTENANT_DAILY_CEILING: want a non-negative USD amount, got %q\n", brand.Binary, brand.EnvPrefix, spec)
				return 1
			}
			tenantCeiling = int64(usd * 1e9)
			ceilingDesc = fmt.Sprintf("$%.2f/day", usd)
		}
		// Per-tenant per-minute call rate cap (M14 quotas). 0 = unlimited.
		tenantRate := 0
		rateDesc := "unlimited"
		if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TENANT_RATE_PER_MIN")); spec != "" {
			n, perr := strconv.Atoi(spec)
			if perr != nil || n < 0 {
				fmt.Fprintf(stderr, "%s: %sTENANT_RATE_PER_MIN: want a non-negative integer, got %q\n", brand.Binary, brand.EnvPrefix, spec)
				return 1
			}
			tenantRate = n
			rateDesc = fmt.Sprintf("%d/min", n)
		}
		reg, terr := tenant.New(filepath.Join(baseDir, "tenants"), func(id, tdir string) (io.Closer, error) {
			tgov, gerr := gov.WithLimits(tenantCeiling, tenantRate)
			if gerr != nil {
				return nil, fmt.Errorf("tenant %q governor: %w", id, gerr)
			}
			tcfg := cfg // copy the primary config value
			tcfg.BaseDir = tdir
			tcfg.TenantID = id   // stamp tenant identity onto every run's ctx (M219)
			tcfg.Provider = tgov // isolated spend ledger + per-tenant ceiling
			tcfg.Warden = nil    // fresh per-tenant warden (isolated HALT)
			tcfg.Edict = nil     // fresh per-tenant policy engine
			tcfg.OnReload = nil  // no per-tenant reload wiring yet
			tk, oerr := kernelruntime.Open(tcfg)
			if oerr != nil {
				return nil, oerr
			}
			tgov.SetBus(tk.Bus()) // budget events land in the tenant's journal
			if redactor != nil {
				tk.Bus().SetRedactor(redactor) // same scrub on the tenant's journal
			}
			// Durable runtime policy per tenant (M22): replay this tenant's OWN
			// policy.changed history so its runtime deny rules / level / mode
			// changes survive a restart, exactly as the primary does — each
			// tenant's journal is its own source of truth. Best-effort: a
			// journal read error leaves the tenant on its boot policy rather
			// than failing the lazy open. Gated on the same AGEZT_EDICT_DURABLE.
			if strings.EqualFold(os.Getenv(brand.EnvPrefix+"EDICT_DURABLE"), "on") {
				if overlay, rerr := replayPolicyOverlay(tk); rerr == nil {
					tk.Edict().ApplyOverlay(overlay)
				}
			}
			return tk, nil
		})
		if terr != nil {
			fmt.Fprintf(stderr, "%s: tenant registry: %v\n", brand.Binary, terr)
			return 1
		}
		tenantReg = reg
		srv.SetTenants(reg)
		defer reg.CloseAll()
		root := filepath.Join(baseDir, "tenants")
		if infos, _ := reg.List(); infos != nil {
			tenantsDesc = fmt.Sprintf("enabled (root=%s, %d on disk, ceiling=%s, rate=%s)", root, len(infos), ceilingDesc, rateDesc)
		} else {
			tenantsDesc = fmt.Sprintf("enabled (root=%s, ceiling=%s, rate=%s)", root, ceilingDesc, rateDesc)
		}
	}

	fmt.Fprintf(stdout, "%s %s — daemon ready (protocol v%d)\n", brand.Name, brand.Version, brand.ProtocolVersion)
	fmt.Fprintf(stdout, "  base dir         : %s\n", baseDir)
	fmt.Fprintf(stdout, "  governor         : %s\n", govDesc)
	if adv := modelAdvisory(cat, model); adv != "" {
		fmt.Fprintf(stdout, "  model advisory   : ⚠ %s\n", adv)
	}
	fmt.Fprintf(stdout, "  credentials      : %s\n", credDesc)
	fmt.Fprintf(stdout, "  redaction        : %s\n", redactDesc)
	fmt.Fprintf(stdout, "  tools            : %s\n", toolsDesc)
	fmt.Fprintf(stdout, "  policy engine    : edict (defaults from DECISIONS F3; %s)\n", askPolicyDesc)
	fmt.Fprintf(stdout, "  delegation       : %s\n", delegationBanner(k))
	fmt.Fprintf(stdout, "  run timeout      : %s\n", runTimeoutDesc)
	fmt.Fprintf(stdout, "  tool timeout     : %s\n", toolTimeoutDesc)
	fmt.Fprintf(stdout, "  approval timeout : %s\n", approvalTimeoutDesc)
	fmt.Fprintf(stdout, "  warden           : %s\n", wardDesc)
	fmt.Fprintf(stdout, "  control plane    : %s\n", srv.Addr())
	fmt.Fprintf(stdout, "  cancel-on-disc.  : %s\n", cancelOnDisconnectDesc)
	fmt.Fprintf(stdout, "  tenancy          : %s\n", tenantsDesc)
	fmt.Fprintf(stdout, "  recovery         : %s\n", recoveryDesc)
	fmt.Fprintf(stdout, "  knowledge        : memory %s · world model %s (%d entities) · skills %s/forge %s (%d active)\n",
		onOff(memOn), onOff(worldOn), k.World().Count(), onOff(skillOn), onOff(forgeOn), k.Forge().Count())
	fmt.Fprintf(stdout, "  skill auto-quar. : %s\n", autoQDesc)
	fmt.Fprintf(stdout, "  skill auto-shadow: %s\n", autoShadowDesc)
	fmt.Fprintf(stdout, "  skill shadow-eval: %s\n", shadowEvalDesc)
	fmt.Fprintf(stdout, "  skill auto-promo.: %s\n", autoPromoteDesc)

	// Telegram channel (SPEC-04 §1) — duplex when AGEZT_TELEGRAM_TOKEN is
	// set. Built before Pulse so its brief sink can tee with the log sink.
	tgChan, tgSink, tgDesc := buildTelegram(ctx, k)
	if tgChan != nil {
		go tgChan.Start(ctx)
		fmt.Fprintf(stdout, "  telegram         : %s\n", tgDesc)
	} else {
		fmt.Fprintf(stdout, "  telegram         : disabled (set AGEZT_TELEGRAM_TOKEN)\n")
	}

	// Slack channel (SPEC-04 §1) — duplex when AGEZT_SLACK_TOKEN is set. Serves
	// the Events API endpoint for inbound (HMAC-verified) and chat.postMessage for
	// outbound; briefs tee to it like Telegram.
	slChan, slSink, slDesc := buildSlack(ctx, k)
	if slChan != nil {
		go slChan.Start(ctx)
		fmt.Fprintf(stdout, "  slack            : %s\n", slDesc)
	} else {
		fmt.Fprintf(stdout, "  slack            : disabled (set AGEZT_SLACK_TOKEN)\n")
	}

	// Discord channel (SPEC-04 §1) — duplex when AGEZT_DISCORD_TOKEN is set.
	// Serves the Interactions endpoint for inbound slash commands (Ed25519-verified)
	// and posts via the bot token for outbound; briefs tee to it like the others.
	dcChan, dcSink, dcDesc := buildDiscord(ctx, k)
	if dcChan != nil {
		go dcChan.Start(ctx)
		fmt.Fprintf(stdout, "  discord          : %s\n", dcDesc)
	} else {
		fmt.Fprintf(stdout, "  discord          : disabled (set AGEZT_DISCORD_TOKEN)\n")
	}

	// Generic webhook channel (SPEC-04 §1) — vendor-neutral duplex. Any external
	// system POSTs a signed JSON message and gets the agent's reply synchronously;
	// briefs/`agt send` tee to a configured outbound URL. Enabled when a secret
	// (inbound) or an outbound URL is set.
	whChan, whSink, whDesc := buildWebhook(ctx, k)
	if whChan != nil {
		go whChan.Start(ctx)
		fmt.Fprintf(stdout, "  webhook channel  : %s\n", whDesc)
	} else {
		fmt.Fprintf(stdout, "  webhook channel  : disabled (set AGEZT_WEBHOOK_SECRET + AGEZT_WEBHOOK_ADDR)\n")
	}

	// Email channel (SPEC-04 §1) — outbound-only over SMTP. Briefs/`agt send` mail
	// the allowlisted recipients. Enabled when AGEZT_EMAIL_SMTP_ADDR is set.
	emChan, emSink, emDesc := buildEmail(ctx, k)
	if emChan != nil {
		go emChan.Start(ctx)
		fmt.Fprintf(stdout, "  email channel    : %s\n", emDesc)
	} else {
		fmt.Fprintf(stdout, "  email channel    : disabled (set AGEZT_EMAIL_SMTP_ADDR + AGEZT_EMAIL_FROM)\n")
	}

	// Pulse — the proactive heart (SPEC-03). On by default; the resident
	// engine runs on the daemon ctx so `agt halt`/SIGTERM/`agt shutdown`
	// stop it with everything else. AGEZT_PULSE=off disables it. When a channel
	// is configured, briefs tee to it (closes the Jarvis loop).
	if eng, pulseDesc := buildPulse(k, ward, model, stdout, combineSinks(tgSink, slSink, dcSink, whSink, emSink)); eng != nil {
		eng.Start(ctx)
		srv.SetPulse(eng)
		fmt.Fprintf(stdout, "  pulse            : %s\n", pulseDesc)
	} else {
		fmt.Fprintf(stdout, "  pulse            : disabled (AGEZT_PULSE=off)\n")
	}

	// Reflection (SPEC-05 §6). Always available via `agt reflect run`; set
	// AGEZT_REFLECT_EVERY (e.g. 24h) to also run a pass on a timer (mirrors
	// the Pulse ticker, on the daemon ctx). Absent → on-demand only.
	if reflectDesc := startReflectTicker(ctx, k, stdout); reflectDesc != "" {
		fmt.Fprintf(stdout, "  reflection       : %s\n", reflectDesc)
	} else {
		fmt.Fprintf(stdout, "  reflection       : on-demand (agt reflect run; set AGEZT_REFLECT_EVERY for a timer)\n")
	}

	// Web UI (SPEC-07) — the SSE Live Monitor + read panels over the same
	// bus/control plane the CLI uses. Off unless AGEZT_WEB_ADDR is set;
	// runs on the daemon ctx (halt/shutdown stop it), localhost + token.
	if webDesc := buildWebUI(ctx, k, baseDir, stdout); webDesc != "" {
		fmt.Fprintf(stdout, "  web ui           : %s\n", webDesc)
	} else {
		fmt.Fprintf(stdout, "  web ui           : disabled (set AGEZT_WEB_ADDR, e.g. 127.0.0.1:8787)\n")
	}

	// OpenAI-compatible API (P7-API-01) — POST /v1/chat/completions,
	// POST /v1/responses, and GET /v1/models so any OpenAI client drives Agezt
	// through the same tool-loop + Edict + journal. Off unless AGEZT_API_ADDR is
	// set; loopback + token.
	if apiDesc := buildOpenAIAPI(ctx, k, tenantReg, stdout); apiDesc != "" {
		fmt.Fprintf(stdout, "  openai api       : %s\n", apiDesc)
	} else {
		fmt.Fprintf(stdout, "  openai api       : disabled (set AGEZT_API_ADDR, e.g. 127.0.0.1:8799)\n")
	}

	// Outbound webhooks (P7-API-02) — POST journal events to operator-configured
	// endpoints (HMAC-signed), so external systems react to Agezt in real time.
	// Runs on the daemon ctx (halt/shutdown stop it). Off unless AGEZT_WEBHOOKS
	// is set.
	if whDesc := buildWebhooks(ctx, k, stdout); whDesc != "" {
		fmt.Fprintf(stdout, "  webhooks         : %s\n", whDesc)
	} else {
		fmt.Fprintf(stdout, "  webhooks         : disabled (set AGEZT_WEBHOOKS, e.g. https://host/hook|agent.>|secret)\n")
	}

	// Anomaly auto-halt (SPEC-06 §5): a global tool-call-rate circuit breaker
	// that auto-halts the kernel if a runaway/looping agent floods tool calls.
	// On by default; AGEZT_ANOMALY_MAX_TOOLCALLS=0 disables.
	fmt.Fprintf(stdout, "  anomaly halt     : %s\n", buildAnomaly(ctx, k, stdout))

	// draining flips true at shutdown so /readyz reports not-ready and the daemon
	// drains in-flight runs before exiting (M136). Shared with buildRESTAPI's
	// readiness probe; an atomic so the shutdown goroutine and the HTTP handler
	// race cleanly.
	var draining atomic.Bool

	// Native REST API (P7-API-02) — first-party /api/v1 surface: submit runs
	// (sync or SSE), inspect a run's journaled arc, health/models. Same governed
	// loop as `agt run`. Off unless AGEZT_REST_ADDR is set; loopback + token.
	if restDesc := buildRESTAPI(ctx, k, tenantReg, &draining, stdout); restDesc != "" {
		fmt.Fprintf(stdout, "  rest api         : %s\n", restDesc)
	} else {
		fmt.Fprintf(stdout, "  rest api         : disabled (set AGEZT_REST_ADDR, e.g. 127.0.0.1:8800)\n")
	}

	// Record the network-exposed HTTP servers (M137) so `agt status` and the
	// doctor exposure check can flag a non-loopback bind — the agent reachable
	// beyond localhost, gated only by a token. Built from the configured addrs;
	// the per-server boot banner already warns once, this makes it persistent.
	var httpBindings []controlplane.HTTPBinding
	for _, b := range []struct{ name, env string }{
		{"web ui", "WEB_ADDR"},
		{"rest api", "REST_ADDR"},
		{"openai api", "API_ADDR"},
	} {
		if addr := strings.TrimSpace(os.Getenv(brand.EnvPrefix + b.env)); addr != "" {
			httpBindings = append(httpBindings, controlplane.HTTPBinding{
				Name: b.name, Addr: addr, Loopback: isLoopback(addr),
			})
		}
	}
	srv.SetHTTPBindings(httpBindings)

	// Record the resolved AWS credential chain (M307) so `agt status` can report
	// which keyless/ambient layer engaged (IRSA, SSO, assume-role, IMDS) — the
	// boot banner's credentials line scrolls past, and on EKS an operator wants to
	// confirm IRSA is live without grepping pod logs.
	srv.SetCredChain(awsChainDesc)

	// Record the configured messaging channels (M141) so `agt status` can report
	// what's listening — the per-channel boot banner scrolls past. Read-only from
	// the same env the buildX functions consume.
	srv.SetChannels(collectChannels())

	// Wire operator-initiated outbound (`agt send`, M142) to the live channels.
	// Built from the channels actually constructed above so a kind only sends when
	// it's configured; senders journal channel.outbound via each channel's Send.
	liveChannels := map[string]channel.Channel{}
	if tgChan != nil {
		liveChannels["telegram"] = tgChan
	}
	if slChan != nil {
		liveChannels["slack"] = slChan
	}
	if dcChan != nil {
		liveChannels["discord"] = dcChan
	}
	if whChan != nil {
		liveChannels["webhook"] = whChan
	}
	if emChan != nil {
		liveChannels["email"] = emChan
	}
	channelSend := func(sctx context.Context, kind, id, text string) error {
		ch, ok := liveChannels[kind]
		if !ok {
			return fmt.Errorf("channel %q not configured", kind)
		}
		return ch.Send(sctx, channel.Outbound{ChannelID: id, Text: text, Priority: channel.PriorityNotify})
	}
	srv.SetChannelSender(channelSend)

	// Bind the proactive-messaging tool (`notify`, M143) to the live channels. The
	// tool itself was registered into the tool map BEFORE the kernel started (see
	// notifyTool below), so the map is never written while the agent loop reads it;
	// Bind only wires the sender, synchronized against Invoke. Destinations stay
	// pinned to each channel's configured allowlist (the agent supplies only text).
	if notifyTool != nil {
		notifyTool.Bind(channelSend, notifyTargets)
		fmt.Fprintf(stdout, "  notify tool      : enabled (%d channel(s) the agent can ping)\n", len(notifyTargets))
	}

	// Scheduled intents (autonomy) — fire operator-configured intents on a timer
	// through the governed loop. Runs on the daemon ctx (halt/shutdown stop it).
	// Off unless AGEZT_SCHEDULE is set.
	// AGEZT_SCHEDULE_NOTIFY=on (M152) delivers each scheduled run's answer to the
	// operator's configured channels, so a proactive digest reaches them rather than
	// only landing in the journal. Reuses the channel allowlists + sender.
	var onScheduledAnswer func(context.Context, string, string)
	if strings.TrimSpace(os.Getenv(brand.EnvPrefix+"SCHEDULE_NOTIFY")) == "on" && len(notifyTargets) > 0 {
		onScheduledAnswer = func(dctx context.Context, id, answer string) {
			deliverScheduled(dctx, channelSend, notifyTargets, id, answer)
		}
	}
	if schedDesc := buildCadence(ctx, k, stdout, onScheduledAnswer); schedDesc != "" {
		fmt.Fprintf(stdout, "  schedule         : %s\n", schedDesc)
	} else {
		fmt.Fprintf(stdout, "  schedule         : disabled (set AGEZT_SCHEDULE, e.g. \"1h=summarise new commits\")\n")
	}

	// Chronos standing-order runner (SPEC-16 §4): fires an order's plan on its
	// event/cron triggers, bounded by its budget + trust ceiling, then briefs the
	// result to the order's configured channel. Wired here (after channelSend +
	// notifyTargets) so briefing can reuse the channel allowlists + sender.
	// briefTargets is the recipient allowlist per channel kind for standing-order
	// briefings: the notify channels (telegram/slack/discord) plus the outbound
	// webhook's allowlist, so an order's `--channel webhook` reaches it too.
	briefTargets := map[string][]string{}
	for kind, ids := range notifyTargets {
		briefTargets[kind] = ids
	}
	if _, ok := liveChannels["webhook"]; ok {
		if wh := splitNonEmpty(os.Getenv(brand.EnvPrefix + "WEBHOOK_CHANNELS")); len(wh) > 0 {
			briefTargets["webhook"] = wh
		}
	}
	standingBrief := func(bctx context.Context, kind, text string) {
		for _, recip := range briefTargets[kind] {
			_ = channelSend(bctx, kind, recip, text)
		}
	}
	fmt.Fprintf(stdout, "  standing orders  : %s\n", buildStandingRunner(ctx, k, standingBrief))

	fmt.Fprintf(stdout, "  client commands  : %s run | halt | resume | why <id> | journal verify\n", brand.CLI)
	fmt.Fprintf(stdout, "Press Ctrl+C to stop.\n")

	// Stream all events to stdout so the operator sees activity.
	sub, err := k.Bus().Subscribe(">", 256)
	if err == nil {
		go func() {
			for ev := range sub.C {
				fmt.Fprintf(stdout, "  [evt seq=%d kind=%s subject=%s]\n", ev.Seq, ev.Kind, ev.Subject)
			}
		}()
		defer sub.Cancel()
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	// Also block on the control-plane's shutdown channel so `agt
	// shutdown` reaches the same orderly exit path as SIGTERM. The
	// CmdShutdown handler ACKs the client first, then closes this
	// channel; main() unblocks here and drops into the existing
	// halt-then-exit sequence.
	select {
	case s := <-sig:
		fmt.Fprintf(stdout, "\n%s: shutting down (signal=%v)...\n", brand.Binary, s)
	case <-srv.Shutdown():
		fmt.Fprintf(stdout, "\n%s: shutting down (requested via %s shutdown)...\n", brand.Binary, brand.CLI)
	}

	// Graceful drain (M136): flip readiness to not-ready FIRST — /readyz now
	// reports "draining", so a load balancer / k8s readiness probe stops routing
	// new traffic here while the process stays alive. Then wait (bounded) for
	// in-flight runs to finish before halting them, so a rolling restart doesn't
	// kill work mid-flight. AGEZT_DRAIN_TIMEOUT tunes the wait (default 15s; 0 =
	// no wait, the old immediate-halt behavior).
	draining.Store(true)
	drainTimeout := 15 * time.Second
	if v := os.Getenv(brand.EnvPrefix + "DRAIN_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d >= 0 {
			drainTimeout = d
		}
	}
	if k != nil && drainTimeout > 0 {
		if n := k.ActiveRuns(); n > 0 {
			fmt.Fprintf(stdout, "  draining: waiting up to %s for %d in-flight run(s)...\n", drainTimeout, n)
			if drainWait(k.ActiveRuns, drainTimeout) {
				fmt.Fprintf(stdout, "  drained: all in-flight runs completed\n")
			} else {
				fmt.Fprintf(stdout, "  drain timeout: %d run(s) still in flight — cancelling\n", k.ActiveRuns())
			}
		}
	}

	cancel()
	// Give any still-in-flight runs a moment to react to halt.
	deadline := time.Now().Add(2 * time.Second)
	for k != nil && !k.IsHalted() && time.Now().Before(deadline) {
		k.Halt()
		time.Sleep(50 * time.Millisecond)
	}
	return 0
}

// drainWait blocks until active() reports 0 (drained → true) or timeout elapses
// (→ false), polling every 100ms. The graceful-shutdown helper (M136), extracted
// so the wait logic is testable without standing up the whole daemon. timeout<=0
// means "don't wait": true only if nothing is in flight already.
func drainWait(active func() int, timeout time.Duration) bool {
	if timeout <= 0 {
		return active() == 0
	}
	deadline := time.Now().Add(timeout)
	for active() > 0 {
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
	return true
}

// defaultChannelHistory is the per-conversation context window (in messages) the
// channels give the agent when AGEZT_CHANNEL_HISTORY is unset. Small enough to
// bound token cost, large enough for genuine multi-turn chat. 0 disables.
const defaultChannelHistory = 10

// channelHistoryLimit reads AGEZT_CHANNEL_HISTORY (messages of prior conversation
// to include as context); defaults to defaultChannelHistory, 0 disables, a
// malformed/negative value falls back to the default.
func channelHistoryLimit() int {
	v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "CHANNEL_HISTORY"))
	if v == "" {
		return defaultChannelHistory
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return defaultChannelHistory
	}
	return n
}

// makeChannelHandler builds the inbound handler shared by every channel. It gives
// the agent multi-turn context (M144): when prior conversation exists for this
// (kind, channel id), it prepends a compact transcript so a follow-up like "and
// tomorrow?" is understood, then runs the governed loop under the message's
// correlation. With no prior context (or history disabled) it runs the raw
// message text — unchanged first-turn behavior.
// visionGate rejects an image-carrying run whose effective model is not a
// confirmed vision-capable model, mirroring the control plane's M91 gate
// (server.go) so the OpenAI API and channel run paths — which call RunWith
// directly, bypassing that gate — give a clear pre-flight error instead of a
// wasted provider call and a cryptic downstream failure (M255). Confirmed-or-
// reject: an unknown or unpriced-but-known non-vision model is refused.
func visionGate(k *kernelruntime.Kernel, model string, images []string) error {
	return gateVisionWith(k.Catalog(), k.Model(), model, images)
}

// gateVisionWith is the pure core of visionGate (catalog + default model
// injected, so it's testable without a live kernel). eff = model, or
// defaultModel when model is empty. Confirmed-or-reject: an unknown or
// known-but-non-vision model is refused when images are present.
func gateVisionWith(cat *catalog.Catalog, defaultModel, model string, images []string) error {
	if len(images) == 0 {
		return nil
	}
	eff := model
	if eff == "" {
		eff = defaultModel
	}
	visionOK := false
	if cat != nil {
		if _, m := cat.FindModel(eff); m != nil {
			visionOK = m.SupportsVision()
		}
	}
	if !visionOK {
		return fmt.Errorf("model %q does not support vision (image input); attach images only to a vision-capable model", eff)
	}
	return nil
}

func makeChannelHandler(k *kernelruntime.Kernel) channel.InboundHandler {
	limit := channelHistoryLimit()
	return func(hctx context.Context, msg channel.UnifiedMessage, corr string) (string, error) {
		intent := msg.Text
		if h := channel.ConversationHistory(k.Journal(), msg.ChannelKind, msg.ChannelID, msg.Sender, limit); h != "" {
			intent = h
		}
		// Inbound image attachments (M247): forward them to the run the same way
		// the control plane and OpenAI API do, so a photo sent to the bot reaches
		// a vision model. An image with no caption gets a default instruction.
		if len(msg.Images) > 0 {
			// Pre-gate vision capability (M255) so a non-vision model gives a clear
			// reply instead of a downstream provider error.
			if err := visionGate(k, "", msg.Images); err != nil {
				return "", err
			}
			hctx = kernelruntime.WithImages(hctx, msg.Images)
			if strings.TrimSpace(intent) == "" {
				intent = "Describe the attached image(s)."
			}
		}
		return k.RunWith(hctx, corr, intent)
	}
}

// buildTelegram constructs the in-process Telegram channel when
// AGEZT_TELEGRAM_TOKEN is set, plus a Pulse brief sink that forwards briefs to
// the allowlisted chats. Returns (nil, nil, "") when no token is configured.
//
//	AGEZT_TELEGRAM_TOKEN    bot token (required to enable)
//	AGEZT_TELEGRAM_CHAT_ID  comma-separated allowlist of chat ids that may
//	                        drive the agent AND receive Pulse briefs
//
// The inbound handler runs the normal agent loop under the channel's
// correlation, so `agt why`/`agt inbox` link the Telegram message to the task.
func buildTelegram(ctx context.Context, k *kernelruntime.Kernel) (*telegram.Channel, pulse.BriefSink, string) {
	token := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TELEGRAM_TOKEN"))
	if token == "" {
		return nil, nil, ""
	}
	chatIDs := splitNonEmpty(os.Getenv(brand.EnvPrefix + "TELEGRAM_CHAT_ID"))
	allow := channel.NewAllowlist(chatIDs)

	handler := makeChannelHandler(k)
	ch := telegram.New(telegram.Config{
		Token:     token,
		BaseURL:   strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TELEGRAM_API_BASE")), // empty → public Bot API
		Allowlist: allow,
		Bus:       k.Bus(),
		Handler:   handler,
	})

	// Pulse briefs → the allowlisted chats. Nil sink when no chat configured
	// (the bot can still receive commands once a chat is allowlisted).
	var sink pulse.BriefSink
	if len(chatIDs) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, id := range chatIDs {
				if err := ch.Send(ctx, channel.Outbound{ChannelID: id, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	desc := fmt.Sprintf("listening, allowlist=%d chat(s)", len(chatIDs))
	if len(chatIDs) == 0 {
		desc = "listening, NO allowlist (outbound-only; set AGEZT_TELEGRAM_CHAT_ID to allow commands)"
	}
	return ch, sink, desc
}

// buildSlack constructs the in-process Slack channel when AGEZT_SLACK_TOKEN is
// set, plus a Pulse brief sink to the allowlisted channels. Returns (nil, nil,
// "") when no token is configured.
//
//	AGEZT_SLACK_TOKEN           bot token (xoxb-…), required to enable
//	AGEZT_SLACK_SIGNING_SECRET  app signing secret, required for inbound
//	AGEZT_SLACK_ADDR            local addr to serve /slack/events (fronted by a
//	                            tunnel/reverse proxy); empty → outbound-only
//	AGEZT_SLACK_CHANNELS        comma-separated allowlist of channel ids that may
//	                            drive the agent AND receive Pulse briefs
//
// The inbound handler runs the normal agent loop under the channel's correlation,
// so `agt why`/`agt inbox` link the Slack message to the task.
func buildSlack(ctx context.Context, k *kernelruntime.Kernel) (*slack.Channel, pulse.BriefSink, string) {
	token := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "SLACK_TOKEN"))
	if token == "" {
		return nil, nil, ""
	}
	secret := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "SLACK_SIGNING_SECRET"))
	addr := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "SLACK_ADDR"))
	channelIDs := splitNonEmpty(os.Getenv(brand.EnvPrefix + "SLACK_CHANNELS"))

	handler := makeChannelHandler(k)
	ch := slack.New(slack.Config{
		Token:         token,
		SigningSecret: secret,
		Addr:          addr,
		BaseURL:       strings.TrimSpace(os.Getenv(brand.EnvPrefix + "SLACK_API_BASE")), // empty → public Web API
		Allowlist:     channel.NewAllowlist(channelIDs),
		Bus:           k.Bus(),
		Handler:       handler,
	})

	var sink pulse.BriefSink
	if len(channelIDs) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, id := range channelIDs {
				if err := ch.Send(ctx, channel.Outbound{ChannelID: id, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	switch {
	case addr == "" && len(channelIDs) == 0:
		return ch, sink, "outbound-only, NO allowlist (set AGEZT_SLACK_ADDR + AGEZT_SLACK_CHANNELS to receive commands)"
	case addr == "":
		return ch, sink, fmt.Sprintf("outbound-only, allowlist=%d channel(s) (set AGEZT_SLACK_ADDR to receive commands)", len(channelIDs))
	case secret == "":
		return ch, sink, "inbound DISABLED (set AGEZT_SLACK_SIGNING_SECRET); outbound only"
	default:
		return ch, sink, fmt.Sprintf("events at %s%s, allowlist=%d channel(s)", addr, slack.EventsPath, len(channelIDs))
	}
}

// buildWebhook constructs the vendor-neutral webhook channel. Enabled when an
// inbound secret OR an outbound URL is configured. Returns (nil, nil, "") when
// neither is set.
//
//	AGEZT_WEBHOOK_SECRET        HMAC-SHA256 signing key (enables signed inbound)
//	AGEZT_WEBHOOK_ADDR          local addr to serve the inbound route (fronted by
//	                            a tunnel/reverse proxy); empty → no inbound listener
//	AGEZT_WEBHOOK_PATH          inbound route (default /webhook)
//	AGEZT_WEBHOOK_CHANNELS      comma-separated allowlist of channel ids that may
//	                            drive the agent AND receive Pulse briefs
//	AGEZT_WEBHOOK_OUTBOUND_URL  where Send / briefs POST (signed); empty → inbound-only
//
// The inbound handler runs the normal agent loop under the channel's correlation,
// so `agt why`/`agt inbox` link the webhook command to the task.
func buildWebhook(ctx context.Context, k *kernelruntime.Kernel) (*webhookchan.Channel, pulse.BriefSink, string) {
	secret := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WEBHOOK_SECRET"))
	outboundURL := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WEBHOOK_OUTBOUND_URL"))
	if secret == "" && outboundURL == "" {
		return nil, nil, ""
	}
	addr := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WEBHOOK_ADDR"))
	path := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WEBHOOK_PATH"))
	channelIDs := splitNonEmpty(os.Getenv(brand.EnvPrefix + "WEBHOOK_CHANNELS"))

	ch := webhookchan.New(webhookchan.Config{
		Addr:        addr,
		Path:        path,
		Secret:      secret,
		Allowlist:   channel.NewAllowlist(channelIDs),
		OutboundURL: outboundURL,
		Bus:         k.Bus(),
		Handler:     makeChannelHandler(k),
	})

	var sink pulse.BriefSink
	if outboundURL != "" && len(channelIDs) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, id := range channelIDs {
				if err := ch.Send(ctx, channel.Outbound{ChannelID: id, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	switch {
	case secret == "":
		return ch, sink, fmt.Sprintf("outbound-only → %s, allowlist=%d (set AGEZT_WEBHOOK_SECRET + AGEZT_WEBHOOK_ADDR for inbound)", outboundURL, len(channelIDs))
	case addr == "":
		return ch, sink, fmt.Sprintf("inbound configured but not listening (set AGEZT_WEBHOOK_ADDR), allowlist=%d", len(channelIDs))
	default:
		p := path
		if p == "" {
			p = webhookchan.DefaultPath
		}
		return ch, sink, fmt.Sprintf("inbound at %s%s, allowlist=%d channel(s)", addr, p, len(channelIDs))
	}
}

// buildEmail constructs the outbound email channel when AGEZT_EMAIL_SMTP_ADDR is
// set. Returns (nil, nil, "") otherwise.
//
//	AGEZT_EMAIL_SMTP_ADDR   SMTP server host:port (e.g. smtp.example.com:587), enables
//	AGEZT_EMAIL_FROM        sender address
//	AGEZT_EMAIL_USERNAME    SMTP AUTH username (with PASSWORD); empty → no auth
//	AGEZT_EMAIL_PASSWORD    SMTP AUTH password
//	AGEZT_EMAIL_RECIPIENTS  comma-separated allowlist of recipient addresses that may
//	                        be mailed (briefs + `agt send`)
//
// Outbound-only — there's no inbound email surface (IMAP/MX is out of scope).
func buildEmail(ctx context.Context, k *kernelruntime.Kernel) (*email.Channel, pulse.BriefSink, string) {
	addr := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "EMAIL_SMTP_ADDR"))
	if addr == "" {
		return nil, nil, ""
	}
	from := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "EMAIL_FROM"))
	recipients := splitNonEmpty(os.Getenv(brand.EnvPrefix + "EMAIL_RECIPIENTS"))

	ch := email.New(email.Config{
		Addr:      addr,
		From:      from,
		Username:  strings.TrimSpace(os.Getenv(brand.EnvPrefix + "EMAIL_USERNAME")),
		Password:  os.Getenv(brand.EnvPrefix + "EMAIL_PASSWORD"),
		Allowlist: channel.NewAllowlist(recipients),
		Bus:       k.Bus(),
	})

	var sink pulse.BriefSink
	if len(recipients) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, id := range recipients {
				if err := ch.Send(ctx, channel.Outbound{ChannelID: id, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	switch {
	case from == "":
		return ch, sink, "configured but NO from address (set AGEZT_EMAIL_FROM)"
	case len(recipients) == 0:
		return ch, sink, fmt.Sprintf("outbound via %s, NO recipients (set AGEZT_EMAIL_RECIPIENTS)", addr)
	default:
		return ch, sink, fmt.Sprintf("outbound via %s, %d recipient(s)", addr, len(recipients))
	}
}

// buildDiscord constructs the in-process Discord channel when AGEZT_DISCORD_TOKEN
// is set, plus a Pulse brief sink to the allowlisted channels. Returns
// (nil, nil, "") when no token is configured.
//
//	AGEZT_DISCORD_TOKEN       bot token, required to enable
//	AGEZT_DISCORD_PUBLIC_KEY  app public key (hex), required for inbound verification
//	AGEZT_DISCORD_APP_ID      application id, required for follow-up replies
//	AGEZT_DISCORD_ADDR        local addr to serve /discord/interactions (fronted by
//	                          a tunnel/reverse proxy); empty → outbound-only
//	AGEZT_DISCORD_CHANNELS    comma-separated allowlist of channel ids that may
//	                          drive the agent AND receive Pulse briefs
//
// The inbound handler runs the normal agent loop under the channel's correlation,
// so `agt why`/`agt inbox` link the Discord command to the task.
func buildDiscord(ctx context.Context, k *kernelruntime.Kernel) (*discord.Channel, pulse.BriefSink, string) {
	token := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "DISCORD_TOKEN"))
	if token == "" {
		return nil, nil, ""
	}
	pubKey := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "DISCORD_PUBLIC_KEY"))
	appID := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "DISCORD_APP_ID"))
	addr := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "DISCORD_ADDR"))
	channelIDs := splitNonEmpty(os.Getenv(brand.EnvPrefix + "DISCORD_CHANNELS"))

	handler := makeChannelHandler(k)
	ch := discord.New(discord.Config{
		Token:         token,
		PublicKey:     pubKey,
		ApplicationID: appID,
		Addr:          addr,
		BaseURL:       strings.TrimSpace(os.Getenv(brand.EnvPrefix + "DISCORD_API_BASE")), // empty → public API
		Allowlist:     channel.NewAllowlist(channelIDs),
		Bus:           k.Bus(),
		Handler:       handler,
	})

	var sink pulse.BriefSink
	if len(channelIDs) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, id := range channelIDs {
				if err := ch.Send(ctx, channel.Outbound{ChannelID: id, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	switch {
	case addr == "" && len(channelIDs) == 0:
		return ch, sink, "outbound-only, NO allowlist (set AGEZT_DISCORD_ADDR + AGEZT_DISCORD_CHANNELS to receive commands)"
	case addr == "":
		return ch, sink, fmt.Sprintf("outbound-only, allowlist=%d channel(s) (set AGEZT_DISCORD_ADDR to receive commands)", len(channelIDs))
	case pubKey == "":
		return ch, sink, "inbound DISABLED (set AGEZT_DISCORD_PUBLIC_KEY); outbound only"
	default:
		return ch, sink, fmt.Sprintf("interactions at %s%s, allowlist=%d channel(s)", addr, discord.InteractionsPath, len(channelIDs))
	}
}

// collectChannels reports the configured messaging channels for `agt status`
// (M141), read-only from the same env the buildX functions consume. A channel is
// listed when its token is set; Inbound reflects whether it can actually receive
// and act on commands (Telegram always can; Slack/Discord need a listen addr plus
// the inbound secret/public key), so a half-configured webhook channel shows up
// as outbound-only rather than silently looking active.
func collectChannels() []controlplane.ChannelInfo {
	env := func(suffix string) string { return strings.TrimSpace(os.Getenv(brand.EnvPrefix + suffix)) }
	var out []controlplane.ChannelInfo
	if env("TELEGRAM_TOKEN") != "" {
		out = append(out, controlplane.ChannelInfo{
			Kind:      "telegram",
			Inbound:   true, // long-polls whenever a token is set
			Allowlist: len(splitNonEmpty(os.Getenv(brand.EnvPrefix + "TELEGRAM_CHAT_ID"))),
		})
	}
	if env("SLACK_TOKEN") != "" {
		addr := env("SLACK_ADDR")
		out = append(out, controlplane.ChannelInfo{
			Kind:      "slack",
			Inbound:   addr != "" && env("SLACK_SIGNING_SECRET") != "",
			Addr:      addr,
			Allowlist: len(splitNonEmpty(os.Getenv(brand.EnvPrefix + "SLACK_CHANNELS"))),
		})
	}
	if env("DISCORD_TOKEN") != "" {
		addr := env("DISCORD_ADDR")
		out = append(out, controlplane.ChannelInfo{
			Kind:      "discord",
			Inbound:   addr != "" && env("DISCORD_PUBLIC_KEY") != "",
			Addr:      addr,
			Allowlist: len(splitNonEmpty(os.Getenv(brand.EnvPrefix + "DISCORD_CHANNELS"))),
		})
	}
	if env("WEBHOOK_SECRET") != "" || env("WEBHOOK_OUTBOUND_URL") != "" {
		addr := env("WEBHOOK_ADDR")
		out = append(out, controlplane.ChannelInfo{
			Kind:      "webhook",
			Inbound:   addr != "" && env("WEBHOOK_SECRET") != "",
			Addr:      addr,
			Allowlist: len(splitNonEmpty(os.Getenv(brand.EnvPrefix + "WEBHOOK_CHANNELS"))),
		})
	}
	if env("EMAIL_SMTP_ADDR") != "" {
		out = append(out, controlplane.ChannelInfo{
			Kind:      "email",
			Inbound:   false, // outbound-only
			Addr:      env("EMAIL_SMTP_ADDR"),
			Allowlist: len(splitNonEmpty(os.Getenv(brand.EnvPrefix + "EMAIL_RECIPIENTS"))),
		})
	}
	return out
}

// combineSinks tees the configured channel brief sinks (Telegram, Slack, Discord)
// into one Pulse sink. Nil entries are dropped; returns nil when none are configured.
func combineSinks(sinks ...pulse.BriefSink) pulse.BriefSink {
	var live pulse.MultiSink
	for _, s := range sinks {
		if s != nil {
			live = append(live, s)
		}
	}
	switch len(live) {
	case 0:
		return nil
	case 1:
		return live[0]
	default:
		return live
	}
}

// briefSink returns the Pulse sink: the log sink alone, or teed with extra
// (Telegram) when configured.
func briefSink(stdout io.Writer, extra pulse.BriefSink) pulse.BriefSink {
	log := pulse.LogSink{W: stdout}
	if extra == nil {
		return log
	}
	return pulse.MultiSink{log, extra}
}

// formatBrief renders a Pulse brief as plain Telegram text.
func formatBrief(b pulse.Brief) string {
	if b.Body != "" {
		return "📣 " + b.Title + "\n" + b.Body
	}
	return "📣 " + b.Title
}

// splitNonEmpty splits a comma list, trimming and dropping blanks.
func splitNonEmpty(s string) []string {
	var out []string
	for part := range strings.SplitSeq(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// startReflectTicker starts a periodic reflection pass when AGEZT_REFLECT_EVERY
// is a valid positive duration, on the daemon ctx (so halt/shutdown stop it).
// Returns a banner description, or "" when no timer is configured. Mirrors the
// Pulse ticker lifecycle.
func startReflectTicker(ctx context.Context, k *kernelruntime.Kernel, stdout io.Writer) string {
	raw := os.Getenv(brand.EnvPrefix + "REFLECT_EVERY")
	if raw == "" {
		return ""
	}
	every, err := time.ParseDuration(raw)
	if err != nil || every <= 0 {
		fmt.Fprintf(stdout, "  reflection       : invalid AGEZT_REFLECT_EVERY %q (%v) — on-demand only\n", raw, err)
		return ""
	}
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				corr := "reflect-" + ulid.New()
				if _, err := k.Reflect().Reflect(ctx, corr); err != nil {
					fmt.Fprintf(stdout, "reflection pass failed: %v\n", err)
				}
			}
		}
	}()
	return "every " + every.String()
}

// onOff renders a boolean as a banner-friendly enabled/disabled token.
func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// buildWebUI starts the Web UI resident when AGEZT_WEB_ADDR is set, on the
// daemon ctx (so `agt halt`/SIGTERM/`agt shutdown` stop it). It serves the SSE
// Live Monitor + read panels (SPEC-07) — token-authed and read-only (SPEC-06)
// — over the same bus and control plane the CLI uses, so the two views are
// guaranteed consistent. Returns a banner description (the tokenized URL), or
// "" when disabled.
//
// httpReadHeaderTimeout bounds how long a client may take to send request headers
// — the standard slow-loris mitigation (M419). It is SSE-safe: it bounds only the
// header read, not a long-lived streaming response body, so it applies uniformly to
// the web UI (/events), the OpenAI-compat API (streaming completions), and the REST
// server. A WriteTimeout would kill those streams, so it is deliberately NOT set.
// httpIdleTimeout caps how long an idle keep-alive connection is held. (The
// control-plane TCP server has its own read deadline; these cover the HTTP surfaces.)
const (
	httpReadHeaderTimeout = 10 * time.Second
	httpIdleTimeout       = 120 * time.Second
)

// newGuardedHTTPServer builds an http.Server with the slow-loris timeouts applied
// uniformly to every HTTP surface (web UI, OpenAI-compat API, REST). WriteTimeout is
// intentionally left unset so long-lived SSE/streaming responses are not killed
// mid-flight (M419).
func newGuardedHTTPServer(h http.Handler) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		IdleTimeout:       httpIdleTimeout,
	}
}

//	AGEZT_WEB_ADDR  host:port to serve on (e.g. 127.0.0.1:8787); unset = off.
//
// We never bind 0.0.0.0 implicitly: the operator supplies the host, and the
// banner warns if it isn't loopback (public exposure is their explicit choice,
// SPEC-06).
func buildWebUI(ctx context.Context, k *kernelruntime.Kernel, baseDir string, stdout io.Writer) string {
	addr := os.Getenv(brand.EnvPrefix + "WEB_ADDR")
	if addr == "" {
		return ""
	}
	// Fresh random token, minted like the control plane's (crypto/rand → hex).
	tokBytes := make([]byte, 32)
	if _, err := rand.Read(tokBytes); err != nil {
		fmt.Fprintf(stdout, "  web ui           : disabled (token mint failed: %v)\n", err)
		return ""
	}
	token := hex.EncodeToString(tokBytes)

	// Reuse the same control-plane client `agt` builds — every read panel is a
	// proxied Cmd* call, so there is zero query duplication and full parity.
	client, err := controlplane.NewClient(baseDir)
	if err != nil {
		fmt.Fprintf(stdout, "  web ui           : disabled (control-plane client: %v)\n", err)
		return ""
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(stdout, "  web ui           : disabled (listen %s: %v)\n", addr, err)
		return ""
	}
	srv := newGuardedHTTPServer(webui.New(k.Bus(), client, token).Handler())
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(stdout, "web ui server error: %v\n", err)
		}
	}()
	// Stop with the daemon: graceful shutdown on ctx cancel.
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	desc := "http://" + ln.Addr().String() + "/?token=" + token
	if !isLoopback(addr) {
		desc += "  [WARNING: not loopback — reachable beyond localhost]"
	}
	return desc
}

// kernelAPIEngine adapts *kernelruntime.Kernel to openaiapi.Engine: it adds
// DefaultModel/ModelIDs (drawn from the configured model + synced catalog) on
// top of the run/correlation methods the kernel already exposes.
type kernelAPIEngine struct{ k *kernelruntime.Kernel }

func (e kernelAPIEngine) NewCorrelation() string        { return e.k.NewCorrelation() }
func (e kernelAPIEngine) SubjectForRun(c string) string { return e.k.SubjectForRun(c) }
func (e kernelAPIEngine) RunModel(ctx context.Context, corr, intent, model string, images []string, jsonMode bool) (string, error) {
	// Honour the requested model for this run (empty → kernel default).
	ctx = kernelruntime.WithModel(ctx, model)
	// Structured-output request (M314): a client's response_format flows to the
	// provider's CompletionRequest.JSONMode. No-op when false.
	ctx = kernelruntime.WithJSONMode(ctx, jsonMode)
	// Carry any multimodal attachments (M246) the same way the control plane
	// does, so a vision request to the OpenAI-compatible API reaches the model.
	if len(images) > 0 {
		// Pre-gate vision capability (M255): the API path bypasses the control
		// plane's M91 gate, so reject a non-vision model here with a clear error
		// rather than wasting a provider call.
		if err := visionGate(e.k, model, images); err != nil {
			return "", err
		}
		ctx = kernelruntime.WithImages(ctx, images)
	}
	return e.k.RunWith(ctx, corr, intent)
}

// UsageFor implements openaiapi.UsageReporter (M282): sum the REAL provider
// token usage for a run by folding its budget.consumed events (each LLM call the
// governor priced). Returns ok=false when nothing was consumed (a free/local/
// mock model) so the API falls back to its estimate instead of reporting 0/0.
func (e kernelAPIEngine) UsageFor(corr string) (int, int, bool) {
	in, out, found := 0, 0, false
	_ = e.k.Journal().Range(func(ev *event.Event) error {
		if ev.Kind != event.KindBudgetConsumed {
			return nil
		}
		var p struct {
			CorrelationID string `json:"correlation_id"`
			InputTokens   int    `json:"input_tokens"`
			OutputTokens  int    `json:"output_tokens"`
		}
		if json.Unmarshal(ev.Payload, &p) != nil || p.CorrelationID != corr {
			return nil
		}
		in += p.InputTokens
		out += p.OutputTokens
		found = true
		return nil
	})
	if !found || (in == 0 && out == 0) {
		return 0, 0, false
	}
	return in, out, true
}

func (e kernelAPIEngine) DefaultModel() string { return e.k.Model() }
func (e kernelAPIEngine) ModelIDs() []string {
	cat := e.k.Catalog()
	if cat == nil {
		return nil
	}
	var ids []string
	for _, p := range cat.ProviderList() {
		for id := range p.Models {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// EventsForCorrelation returns the journaled events of a run, in order, by
// ranging the journal (the restapi run-inspection route, P7-API-02). Empty when
// the correlation is unknown.
func (e kernelAPIEngine) EventsForCorrelation(corr string) ([]*event.Event, error) {
	var out []*event.Event
	err := e.k.Journal().Range(func(ev *event.Event) error {
		if ev.CorrelationID == corr {
			out = append(out, ev)
		}
		return nil
	})
	return out, err
}

// replayPolicyOverlay reads the journal, decodes every policy.changed event
// (runtime deny-rule add/rm + trust-level changes, M18/M19), and projects
// them into the net overlay to restore onto the engine (M20). The journal is
// the source of truth; the engine overlay is a projection. Order is preserved
// by Range (append-only journal), which ProjectPolicyChanges relies on for
// last-wins level semantics and add/rm rule bookkeeping.
func replayPolicyOverlay(k *kernelruntime.Kernel) (edict.PolicyOverlay, error) {
	// Compaction (M95): if a snapshot exists, seed the fold with its collapsed
	// changes and replay only the journal events recorded AFTER it. ProjectPolicyChanges
	// is resumable (snapshot.ToChanges + later changes folds to the same overlay as the
	// full history), so this is equivalent to the uncompacted replay.
	//
	// Integrity (M176): the snapshot is trusted ONLY when its content hash equals the
	// latest journaled policy.compacted hash, binding it to the tamper-evident journal.
	// A corrupt snapshot, one edited on disk to loosen policy, or one predating the
	// binding fails this check and is ignored — the journal (the source of truth) is
	// folded in full instead.
	snap, serr := edict.LoadOverlaySnapshot(overlaySnapshotPath(k))
	if serr != nil {
		snap = nil
	}

	type seqChange struct {
		seq int64
		ch  edict.PolicyChange
	}
	var all []seqChange
	var journaledHash string // latest policy.compacted content hash
	err := k.Journal().Range(func(ev *event.Event) error {
		switch ev.Kind {
		case event.KindPolicyChanged:
			var ch edict.PolicyChange
			// A single malformed historical payload must not wedge boot; skip it
			// (ProjectPolicyChanges also skips malformed content).
			if json.Unmarshal(ev.Payload, &ch) == nil {
				all = append(all, seqChange{ev.Seq, ch})
			}
		case event.KindPolicyCompacted:
			var p struct {
				ContentHash string `json:"content_hash"`
			}
			if json.Unmarshal(ev.Payload, &p) == nil {
				journaledHash = p.ContentHash // last one wins
			}
		}
		return nil
	})
	if err != nil {
		return edict.PolicyOverlay{}, err
	}

	var changes []edict.PolicyChange
	fromSeq := int64(-1)
	if snap != nil && journaledHash != "" && snap.ContentHash() == journaledHash {
		changes = append(changes, snap.Changes...)
		fromSeq = snap.ThroughSeq
	}
	for _, sc := range all {
		if sc.seq > fromSeq {
			changes = append(changes, sc.ch)
		}
	}
	return edict.ProjectPolicyChanges(changes), nil
}

// overlaySnapshotPath is the per-kernel durable-policy snapshot location (M95),
// under the kernel's own base dir so each tenant snapshots independently.
func overlaySnapshotPath(k *kernelruntime.Kernel) string {
	return filepath.Join(k.BaseDir(), "runtime", edict.OverlaySnapshotFile)
}

// orphanRun is a run that was received but never completed in a prior
// session — found at boot by runScan.
type orphanRun struct {
	Corr      string
	Intent    string
	StartedMS int64
}

// runScan folds the journal's task.* events to find orphaned runs (M28). A
// run is orphaned when it has a task.received but no terminal event:
// neither a task.completed (it finished), a task.failed (it errored out
// live — M30), nor a task.abandoned (we already reconciled it on an
// earlier boot — the idempotency guard). Pure and fed one event at a
// time, so it's unit-testable without a kernel.
type runScan struct {
	received  map[string]*orphanRun
	completed map[string]bool
	failed    map[string]bool
	abandoned map[string]bool
}

func newRunScan() *runScan {
	return &runScan{
		received:  map[string]*orphanRun{},
		completed: map[string]bool{},
		failed:    map[string]bool{},
		abandoned: map[string]bool{},
	}
}

func (s *runScan) observe(e *event.Event) {
	switch e.Kind {
	case event.KindTaskReceived:
		o := &orphanRun{Corr: e.CorrelationID, StartedMS: e.TSUnixMS}
		var p struct {
			Intent string `json:"intent"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		o.Intent = p.Intent
		s.received[e.CorrelationID] = o
	case event.KindTaskCompleted:
		s.completed[e.CorrelationID] = true
	case event.KindTaskFailed:
		s.failed[e.CorrelationID] = true
	case event.KindTaskAbandoned:
		s.abandoned[e.CorrelationID] = true
	}
}

// orphans returns the orphaned runs, sorted by start time then correlation
// id for deterministic output (and stable abandon-event ordering).
func (s *runScan) orphans() []orphanRun {
	var out []orphanRun
	for corr, o := range s.received {
		if !s.completed[corr] && !s.failed[corr] && !s.abandoned[corr] {
			out = append(out, *o)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedMS != out[j].StartedMS {
			return out[i].StartedMS < out[j].StartedMS
		}
		return out[i].Corr < out[j].Corr
	})
	return out
}

// reconcileOrphanRuns scans the journal at boot for runs that were in-flight
// when a prior daemon exited and publishes a task.abandoned event for each,
// so `agt runs` shows them as "abandoned" rather than "running" forever
// (M28). Idempotent: a run already carrying task.abandoned is skipped, so
// repeated restarts don't re-abandon. Returns the count reconciled. MUST run
// before any new Run is dispatched (so the scan can't see a live run).
func reconcileOrphanRuns(k *kernelruntime.Kernel) (int, error) {
	scan := newRunScan()
	if err := k.Journal().Range(func(e *event.Event) error {
		scan.observe(e)
		return nil
	}); err != nil {
		return 0, err
	}
	orphans := scan.orphans()
	for _, o := range orphans {
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "task",
			Kind:          event.KindTaskAbandoned,
			Actor:         "kernel",
			CorrelationID: o.Corr,
			Payload: map[string]any{
				"intent":          o.Intent,
				"reason":          "daemon restart: run was in-flight and never completed",
				"started_unix_ms": o.StartedMS,
			},
		})
	}
	return len(orphans), nil
}

// modelAdvisory returns a one-line agent-readiness advisory for the selected
// primary model (M24), or "" when the model is unknown to the catalog or has no
// concerns. It surfaces the same catalog.Model.AgentWarnings that
// `agt provider check --caps` reports, but at boot — the moment an operator
// would want to know the headline gap: a model that doesn't advertise tool-use,
// which the tool-driven agent loop relies on. Unknown models (the offline mock,
// a model absent from the catalog) yield no advisory rather than a false alarm.
func modelAdvisory(cat *catalog.Catalog, model string) string {
	if cat == nil || model == "" {
		return ""
	}
	_, m := cat.FindModel(model)
	if m == nil {
		return ""
	}
	return strings.Join(m.AgentWarnings(), "; ")
}

// credSecrets returns the non-empty values of every vault entry plus any extra
// operator-supplied literals, for seeding the secret redactor (M15). Values, not
// names — the redactor scrubs the actual secret strings wherever they appear in
// event payloads. Extra literals (AGEZT_REDACT_EXTRA, ';'-separated) cover
// site-specific secrets not in the provider vault and not matching a built-in
// pattern (internal API tokens, DB passwords, …).
func credSecrets(store *creds.Store) []string {
	names := store.Names()
	vals := make([]string, 0, len(names))
	for _, n := range names {
		if v := store.Get(n); v != "" {
			vals = append(vals, v)
		}
	}
	vals = append(vals, extraRedactLiterals()...)
	return vals
}

// extraRedactLiterals parses AGEZT_REDACT_EXTRA into a list of additional literal
// secrets to scrub. Entries are ';'-separated and trimmed; empties are dropped.
func extraRedactLiterals() []string {
	spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "REDACT_EXTRA"))
	if spec == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(spec, ";") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// buildOpenAIAPI starts the OpenAI-compatible HTTP resident when AGEZT_API_ADDR
// is set, mirroring buildWebUI's lifecycle (daemon ctx, graceful shutdown,
// minted token, loopback warning). Returns the banner description or "".
func buildOpenAIAPI(ctx context.Context, k *kernelruntime.Kernel, reg *tenant.Registry, stdout io.Writer) string {
	addr := os.Getenv(brand.EnvPrefix + "API_ADDR")
	if addr == "" {
		return ""
	}
	tokBytes := make([]byte, 32)
	if _, err := rand.Read(tokBytes); err != nil {
		fmt.Fprintf(stdout, "  openai api       : disabled (token mint failed: %v)\n", err)
		return ""
	}
	token := hex.EncodeToString(tokBytes)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(stdout, "  openai api       : disabled (listen %s: %v)\n", addr, err)
		return ""
	}
	api := openaiapi.New(kernelAPIEngine{k}, k.Bus(), token)
	if reg != nil {
		// Tenant routing: an X-Agezt-Tenant header serves the request from that
		// tenant's isolated kernel + bus (opened on demand).
		api.SetTenantResolver(func(id string) (openaiapi.Engine, *bus.Bus, error) {
			t, err := reg.Acquire(id, time.Now())
			if err != nil {
				return nil, nil, err
			}
			tk, ok := t.Kernel.(*kernelruntime.Kernel)
			if !ok {
				return nil, nil, fmt.Errorf("tenant %q: unexpected kernel type", id)
			}
			return kernelAPIEngine{tk}, tk.Bus(), nil
		})
		api.SetTenantAuthorizer(reg.Authorize)
	}
	srv := newGuardedHTTPServer(api.Handler())
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(stdout, "openai api server error: %v\n", err)
		}
	}()
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	desc := "http://" + ln.Addr().String() + "/v1  (Authorization: Bearer " + token + ")"
	if !isLoopback(addr) {
		desc += "  [WARNING: not loopback — reachable beyond localhost]"
	}
	return desc
}

// buildRESTAPI starts the native REST resident when AGEZT_REST_ADDR is set,
// mirroring buildOpenAIAPI's lifecycle (daemon ctx, graceful shutdown, minted
// token, loopback warning). Returns the banner description or "".
func buildRESTAPI(ctx context.Context, k *kernelruntime.Kernel, reg *tenant.Registry, draining *atomic.Bool, stdout io.Writer) string {
	addr := os.Getenv(brand.EnvPrefix + "REST_ADDR")
	if addr == "" {
		return ""
	}
	tokBytes := make([]byte, 32)
	if _, err := rand.Read(tokBytes); err != nil {
		fmt.Fprintf(stdout, "  rest api         : disabled (token mint failed: %v)\n", err)
		return ""
	}
	token := hex.EncodeToString(tokBytes)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(stdout, "  rest api         : disabled (listen %s: %v)\n", addr, err)
		return ""
	}
	rest := restapi.New(kernelAPIEngine{k}, k.Bus(), token, brand.Version)
	// Readiness probe (M134): /readyz reports not-ready while the daemon is
	// halted, so a load balancer / k8s readiness probe pulls it from rotation
	// without the process dying. Liveness (/healthz) stays up regardless.
	rest.SetReadiness(func() (bool, string) {
		if draining.Load() {
			return false, "draining"
		}
		if k.IsHalted() {
			return false, "halted"
		}
		return true, ""
	})
	// Prometheus /metrics (M135): expose the cheap in-memory operational gauges
	// (same data as status/budget/disk) so the daemon can be wired into Grafana /
	// alerting. All reads are O(1) or O(segments); no per-scrape journal fold.
	rest.SetMetrics(func() []restapi.Metric {
		boolf := func(b bool) float64 {
			if b {
				return 1
			}
			return 0
		}
		schedTotal, schedEnabled := 0, 0
		if st := k.Schedules(); st != nil {
			for _, e := range st.List() {
				schedTotal++
				if e.Enabled {
					schedEnabled++
				}
			}
		}
		headSeq, _ := k.Journal().Head()
		if headSeq < 0 {
			headSeq = 0
		}
		var spent, ceiling int64
		if gov, ok := k.Provider().(*governor.Governor); ok {
			snap := gov.Snapshot()
			spent, ceiling = snap.SpentMicrocents, snap.CeilingMicrocents
		}
		base := k.BaseDir()
		var journalBytes int64
		_ = filepath.Walk(filepath.Join(base, "journal"), func(_ string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				journalBytes += info.Size()
			}
			return nil
		})
		var diskFree, diskTotal uint64
		if f, tot, err := pulse.DiskUsage(base); err == nil {
			diskFree, diskTotal = f, tot
		}
		diskRatio := 0.0
		if diskTotal > 0 {
			diskRatio = float64(diskFree) / float64(diskTotal)
		}
		pending := 0
		if ap := k.Approvals(); ap != nil {
			pending = ap.PendingCount()
		}
		return []restapi.Metric{
			{Name: "up", Help: "1 if the daemon is serving", Value: 1},
			{Name: "halted", Help: "1 if the daemon is halted", Value: boolf(k.IsHalted())},
			{Name: "uptime_seconds", Help: "seconds since the daemon started", Value: time.Since(k.StartTime()).Seconds()},
			{Name: "active_runs", Help: "runs currently in flight", Value: float64(k.ActiveRuns())},
			{Name: "journal_head_seq", Help: "latest journal sequence number", Value: float64(headSeq)},
			{Name: "memory_records", Help: "live memory records", Value: float64(k.Memory().Count())},
			{Name: "world_entities", Help: "live world-model entities", Value: float64(k.World().Count())},
			{Name: "active_skills", Help: "active skills", Value: float64(k.Forge().Count())},
			{Name: "schedules_total", Help: "scheduled intents", Value: float64(schedTotal)},
			{Name: "schedules_enabled", Help: "enabled scheduled intents", Value: float64(schedEnabled)},
			{Name: "pending_approvals", Help: "HITL approvals awaiting an operator", Value: float64(pending)},
			{Name: "spend_today_microcents", Help: "today's spend in microcents ($1=1e9)", Value: float64(spent)},
			{Name: "budget_ceiling_microcents", Help: "daily budget ceiling in microcents (0=unbounded)", Value: float64(ceiling)},
			{Name: "journal_bytes", Help: "journal size on disk in bytes", Value: float64(journalBytes)},
			{Name: "disk_free_bytes", Help: "free bytes on the journal filesystem", Value: float64(diskFree)},
			{Name: "disk_free_ratio", Help: "free fraction of the journal filesystem (0..1)", Value: diskRatio},
		}
	})
	if reg != nil {
		// Tenant routing: an X-Agezt-Tenant header serves the request from that
		// tenant's isolated kernel + bus (opened on demand).
		rest.SetTenantResolver(func(id string) (restapi.Engine, *bus.Bus, error) {
			t, err := reg.Acquire(id, time.Now())
			if err != nil {
				return nil, nil, err
			}
			tk, ok := t.Kernel.(*kernelruntime.Kernel)
			if !ok {
				return nil, nil, fmt.Errorf("tenant %q: unexpected kernel type", id)
			}
			return kernelAPIEngine{tk}, tk.Bus(), nil
		})
		rest.SetTenantAuthorizer(reg.Authorize)
	}
	srv := newGuardedHTTPServer(rest.Handler())
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(stdout, "rest api server error: %v\n", err)
		}
	}()
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	desc := "http://" + ln.Addr().String() + "/api/v1  (Authorization: Bearer " + token + ")"
	if !isLoopback(addr) {
		desc += "  [WARNING: not loopback — reachable beyond localhost]"
	}
	return desc
}

// buildWebhooks starts the outbound-webhook dispatcher when AGEZT_WEBHOOKS is
// set. It subscribes to the bus on the daemon ctx (so halt/shutdown stop it) and
// POSTs matching events to the configured sinks. Returns the banner description;
// "" only when the env var is unset (an empty/invalid spec returns a one-line
// reason so the operator sees the misconfiguration).
func buildWebhooks(ctx context.Context, k *kernelruntime.Kernel, stdout io.Writer) string {
	spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WEBHOOKS"))
	if spec == "" {
		return ""
	}
	sinks, err := webhook.ParseSinks(spec)
	if err != nil {
		return "disabled (" + err.Error() + ")"
	}
	if len(sinks) == 0 {
		return ""
	}
	// Egress guard (M416, SPEC-06): outbound webhook deliveries are subject to the
	// same default-deny egress policy as the http/browser tools, so a configured
	// sink cannot reach loopback / RFC1918 / the cloud-metadata endpoint. Operators
	// who legitimately deliver to an internal sink opt the range back in.
	var guardOpts []netguard.Option
	egress := "guarded"
	if os.Getenv(brand.EnvPrefix+"WEBHOOK_ALLOW_LOOPBACK") == "1" {
		guardOpts = append(guardOpts, netguard.AllowLoopback())
		egress = "loopback-ok"
	}
	if os.Getenv(brand.EnvPrefix+"WEBHOOK_ALLOW_PRIVATE") == "1" {
		guardOpts = append(guardOpts, netguard.AllowPrivate())
		if egress == "loopback-ok" {
			egress = "loopback+private-ok"
		} else {
			egress = "private-ok"
		}
		fmt.Fprintln(stdout, "WARNING: AGEZT_WEBHOOK_ALLOW_PRIVATE=1 lets webhook sinks reach the private network.")
	}
	client := netguard.New(guardOpts...).HTTPClient(webhook.DefaultTimeout)
	webhook.NewDispatcher(k.Bus(), sinks, stdout, webhook.WithClient(client)).Start(ctx)
	return webhook.Describe(sinks) + " [egress=" + egress + "]"
}

// buildAnomaly starts the anomaly auto-halt circuit breaker (SPEC-06 §5). It
// watches the global tool-call rate and auto-halts the kernel on a runaway
// spike — a safety backstop above the per-run loop guard (M116). On by default;
// AGEZT_ANOMALY_MAX_TOOLCALLS sets the ceiling (0 disables),
// AGEZT_ANOMALY_WINDOW the measurement window. Returns a banner description.
func buildAnomaly(ctx context.Context, k *kernelruntime.Kernel, stdout io.Writer) string {
	max := 120 // ~12 tool calls/sec sustained — only a tight loop hits this
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ANOMALY_MAX_TOOLCALLS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			max = n
		}
	}
	window := 10 * time.Second
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ANOMALY_WINDOW")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			window = d
		}
	}
	if max <= 0 {
		return "disabled (AGEZT_ANOMALY_MAX_TOOLCALLS=0)"
	}
	started := anomaly.Start(ctx, k.Bus(), anomaly.Config{MaxToolCalls: max, Window: window}, func(reason string) {
		fmt.Fprintf(stdout, "  ⚠ anomaly auto-halt engaged: %s\n", reason)
		k.HaltWith(reason)
	})
	if !started {
		return "disabled"
	}
	return fmt.Sprintf("on (halt if >%d tool calls / %s; set AGEZT_ANOMALY_MAX_TOOLCALLS=0 to disable)", max, window)
}

// buildStandingRunner starts the event-trigger half of Chronos (SPEC-16 §4): when
// a journal event matches an enabled order's event trigger, the order's plan is
// launched as a run (bounded by its budget ceiling) and a standing.fired event is
// journaled. Cron triggers are handled by the schedule engine, not here.
func buildStandingRunner(ctx context.Context, k *kernelruntime.Kernel, brief func(ctx context.Context, kind, text string)) string {
	fire := func(fctx context.Context, o standing.Order, subject string) {
		// A fired order launches a full governed run (provider/tool/plugin code) and
		// then briefs over the network — any of which can panic. This goroutine is
		// dispatched with a bare `go fire(...)` by the runner/cron loop, so its own
		// recover() does NOT cover us; without this defer a single bad run would take
		// down the whole daemon. Contain the panic to this order and journal it as a
		// standing.error so it stays diagnosable (`agt journal`).
		defer func() {
			if r := recover(); r != nil {
				_, _ = k.Bus().Publish(event.Spec{
					Subject: "standing." + o.ID,
					Kind:    event.KindStandingError,
					Actor:   "standing",
					Payload: map[string]any{"id": o.ID, "name": o.Name, "trigger_subject": subject, "panic": fmt.Sprintf("%v", r)},
				})
			}
		}()
		corr := k.NewCorrelation()
		intent := strings.TrimSpace(o.Plan)
		if intent == "" {
			intent = o.Name
		}
		// Ground the run in the order's scope (SPEC-16 §4): the agent is told what
		// this standing order watches.
		intent = standing.ScopedIntent(o, intent)
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "standing." + o.ID,
			Kind:          event.KindStandingFired,
			Actor:         "standing",
			CorrelationID: corr,
			Payload:       map[string]any{"id": o.ID, "name": o.Name, "trigger_subject": subject, "intent": intent},
		})
		rctx := fctx
		if o.Initiative.BudgetPerRunMc > 0 {
			rctx = kernelruntime.WithMaxCost(rctx, o.Initiative.BudgetPerRunMc)
		}
		// Cap autonomous action at the order's max_trust ceiling (SPEC-16 §4): a
		// normally auto-allowed tool is downgraded to Ask/Deny within this run.
		if lvl, perr := edict.ParseTrustLevel(o.Initiative.MaxTrust); perr == nil {
			rctx = kernelruntime.WithTrustCeiling(rctx, lvl)
		}
		answer, _ := k.RunWith(rctx, corr, intent)
		// Brief the result to the order's configured channel (SPEC-16 §4 briefing).
		if text, ok := standing.BriefText(o, answer); ok && brief != nil {
			brief(fctx, o.BriefingChan, text)
		}
	}
	evOK := standing.StartRunner(ctx, k.Bus(), k.Standing(), standing.RunnerConfig{}, fire)
	cronOK := standing.StartCron(ctx, k.Standing(), nil, fire)
	if !evOK && !cronOK {
		return "disabled"
	}
	return fmt.Sprintf("on (event + cron triggers; %d order(s) defined)", k.Standing().Count())
}

// delegationBanner renders the active multi-agent delegation ceilings (M58) for
// the boot banner — the same effective caps `agt status` reports (M49), so the
// governance is visible at startup, not only on demand. "off" when the delegate
// tool is disabled; 0 fan-out / spend render as "unbounded".
func delegationBanner(k *kernelruntime.Kernel) string {
	l := k.SubAgentLimits()
	if !l.Enabled {
		return "off (AGEZT_SUBAGENT=off)"
	}
	fanout := "unbounded"
	if l.MaxFanout > 0 {
		fanout = fmt.Sprintf("≤%d", l.MaxFanout)
	}
	spend := "unbounded"
	if l.MaxSpendMicrocents > 0 {
		spend = fmt.Sprintf("$%.4f", float64(l.MaxSpendMicrocents)/1e9)
	}
	return fmt.Sprintf("depth≤%d, fan-out %s, spend %s", l.MaxDepth, fanout, spend)
}

// buildCadence starts the scheduled-intents resident when AGEZT_SCHEDULE is set.
// Each firing journals a schedule.fired event (carrying the run's correlation so
// `agt why` links the schedule to the run) and then runs the intent through the
// normal governed loop. Returns the banner description; "" only when the env var
// is unset and the store is empty.
// deliverScheduled sends a scheduled run's answer to every configured channel
// recipient (M152), prefixed with the schedule id so the operator knows which job
// produced it. Empty answers are skipped. Returns the number of successful
// deliveries (for testing). Channel kinds are iterated in sorted order for
// deterministic delivery.
func deliverScheduled(ctx context.Context, send func(context.Context, string, string, string) error, targets map[string][]string, id, answer string) int {
	if strings.TrimSpace(answer) == "" || send == nil {
		return 0
	}
	text := "[scheduled: " + id + "]\n" + answer
	kinds := make([]string, 0, len(targets))
	for k := range targets {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	sent := 0
	for _, kind := range kinds {
		for _, recip := range targets[kind] {
			if send(ctx, kind, recip, text) == nil {
				sent++
			}
		}
	}
	return sent
}

func buildCadence(ctx context.Context, k *kernelruntime.Kernel, stdout io.Writer, onAnswer func(ctx context.Context, id, answer string)) string {
	store := k.Schedules()
	if store == nil {
		return ""
	}
	// Sync AGEZT_SCHEDULE env jobs into the store (idempotent: replaces the
	// previous env-sourced entries, leaves operator-managed ones untouched).
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "SCHEDULE")); spec != "" {
		jobs, err := cadence.ParseJobs(spec)
		if err != nil {
			return "disabled (" + err.Error() + ")"
		}
		if err := store.SyncEnv(jobs, time.Now()); err != nil {
			return "disabled (" + err.Error() + ")"
		}
	} else {
		_ = store.SyncEnv(nil, time.Now()) // env cleared → drop stale env entries
	}
	// The engine always runs (so operator-added schedules fire even with no env
	// spec). With no entries it simply ticks idly.
	run := func(runCtx context.Context, id, intent, model string) error {
		corr := k.NewCorrelation()
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "schedule.fired",
			Kind:          event.KindScheduleFired,
			Actor:         "schedule",
			CorrelationID: corr,
			// schedule_id (M55) attributes the firing to its schedule entry, so
			// `agt schedule fires --id <sched>` can filter and `agt schedule list`
			// can show a schedule's last outcome.
			Payload: map[string]any{"schedule_id": id, "intent": intent, "model": model},
		})
		ans, err := k.RunWith(kernelruntime.WithModel(runCtx, model), corr, intent)
		// Deliver the scheduled run's answer to the operator's channels when
		// AGEZT_SCHEDULE_NOTIFY is on (M152): a proactive morning digest reaches
		// you instead of sitting silently in the journal. Only on success with a
		// non-empty answer; off entirely when onAnswer is nil.
		if err == nil && onAnswer != nil {
			onAnswer(runCtx, id, ans)
		}
		return err
	}
	cadence.NewEngine(store, run, 0, stdout).Start(ctx)

	entries := store.List()
	if len(entries) == 0 {
		return "active (no schedules yet — add with `agt schedule add`)"
	}
	return cadence.Describe(entries)
}

// isLoopback reports whether the host portion of addr binds to loopback only.
func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "" {
		return false // empty host = all interfaces
	}
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// buildPulse constructs the resident Pulse engine from env config, or returns
// (nil, "") when AGEZT_PULSE=off. Observers are wired only when configured:
//
//	AGEZT_PULSE_CADENCE      beat interval (default 60s)
//	AGEZT_PULSE_DIAL         quiet|balanced|chatty (default balanced)
//	AGEZT_PULSE_QUIET_HOURS  e.g. "22-7" (only alerts break through)
//	AGEZT_PULSE_PROBE        "name=ci;argv=make test" → green↔red CI detector
//	AGEZT_PULSE_DISK         "/:10" → alert under 10% free on "/"
//	AGEZT_PULSE_LLM=on       enable the optional cheap-LLM salience refine
func buildPulse(k *kernelruntime.Kernel, ward warden.Engine, model string, stdout io.Writer, extraSink pulse.BriefSink) (*pulse.Engine, string) {
	if strings.EqualFold(os.Getenv(brand.EnvPrefix+"PULSE"), "off") {
		return nil, ""
	}
	cadence := 60 * time.Second
	if v := os.Getenv(brand.EnvPrefix + "PULSE_CADENCE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cadence = d
		}
	}
	dial := pulse.ParseDial(os.Getenv(brand.EnvPrefix + "PULSE_DIAL"))
	qh := pulse.ParseQuietHours(os.Getenv(brand.EnvPrefix + "PULSE_QUIET_HOURS"))

	var obs []pulse.Observer
	var parts []string
	if spec := os.Getenv(brand.EnvPrefix + "PULSE_PROBE"); spec != "" {
		if name, argv, ok := pulse.ParseProbeSpec(spec); ok {
			obs = append(obs, pulse.NewProbeObserver(name, argv, ward, k.State()))
			parts = append(parts, "probe:"+name)
		}
	}
	if spec := os.Getenv(brand.EnvPrefix + "PULSE_DISK"); spec != "" {
		if path, pctStr, ok := strings.Cut(spec, ":"); ok {
			if pct, err := strconv.ParseFloat(pctStr, 64); err == nil && pct > 0 {
				obs = append(obs, pulse.NewDiskObserver(path, pct, pulse.DiskUsage))
				parts = append(parts, "disk:"+path)
			}
		}
	}
	useLLM := strings.EqualFold(os.Getenv(brand.EnvPrefix+"PULSE_LLM"), "on")

	eng := pulse.New(pulse.Config{
		Bus:        k.Bus(),
		State:      k.State(),
		Warden:     ward,
		Provider:   k.Provider(),
		Model:      model,
		Relevance:  k.World(), // world-model relevance signal (SPEC-05 §3.4)
		Observers:  obs,
		Dial:       dial,
		Cadence:    cadence,
		QuietHours: qh,
		UseLLM:     useLLM,
		Sink:       briefSink(stdout, extraSink),
	})
	observers := "no observers configured"
	if len(parts) > 0 {
		observers = strings.Join(parts, ",")
	}
	return eng, fmt.Sprintf("dial=%s cadence=%s observers=[%s]", dial, cadence, observers)
}

// buildGovernor constructs the routing layer: one primary provider
// (chosen from the catalog) plus an always-on fallback. Returns the
// Governor (also serves as agent.Provider), a human-readable
// description for the banner, and the default model name for the
// kernel config.
//
// **Provider selection (M1.g — catalog-driven):**
//
//	$AGEZT_PROVIDER=mock            → offline 2-turn demo mock.
//	$AGEZT_PROVIDER=<catalog-id>    → e.g. "anthropic", "ollama-local",
//	                                  "groq", "openai" — any provider
//	                                  in the synced catalog.
//	(unset)                          → auto-pick: first credentialed
//	                                  catalog provider whose family is
//	                                  supported by `compat`. Falls back
//	                                  to mock if none.
//	$AGEZT_MODEL=<model-id>         → override the model within the
//	                                  selected provider. If unset, the
//	                                  alphabetically-first model in the
//	                                  provider's catalog entry is used.
//
// Fallback chain: the offline demo mock is *always* registered as
// IsFallback=true (unless the primary IS the mock) so a transient
// primary failure surfaces a degraded-but-working answer rather than
// a hard error.
func buildGovernor(cat *catalog.Catalog, lookup func(string) string) (*governor.Governor, string, string, error) {
	reg := governor.NewRegistry()
	primary, primaryDesc, model, authMode, err := selectPrimary(cat, lookup)
	if err != nil {
		return nil, "", "", err
	}
	// Demo escape hatch: AGEZT_DEMO_FAIL_PRIMARY=1 wraps the primary in
	// an always-erroring shim so the fallback chain is observable from
	// `agt run`. Used by the M1.b PHASE report and never in production.
	// We rename the shim to "<orig>-failshim" so it never collides with
	// a mock fallback that shares the original name.
	demoFail := os.Getenv(brand.EnvPrefix+"DEMO_FAIL_PRIMARY") == "1"
	origPrimaryName := primary.Name()
	primaryName := origPrimaryName
	if demoFail {
		primaryName = primaryName + "-failshim"
		primary = &alwaysFailProvider{name: primaryName}
		primaryDesc = "[demo-shim:always-fail] " + primaryDesc
	}
	if err := reg.Register(&governor.ProviderInfo{
		Name:     primaryName,
		Provider: primary,
		AuthMode: authMode,
		Models:   catalogModelIDs(cat, origPrimaryName),
	}); err != nil {
		return nil, "", "", fmt.Errorf("register primary: %w", err)
	}
	// Track which catalog providers actually got registered — the eligible
	// set for cross-provider down-routing (M40). Keyed by catalog provider id
	// (not the shim Name), so it matches catalog lookups.
	registered := map[string]bool{origPrimaryName: true}

	// Register every OTHER credentialed + supported catalog provider as a
	// model-routable alternate (SPEC-15 §1): a request naming one of their
	// models is routed to that provider (per-request model routing), while the
	// primary stays the default. Build failures are skipped, never fatal — a
	// misconfigured alternate must not stop the daemon. Each compat provider's
	// Name() is its unique catalog id (wrapNamed), so there are no collisions.
	extraProviders := 0
	for _, entry := range cat.ProviderList() {
		if entry.ID == origPrimaryName {
			continue // already the primary
		}
		if !compat.IsSupportedFamily(entry.Family()) || !entry.HasCredentials(lookup) {
			continue
		}
		p, _, _, auth, err := buildFromCatalog(entry, "", lookup)
		if err != nil {
			continue
		}
		if err := reg.Register(&governor.ProviderInfo{
			Name:     p.Name(),
			Provider: p,
			AuthMode: auth,
			Models:   catalogModelIDs(cat, entry.ID),
		}); err != nil {
			continue // duplicate name or similar — skip gracefully
		}
		registered[entry.ID] = true
		extraProviders++
	}

	// Always add the offline demo mock as a last-resort fallback —
	// unless the primary IS the (unshimmed) mock (avoid duplicate-name
	// register). Under DEMO_FAIL_PRIMARY=1 the shim is renamed, so we
	// always register the fresh mock as the fallback.
	fallbackDesc := ""
	if demoFail || primaryName != "mock" {
		fb := newDemoMock()
		if err := reg.Register(&governor.ProviderInfo{
			Name:       fb.Name(),
			Provider:   fb,
			AuthMode:   governor.AuthLocal,
			IsFallback: true,
		}); err != nil {
			return nil, "", "", fmt.Errorf("register fallback: %w", err)
		}
		fallbackDesc = " → fallback=mock(offline)"
	}

	ceiling := governor.DefaultDailyCeilingMicrocents

	// Optional primary call-rate cap (M106): AGEZT_RATE_PER_MIN=<n> bounds how
	// many completion calls the PRIMARY governor admits per minute (tenants have
	// AGEZT_TENANT_RATE_PER_MIN). 0 / unset = unlimited. A throttled call is
	// journaled as rate.limited and surfaced by `agt ratelimit log`. Malformed =
	// hard startup error (fast feedback, mirrors the other numeric knobs).
	ratePerMin := 0
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "RATE_PER_MIN")); spec != "" {
		n, perr := strconv.Atoi(spec)
		if perr != nil || n < 0 {
			return nil, "", "", fmt.Errorf("AGEZT_RATE_PER_MIN: want a non-negative integer, got %q", spec)
		}
		ratePerMin = n
	}

	// Optional per-task-type routing override (M1.cc). Operators set
	// AGEZT_TASK_ROUTES="plan=anthropic;code=anthropic,openai;..." to
	// pin specific task types to specific providers. Unrecognised
	// provider names degrade silently to the default chain (see the
	// TaskRoutes doc), so a typo doesn't take down the daemon — but
	// a syntactically-malformed entry IS a hard startup error so the
	// operator gets fast feedback instead of silent misrouting.
	var taskRoutes governor.TaskRoutes
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TASK_ROUTES")); spec != "" {
		parsed, err := governor.ParseTaskRoutesEnv(spec)
		if err != nil {
			return nil, "", "", fmt.Errorf("AGEZT_TASK_ROUTES: %w", err)
		}
		taskRoutes = parsed
	}
	// Hard-pin routes (M1.kk). Same env-var syntax; restrictive
	// rather than preferential semantics.
	var taskRequires governor.TaskRouteRequires
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TASK_ROUTE_REQUIRES")); spec != "" {
		parsed, err := governor.ParseTaskRoutesEnv(spec)
		if err != nil {
			return nil, "", "", fmt.Errorf("AGEZT_TASK_ROUTE_REQUIRES: %w", err)
		}
		taskRequires = governor.TaskRouteRequires(parsed)
	}

	// Per-task-type model override (M1.ll).
	var taskModels governor.TaskModelOverrides
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TASK_MODEL_OVERRIDES")); spec != "" {
		parsed, err := governor.ParseTaskModelOverridesEnv(spec)
		if err != nil {
			return nil, "", "", fmt.Errorf("AGEZT_TASK_MODEL_OVERRIDES: %w", err)
		}
		taskModels = parsed
	}

	// Per-task-type daily budget caps (M1.zz). Layered on top of
	// DAILY_CEILING; both must pass for a call to proceed.
	var taskBudgets map[string]int64
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TASK_BUDGETS")); spec != "" {
		parsed, err := governor.ParseTaskBudgetsEnv(spec)
		if err != nil {
			return nil, "", "", fmt.Errorf("AGEZT_TASK_BUDGETS: %w", err)
		}
		taskBudgets = parsed
	}

	// Model capability gate (M25). Opt-in via AGEZT_MODEL_STRICT=on: a
	// tools-bearing request to a catalog-known model that lacks tool-use is
	// rejected pre-flight instead of failing deep in the provider call. The
	// catalog backs the lookup; per-tenant governors inherit it via
	// WithLimits (the whole Config is copied).
	strictCaps := strings.EqualFold(os.Getenv(brand.EnvPrefix+"MODEL_STRICT"), "on")
	// Strict pricing (M193/M194). Opt-in via AGEZT_PRICING_STRICT=on: a request
	// for a model with no known price is refused BEFORE any provider call rather
	// than charged $0 (which would silently bypass the daily/task budget).
	// Known-free models (local/mock) still pass. Off by default.
	strictPricing := strings.EqualFold(os.Getenv(brand.EnvPrefix+"PRICING_STRICT"), "on")
	// Capability down-routing (M37). Opt-in via AGEZT_MODEL_DOWNROUTE=on: a
	// tools-bearing request to a tool-incapable model is remapped to a
	// tool-capable sibling in the same provider instead of being rejected
	// (M25). Pairs naturally with strict mode (reroute-if-possible, else
	// reject), but works independently too.
	// AGEZT_MODEL_DOWNROUTE_CROSS=on widens the substitute search to OTHER
	// registered+credentialed providers when the model's own provider has no
	// tool-capable sibling (M40). It implies down-routing. Without it, the
	// search stays same-provider only (M37).
	crossDownRoute := strings.EqualFold(os.Getenv(brand.EnvPrefix+"MODEL_DOWNROUTE_CROSS"), "on")
	downRoute := crossDownRoute || strings.EqualFold(os.Getenv(brand.EnvPrefix+"MODEL_DOWNROUTE"), "on")

	// The alternative finder: same-provider by default, cross-provider (among
	// the actually-registered providers) when enabled.
	altFinder := cat.ToolCapableAlternative
	if crossDownRoute {
		altFinder = func(model string) (string, bool) {
			return cat.ToolCapableAlternativeAmong(model, func(provID string) bool { return registered[provID] })
		}
	}

	gov, err := governor.New(governor.Config{
		Registry:                reg,
		DailyCeilingMicrocents:  ceiling,
		RateLimitPerMin:         ratePerMin,
		TaskRoutes:              taskRoutes,
		TaskRouteRequires:       taskRequires,
		TaskModelOverrides:      taskModels,
		TaskBudgets:             taskBudgets,
		StrictModelCapabilities: strictCaps,
		StrictPricing:           strictPricing,
		DownRouteToolModels:     downRoute,
		ModelToolCapable: func(model string) (bool, bool) {
			_, m := cat.FindModel(model)
			if m == nil {
				return false, false
			}
			return m.ToolCall, true
		},
		ToolCapableAlternative: altFinder,
		ModelJSONNative: func(model string) (bool, bool) {
			p, m := cat.FindModel(model)
			if p == nil || m == nil {
				return false, false
			}
			return catalog.FamilySupportsNativeJSONMode(p.Family()), true
		},
	})
	if err != nil {
		return nil, "", "", err
	}
	desc := fmt.Sprintf("primary=%s%s, daily_ceiling=$%.2f",
		primaryDesc, fallbackDesc, float64(ceiling)/1e9)
	if strictCaps {
		desc += ", strict-capabilities"
	}
	if downRoute {
		if crossDownRoute {
			desc += ", tool-downrouting(cross)"
		} else {
			desc += ", tool-downrouting"
		}
	}
	if extraProviders > 0 {
		desc += fmt.Sprintf(", model-routable_alternates=%d", extraProviders)
	}
	if len(taskRoutes) > 0 {
		desc += fmt.Sprintf(", task_routes=%d", len(taskRoutes))
	}
	if len(taskBudgets) > 0 {
		desc += fmt.Sprintf(", task_budgets=%d", len(taskBudgets))
	}
	return gov, desc, model, nil
}

// catalogModelIDs returns the sorted model ids the catalog lists for the given
// provider id, used to populate ProviderInfo.Models for per-request routing.
// Returns nil when the catalog or entry is absent (e.g. the mock primary).
func catalogModelIDs(cat *catalog.Catalog, providerID string) []string {
	if cat == nil {
		return nil
	}
	entry, ok := cat.Providers[providerID]
	if !ok || len(entry.Models) == 0 {
		return nil
	}
	ids := make([]string, 0, len(entry.Models))
	for m := range entry.Models {
		ids = append(ids, m)
	}
	sort.Strings(ids)
	return ids
}

// selectPrimary returns the primary provider, a banner description,
// the resolved model id, the auth-mode tag for the Governor's
// registry, and an error.
//
// Selection precedence (M1.g):
//
//  1. AGEZT_PROVIDER=mock        → fixture (bypasses catalog).
//  2. AGEZT_PROVIDER=<catalog id> → look up in cat; compat.Build it.
//  3. AGEZT_PROVIDER unset       → auto-pick: first catalog provider
//     that (a) is in a compat-supported
//     family and (b) has credentials.
//     If none, fall back to mock with a
//     stderr note so the operator knows
//     the catalog wasn't usable.
//
// The model id within a chosen provider comes from AGEZT_MODEL when
// set; otherwise compat.FirstModelID picks the alphabetically-first
// model in the catalog entry.
func selectPrimary(cat *catalog.Catalog, lookup func(string) string) (agent.Provider, string, string, governor.AuthMode, error) {
	// AGEZT_PROVIDER and AGEZT_MODEL are *config*, not credentials —
	// always read from env directly (operators may want a one-off
	// override that doesn't sit in the vault).
	want := strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "PROVIDER")))
	modelOverride := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "MODEL"))

	// 1. Explicit mock — fixture, bypasses catalog entirely.
	if want == "mock" {
		p := newDemoMock()
		return p, "mock(offline; scripted shell+final)", "mock", governor.AuthLocal, nil
	}

	// 2. Explicit catalog id.
	if want != "" {
		entry, ok := cat.Providers[want]
		if !ok {
			return nil, "", "", "", fmt.Errorf(
				"%sPROVIDER=%q not in catalog; run `agt catalog sync` then `agt catalog list`",
				brand.EnvPrefix, want)
		}
		return buildFromCatalog(entry, modelOverride, lookup)
	}

	// 3. Auto-pick: walk the catalog, take the first supported +
	//    credentialed entry. Deterministic via ProviderList()'s sort.
	//    HasCredentials uses the chained lookup so vault entries count
	//    alongside env vars.
	for _, entry := range cat.ProviderList() {
		if !compat.IsSupportedFamily(entry.Family()) {
			continue
		}
		if !entry.HasCredentials(lookup) {
			continue
		}
		return buildFromCatalog(entry, modelOverride, lookup)
	}

	// Nothing usable — degrade to offline mock so the daemon still
	// starts. The banner will surface the fallback.
	p := newDemoMock()
	return p, "mock(offline; auto-picked because catalog had no credentialed+supported provider — run `agt catalog sync` and set credentials)",
		"mock", governor.AuthLocal, nil
}

// buildFromCatalog finalises a catalog entry into a wire Provider.
// Shared by both the explicit-id path and the auto-pick path.
// `lookup` is the chained vault+env credential resolver from runDaemon.
func buildFromCatalog(entry *catalog.Provider, modelOverride string, lookup func(string) string) (agent.Provider, string, string, governor.AuthMode, error) {
	modelID := modelOverride
	if modelID == "" {
		modelID = compat.FirstModelID(entry)
	}
	if modelID == "" {
		return nil, "", "", "", fmt.Errorf("provider %q in catalog has no models; set %sMODEL", entry.ID, brand.EnvPrefix)
	}
	prov, _, err := compat.Build(entry, modelID, lookup)
	if err != nil {
		return nil, "", "", "", err
	}
	auth := governor.AuthAPIKey
	if len(entry.Env) == 0 {
		auth = governor.AuthLocal
	}
	desc := fmt.Sprintf("%s(catalog; family=%s, model=%s)", entry.ID, entry.Family(), modelID)
	return prov, desc, modelID, auth, nil
}

// wireNetguardAudit points the http/browser tools' egress-guard OnBlock at the
// kernel bus, so a refused dial (SSRF / metadata attempt) is journaled as a
// netguard.blocked event (M109). Called after the kernel exists because the
// tools are built earlier; a nil bus or a missing tool is a harmless no-op.
func wireNetguardAudit(tools map[string]agent.Tool, b *bus.Bus) {
	if b == nil {
		return
	}
	publish := func(tool string) func(ip, reason string) {
		return func(ip, reason string) {
			_, _ = b.Publish(event.Spec{
				Subject: "netguard.block",
				Kind:    event.KindNetguardBlocked,
				Actor:   tool,
				Payload: map[string]any{"ip": ip, "reason": reason, "tool": tool},
			})
		}
	}
	if ht, ok := tools["http"].(*httptool.Tool); ok {
		ht.OnBlock = publish("http")
	}
	if br, ok := tools["browser"].(*browser.Tool); ok {
		br.OnBlock = publish("browser")
	}
}

// pluginLogLine formats a plugin's stderr line for the daemon log, scrubbing
// any secret of a known format it may have printed (M229). A third-party
// plugin's stderr is untrusted output that lands directly in the operator's
// logs — a path the bus redactor (which only covers journaled events) does not
// touch. Pattern-based redaction is the right fit here: a plugin leaks its OWN
// secrets, which the daemon doesn't hold as literals but whose formats (sk-,
// Telegram, Groq, …) the built-in detectors catch.
func pluginLogLine(r *redact.Redactor, prefix, line string) string {
	return fmt.Sprintf("[plugin:%s] %s", prefix, r.Redact(line))
}

// buildTools registers the in-process tools. Each tool gets its own
// configuration from env vars; defaults are safe (file tool scoped to a
// per-instance workspace, http tool default-deny). The shell tool runs
// every command through the supplied Warden engine.
func buildTools(baseDir string, stderr io.Writer, ward warden.Engine) (map[string]agent.Tool, []kernelruntime.PluginInfo, string, error) {
	out := map[string]agent.Tool{}
	var registered []string
	// Manifest of external plugins that successfully spawned.
	// Surfaced to the kernel via Config.Plugins so the control
	// plane can serve `agt plugin list`. Stays nil when no
	// AGEZT_PLUGINS entries are configured.
	var manifestEntries []kernelruntime.PluginInfo

	// shell — always registered, routed through Warden. Effective
	// isolation profile depends on host OS (M1.c: always ProfileNone
	// with the request journaled as a downgrade on non-Linux).
	out["shell"] = shell.NewWithWarden(ward)
	registered = append(registered, "shell(warden=requested-namespace)")

	// file — scoped to $AGEZT_WORKSPACE (default <baseDir>/workspace).
	wsRoot := os.Getenv(brand.EnvPrefix + "WORKSPACE")
	if wsRoot == "" {
		wsRoot = filepath.Join(baseDir, "workspace")
	}
	ft, err := filetool.New(wsRoot)
	if err != nil {
		return nil, nil, "", fmt.Errorf("file tool: %w", err)
	}
	out["file"] = ft
	registered = append(registered, "file(root="+ft.Root()+")")

	// http — default-deny; allowed hosts via $AGEZT_HTTP_ALLOWED_HOSTS
	// (comma-separated). $AGEZT_HTTP_ALLOW_ALL=1 bypasses (DANGEROUS).
	ht := httptool.New()
	if hostsCSV := os.Getenv(brand.EnvPrefix + "HTTP_ALLOWED_HOSTS"); hostsCSV != "" {
		for h := range strings.SplitSeq(hostsCSV, ",") {
			if h = strings.TrimSpace(h); h != "" {
				ht.AllowedHosts = append(ht.AllowedHosts, h)
			}
		}
	}
	if os.Getenv(brand.EnvPrefix+"HTTP_ALLOW_ALL") == "1" {
		ht.AllowAll = true
		fmt.Fprintln(stderr, "WARNING: AGEZT_HTTP_ALLOW_ALL=1 disables the http host allowlist.")
	}
	// Egress guard (M16): by default the http tool refuses internal/metadata
	// addresses even for allowlisted/AllowAll hosts. Relax per range for local use.
	egress := "guarded"
	if os.Getenv(brand.EnvPrefix+"HTTP_ALLOW_LOOPBACK") == "1" {
		ht.AllowLoopback = true
		egress = "loopback-ok"
	}
	if os.Getenv(brand.EnvPrefix+"HTTP_ALLOW_PRIVATE") == "1" {
		ht.AllowPrivate = true
		if egress == "loopback-ok" {
			egress = "loopback+private-ok"
		} else {
			egress = "private-ok"
		}
		fmt.Fprintln(stderr, "WARNING: AGEZT_HTTP_ALLOW_PRIVATE=1 lets the http tool reach the private network.")
	}
	out["http"] = ht
	if ht.AllowAll {
		registered = append(registered, fmt.Sprintf("http(allow_all=true, egress=%s)", egress))
	} else {
		registered = append(registered, fmt.Sprintf("http(hosts=%d, egress=%s)", len(ht.AllowedHosts), egress))
	}

	// browser.read — same allowlist pattern as http (uses AGEZT_BROWSER_*
	// env vars; deliberately separate from http's allowlist so operators
	// can grant browser-read access to a wider domain set than POSTs).
	br := browser.New()
	if hostsCSV := os.Getenv(brand.EnvPrefix + "BROWSER_ALLOWED_HOSTS"); hostsCSV != "" {
		for h := range strings.SplitSeq(hostsCSV, ",") {
			if h = strings.TrimSpace(h); h != "" {
				br.AllowedHosts = append(br.AllowedHosts, h)
			}
		}
	}
	if os.Getenv(brand.EnvPrefix+"BROWSER_ALLOW_ALL") == "1" {
		br.AllowAll = true
		fmt.Fprintln(stderr, "WARNING: AGEZT_BROWSER_ALLOW_ALL=1 disables the browser host allowlist.")
	}
	// Egress guard (M16): browser.read refuses internal/metadata addresses by
	// default, even for allowlisted/AllowAll hosts. Relax per range for local use.
	if os.Getenv(brand.EnvPrefix+"BROWSER_ALLOW_LOOPBACK") == "1" {
		br.AllowLoopback = true
	}
	if os.Getenv(brand.EnvPrefix+"BROWSER_ALLOW_PRIVATE") == "1" {
		br.AllowPrivate = true
		fmt.Fprintln(stderr, "WARNING: AGEZT_BROWSER_ALLOW_PRIVATE=1 lets browser.read reach the private network.")
	}
	// Browser cookies (M1.mm) — off by default; operator opts in
	// when they need session-following reads. In-memory jar; lost
	// on daemon restart.
	if os.Getenv(brand.EnvPrefix+"BROWSER_COOKIES") == "1" {
		if err := br.EnableCookies(); err != nil {
			fmt.Fprintf(stderr, "WARNING: AGEZT_BROWSER_COOKIES=1 but jar init failed: %v\n", err)
		}
	}
	out["browser.read"] = br
	if br.AllowAll {
		registered = append(registered, "browser.read(allow_all=true)")
	} else {
		registered = append(registered, fmt.Sprintf("browser.read(hosts=%d)", len(br.AllowedHosts)))
	}

	// coding — external coding-agent bridge (P6-CODE). Registered only when
	// AGEZT_CODING_CMD is set (the command that runs Claude Code / Codex / Aider
	// / any agent). It operates on a git worktree of the workspace and returns a
	// diff; it never merges. Off by default.
	if codingCmd := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "CODING_CMD")); codingCmd != "" {
		if ct := coding.New(codingCmd, coding.AbsRepo(wsRoot)); ct != nil {
			out["coding"] = ct
			registered = append(registered, "coding(external agent)")
		}
	}

	// acp_agent — external ACP-agent bridge (SPEC-15 §3, the inverse of `agt
	// acp`). Registered only when AGEZT_ACP_AGENT_CMD is set (the command that
	// launches an external agent speaking the Agent Client Protocol over stdio,
	// e.g. `claude-code-acp` or `codex acp`). It drives that agent over JSON-RPC
	// and relays its answer. Off by default.
	if acpCmd := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ACP_AGENT_CMD")); acpCmd != "" {
		if at := acpagent.New(acpCmd, acpagent.AbsCwd(wsRoot)); at != nil {
			out["acp_agent"] = at
			registered = append(registered, "acp_agent(external agent)")
		}
	}

	// remote_run — mesh delegation to a peer Agezt node over its REST API (M8).
	// Registered only when AGEZT_PEERS is set (name=url|token,…). A malformed
	// spec is a hard startup error so a misconfigured mesh is caught early.
	{
		peerSpec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "PEERS"))
		tenantSpec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TENANT_PEERS"))
		if peerSpec != "" || tenantSpec != "" {
			peers, err := peer.ParsePeers(peerSpec)
			if err != nil {
				return nil, nil, "", fmt.Errorf("AGEZT_PEERS: %w", err)
			}
			// Per-tenant peer sets (M219): a tenant's runs route against its own peers,
			// falling back to the global set. Parsed/validated up front like AGEZT_PEERS.
			tenantPeers, terr := peer.ParseTenantPeers(tenantSpec)
			if terr != nil {
				return nil, nil, "", fmt.Errorf("AGEZT_TENANT_PEERS: %w", terr)
			}
			if pt := peer.NewWithTenants(peers, tenantPeers); pt != nil {
				out["remote_run"] = pt
				if len(tenantPeers) > 0 {
					registered = append(registered, fmt.Sprintf("remote_run(%d peer(s), %d tenant override(s))", len(peers), len(tenantPeers)))
				} else {
					registered = append(registered, fmt.Sprintf("remote_run(%d peer(s))", len(peers)))
				}
			}
		}
	}

	// External plugins via AGEZT_PLUGINS env var (M1.y). Format:
	//   AGEZT_PLUGINS="<prefix>=<path> <args...>"[,...]
	// e.g. AGEZT_PLUGINS="search=/usr/local/bin/agezt-search,scrape=/opt/scraper"
	// Each plugin is spawned at daemon start; its tools register
	// under the given prefix. A plugin that fails to initialize is
	// logged to stderr and skipped; the daemon continues with
	// non-plugin tools so a broken plugin can't take down the
	// kernel.
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "PLUGINS")); spec != "" {
		// Parse pin spec first (M1.ff). A syntactically-bad pin is a
		// hard startup error — operators want fast feedback on a
		// security setting, not a silently-broken pin that lets a
		// modified binary slip through next reboot. Unknown pins
		// (for plugins not in AGEZT_PLUGINS) become warnings after
		// the plugin loop runs.
		var pins plugin.PinSpec
		if pinSpec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "PLUGIN_PINS")); pinSpec != "" {
			parsed, err := plugin.ParsePinSpec(pinSpec)
			if err != nil {
				return nil, nil, "", fmt.Errorf("AGEZT_PLUGIN_PINS: %w", err)
			}
			pins = parsed
		}
		// Tool allowlist (M1.hh) — same hard-error semantics as pins.
		var allowedTools plugin.ToolAllowlistSpec
		if allowSpec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "PLUGIN_TOOLS")); allowSpec != "" {
			parsed, err := plugin.ParseToolAllowlistSpec(allowSpec)
			if err != nil {
				return nil, nil, "", fmt.Errorf("AGEZT_PLUGIN_TOOLS: %w", err)
			}
			allowedTools = parsed
		}
		// Parse the spec up front (M223). A malformed entry — missing
		// '=', empty path, or a duplicate prefix — is a hard startup
		// error, matching the pin/allowlist specs parsed just above. A
		// repeated prefix used to spawn two processes whose tools then
		// collided with a misleading "in-process version" warning;
		// rejecting it surfaces the typo instead.
		entries, err := plugin.ParsePluginSpec(spec)
		if err != nil {
			return nil, nil, "", fmt.Errorf("AGEZT_PLUGINS: %w", err)
		}
		usedPrefixes := make([]string, 0, len(entries))
		// Scrub secrets of known formats from plugin stderr before it reaches the
		// daemon log (M229). Pattern-only (no literals) — a plugin leaks its own
		// secrets, not the daemon's.
		pluginRedactor := redact.New()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for _, e := range entries {
			prefix := e.Prefix
			usedPrefixes = append(usedPrefixes, prefix)
			cfg := plugin.Config{
				Path: e.Path,
				Args: e.Args,
				Logger: func(line string) {
					fmt.Fprintln(stderr, pluginLogLine(pluginRedactor, prefix, line))
				},
				PinnedHash:   pins[prefix],         // empty if no pin configured for this prefix
				AllowedTools: allowedTools[prefix], // nil if no allowlist for this prefix
			}
			p, err := plugin.Spawn(ctx, cfg)
			if err != nil {
				fmt.Fprintf(stderr, "WARNING: plugin %q (%s) failed to start: %v\n", prefix, e.Path, err)
				continue
			}
			pluginTools := p.Tools(prefix + ".")
			loadedCount := 0
			for name, tool := range pluginTools {
				if _, conflict := out[name]; conflict {
					fmt.Fprintf(stderr, "WARNING: plugin %q tool %q conflicts with existing tool — keeping in-process version\n", prefix, name)
					continue
				}
				out[name] = tool
				loadedCount++
			}
			registered = append(registered, fmt.Sprintf("plugin:%s(%d tools)", prefix, len(pluginTools)))
			// Record manifest entry. tool_count is the post-conflict
			// count — what the model actually sees — not the raw
			// plugin advertisement, so the operator can spot when a
			// conflict shadowed a tool they expected.
			manifestEntries = append(manifestEntries, kernelruntime.PluginInfo{
				Prefix:       prefix,
				Path:         e.Path,
				Args:         append([]string(nil), e.Args...),
				ToolCount:    loadedCount,
				HashPinned:   pins[prefix] != "",
				AllowedTools: append([]string(nil), allowedTools[prefix]...),
			})
		}
		// Warn about pin entries that didn't match any spawned plugin
		// (typo, removed plugin, etc.) — surfaces stale config without
		// failing the daemon.
		for _, stale := range pins.UnusedPins(usedPrefixes) {
			fmt.Fprintf(stderr, "WARNING: AGEZT_PLUGIN_PINS has entry for %q but no plugin with that prefix was loaded\n", stale)
		}
		for _, stale := range allowedTools.Unused(usedPrefixes) {
			fmt.Fprintf(stderr, "WARNING: AGEZT_PLUGIN_TOOLS has entry for %q but no plugin with that prefix was loaded\n", stale)
		}
	}

	return out, manifestEntries, strings.Join(registered, ", "), nil
}

// newDemoMock returns a Provider scripted with the canonical M0.5 demo:
//
//  1. Round 1: assistant requests `shell` with a directory-listing command.
//  2. Round 2: assistant returns a final text answer that mentions the
//     project (the real LLM would synthesise this from the tool output;
//     the mock just acknowledges the loop completed).
//
// Deterministic; satisfies the demo gate `agt run "list the files here and
// tell me what this project is"` end-to-end with no external services.
// injectDemoVisionModel adds a synthetic vision-capable "mock" catalog entry
// (M93 demo) so the offline mock model passes the M91 vision gate. Production
// catalogs are untouched; this only fires under AGEZT_DEMO_VISION=1.
func injectDemoVisionModel(cat *catalog.Catalog) {
	if cat == nil {
		return
	}
	if cat.Providers == nil {
		cat.Providers = map[string]*catalog.Provider{}
	}
	cat.Providers["mock"] = &catalog.Provider{
		ID:   "mock",
		Name: "Mock (demo vision)",
		Models: map[string]*catalog.Model{
			"mock": {
				ID:         "mock",
				Name:       "Mock Vision (demo)",
				Modalities: catalog.Modalities{Input: []string{"text", "image"}, Output: []string{"text"}},
			},
		},
	}
}

func newDemoMock() agent.Provider {
	// Demo escape hatch: AGEZT_DEMO_VISION=1 returns a mock that reflects its
	// input — it reports how many image attachments the user message carried
	// (M93), so the vision-input path (agt run --image on a vision-capable
	// model) is observable end-to-end offline. Pairs with injectDemoVisionModel,
	// which makes the "mock" model pass the M91 vision gate.
	if os.Getenv(brand.EnvPrefix+"DEMO_VISION") == "1" {
		return &mock.Provider{Responder: func(req agent.CompletionRequest) agent.CompletionResponse {
			n := 0
			for _, m := range req.Messages {
				if m.Role == agent.RoleUser {
					n = len(m.Images)
				}
			}
			return mock.FinalText(fmt.Sprintf(
				"[offline-mock vision] received %d image attachment(s); a real vision model would describe them here.", n))
		}}
	}
	// Demo escape hatch: AGEZT_DEMO_SSRF=1 scripts the lead to fetch the cloud
	// metadata endpoint via the http tool, so the egress-guard block + the M109
	// netguard.blocked audit are observable offline. Pair with
	// AGEZT_HTTP_ALLOW_ALL=1 so the HOST allowlist passes and the IP guard (not
	// the allowlist) is what refuses the dial.
	if os.Getenv(brand.EnvPrefix+"DEMO_SSRF") == "1" {
		return mock.New(
			mock.ToolUse("call-1", "http", map[string]any{"method": "GET", "url": "http://169.254.169.254/latest/meta-data/"}),
			mock.FinalText("[offline-mock] I attempted to read the cloud metadata endpoint; the egress guard refused the connection."),
		)
	}
	// Demo escape hatch: AGEZT_DEMO_LOOP=1 returns a mock that ALWAYS requests
	// the same shell call, so the M116 loop guard is observable: the call runs at
	// most MaxIdenticalToolCalls times, then the loop refuses further executions
	// (the run still exhausts MaxIter since the mock never adapts).
	if os.Getenv(brand.EnvPrefix+"DEMO_LOOP") == "1" {
		return &mock.Provider{Responder: func(agent.CompletionRequest) agent.CompletionResponse {
			return mock.ToolUse("call-loop", "shell", map[string]any{"command": "true"})
		}}
	}
	// Demo escape hatch: AGEZT_DEMO_FILE_EDIT=1 scripts the lead to write a file
	// then SURGICALLY edit it with the file tool's replace op (M114), so the
	// partial-edit path is observable end-to-end offline.
	if os.Getenv(brand.EnvPrefix+"DEMO_FILE_EDIT") == "1" {
		return mock.New(
			mock.ToolUse("call-1", "file", map[string]any{"op": "write", "path": "notes.txt", "content": "status = draft\nowner = nobody\n"}),
			mock.ToolUse("call-2", "file", map[string]any{"op": "replace", "path": "notes.txt", "find": "draft", "replacement": "published"}),
			mock.FinalText("[offline-mock] I wrote notes.txt and edited it in place: status is now 'published'."),
		)
	}
	// Demo escape hatch: AGEZT_DEMO_CACHED=1 scripts one answer carrying synthetic
	// usage where most of the prompt was prompt-cached (M289) on a priced model
	// whose catalog entry has a cache_read price — so the Governor's cache-aware
	// billing is observable offline: budget.consumed shows cached_input_tokens and
	// a cost lower than charging every input token at the full rate.
	if os.Getenv(brand.EnvPrefix+"DEMO_CACHED") == "1" {
		return mock.New(mock.WithUsage(
			mock.FinalText("[offline-mock] answered with a mostly-cached prompt."),
			agent.Usage{InputTokens: 10000, CachedInputTokens: 9000, CacheWriteInputTokens: 500, OutputTokens: 200, Model: "claude-sonnet-4-6"},
		))
	}
	// Demo escape hatch: AGEZT_DEMO_ECHO=1 makes the mock ECHO the last user
	// message back as the answer, so the exact intent the agent received is
	// observable — used to prove the M144 channel-conversation transcript actually
	// reaches the loop. Network-free; no scripted list.
	if os.Getenv(brand.EnvPrefix+"DEMO_ECHO") == "1" {
		p := mock.New()
		p.Responder = func(req agent.CompletionRequest) agent.CompletionResponse {
			last := ""
			for _, m := range req.Messages {
				if m.Role == agent.RoleUser {
					last = m.Content
				}
			}
			return mock.FinalText("[echo]\n" + last)
		}
		return p
	}
	// Demo escape hatch: AGEZT_DEMO_NOTIFY=1 scripts the agent to call the `notify`
	// tool mid-run (M143) so the proactive-messaging path (agent → configured
	// channel allowlist → channel.outbound) is observable offline. Requires a
	// channel with an allowlist configured for the tool to be registered.
	if os.Getenv(brand.EnvPrefix+"DEMO_NOTIFY") == "1" {
		return mock.New(
			mock.ToolUse("call-1", "notify", map[string]any{"text": "Starting the long task — I'll report back when it's done."}),
			mock.FinalText("[offline-mock] I pinged you over the configured channel, then finished the task."),
		)
	}
	// Demo escape hatch: AGEZT_DEMO_DELEGATE=1 scripts a single delegation so
	// the multi-agent path (the `delegate` tool, subagent.spawned, M41 run
	// links) is observable from `agt run` with no external services. The lead
	// delegates once; the sub-agent answers; the lead finalises. The mock
	// replays responses sequentially across the lead+child Complete calls
	// (lead-r1 → child-r1 → lead-r2).
	if v := os.Getenv(brand.EnvPrefix + "DEMO_DELEGATE"); v == "1" {
		return mock.New(
			mock.ToolUse("call-1", "delegate", map[string]any{"task": "summarize the kernel package layout"}),
			mock.FinalText("[offline-mock sub-agent] kernel/ holds event, journal, bus, agent, runtime, and controlplane."),
			mock.FinalText("[offline-mock lead] I delegated the kernel-layout summary to a sub-agent; it reported the core packages."),
		)
	} else if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 2 {
		// AGEZT_DEMO_DELEGATE=N (N≥2) scripts the lead attempting N delegations
		// in N rounds so the M46 fan-out cap is observable: run with
		// AGEZT_SUBAGENT_FANOUT=N-1 and the final attempt is refused (a tool
		// error the lead reports), while N-1 sub-agents spawn. The script feeds
		// N-1 child answers then the lead's final — consumed in call order
		// (lead-rk → child-k), so the refused Nth call falls straight through to
		// the lead's final response.
		//
		// Each response carries synthetic token usage on a priced model so the
		// Governor journals a non-zero budget.consumed per call (M47) — the
		// lead's calls under the lead correlation, each child's under its own —
		// making per-run / per-delegation spend visible in `agt runs stats`.
		withUsage := func(r agent.CompletionResponse) agent.CompletionResponse {
			return mock.WithUsage(r, agent.Usage{InputTokens: 2000, OutputTokens: 1000, Model: "claude-sonnet-4-6"})
		}
		resp := make([]agent.CompletionResponse, 0, 2*n)
		for i := 1; i < n; i++ {
			resp = append(resp,
				withUsage(mock.ToolUse(fmt.Sprintf("call-%d", i), "delegate", map[string]any{"task": fmt.Sprintf("subtask %d", i)})),
				withUsage(mock.FinalText(fmt.Sprintf("[offline-mock sub-agent %d] done.", i))),
			)
		}
		resp = append(resp,
			withUsage(mock.ToolUse(fmt.Sprintf("call-%d", n), "delegate", map[string]any{"task": fmt.Sprintf("subtask %d", n)})),
			withUsage(mock.FinalText(fmt.Sprintf("[offline-mock lead] spawned %d sub-agent(s); the fan-out cap refused the rest.", n-1))),
		)
		return mock.New(resp...)
	}
	listCmd := "ls -la"
	if runtime.GOOS == "windows" {
		listCmd = "dir"
	}
	return mock.New(
		mock.ToolUse("call-1", "shell", map[string]string{"command": listCmd}),
		mock.FinalText(
			"[offline-mock] I ran a directory listing via the shell tool. This project is "+
				brand.Name+" — an open-source, MIT-licensed agentic operating system written in Go. "+
				"The M0.5 foundation under kernel/ (event, journal, state, bus, agent, runtime, "+
				"controlplane) plus the in-process plugins under plugins/ are what just executed this run; "+
				"every step you saw was journaled and BLAKE3-chained.",
		),
	)
}

// selectAskPolicy maps AGEZT_APPROVAL_MODE to an edict.AskPolicy.
//
//	allow  (default) — Ask-class levels fold to Allow + WouldAsk=true;
//	                   journal stays honest, no operator interaction.
//	deny             — Ask-class levels fold to Deny; strict mode.
//	prompt           — Ask-class levels block the agent until an operator
//	                   runs `agt approve <id>` or `agt deny <id>`.
//
// Returns the policy and a banner-friendly description.
func selectAskPolicy() (edict.AskPolicy, string) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "APPROVAL_MODE"))) {
	case "deny":
		return edict.AskDeny, "AskDeny (strict; only L4 calls run)"
	case "prompt":
		return edict.AskPrompt, "AskPrompt (live HITL via `agt approve|deny`)"
	case "", "allow":
		return edict.AskAllow, "AskAllow (Ask-class folded to Allow + WouldAsk)"
	default:
		// Unknown values fall back to the safe default; surface the
		// fact in the banner so the operator notices the typo.
		return edict.AskAllow, fmt.Sprintf("AskAllow (unknown %sAPPROVAL_MODE=%q ignored)",
			brand.EnvPrefix, os.Getenv(brand.EnvPrefix+"APPROVAL_MODE"))
	}
}

// alwaysFailProvider is the demo shim used by AGEZT_DEMO_FAIL_PRIMARY=1
// to force the Governor's fallback chain to engage on every call. The
// returned error is non-cancel/non-budget so shouldFallback returns true.
type alwaysFailProvider struct{ name string }

func (p *alwaysFailProvider) Name() string { return p.name }
func (p *alwaysFailProvider) Complete(ctx context.Context, _ agent.CompletionRequest) (*agent.CompletionResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("demo-shim: simulated primary failure")
}

// keep import honest
var _ = event.GenesisHash

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
// Provider selection is configured through the Web UI / Config Center and
// encrypted credential vault. A fresh daemon starts unconfigured, serves the
// Setup screen, and only needs a provider key/model before LLM runs can execute.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/agezt/agezt/cmd/agezt/internal/daemonconfig"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/alerter"
	"github.com/agezt/agezt/kernel/anomaly"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/cadence/systemtasks"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/channelwire"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/market"
	kernelmemory "github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/pulse"
	"github.com/agezt/agezt/kernel/redact"
	"github.com/agezt/agezt/kernel/resume"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/selfrepair"
	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/kernel/state"
	"github.com/agezt/agezt/kernel/stt"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/kernel/toolreg"
	"github.com/agezt/agezt/kernel/ulid"
	"github.com/agezt/agezt/kernel/update"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/kernel/workflow"
	"github.com/agezt/agezt/plugins/builtinchannels"
	"github.com/agezt/agezt/plugins/builtinguardians"
	"github.com/agezt/agezt/plugins/builtinmarket"
	"github.com/agezt/agezt/plugins/builtinskills"
	"github.com/agezt/agezt/plugins/providerboot"
	"github.com/agezt/agezt/plugins/providers/embed"
	"github.com/agezt/agezt/plugins/providers/image"
	"github.com/agezt/agezt/plugins/providers/rerank"
	"github.com/agezt/agezt/plugins/providers/voice"
	"github.com/agezt/agezt/plugins/tools/codeexec"
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
	case "watchdog":
		return runWatchdog(stdout, stderr)
	case "update":
		return runUpdate(stdout, stderr)
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
	fmt.Fprintf(w, "  watchdog  supervise the daemon, restarting it if it exits (self-healing)\n")
	fmt.Fprintf(w, "  update    check for updates or apply a new version (M860)\n")
	fmt.Fprintf(w, "  version   show version and exit\n")
	fmt.Fprintf(w, "  help      show this help\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "Environment:\n")
	fmt.Fprintf(w, "  %sHOME             base directory (default: ~/%s)\n", brand.EnvPrefix, brand.ConfigDir)
	fmt.Fprintf(w, "  ANTHROPIC_API_KEY    required to enable the Anthropic provider\n")
	fmt.Fprintf(w, "  %sPROVIDER         catalog provider id to use; unset = unconfigured (set one in Setup)\n", brand.EnvPrefix)
	fmt.Fprintf(w, "  %sMODEL            model id for runs; unset = resolved from routing/fallback chain (no built-in default)\n", brand.EnvPrefix)
	fmt.Fprintf(w, "  %sSYSTEM_PROMPT    system prompt for every run (optional)\n", brand.EnvPrefix)
}

// runUpdate checks for updates and optionally applies them. When called with no
// arguments it performs a check; `agezt update --apply` triggers a drain-and-swap.
// runUpdate → boot_ops.go

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
	// (Deliberately NOT in daemonconfig.Load: this read happens before
	// injectConfig bridges the config store/vault into the env.)
	if addr, alive := controlplane.ProbeExisting(baseDir); alive {
		if strings.TrimSpace(os.Getenv(brand.EnvPrefix+"FORCE_START")) != "1" {
			fmt.Fprintf(stderr, "%s: a daemon is already running at %s (base dir %s)\n", brand.Binary, addr, baseDir)
			fmt.Fprintf(stderr, "Hint: stop it with `%s shutdown`, or set %sFORCE_START=1 to override.\n", brand.CLI, brand.EnvPrefix)
			return 1
		}
		fmt.Fprintf(stderr, "%s: warning: %sFORCE_START=1 — starting despite a live daemon at %s\n", brand.Binary, brand.EnvPrefix, addr)
	}

	// Load catalog once; share with providerboot.Boot + runtime.Config so
	// the daemon and the kernel see the same snapshot. An empty catalog
	// on disk is fine: provider selection falls through to the unconfigured
	// sentinel and surfaces a hint in the banner.
	catStore := catalog.NewStore(filepath.Join(baseDir, "catalog"))
	// Validate the catalog file loads; the value is reloaded post-seed (below),
	// so only the error matters here.
	_, err = catStore.Load()
	if err != nil {
		fmt.Fprintf(stderr, "%s: catalog load: %v\n", brand.Binary, err)
		return 1
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
	// Machine-bound at-rest encryption (M934): a plaintext vault left over from
	// earlier versions is upgraded in place on boot — every stored key becomes
	// an AES-256-GCM envelope keyed to this machine+user, so a creds.json that
	// leaves the machine (backup, cloud sync, accidental commit) doesn't leak.
	// AGEZT_VAULT_AUTOENCRYPT=off opts out; AGEZT_VAULT_PASSPHRASE still wins.
	credsUpgraded := false
	if up, uerr := credStore.EncryptInPlace(); uerr != nil {
		fmt.Fprintf(stderr, "%s: creds encrypt-in-place: %v (continuing with the plaintext vault)\n", brand.Binary, uerr)
	} else {
		credsUpgraded = up
	}

	// Config Center bridge (M693): inject the config store + AGEZT_* vault secrets
	// into the process environment so the existing os.Getenv consumers (provider,
	// channels, interfaces) read operator edits unchanged. The real environment
	// wins; the store/vault only fill gaps. Must run BEFORE providerboot.Boot +
	// channel construction read the env. configPinned (schema vars set in the real env) is
	// handed to the control plane so the Config Center can show them read-only.
	configPinned := injectConfig(baseDir, credStore, stdout)

	// Typed boot config (Phase 2.5): one Load call parses every post-inject
	// AGEZT_* boot setting — parse shapes, defaults, warn-and-degrade messages,
	// and the historically-fatal cases (which surface here as an error with the
	// same wording and exit). MUST run after injectConfig so the reads see
	// operator edits from the Config Center store + vault. Reads that must stay
	// live at call time (COUNCIL_MEMBERS, DRAIN_TIMEOUT, the tenant lazy-open's
	// EDICT_DURABLE) or that happen pre-inject (FORCE_START) stay inline —
	// see the daemonconfig package comment.
	dcfg, derr := daemonconfig.Load(nil, stderr)
	if derr != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.Binary, derr)
		return 1
	}

	// Make ChatGPT ("Sign in with ChatGPT") discoverable in Models; it only
	// registers as a live provider once the operator signs in.
	providerboot.SeedChatGPTCatalog(catStore)
	cat, _ := catStore.Load()

	// Credential resolution chain (M1.dd):
	//   1. agezt vault (M1.w) — provider-scoped keys first; legacy bare vault
	//      keys only when that env name is unique in the catalog
	//   2. process env — `export FOO=...` stays globally visible
	//   3. ~/.aws/credentials + ~/.aws/config (AWS_PROFILE-aware)
	//   4. EC2 IMDSv2 — instance-role credentials, refreshed on expiry
	// The AWS-specific stages (3-4) answer ONLY the AWS_* names they
	// know about; every other name falls through. Operators on a
	// non-EC2 host pay only a brief, neg-cached IMDS timeout on the
	// first lookup (then nothing for 30s) — the chain remains fast.
	credLookup, awsChainDesc := buildAWSCredChain(catalogScopedVaultLookup(cat, credStore.Lookup))
	credCount := len(credStore.Names())
	atRest := "plaintext (set " + creds.PassphraseEnvVar + ", or unset " + creds.AutoEncryptEnvVar + " on a host with a machine id)"
	switch {
	case credStore.IsEncrypted():
		atRest = "encrypted (AES-256-GCM"
		if credsUpgraded {
			atRest += "; auto-upgraded this boot"
		}
		atRest += ")"
	case credCount == 0 && creds.MachinePassphrase() != "":
		atRest = "empty (will encrypt machine-bound on first save)"
	}
	credDesc := fmt.Sprintf("vault entries=%d at %s — at-rest: %s — %s", credCount, credStore.Path, atRest, awsChainDesc)

	bootRes, err := providerboot.Boot(providerboot.Deps{Catalog: cat, Lookup: credLookup, BaseDir: baseDir, Stderr: stderr})
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.Binary, err)
		return 1
	}
	gov, govDesc, model := bootRes.Governor, bootRes.Desc, bootRes.Model

	// Warden is constructed before the kernel so tools that close over
	// it (shell) can be built before runtime.Open. Bus is attached
	// post-Open via SetBus, same pattern as the Governor.
	wardOpts, wardBackendDesc := wardenOptionsFromEnv()
	ward := warden.NewWithOptions(nil, wardOpts)
	wardDesc := fmt.Sprintf("requested=namespace, effective=%s (M1.c facade; downgrades journaled)%s",
		ward.EffectiveProfile(warden.ProfileNamespace), wardBackendDesc)

	// Edict policy mode: AGEZT_APPROVAL_MODE=allow|deny|prompt
	// (M1.a default: allow; M1.d adds prompt for live HITL).
	askPolicy, askPolicyDesc := selectAskPolicy()
	// Operator-extensible hard-deny rules (M17): AGEZT_EDICT_DENY appends
	// site-specific rules to the built-in set (e.g. "git push;shell:/etc/shadow").
	// Parsed (and rejected when malformed) by daemonconfig.Load above.
	hardDeny := edict.DefaultHardDeny()
	if extra := dcfg.Policy.EdictDeny; len(extra) > 0 {
		hardDeny = append(hardDeny, extra...)
		askPolicyDesc += fmt.Sprintf("; +%d operator deny rule(s)", len(extra))
	}
	edictOpts := edict.Options{AskPolicy: askPolicy, HardDeny: hardDeny}
	// Master permissive switch (M611): AGEZT_ALLOW_ALL=1 sets EVERY governed
	// capability to L4 (allow) so nothing is denied or prompts — a single-operator
	// dev convenience ("default everything allowed, restrict later"). The built-in
	// catastrophe hard-deny rails (fork-bomb, dd-to-raw-device) deliberately stay,
	// since they guard against self-destruction rather than gate normal tools, and
	// are no-ops on Windows anyway. Loud banner so this is never silent.
	permissive := dcfg.Policy.AllowAll
	if permissive {
		lv := make(map[edict.Capability]edict.TrustLevel, len(edict.AllCapabilities()))
		for _, c := range edict.AllCapabilities() {
			lv[c] = edict.LevelAllow
		}
		edictOpts.Levels = lv
		edictOpts.UnknownAllow = true // also allow capabilities not in the known set (M613)
		askPolicyDesc += "; ALLOW_ALL (every capability L4)"
		fmt.Fprintln(stderr, "WARNING: AGEZT_ALLOW_ALL=1 — every capability is set to allow (L4). Not for production; restrict via the Policy view or unset to restore defaults.")
	}
	edictEng := edict.New(edictOpts)

	// Register the built-in channel manifests (Telegram, WhatsApp, …) BEFORE
	// anything reads the channel registry: notifyTargets just below derives its
	// env names from the manifests, the Channels wizard lists them, and
	// collectChannels() derives `agt status`'s configured-channel view from the
	// registry, so it must be populated by the time collectChannels() feeds
	// Deps.Channels at server construction below.
	// (This call used to sit further down, nested inside an
	// `if k.Forge() != nil` block — pure registration has no business being
	// conditional on the Forge.)
	builtinchannels.RegisterAll()

	// Derive the proactive-messaging targets (`notify`/`send_media`, M143)
	// BEFORE buildTools: the registry specs gate themselves on this map
	// (empty ⇒ not registered), so the tool map is complete before the kernel
	// (and its HTTP servers / channels) start and is never written while the
	// agent loop reads it (a fatal concurrent-map race otherwise). The sender
	// needs the live channels (built after the kernel), so the tools are built
	// unbound and wired later by toolSet.ConfigureLate. Env names come from
	// each kind's manifest (RequiredEnv + AllowlistEnv, Phase 2.1 PR 8): a
	// channel kind contributes targets only when its required env AND a
	// non-empty allowlist are set, so a half-configured channel never
	// advertises a tool that can't send. The kind list stays deliberately
	// restricted to the three chat channels the notify/send_media/briefing
	// surface has always targeted.
	notifyTargets := map[string][]string{}
	for _, kind := range []string{"telegram", "slack", "discord"} {
		m, ok := channel.LookupManifest(kind)
		if !ok || m.AllowlistEnv == "" {
			continue
		}
		configured := len(m.RequiredEnv) > 0
		for _, env := range m.RequiredEnv {
			if strings.TrimSpace(os.Getenv(env)) == "" {
				configured = false
				break
			}
		}
		if !configured {
			continue
		}
		if ids := splitNonEmpty(os.Getenv(m.AllowlistEnv)); len(ids) > 0 {
			notifyTargets[kind] = ids
		}
	}

	tools, toolSet, pluginManifest, pluginToolCaps, toolsDesc, err := buildTools(baseDir, stderr, ward, notifyTargets)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.Binary, err)
		return 1
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
	// `credStore`, and rebuilds via providerboot.Reload — the same
	// SelectPrimary → BuildFromCatalog path Boot uses — so the live
	// post-reload registry matches what a fresh boot would have
	// produced for the same on-disk state.
	// Memory-lite (ROADMAP §2.3): on by default. The agent reads recalled
	// records as injected context, can remember/recall/forget via the
	// in-process `memory` tool, and multi-tool runs are auto-distilled into
	// durable facts. Set AGEZT_MEMORY=off to disable the per-run behaviour
	// (the store and `agt memory` CLI stay available either way).
	memOn := dcfg.Knowledge.Memory
	// How many tool calls a run must make before it's worth an auto-distillation
	// pass (M993). Higher = fewer, more meaningful auto-memories — simple/short
	// runs no longer each spawn distilled notes. Default 6 (was 4); override with
	// AGEZT_MEMORY_DISTILL_MIN_TOOLS (0 or negative falls back to the default).
	distillMinTools := dcfg.Knowledge.MemoryDistillMinTools
	// Operator profile (M1000): learn who the operator is and inject it into every
	// run. Requires memory; default on, AGEZT_USER_PROFILE=off disables both the
	// injection and the daily auto-synthesis.
	profileOn := dcfg.Knowledge.UserProfile
	// Taste overlay: inject curated "what good looks like" exemplars into runs.
	// Default on; AGEZT_TASTE_INJECT=off disables the injection (the store and
	// `agt taste` CLI stay live regardless).
	tasteOn := dcfg.Knowledge.TasteInject
	// World-model per-run behaviour (entity injection + the `world` tool).
	// The graph store and `agt world` CLI always work; this only gates the
	// in-run wiring. AGEZT_WORLDMODEL=off disables it.
	worldOn := dcfg.Knowledge.WorldModel
	// Forge / skills (SPEC-05 §4-5). Active skills inject into runs and Forge
	// proposes drafts after complex tasks. Store + `agt skill` CLI stay live
	// regardless. AGEZT_SKILLS=off disables injection; AGEZT_FORGE=off
	// disables post-run proposal.
	skillOn := dcfg.Knowledge.Skills
	forgeOn := dcfg.Knowledge.Forge
	// Host-environment preamble (M609): on by default — the model needs to know
	// its OS/shell/workspace to act correctly (esp. on Windows). AGEZT_ENV_INJECT=off
	// disables it for operators who pin everything via a custom system prompt.
	envInjectOn := dcfg.Misc.EnvInject
	// Durable run resume (M1002): on by default (default-allow posture). A root
	// run's dispatch context + conversation snapshot are persisted so a restart
	// — stop/start, self-update, or hard kill — re-dispatches the run instead of
	// abandoning it. AGEZT_RESUME=off restores the historical cancel-and-drop;
	// AGEZT_RESUME_SNAPSHOT_MAX_BYTES caps a serialized ticket.
	resumeOn := dcfg.Lifecycle.Resume
	resumeSnapshotMaxBytes := dcfg.Lifecycle.ResumeSnapshotMaxBytes
	// Multi-agent delegation (P6-MULTI-01): the `delegate` tool lets a lead
	// agent spawn bounded sub-agents (AGEZT_SUBAGENT / _DEPTH / _FANOUT /
	// _SPEND_CAP / _MAX_TOTAL — defaults, rails, and the M843 deep-delegation
	// tree bound live in daemonconfig.Load).
	subAgentOn := dcfg.SubAgents.Enabled
	subAgentDepth := dcfg.SubAgents.Depth
	subAgentFanout := dcfg.SubAgents.Fanout
	subAgentSpendCap := dcfg.SubAgents.SpendCapMicrocents
	subAgentTotal := dcfg.SubAgents.MaxTotal

	// Artifact offload threshold (SPEC-04 §3.6) + context budget caps
	// (SPEC-04 §3 / SPEC-10 §3 / M395 / M398): parsed (with their
	// warn-and-degrade messages) by daemonconfig.Load.
	artifactThreshold := dcfg.Context.ArtifactThreshold
	contextBudget := dcfg.Context.Budget
	contextBudgetAuto := dcfg.Context.BudgetAuto
	contextProtectFirst := dcfg.Context.ProtectFirst
	contextSummarize := dcfg.Context.Summarize

	// Run-safety guards: observation deltas, epistemic/intent HITL gating, and
	// the prompt-injection guard posture (raw value parsed here — unset/unknown
	// means warn mode; "on"/"block" → HITL approval; "off"/"0" → no active
	// intervention).
	observationDeltas := dcfg.Guards.ObservationDeltas
	epistemicEscalation := dcfg.Guards.EpistemicEscalation
	intentRegretGating := dcfg.Guards.IntentRegretGating
	promptInjectionMode := kernelruntime.ParsePromptInjectionMode(dcfg.Guards.PromptInjectionGuard)
	autoApproveCaps, autoApproveDesc := selectAutoApproveCapabilities()
	autoPromoteScriptTools := dcfg.Misc.ToolforgeAutoPromote
	disableHeuristicBypass := dcfg.Guards.DisableHeuristicBypass

	// AGEZT_SKILL_SHADOWEVAL=on judges the shadow skills relevant to a completed
	// run against what actually happened (SPEC-05 §5.2). Off by default — it spends
	// extra provider calls per run, so the operator opts in.
	shadowEval := dcfg.Knowledge.SkillShadowEval

	// Provider embeddings for memory recall (M901, DECISIONS C5 opt-in): when
	// AGEZT_EMBED_URL + AGEZT_EMBED_MODEL are both set, recall ranks by TRUE
	// semantic similarity from an OpenAI-compatible /v1/embeddings endpoint —
	// a local Ollama ("http://localhost:11434" + nomic-embed-text, zero cost,
	// no key) or a hosted API (api.openai.com/v1 + text-embedding-3-small +
	// AGEZT_EMBED_KEY). Unset (default) keeps the local feature-hash embedder.
	// Recall falls back to local on any embedder failure, so a wrong URL
	// degrades quality, never availability.
	var memEmbedder kernelmemory.Embedder
	if ec := dcfg.Sidecars.Embed; ec.Enabled() {
		memEmbedder = embed.New(ec.URL, ec.Model, ec.Key)
	}

	// Voice adapter (STT + TTS) over an OpenAI-compatible endpoint, same shape as
	// the embeddings adapter. Each half is independent: set AGEZT_STT_URL +
	// AGEZT_STT_MODEL to let agents transcribe inbound audio, and/or AGEZT_TTS_URL
	// + AGEZT_TTS_MODEL to let them synthesize spoken replies. Local (faster-
	// whisper / Kokoro behind an OpenAI shim) or hosted (api.openai.com/v1 +
	// AGEZT_STT_KEY / AGEZT_TTS_KEY). Unset → no voice tool is registered.
	// AGEZT_STT_PROVIDER / AGEZT_TTS_PROVIDER pick the wire dialect (default
	// "openai", any OpenAI-compatible endpoint). Native providers — ElevenLabs,
	// Deepgram, Cartesia — speak their own shapes and supply their own default
	// base URL, so a URL is optional for them. Boot-resilient: a misconfigured
	// half warns and disables itself rather than failing the daemon.
	voiceAdapter := &voice.Adapter{}
	if sc := dcfg.Sidecars.STT; sc.Attempt {
		if sttClient, err := voice.NewSTT(sc.Provider, voice.Config{BaseURL: sc.URL, Model: sc.Model, APIKey: sc.Key}); err != nil {
			fmt.Fprintf(stderr, "%s: transcription disabled: %v\n", brand.Binary, err)
		} else {
			voiceAdapter.STT = sttClient
		}
	}
	if tc := dcfg.Sidecars.TTS; tc.Attempt {
		if ttsClient, err := voice.NewTTS(tc.Provider, voice.Config{BaseURL: tc.URL, Model: tc.Model, Voice: tc.Voice, APIKey: tc.Key}); err != nil {
			fmt.Fprintf(stderr, "%s: synthesis disabled: %v\n", brand.Binary, err)
		} else {
			voiceAdapter.TTS = ttsClient
		}
	}
	var voiceCfg kernelruntime.Voice
	if voiceAdapter.HasSTT() || voiceAdapter.HasTTS() {
		voiceCfg = voiceAdapter // typed-nil avoidance: only assign when something is configured
	}

	// Image generation (M997): when AGEZT_IMAGE_URL + AGEZT_IMAGE_MODEL are set,
	// the `image_generate` tool is registered, generating images via an
	// OpenAI-compatible /v1/images/generations endpoint (api.openai.com/v1 +
	// dall-e-3 + AGEZT_IMAGE_KEY, or a local/compatible gateway). Unset → no tool.
	var imageCfg kernelruntime.ImageGen
	if ic := dcfg.Sidecars.Image; ic.Enabled() {
		imageCfg = image.New(ic.URL, ic.Model, ic.Key)
	}

	// Reranking (M997): when AGEZT_RERANK_URL + AGEZT_RERANK_MODEL are set, the
	// `rerank` tool is registered, reordering candidate documents via a
	// Cohere/Jina-style /rerank endpoint. Unset → no tool.
	var rerankCfg kernelruntime.Reranker
	if rc := dcfg.Sidecars.Rerank; rc.Enabled() {
		rerankCfg = rerank.New(rc.URL, rc.Model, rc.Key)
	}

	cfg := kernelruntime.Config{
		BaseDir:          baseDir,
		Provider:         gov, // Governor implements agent.Provider
		Tools:            tools,
		Plugins:          pluginManifest,
		ToolCapabilities: pluginToolCaps, // M900: manifest-declared policy axes

		Model:                      model,
		System:                     dcfg.Misc.SystemPrompt, // AGEZT_SYSTEM_PROMPT
		Warden:                     ward,
		Edict:                      edictEng,
		AutoApproveCapabilities:    autoApproveCaps,
		AutoPromoteScriptTools:     autoPromoteScriptTools,
		Catalog:                    cat,
		MemoryInject:               memOn,
		MemoryTool:                 memOn,
		MemoryDistill:              memOn,
		ProfileInject:              profileOn,
		TasteInject:                tasteOn,
		MemoryTopK:                 5,
		MemoryDistillMinTools:      distillMinTools,
		MemoryEmbedder:             memEmbedder, // M901: provider embeddings opt-in (nil = local hashing)
		Voice:                      voiceCfg,    // voice adapter opt-in (nil = no voice tool)
		ImageGenerator:             imageCfg,    // M997: image generation opt-in (nil = no image tool)
		Reranker:                   rerankCfg,   // M997: reranking opt-in (nil = no rerank tool)
		WorldInject:                worldOn,
		WorldTool:                  worldOn,
		WorldTopK:                  5,
		EnvironmentInject:          envInjectOn,
		WorkspaceRoot:              workspaceRoot(baseDir),
		SkillInject:                skillOn,
		SkillTopK:                  3,
		SkillForge:                 forgeOn,
		SkillForgeMinTools:         4,
		ArtifactThreshold:          artifactThreshold,
		ContextBudget:              contextBudget,
		ContextBudgetAuto:          contextBudgetAuto,
		ContextProtectFirst:        contextProtectFirst,
		ContextSummarize:           contextSummarize,
		ObservationDeltas:          observationDeltas,
		EpistemicEscalation:        epistemicEscalation,
		IntentRegretGating:         intentRegretGating,
		PromptInjectionGuard:       promptInjectionMode,
		DisableHeuristicBypass:     disableHeuristicBypass,
		ShadowEval:                 shadowEval,
		SubAgentTool:               subAgentOn,
		ResumeEnabled:              resumeOn,               // M1002: durable in-flight-run resume across restarts
		ResumeSnapshotMaxBytes:     resumeSnapshotMaxBytes, // M1002
		MarketTool:                 true,                   // agents can discover + install capability packs mid-task
		SubAgentMaxDepth:           subAgentDepth,
		SubAgentMaxFanout:          subAgentFanout,
		SubAgentMaxSpendMicrocents: subAgentSpendCap,
		SubAgentMaxTotal:           subAgentTotal,
	}
	// Pre-Open registry hooks (Phase 2.2): each built spec may mutate the
	// runtime Config before Open. Today that's code_exec wiring itself in as
	// cfg.ScriptRunner (M794) — forged tools execute through the same sandbox
	// (warden isolation, scrubbed env); without the sandbox the forge reports
	// itself unavailable.
	toolSet.ApplyPreOpen(&cfg)
	// Per-run wall-clock timeout (M31): AGEZT_RUN_TIMEOUT=<duration> caps how
	// long a single run may take inside a live session. Off by default (only
	// MaxIter + explicit halt bound a run); a positive duration arms the cap.
	// A malformed value is a hard startup error (fast feedback over silent
	// misconfig); a non-positive value is treated as "off".
	runTimeoutDesc := "disabled (set " + brand.EnvPrefix + "RUN_TIMEOUT, e.g. 5m)"
	if d := dcfg.RunLoop.RunTimeout; d > 0 {
		cfg.MaxDuration = d
		runTimeoutDesc = fmt.Sprintf("%s per run (task.failed reason=timeout on overrun)", d)
	}
	// Per-run tool-round cap (M824): AGEZT_MAX_ITER sets how many tool-call rounds
	// a single run may take before it stops with max_iters. Defaults to the agent
	// package's DefaultMaxIter. A malformed or non-positive value is a hard startup
	// error (fast feedback). Raise it for deep agentic tasks; the chat's "Continue"
	// affordance resumes a run that still hit the cap.
	maxIterDesc := fmt.Sprintf("%d per run (default; set %sMAX_ITER to change)", agent.DefaultMaxIter, brand.EnvPrefix)
	if n := dcfg.RunLoop.MaxIter; n > 0 {
		cfg.MaxIter = n
		maxIterDesc = fmt.Sprintf("%d per run", n)
	}

	// Autonomous continue past the round cap (M833): when a run exhausts its
	// tool-round budget without finishing, the loop keeps going on its own — it
	// injects a "keep working" turn and grants another batch of rounds, up to
	// AGEZT_MAX_AUTO_CONTINUE times, until the task completes. Defaults to the
	// agent package's DefaultMaxAutoContinue; a negative value disables it (a run
	// then stops at the cap with max_iters). Set it high for long unattended jobs.
	// AGEZT_AUTO_CONTINUE_WAIT tunes the breather before each continuation.
	autoContinueDesc := fmt.Sprintf("%d×%d rounds (default; set %sMAX_AUTO_CONTINUE)", agent.DefaultMaxAutoContinue, cfg.MaxIter, brand.EnvPrefix)
	if dcfg.RunLoop.MaxAutoContinueSet {
		n := dcfg.RunLoop.MaxAutoContinue
		cfg.MaxAutoContinue = n
		switch {
		case n < 0:
			autoContinueDesc = "disabled (stops at the round cap)"
		case n == 0:
			autoContinueDesc = fmt.Sprintf("%d×%d rounds (default)", agent.DefaultMaxAutoContinue, cfg.MaxIter)
		default:
			autoContinueDesc = fmt.Sprintf("%d×%d rounds", n, cfg.MaxIter)
		}
	}
	cfg.AutoContinueWait = dcfg.RunLoop.AutoContinueWait

	// In-turn parallel tool dispatch (M880): AGEZT_PARALLEL_TOOLS caps how many
	// tool calls from ONE assistant turn execute concurrently. Defaults to the
	// agent package's DefaultMaxParallelTools; 1 disables (strictly sequential).
	if n := dcfg.RunLoop.MaxParallelTools; n > 0 {
		cfg.MaxParallelTools = n
	}

	// Tool discovery (CH-03): AGEZT_TOOL_DISCOVERY_MAX=N trims each provider
	// request to the N most relevant tool schemas using the built-in lexical
	// selector. Off by default so existing deployments keep offering every tool.
	cfg.ToolDiscoveryMax = dcfg.RunLoop.ToolDiscoveryMax

	// Per-tool-call timeout (M34): AGEZT_TOOL_TIMEOUT=<duration> bounds each
	// individual tool invocation. Unlike the per-run cap, an overrun fails
	// only that tool call (the model gets an error result and can adapt) —
	// the run continues. Off by default.
	toolTimeoutDesc := "disabled (set " + brand.EnvPrefix + "TOOL_TIMEOUT, e.g. 30s)"
	if d := dcfg.RunLoop.ToolTimeout; d > 0 {
		cfg.ToolTimeout = d
		toolTimeoutDesc = fmt.Sprintf("%s per tool call (error result on overrun; run continues)", d)
	}
	// HITL approval window (M100): AGEZT_APPROVAL_TIMEOUT=<duration> sets how long
	// a prompt-mode approval blocks waiting for an operator before it auto-denies
	// (DecisionTimeout). Default is approval.DefaultTimeout (5m); right-size it for
	// the deployment — a short window for unattended runs, longer for an operator
	// at the console. Non-positive = use default.
	approvalTimeoutDesc := "default (5m; set " + brand.EnvPrefix + "APPROVAL_TIMEOUT, e.g. 2m)"
	if d := dcfg.Policy.ApprovalTimeout; d > 0 {
		cfg.ApprovalTimeout = d
		approvalTimeoutDesc = fmt.Sprintf("%s per HITL approval (auto-deny on overrun)", d)
	}
	// Secret redaction (M15 / SPEC-06): scrub secrets from every durably-published
	// event before it enters the hash-chained (permanent) journal. On by default;
	// AGEZT_REDACT=off disables. Seeded with the configured provider keys (exact
	// literals) plus built-in high-confidence secret patterns.
	var redactor *redact.Redactor
	redactDesc := "disabled (" + brand.EnvPrefix + "REDACT=off)"
	if dcfg.Misc.Redact {
		redactor = redact.New()
		lits := credSecrets(credStore)
		redactor.SetSecrets(lits)
		redactDesc = fmt.Sprintf("enabled (%d literal secrets + built-in patterns)", len(lits))
		if n := len(extraRedactLiterals()); n > 0 {
			redactDesc += fmt.Sprintf(", %d via %sREDACT_EXTRA", n, brand.EnvPrefix)
		}
	}

	// Forward-declared so OnReload (a closure that runs after Open) can hot-swap
	// the kernel's live default model. Assigned just below by kernelruntime.Open.
	var k *kernelruntime.Kernel
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
		// catStore stays stable; the catalog data was reloaded by the Kernel —
		// but providerboot needs the actual *catalog.Catalog. Re-load locally so
		// we don't depend on Kernel internals.
		c, err := catStore.Load()
		if err != nil {
			return fmt.Errorf("catalog: %w", err)
		}
		// providerboot.Reload shares Boot's selection + registration path, so
		// the live post-reload registry matches what a fresh boot would have
		// produced for the same on-disk state (M928/M816 parity).
		model2, err := providerboot.Reload(gov, providerDeps(c, credStore, baseDir, stderr))
		if err != nil {
			return err
		}
		// Hot-swap the live default model to match the freshly-selected provider
		// (M816). k is non-nil whenever Reload runs (control plane only
		// dispatches it post-Open); guard anyway for safety.
		if k != nil {
			k.SetModel(model2)
		}
		return nil
	}

	// Vision sidecar picker (M821): returns a keyed vision-capable model id the
	// governor can route to, or ("", false) if none. Eligibility is the shared
	// providerboot.Eligible predicate (supported family + credentialed) so the
	// pick is always routable. Uses the LIVE catalog (k.Catalog()) so a freshly
	// synced/credentialed vision provider is picked up without a restart. Injected
	// into the runtime so DescribeImages can caption images for non-vision models.
	cfg.VisionModel = func() (string, bool) {
		if k == nil {
			return "", false
		}
		cat := k.Catalog()
		if cat == nil {
			return "", false
		}
		lookup, _ := buildAWSCredChain(catalogScopedVaultLookup(cat, credStore.Lookup))
		return cat.VisionCapableAmong(func(provID string) bool {
			return providerboot.Eligible(cat.Providers[provID], lookup)
		})
	}

	// Keyed-model predicate (M838 bugfix): true when some registered+credentialed
	// provider actually serves the model id. Delegation uses it to drop unkeyed
	// models from a sub-agent's chain so a delegate never lands on a provider with
	// no API key. Built like VisionModel — the daemon owns the keyed set.
	cfg.ModelAvailable = func(modelID string) bool {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			return false
		}
		if k == nil {
			return true // set not built yet → don't block a non-empty id
		}
		cat := k.Catalog()
		if cat == nil {
			return true
		}
		lookup, _ := buildAWSCredChain(catalogScopedVaultLookup(cat, credStore.Lookup))
		// Accept a bare ("model") or provider-qualified ("provider/model") id.
		want := modelID
		if i := strings.IndexByte(modelID, '/'); i >= 0 {
			want = modelID[i+1:]
		}
		for _, p := range cat.ProviderList() {
			e := cat.Providers[p.ID]
			if !providerboot.Eligible(e, lookup) {
				continue
			}
			if _, ok := e.Models[modelID]; ok {
				return true
			}
			if _, ok := e.Models[want]; ok {
				return true
			}
		}
		return false
	}

	// Council of Elders default membership (M837): one seat per KEYED provider's
	// best model, so the panel speaks across providers. AGEZT_COUNCIL_MEMBERS (a
	// comma-separated model list) overrides. Built like VisionModel — the daemon
	// owns the registered+credentialed set; never picks an unkeyed model.
	cfg.CouncilMembers = func() []kernelruntime.CouncilMember {
		// Deliberately a LIVE env read (not daemonconfig.Load): this closure runs
		// on every council convocation, and the Config Center live-applies edits
		// via os.Setenv — capturing at boot would regress restart-free changes.
		if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "COUNCIL_MEMBERS")); spec != "" {
			var ms []kernelruntime.CouncilMember
			for i, part := range strings.Split(spec, ",") {
				m := strings.TrimSpace(part)
				if m == "" {
					continue
				}
				ms = append(ms, kernelruntime.CouncilMember{Seat: councilSeatName(i), Model: m})
			}
			if len(ms) > 0 {
				return ms
			}
		}
		if k == nil {
			return nil
		}
		cat := k.Catalog()
		if cat == nil {
			return nil
		}
		lookup, _ := buildAWSCredChain(catalogScopedVaultLookup(cat, credStore.Lookup))
		models := cat.BestModelsAcross(func(provID string) bool {
			return providerboot.Eligible(cat.Providers[provID], lookup)
		}, 3)
		// No keyed provider listed a model → fall back to the active model so a
		// single-provider setup still convenes a (degenerate) council.
		if len(models) == 0 {
			if m := strings.TrimSpace(k.Model()); m != "" && m != "mock" {
				models = []string{m}
			}
		}
		out := make([]kernelruntime.CouncilMember, 0, len(models))
		for i, m := range models {
			out = append(out, kernelruntime.CouncilMember{Seat: councilSeatName(i), Model: m})
		}
		return out
	}

	// Ground the Council of Elders in current facts: today's date for every seat
	// plus a shared web research brief (via the always-registered web_search tool).
	// Default on; AGEZT_COUNCIL_WEBSEARCH=off convenes the panel with the date only.
	cfg.CouncilWebSearch = dcfg.Misc.CouncilWebSearch

	var openErr error
	k, openErr = kernelruntime.Open(cfg)
	if openErr != nil {
		fmt.Fprintf(stderr, "%s: open runtime: %v\n", brand.Binary, openErr)
		return 1
	}
	defer k.Close()

	// Post-Open dependency injection (config's kernel, the artifact index for
	// fetch/browser.action/artifacts/code_exec, the db data lake, the
	// council/conductor/research runners, the kernel Binds for the zero-arg
	// tools) is registry-driven: each spec's Configure hook runs in
	// toolSet.Configure below.

	// Wire the bus into the Governor and the Warden so their events
	// land in the journal. MUST happen before any Run is dispatched.
	gov.SetBus(k.Bus())
	ward.SetBus(k.Bus())
	// Skill auto-quarantine (SPEC-05 §5): on by default — an active skill that
	// repeatedly fails in production is pulled automatically (journaled, reversible
	// with `agt skill promote`). AGEZT_SKILL_AUTOQUARANTINE=off disables it.
	autoQDesc := fmt.Sprintf("on (pull active skill after ≥%d failures at ≥%.0f%% rate; set %sSKILL_AUTOQUARANTINE=off to disable)",
		skill.DefaultAutoQuarantineMinFailures, skill.DefaultAutoQuarantineRate*100, brand.EnvPrefix)
	if !dcfg.Knowledge.SkillAutoQuarantine {
		k.Forge().SetAutoQuarantine(0, 0)
		autoQDesc = "off (set " + brand.EnvPrefix + "SKILL_AUTOQUARANTINE=on to enable)"
	}
	// Skill auto-shadow (SPEC-05 §5.2): off by default — staging a draft toward
	// production is opt-in. When on, a freshly-authored draft that passes the
	// deterministic shadow-test auto-advances to shadow. AGEZT_SKILL_AUTOSHADOW=on.
	autoShadowDesc := "off (set " + brand.EnvPrefix + "SKILL_AUTOSHADOW=on to auto-stage drafts that pass the shadow-test)"
	if dcfg.Knowledge.SkillAutoShadow {
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
	if !dcfg.Knowledge.SkillAutoPromote {
		k.Forge().SetAutoPromote(0, 0)
		autoPromoteDesc = "off (set " + brand.EnvPrefix + "SKILL_AUTOPROMOTE=on to enable)"
	}
	// Registry-driven post-Open tool wiring (Phase 2.2): Set.Configure walks the
	// registry-built specs and (a) wires the egress-block audit (M109) — when a
	// netguard-guarded tool refuses a dial, a netguard.blocked event is journaled
	// so an operator can see attempted SSRF / metadata reads — and (b) runs each
	// spec's Configure hook: kernel/artifact-index/data-lake/runner injection for
	// the Set*-injection batch and the kernel Binds for the zero-arg tools
	// (schedule, runs, standing, skill, introspect, overseer, tool_forge, mcp,
	// workflow, workboard). Wired here because the tools are built before the
	// kernel exists (same ordering as gov.SetBus).
	if err := toolSet.Configure(toolreg.KernelDeps{
		K:               k,
		Bus:             k.Bus(),
		Artifacts:       k.ArtifactIndex(),
		Lake:            k.DataLake(),
		Journal:         k.Journal(),
		BaseDir:         baseDir,
		Stdout:          stdout,
		NetguardPublish: netguardPublish(k.Bus()),
	}); err != nil {
		fmt.Fprintf(stderr, "%s: configure tools: %v\n", brand.Binary, err)
		return 1
	}
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
	if dcfg.Policy.EdictDurable {
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

	// Index offloaded tool outputs (M827): watch tool.result events carrying a
	// raw_ref (the agent stored a large output in the blob store) and add a
	// browsable artifact-index entry, so the file manager lists run outputs
	// alongside inbound images. Best-effort; lives on the daemon ctx.
	wireArtifactIndexer(ctx, k)

	// Shared message board (M647/M937): ONE store instance serves every writer —
	// the `board` tool, the control plane's board_send/board_ack, and the REST
	// mailbox. Each store holds the whole message list in memory and saves it
	// whole, so a second instance would silently clobber the other's last write.
	// boardNotify publishes the board.posted event for ANY door's write: subject
	// routing (board.dm.<slug> / board.help[.<slug>] / board.broadcast /
	// board.<topic>) is what lets a standing order wake the addressed agent.
	boardStore, boardErr := board.Open(filepath.Join(baseDir, "board"))
	boardNotify := func(m board.Message, corr string) {
		// Help takes precedence so a help-flagged message wakes responders
		// watching board.help.
		subject := "board." + boardSubjectSlug(m.Topic)
		switch {
		case m.Help && m.To != "" && m.To != board.Everyone:
			subject = "board.help." + boardSubjectSlug(m.To)
		case m.Help:
			subject = "board.help"
		case m.To == board.Everyone:
			subject = "board.broadcast"
		case m.To != "":
			subject = "board.dm." + boardSubjectSlug(m.To)
		}
		payload := map[string]any{"topic": m.Topic, "chars": len(m.Text)}
		if m.ID != "" {
			payload["id"] = m.ID
		}
		if m.From != "" {
			payload["from"] = m.From
		}
		if m.To != "" {
			payload["to"] = m.To
		}
		if m.ReplyTo != "" {
			payload["reply_to"] = m.ReplyTo
		}
		if m.Help {
			payload["help"] = true
		}
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       subject,
			Kind:          event.KindBoardPosted,
			Actor:         "board",
			CorrelationID: corr,
			Payload:       payload,
		})
	}

	// Early control-plane dependencies (Phase 2.6 3b-ii): everything below is
	// computed up front and handed to NewServerWithDeps in one shot, so the
	// server never accepts a connection without them.
	//
	// Board writes over the control plane (M937): `agt`/Go-SDK board_send,
	// board_ack go through the shared instance and fire the same notifier. Only
	// when the shared store opened — a failed open leaves board commands
	// unavailable (boardErr is surfaced in the banner phase below).
	srvBoard, srvBoardNotify := boardStore, boardNotify
	if boardErr != nil {
		srvBoard, srvBoardNotify = nil, nil
	}
	// Cancel-on-disconnect (M35): when AGEZT_CANCEL_ON_DISCONNECT=on, a
	// streaming `agt run` whose client drops (Ctrl-C / killed) cancels its run
	// server-side instead of running on headless. Off by default so a
	// backgrounded `agt run &` (client still alive) is unaffected.
	cancelOnDisconnect := dcfg.Lifecycle.CancelOnDisconnect
	// Self-update engine (M860): wired when AGEZT_UPDATE_ENDPOINT or
	// AGEZT_UPDATE_GITHUB_OWNER/REPO is set. When not configured, update
	// commands report "update is disabled" rather than erroring.
	var updateSvc *update.Service
	if endpoint := dcfg.Lifecycle.UpdateEndpoint; endpoint != "" {
		updateSvc = update.New(update.Config{
			Source:        update.SourceEndpoint,
			Endpoint:      endpoint,
			BaseDir:       baseDir,
			DrainTimeout:  dcfg.Lifecycle.UpdateDrainTimeout,  // default 30s
			CheckInterval: dcfg.Lifecycle.UpdateCheckInterval, // 0 = disabled by default
		})
	} else if owner := dcfg.Lifecycle.UpdateGitHubOwner; owner != "" {
		updateSvc = update.New(update.Config{
			Source:        update.SourceGitHub,
			GitHubOwner:   owner,
			GitHubRepo:    dcfg.Lifecycle.UpdateGitHubRepo, // defaulted to the binary name in Load
			BaseDir:       baseDir,
			DrainTimeout:  30 * time.Second,
			CheckInterval: 0,
		})
	}
	// Record the network-exposed HTTP servers (M137) so `agt status` and the
	// doctor exposure check can flag a non-loopback bind — the agent reachable
	// beyond localhost, gated only by a token. Built from the configured addrs
	// (env-only, so it can run here in the pre-construction deps region, Phase 2.6);
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
	// Multi-tenant registry (ROADMAP P6-MULTI), opt-in via AGEZT_MULTITENANT.
	// Each tenant gets its own isolated base dir under <baseDir>/tenants/<id>
	// and its own kernel — opened with the same provider/tools/model as the
	// primary, but a fresh per-tenant Warden/Edict (so a tenant HALT or policy
	// state is its own) and no reload hook. The primary kernel is unaffected;
	// `agt tenant` manages the registry over the control plane.
	tenantsDesc := "disabled (set " + brand.EnvPrefix + "MULTITENANT=on)"
	var tenantReg *tenant.Registry
	if dcfg.Tenancy.Multitenant {
		// Per-tenant daily spend ceiling (M14 quotas). Each tenant gets its OWN
		// governor (independent ledger) so one tenant exhausting its cap can
		// never block another's runs, while the provider pool stays shared. The
		// ceiling defaults to the primary's; AGEZT_TENANT_DAILY_CEILING (USD)
		// overrides it for every tenant (malformed = daemonconfig.Load error).
		tenantCeiling := gov.DailyCeilingMicrocents()
		ceilingDesc := "inherited"
		if dcfg.Tenancy.DailyCeilingSet {
			tenantCeiling = dcfg.Tenancy.DailyCeilingMicrocents
			ceilingDesc = fmt.Sprintf("$%.2f/day", dcfg.Tenancy.DailyCeilingUSD)
		}
		// Per-tenant per-minute call rate cap (M14 quotas). 0 = unlimited.
		tenantRate := 0
		rateDesc := "unlimited"
		if dcfg.Tenancy.RatePerMinSet {
			tenantRate = dcfg.Tenancy.RatePerMin
			rateDesc = fmt.Sprintf("%d/min", tenantRate)
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
			// than failing the lazy open. Gated on the same AGEZT_EDICT_DURABLE —
			// deliberately a LIVE env read (not daemonconfig.Load): this closure
			// runs at lazy tenant-open time, and the Config Center live-applies
			// edits via os.Setenv.
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
		// Registered before defer srv.Stop() below, so on unwind the control
		// plane stops accepting requests BEFORE the tenant kernels close (the
		// old post-Start wiring closed the tenants first, leaving a brief
		// window where an in-flight request could hit a closed tenant kernel).
		defer reg.CloseAll()
		root := filepath.Join(baseDir, "tenants")
		if infos, _ := reg.List(); infos != nil {
			tenantsDesc = fmt.Sprintf("enabled (root=%s, %d on disk, ceiling=%s, rate=%s)", root, len(infos), ceilingDesc, rateDesc)
		} else {
			tenantsDesc = fmt.Sprintf("enabled (root=%s, ceiling=%s, rate=%s)", root, ceilingDesc, rateDesc)
		}
	}

	// One-shot early wiring (Phase 2.6 3b-ii): the server comes up with every
	// pre-Start dependency already applied — including the tenant registry, so
	// tenant tokens authorize from the very first accepted connection (the old
	// post-Start SetTenants left a boot window where they were rejected as
	// "unauthorized"). The late deps (pulse, channel send, standing fire) arrive
	// via srv.Bind at each point of readiness below.
	srv := controlplane.NewServerWithDeps(k, baseDir, controlplane.Deps{
		ConfigEnvPinned:    configPinned,       // M693: Config Center marks env-pinned fields read-only
		Board:              srvBoard,           // M937: the ONE shared board instance…
		BoardNotify:        srvBoardNotify,     // …and its board.posted notifier
		DiskFree:           pulse.DiskUsage,    // M131: cross-platform probe keeps controlplane pulse-free
		UpdateSvc:          updateSvc,          // M860: nil = update disabled
		Tenants:            tenantReg,          // P6-MULTI: nil = single-tenant
		CancelOnDisconnect: cancelOnDisconnect, // M35
		HTTPBindings:       httpBindings,       // M137: exposure check + `agt status`
		CredChain:          awsChainDesc,       // M307: which AWS credential layer engaged
		Channels:           collectChannels(),  // M141: configured channels for `agt status`
	})
	cancelOnDisconnectDesc := "disabled (set " + brand.EnvPrefix + "CANCEL_ON_DISCONNECT=on)"
	if cancelOnDisconnect {
		cancelOnDisconnectDesc = "on (a dropped `agt run` client cancels its run)"
	}
	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(stderr, "%s: start control plane: %v\n", brand.Binary, err)
		return 1
	}
	defer srv.Stop()

	// Daemon-ready banner: surfaces the stamped build identity so an
	// operator can confirm exactly which commit + build time the
	// running binary was compiled from. brand.BuildStamp() also
	// keeps BuildCommit / BuildTime reachable for the linker — without
	// the reference here, the dead-code eliminator strips them and
	// the `-X` ldflag stamp in the Makefile / scripts/build.sh
	// silently no-ops.
	ver, commit, btime := brand.BuildStamp()
	if commit == "" {
		commit = "unstamped"
	}
	if btime == "" {
		btime = "unstamped"
	}
	fmt.Fprintf(stdout, "%s %s (commit=%s, built=%s) — daemon ready (protocol v%d)\n",
		brand.Name, ver, commit, btime, brand.ProtocolVersion)
	fmt.Fprintf(stdout, "  base dir         : %s\n", baseDir)
	fmt.Fprintf(stdout, "  governor         : %s\n", govDesc)
	// First-run nudge (M816): when the daemon booted UNCONFIGURED (no provider
	// selected — the sentinel primary refuses every LLM run), make the fix
	// impossible to miss — point at both the CLI wizard and the Web UI, which
	// auto-opens its setup screen. Keyed off the PRIMARY name, not the model:
	// the old `model == "mock"` check fired exactly backwards (the sentinel
	// resolves model "", and "mock" only appears via the explicit
	// AGEZT_DEMO_ECHO=1 escape hatch, which is a deliberately configured state).
	if firstRunSetupNeeded(bootRes) {
		fmt.Fprintf(stdout, "  ⚠ setup needed   : no provider key yet — run `%s quickstart`, or open the Web UI (URL below) to add one\n", brand.CLI)
	}
	if adv := modelAdvisory(cat, model); adv != "" {
		fmt.Fprintf(stdout, "  model advisory   : ⚠ %s\n", adv)
	}
	fmt.Fprintf(stdout, "  credentials      : %s\n", credDesc)
	fmt.Fprintf(stdout, "  redaction        : %s\n", redactDesc)
	fmt.Fprintf(stdout, "  tools            : %s\n", toolsDesc)
	fmt.Fprintf(stdout, "  policy engine    : edict (allow-by-default — every capability on unless you opt out; %s)\n", askPolicyDesc)
	fmt.Fprintf(stdout, "  auto-approvals   : %s\n", autoApproveDesc)
	fmt.Fprintf(stdout, "  delegation       : %s\n", delegationBanner(k))
	fmt.Fprintf(stdout, "  run timeout      : %s\n", runTimeoutDesc)
	fmt.Fprintf(stdout, "  max iterations   : %s\n", maxIterDesc)
	fmt.Fprintf(stdout, "  auto-continue    : %s\n", autoContinueDesc)
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

	// The shared inbound handler, built ONCE for every factory-migrated channel
	// (Phase 2.1): channelwire factories receive it through Deps.Handler.
	chanHandler := makeChannelHandler(k)

	// Every channel kind, built and started from its registered manifest
	// (Phase 2.1 PR 8): one uniform loop replaces the 34 hand-written
	// build+start pairs. The per-kind boot-banner label and the "disabled
	// (set ...)" hint live on the manifest (BannerLabel / DisabledHint;
	// empty hint = silent when unconfigured, matching the old push family).
	// Banner order follows Manifests() (sorted by display name) rather than
	// the old hand-ordering.
	//
	// Every configured channel's brief sink is teed below: Pulse briefs and
	// (M782) alert notifications share the same delivery surface. All
	// channels are multi-account now: one brief sink per instance.
	var allInsts [][]chanInstance
	for _, m := range channel.Manifests() {
		insts := wireInstances(channelwire.BuildKind(ctx, m.Kind, k.Bus(), chanHandler))
		label := m.BannerLabel
		if label == "" {
			label = m.Kind
		}
		startInstances(ctx, stdout, m.Kind, label, m.DisabledHint, insts)
		allInsts = append(allInsts, insts)
	}
	channelSinks := combineSinks(instanceSinks(allInsts...)...)

	// Pulse — the proactive heart (SPEC-03). On by default; the resident
	// engine runs on the daemon ctx so `agt halt`/SIGTERM/`agt shutdown`
	// stop it with everything else. AGEZT_PULSE=off disables it. When a channel
	// is configured, briefs tee to it (closes the Jarvis loop).
	if eng, pulseDesc := buildPulse(k, ward, model, stdout, channelSinks); eng != nil {
		eng.Start(ctx)
		// Late-bind the engine + the runtime watch registry (disk watches M767,
		// command probes M768): the adapter builds the observers (the daemon owns
		// the DiskUsage func and the warden) and registers them on the live engine.
		srv.Bind(controlplane.LateDeps{
			Pulse:     eng,
			Observers: pulseObserverAdmin{eng: eng, ward: ward, st: k.State()},
		})
		// Reaper (#53, M903): each beat, scan for dead agents, degraded live agents,
		// and stale artifacts, and surface a low-severity brief when the pile grows.
		// Detection only — retire (graveyard), doctoring, and collect stay gated by
		// the agents/orders that choose to act. Fixed 30-day idle/stale window.
		const reaperWindow = 30 * 24 * time.Hour
		eng.AddObserver(pulse.NewReaperObserver(func() (int, int, int, int, int, int, int, int, int, int) {
			cut := time.Now().Add(-reaperWindow).UnixMilli()
			r := k.ReaperScan(cut, cut)
			return len(r.DeadAgents), len(r.DegradedAgents), len(r.MisconfiguredAgents), len(r.RetryPressure), len(r.RoutingPressure), len(r.RoutingForced), len(r.RoutingForcedFailed), len(r.RoutingForcedExhausted), len(r.RoutingUnstable), r.StaleArtifacts
		}))
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

	if wbSweepDesc := startWorkboardSweepTicker(ctx, k, stdout); wbSweepDesc != "" {
		fmt.Fprintf(stdout, "  workboard sweep  : %s\n", wbSweepDesc)
	} else {
		fmt.Fprintf(stdout, "  workboard sweep  : on-demand (agt workboard sweep; set AGEZT_WORKBOARD_SWEEP_EVERY for a timer)\n")
	}

	// Brain distillation (M804). Always available via `agt memory
	// consolidate`; set AGEZT_BRAIN_DISTILL_EVERY (e.g. 24h) to run the
	// consolidation pass on a timer — the standing "sleep cycle" that merges
	// accumulated near-duplicate memories. Mirrors the reflection ticker.
	if bdDesc := startBrainDistillTicker(ctx, k, stdout); bdDesc != "" {
		fmt.Fprintf(stdout, "  brain distill    : %s\n", bdDesc)
	} else {
		fmt.Fprintf(stdout, "  brain distill    : on-demand (agt memory consolidate; set AGEZT_BRAIN_DISTILL_EVERY for a timer)\n")
	}

	// Operator profile (M1000): inject what we've learned about the operator into
	// every run, and synthesize it on a daily timer. Off with AGEZT_USER_PROFILE=off.
	if pdDesc := startProfileDistillTicker(ctx, k, profileOn, stdout); pdDesc != "" {
		fmt.Fprintf(stdout, "  user profile     : on, auto-synthesis %s (agt memory profile)\n", pdDesc)
	} else {
		fmt.Fprintf(stdout, "  user profile     : off\n")
	}

	// Web UI (SPEC-07) — the primary product surface. Default-on, loopback-bound,
	// and auto-opened in the desktop browser unless AGEZT_WEB_OPEN=off.
	webSurface := buildWebUI(ctx, k, baseDir, stdout)
	if webSurface.desc != "" {
		fmt.Fprintf(stdout, "  %s           : %s\n", bannerColor("web ui", "1;35"), webSurface.desc)
	} else {
		fmt.Fprintf(stdout, "  web ui           : disabled (AGEZT_WEB_ADDR=off; unset it to serve on 127.0.0.1:8787)\n")
	}

	// Tunnel (SPEC-07) — expose a local HTTP service (the Web UI, else the REST
	// API) through a supervised cloudflared/ngrok/tailscale/custom binary.
	// Off unless AGEZT_TUNNEL or AGEZT_TUNNEL_CMD is set; the operator opts in
	// explicitly since this makes the service publicly reachable.
	if tunDesc := buildTunnel(ctx, stdout, webSurface); tunDesc != "" {
		fmt.Fprintf(stdout, "  tunnel           : %s\n", tunDesc)
	} else {
		fmt.Fprintf(stdout, "  tunnel           : disabled (set AGEZT_TUNNEL=cloudflare|ngrok|tailscale|tailscale-funnel or AGEZT_TUNNEL_CMD)\n")
	}

	// OpenAI-compatible API (P7-API-01) — POST /v1/chat/completions,
	// POST /v1/responses, and GET /v1/models so any OpenAI client drives Agezt
	// through the same tool-loop + Edict + journal. Off unless AGEZT_API_ADDR is
	// set; loopback + token.
	if apiDesc := buildOpenAIAPI(ctx, k, tenantReg, baseDir, stdout); apiDesc != "" {
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

	// Alert notifications (M782): push warning/critical alerts — run failures,
	// blocked egress, budget/rate trips, halts — to the configured channels, so
	// the operator hears about problems without the console open. Opt-in via
	// AGEZT_ALERT_NOTIFY=1; uses the same sinks Pulse briefs go through.
	fmt.Fprintf(stdout, "  alert notify     : %s\n", buildAlertNotify(ctx, k, channelSinks))

	// draining flips true at shutdown so /readyz reports not-ready and the daemon
	// drains in-flight runs before exiting (M136). Shared with buildRESTAPI's
	// readiness probe; an atomic so the shutdown goroutine and the HTTP handler
	// race cleanly.
	var draining atomic.Bool

	// Native REST API (P7-API-02) — first-party /api/v1 surface: submit runs
	// (sync or SSE), inspect a run's journaled arc, health/models. Same governed
	// loop as `agt run`. Off unless AGEZT_REST_ADDR is set; loopback + token.
	var restBoard *board.Store
	if boardErr == nil {
		restBoard = boardStore
	}
	if restDesc := buildRESTAPI(ctx, k, tenantReg, baseDir, &draining, restBoard, boardNotify, updateSvc, stdout); restDesc != "" {
		fmt.Fprintf(stdout, "  rest api         : %s\n", restDesc)
	} else {
		fmt.Fprintf(stdout, "  rest api         : disabled (set AGEZT_REST_ADDR, e.g. 127.0.0.1:8800)\n")
	}

	// Wire operator-initiated outbound (`agt send`, M142) to the live channels.
	// Built from the channels actually constructed above so a kind only sends when
	// it's configured; senders journal channel.outbound via each channel's Send.
	liveChannels := map[string]channel.Channel{}
	// Every channel is multi-account: register every instance by its instance key
	// ("telegram", "telegram#bot2", "email#work", …).
	registerInstances(liveChannels, allInsts...)
	// Record which channels actually started, so the Channels wizard can show
	// "live" vs merely "configured (restart to start)".
	liveKinds := make([]string, 0, len(liveChannels))
	for k := range liveChannels {
		// An instance key may be "kind#label"; the per-kind "live" flag keys off
		// the base kind, so any live instance lights up the manifest.
		base, _, _ := strings.Cut(k, "#")
		liveKinds = append(liveKinds, base)
	}
	channel.SetLive(liveKinds)
	channel.SetLiveInstances(liveChannelKeys(liveChannels))
	// channelTargets resolves a send target to one or more live channels. An exact
	// instance key ("email#work") hits that one instance; a bare kind ("email")
	// fans out to every instance of that kind (the default + all "#label"
	// accounts). For a single-account kind this is exactly one channel — identical
	// to the pre-multi-account behavior.
	channelTargets := func(target string) []channel.Channel {
		var out []channel.Channel
		for _, key := range instanceMatch(liveChannelKeys(liveChannels), target) {
			if ch, ok := liveChannels[key]; ok {
				out = append(out, ch)
			}
		}
		return out
	}
	channelSend := func(sctx context.Context, kind, id, text string) error {
		chs := channelTargets(kind)
		if len(chs) == 0 {
			return fmt.Errorf("channel %q not configured", kind)
		}
		var firstErr error
		for _, ch := range chs {
			if err := ch.Send(sctx, channel.Outbound{ChannelID: id, Text: text, Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}
	channelSendMedia := func(sctx context.Context, kind, id, text string, atts []channel.Attachment) error {
		chs := channelTargets(kind)
		if len(chs) == 0 {
			return fmt.Errorf("channel %q not configured", kind)
		}
		var firstErr error
		for _, ch := range chs {
			if err := ch.Send(sctx, channel.Outbound{ChannelID: id, Text: text, Attachments: atts, Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}
	srv.Bind(controlplane.LateDeps{ChannelSend: channelSend})

	// Late registry phase (Phase 2.2 PR 5): Set.ConfigureLate wires the specs
	// that need live infrastructure, at the exact point their hand-wired Binds
	// used to run. notify (M143) gets the channel sender + operator allowlist
	// (destinations stay pinned to each channel's configured allowlist — the
	// agent supplies only text); send_media gets the media sender + the
	// artifact store so it can resolve a ref to bytes; board (M647) gets the
	// SAME store instance the control plane and REST mailbox write through
	// (opened once, before the servers) plus boardNotify, so each post
	// journals a board.posted event (M656) a standing order can trigger on —
	// or board.dm.<recipient> for an addressed message (M788) — and one
	// agent's post wakes another. The posting run's correlation ties into
	// `agt why`.
	lateBoard := boardStore
	if boardErr != nil {
		lateBoard = nil // spec leaves the tool unbound; error surfaced below
	}
	// Tool late-bind + seed/banner steps as a flat table (Phase 2.6 3a) so the
	// sequence is scannable — and a natural place to time steps later. Each run
	// func either prints its own (conditional / multi-line) banner output
	// directly, or returns one ready-made banner line as desc. Banner text and
	// order are unchanged. Only the first step is fatal; every other step is
	// best-effort and surfaces its own failures without blocking startup.
	bootSteps := []bootStep{
		{
			// Late registry phase (Phase 2.2 PR 5): Set.ConfigureLate wires the
			// specs that need live infrastructure, at the exact point their
			// hand-wired Binds used to run. notify (M143) gets the channel sender
			// + operator allowlist (destinations stay pinned to each channel's
			// configured allowlist — the agent supplies only text); send_media
			// gets the media sender + the artifact store so it can resolve a ref
			// to bytes; board (M647) gets the SAME store instance the control
			// plane and REST mailbox write through (opened once, before the
			// servers) plus boardNotify, so each post journals a board.posted
			// event (M656) a standing order can trigger on — or
			// board.dm.<recipient> for an addressed message (M788) — and one
			// agent's post wakes another. The posting run's correlation ties
			// into `agt why`.
			name: "late-configure tools", fatal: true,
			run: func() (string, error) {
				if err := toolSet.ConfigureLate(toolreg.LateDeps{
					KernelDeps: toolreg.KernelDeps{
						K:         k,
						Bus:       k.Bus(),
						Artifacts: k.ArtifactIndex(),
						Lake:      k.DataLake(),
						Journal:   k.Journal(),
						BaseDir:   baseDir,
						Stdout:    stdout,
					},
					ChannelSend:      channelSend,
					ChannelSendMedia: channelSendMedia,
					Board:            lateBoard,
					BoardNotify:      boardNotify,
				}); err != nil {
					return "", err
				}
				if len(notifyTargets) > 0 {
					fmt.Fprintf(stdout, "  notify tool      : enabled (%d channel(s) the agent can ping)\n", len(notifyTargets))
					fmt.Fprintf(stdout, "  send_media tool  : enabled (the agent can send images/voice/files to the operator)\n")
				}
				return "", nil
			},
		},
		{
			// The schedule/runs/standing tools were wired to the live kernel by
			// their registry specs' Configure hooks (toolSet.Configure, just
			// after Open); only the boot banner remains here.
			name: "schedule tool",
			run: func() (string, error) {
				if k.Schedules() == nil {
					return "", nil
				}
				return "  schedule tool    : enabled (the agent can schedule its own future runs)", nil
			},
		},
		{
			// Board banner / failure surface — the bind itself ran in ConfigureLate.
			name: "board tool",
			run: func() (string, error) {
				if boardErr != nil {
					fmt.Fprintf(stderr, "%s: board tool unavailable: %v\n", brand.Binary, boardErr)
					return "", nil
				}
				return "  board tool       : enabled (agents share a persistent message board)", nil
			},
		},
		{
			// The skill tool was bound to the kernel's Forge (M648) by its
			// registry spec's Configure hook; the banner + the Forge-dependent
			// seeding stay here.
			name: "skill tool + marketplace",
			run: func() (string, error) {
				fg := k.Forge()
				if fg == nil {
					return "", nil
				}
				fmt.Fprintf(stdout, "  skill tool       : enabled (the agent can author and manage its own skills)\n")

				// Seed the built-in skill bundles baked into the binary (M852), so
				// capabilities like full browser automation work out of the box — the
				// agent gets a ready, active skill with its scripts on disk. Idempotent
				// (content-addressed); best-effort — a seed failure never blocks startup.
				if seeded, serr := builtinskills.SeedAll(fg, ""); serr != nil {
					fmt.Fprintf(stderr, "  built-in skills  : partial (%v)\n", serr)
				} else if len(seeded) > 0 {
					names := make([]string, 0, len(seeded))
					for _, s := range seeded {
						names = append(names, s.Name)
					}
					fmt.Fprintf(stdout, "  built-in skills  : seeded (%s)\n", strings.Join(names, ", "))
				}
				// Wire the capability marketplace (M-market): the built-in "Official"
				// catalogue (skill/MCP/tool packs) is a plugin the kernel must not import,
				// so it's injected here with this kernel's Forge + MCP as the install
				// targets. Install materializes packs into those existing subsystems. The
				// composite Library also serves synced remote marketplaces from the Store
				// cache (Phase 2); the Syncer fetches them under netguard.
				marketStore := market.NewStore(baseDir)
				k.SetMarket(market.NewManager(market.Config{
					Library: market.NewCompositeLibrary(builtinmarket.New(), marketStore),
					Store:   marketStore,
					Skills:  fg,
					MCP:     k,
					Now:     func() int64 { return time.Now().UnixMilli() },
					Verify:  func(p market.Pack) (bool, error) { return market.VerifyPack(p, "") },
					Syncer:  market.NewSyncer(),
				}))
				return "  marketplace      : enabled (built-in Official + synced remotes; `agt market`)", nil
			},
		},
		// The introspect (M682) and overseer (M850) tools were bound to the live
		// kernel by their registry specs' Configure hooks; banners stay here.
		{name: "introspect tool", run: func() (string, error) {
			return "  introspect tool  : enabled (the agent can read the daemon's own live state)", nil
		}},
		{name: "overseer tool", run: func() (string, error) {
			return "  overseer tool    : enabled (a brain agent can supervise & intervene on the fleet)", nil
		}},
		{
			// Seed the built-in guardian agents (M961): the daemon's internal
			// self-healing fleet (health / doctor / stuck / budget / routing-429 /
			// code), each a System-marked agent with an event or cadence trigger,
			// wielding the tools bound just above. Idempotent by slug (an operator
			// who pauses/removes one is respected); best-effort — a seed failure
			// never blocks startup.
			name: "built-in guardians",
			run: func() (string, error) {
				guards, gerr := builtinguardians.SeedAll(builtinguardians.NewKernelHost(k), "")
				if gerr != nil {
					fmt.Fprintf(stderr, "  built-in guardians: partial (%v)\n", gerr)
					return "", nil
				}
				created := 0
				for _, g := range guards {
					if g.Created {
						created++
					}
				}
				if created > 0 {
					return fmt.Sprintf("  built-in guardians: seeded %d (health, doctor, stuck, budget, routing, code)", created), nil
				} else if len(guards) > 0 {
					return fmt.Sprintf("  built-in guardians: present (%d)", len(guards)), nil
				}
				return "", nil
			},
		},
		{
			// code_exec wiring — the bus bind (M683, code.executed events) and the
			// Conductor's Verifier backend (M997) — moved to its registry spec's
			// Configure hook. Only the banner remains; the type assertion here is
			// display-only (Languages() isn't on agent.Tool).
			name: "code_exec tool",
			run: func() (string, error) {
				if ce, ok := tools["code_exec"].(*codeexec.Tool); ok {
					return fmt.Sprintf("  code_exec tool   : enabled (the agent can write & run code: %s)", strings.Join(ce.Languages(), ", ")), nil
				}
				return "", nil
			},
		},
		{
			// The tool_forge tool (M794) was bound to the live kernel by its
			// registry spec's Configure hook; the banner stays.
			name: "tool_forge tool",
			run: func() (string, error) {
				promoteMode := "auto-promotes tested tools"
				if !autoPromoteScriptTools {
					promoteMode = "operator promotes"
				}
				return "  tool_forge tool  : enabled (the agent can build its own tools; " + promoteMode + ")", nil
			},
		},
		// The workflow (M802) and workboard tools were bound to the live kernel
		// by their registry specs' Configure hooks; banners stay.
		{name: "workflow tool", run: func() (string, error) {
			return "  workflow tool    : enabled (the agent can author & run workflows)", nil
		}},
		{name: "workboard tool", run: func() (string, error) {
			return "  workboard tool   : enabled (agents can coordinate through durable tasks)", nil
		}},
		{
			// The MCP self-install tool (M796) was bound by its registry spec's
			// Configure hook; auto-attach every ENABLED registered server here (it
			// needs the daemon ctx). Per-server failures are reported, never fatal
			// — one broken server must not take the daemon down.
			name: "mcp servers",
			run: func() (string, error) {
				registered := k.MCPStore().Count()
				if registered == 0 {
					return "  mcp self-install : enabled (the agent can attach MCP servers at runtime; Edict mcp.install gates it)", nil
				}
				attached, failures := k.AttachEnabledMCPServers(ctx)
				fmt.Fprintf(stdout, "  mcp servers      : %d attached of %d registered\n", len(attached), registered)
				for name, aerr := range failures {
					fmt.Fprintf(stdout, "    %s: attach failed: %v\n", name, aerr)
				}
				return "", nil
			},
		},
	}
	for _, step := range bootSteps {
		desc, err := step.run()
		if err != nil {
			if step.fatal {
				fmt.Fprintf(stderr, "%s: %s: %v\n", brand.Binary, step.name, err)
				return 1
			}
			continue // best-effort steps report their own failures inside run
		}
		if desc != "" {
			fmt.Fprintln(stdout, desc)
		}
	}

	// Scheduled intents (autonomy) — fire operator-configured intents on a timer
	// through the governed loop. Runs on the daemon ctx (halt/shutdown stop it).
	// Off unless AGEZT_SCHEDULE is set.
	// AGEZT_SCHEDULE_NOTIFY=on (M152) delivers each scheduled run's answer to the
	// operator's configured channels, so a proactive digest reaches them rather than
	// only landing in the journal. Reuses the channel allowlists + sender.
	var onScheduledAnswer func(context.Context, string, string)
	if dcfg.Misc.ScheduleNotify && len(notifyTargets) > 0 {
		onScheduledAnswer = func(dctx context.Context, id, answer string) {
			deliverScheduled(dctx, channelSend, notifyTargets, id, answer)
		}
	}
	if schedDesc := buildCadence(ctx, k, stdout, onScheduledAnswer); schedDesc != "" {
		fmt.Fprintf(stdout, "  schedule         : %s\n", schedDesc)
	} else {
		fmt.Fprintf(stdout, "  schedule         : disabled (set AGEZT_SCHEDULE, e.g. \"1h=summarise new commits\")\n")
	}

	// Background update checker (M860): when updateSvc.CheckInterval > 0, a
	// goroutine fires on that interval. If an update is found, it is
	// auto-applied after the daemon drains (idle). The journal receives an
	// event so the update is auditable. The watchdog is signalled to restart
	// with the new binary; if the update failed the daemon stays running —
	// fail-safe: human must investigate.
	if updateSvc != nil && updateSvc.CheckInterval() > 0 {
		go startUpdateChecker(ctx, k, updateSvc, stdout, stderr)
		fmt.Fprintf(stdout, "  auto-update      : enabled (check every %s)\n", updateSvc.CheckInterval())
	} else if updateSvc != nil {
		fmt.Fprintf(stdout, "  auto-update      : check-only (set AGEZT_UPDATE_CHECK_INTERVAL to enable auto-apply)\n")
	}

	// Standing-order runner (SPEC-16 §4): wakes an order's governed plan on its
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
		if wh := dcfg.Misc.WebhookChannels; len(wh) > 0 {
			briefTargets["webhook"] = wh
		}
	}
	standingBrief := func(bctx context.Context, kind, text string) {
		for _, recip := range briefTargets[kind] {
			_ = channelSend(bctx, kind, recip, text)
		}
	}
	standingDesc, fireStandingNow := buildStandingRunner(ctx, k, standingBrief)
	srv.Bind(controlplane.LateDeps{StandingFire: fireStandingNow})
	fmt.Fprintf(stdout, "  standing orders  : %s\n", standingDesc)
	// Resume interrupted runs (M1002): re-dispatch any run that a prior restart —
	// stop/start, self-update, or hard kill — left mid-flight, so ongoing work
	// survives the interruption. Runs the roster + run infra armed just above.
	fmt.Fprintf(stdout, "  run resume       : %s\n", buildResumer(ctx, k))
	fmt.Fprintf(stdout, "  auto repair      : %s\n", selfrepair.WireAutoRepair(ctx, k, baseDir, boardStore, boardNotify))

	// Workflow triggers (M799): arm cron/event triggers for ENABLED workflows.
	// The runner consults the store live, so canvas/CLI saves take effect
	// without a restart; each firing runs the graph under its own correlation
	// (the workflow.* journal arc is the audit trail either way).
	wfFire := func(fctx context.Context, w workflow.Workflow, payload any, reason string) {
		rctx, cancel := context.WithTimeout(fctx, 15*time.Minute)
		defer cancel()
		_, _ = k.RunWorkflow(rctx, k.NewCorrelation(), w.Name, payload)
	}
	if err := workflow.StartTriggers(ctx, k.Bus(), k.Workflows(), workflow.RunnerConfig{}, wfFire); err != nil {
		fmt.Fprintf(stdout, "  workflows        : trigger runner failed to start: %v\n", err)
	} else {
		cron, evt, hook := 0, 0, 0
		for _, w := range k.Workflows().List() {
			if !w.Enabled {
				continue
			}
			switch w.TriggerSpec().Kind {
			case "cron":
				cron++
			case "event":
				evt++
			case "webhook":
				hook++
			}
		}
		fmt.Fprintf(stdout, "  workflows        : %d defined (%d cron + %d event + %d webhook trigger(s) armed)\n", k.Workflows().Count(), cron, evt, hook)
	}

	fmt.Fprintf(stdout, "  client commands  : %s run | halt | resume | why <id> | journal verify\n", brand.CLI)
	fmt.Fprintf(stdout, "Press Ctrl+C to stop.\n")

	// Stream events to stdout so the operator sees activity — but SKIP the
	// high-rate ephemeral chunks (llm.token, llm.reasoning). With autonomous
	// agents running, those fire hundreds of times per run and bury the console
	// in "[evt seq=0 kind=llm.reasoning …]" noise (M826; mirrors the CLI filter
	// from M819). The meaningful lifecycle events (task/tool/run/…) still print.
	sub, err := k.Bus().Subscribe(">", 256)
	if err == nil {
		go func() {
			for ev := range sub.C {
				if ev.Kind == event.KindLLMToken || ev.Kind == event.KindLLMReasoning {
					continue
				}
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
	// Durable resume (M1002): classify in-flight runs as resumable and notify them
	// BEFORE the drain/cancel below, so a run still cancelled at the end of the
	// drain window keeps its ticket (→ resumed on restart) rather than being
	// recorded as operator-cancelled and dropped. Cooperating agents also get the
	// whole drain window to wrap up. Idempotent with the Suspend inside Close.
	if k != nil {
		if n := k.Suspend("shutdown"); n > 0 {
			fmt.Fprintf(stdout, "  suspend: %d in-flight run(s) marked to resume on restart\n", n)
		}
	}
	// Deliberately a LIVE env read at shutdown time (not daemonconfig.Load):
	// the Config Center live-applies edits via os.Setenv, so an operator can
	// retune the drain window without a restart.
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
	return func(hctx context.Context, msg channel.UnifiedMessage, corr string) (channel.Reply, error) {
		intent := msg.Text
		if h := channel.ConversationHistory(k.Journal(), msg.ChannelKind, msg.ChannelID, msg.ThreadID, msg.Sender, limit); h != "" {
			intent = h
		}
		// Inbound image attachments (M247): forward them to the run the same way
		// the control plane and OpenAI API do, so a photo sent to the bot reaches
		// a vision model. An image with no caption gets a default instruction.
		if len(msg.Images) > 0 {
			var caption string
			if err := visionGate(k, "", msg.Images); err != nil {
				// The active model can't see images. Vision SIDECAR (M821): a keyed
				// vision model describes the image and we inject that text into the
				// run, so a non-vision primary still "reads" the photo instead of
				// failing. If NO vision model is keyed, persist the image anyway
				// (so it's not lost) and surface the clear gate error.
				c, derr := k.DescribeImages(hctx, corr, msg.Images, "")
				if derr != nil {
					if errors.Is(derr, kernelruntime.ErrNoVisionModel) {
						persistInboundImages(k, msg, corr, "")
						return channel.Reply{}, err
					}
					return channel.Reply{}, derr
				}
				caption = c
				if strings.TrimSpace(intent) == "" {
					intent = "Describe the attached image(s)."
				}
				intent += "\n\n[Image description (analyzed by a vision model):\n" + caption + "\n]"
			} else {
				hctx = kernelruntime.WithImages(hctx, msg.Images)
				if strings.TrimSpace(intent) == "" {
					intent = "Describe the attached image(s)."
				}
			}
			// Persist the inbound image(s) as browsable artifacts (M822) — keyed to
			// this run's correlation, with the vision caption (if any) attached.
			persistInboundImages(k, msg, corr, caption)
		}
		// Inbound voice notes: transcribe them so a voice message "just works" —
		// the agent reads the transcript like any text. Best-effort: if no STT is
		// configured, or transcription fails, the audio is still persisted as an
		// artifact (below) and the run proceeds on whatever text there was.
		if len(msg.Audio) > 0 {
			if v := k.Voice(); v != nil && v.HasSTT() {
				var transcripts []string
				for _, du := range msg.Audio {
					_, data, ok := decodeDataURL(du)
					if !ok || len(data) == 0 {
						continue
					}
					if txt, terr := v.Transcribe(hctx, data, "voice.ogg"); terr == nil && strings.TrimSpace(txt) != "" {
						transcripts = append(transcripts, strings.TrimSpace(txt))
					}
				}
				if len(transcripts) > 0 {
					joined := strings.Join(transcripts, "\n")
					if strings.TrimSpace(intent) == "" {
						intent = joined
					} else {
						intent += "\n\n[Voice message transcript:\n" + joined + "\n]"
					}
				}
			}
			persistInboundAudio(k, msg, corr)
		}
		text, rerr := k.RunWith(hctx, corr, intent)
		reply := channel.Reply{Text: text}
		// Voice-in → voice-out: if the user sent a voice message and TTS is
		// configured, speak the answer back as an audio attachment so the
		// conversation stays in voice (opt out with AGEZT_VOICE_REPLY=off).
		if rerr == nil && len(msg.Audio) > 0 && strings.TrimSpace(text) != "" && voiceReplyEnabled() {
			if v := k.Voice(); v != nil && v.HasTTS() {
				if audio, mime, serr := v.Speak(hctx, text); serr == nil && len(audio) > 0 {
					reply.Attachments = append(reply.Attachments, channel.Attachment{
						Kind: "audio", Data: audio, MIME: mime, Filename: "reply" + audioExt(mime),
					})
				}
			}
		}
		return reply, rerr
	}
}

// voiceReplyEnabled reports whether voice-in→voice-out is on (default yes).
func voiceReplyEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "VOICE_REPLY")))
	return v != "off" && v != "0" && v != "false" && v != "no"
}

// audioExt maps a TTS MIME type to a file extension for the outbound clip.
func audioExt(mime string) string {
	switch {
	case strings.Contains(mime, "ogg"), strings.Contains(mime, "opus"):
		return ".ogg"
	case strings.Contains(mime, "mpeg"), strings.Contains(mime, "mp3"):
		return ".mp3"
	case strings.Contains(mime, "wav"):
		return ".wav"
	case strings.Contains(mime, "aac"), strings.Contains(mime, "m4a"), strings.Contains(mime, "mp4"):
		return ".m4a"
	default:
		return ".ogg"
	}
}

// persistInboundAudio saves each inbound channel audio clip (voice note) as a
// browsable artifact entry, keyed to the run correlation. Best-effort: a
// decode/store failure for one clip is skipped, never fatal to the run.
func persistInboundAudio(k *kernelruntime.Kernel, msg channel.UnifiedMessage, corr string) {
	idx := k.ArtifactIndex()
	if idx == nil || len(msg.Audio) == 0 {
		return
	}
	now := time.Now().UnixMilli()
	for n, du := range msg.Audio {
		mime, data, ok := decodeDataURL(du)
		if !ok || len(data) == 0 {
			continue
		}
		_, _ = idx.PutEntry(artifact.Entry{
			Kind:   "audio",
			Source: msg.ChannelKind,
			Sender: msg.Sender,
			Corr:   corr,
			Mime:   mime,
			Name:   fmt.Sprintf("%s-audio-%d%s", msg.ChannelKind, n+1, extForMime(mime)),
		}, data, now)
	}
}

// persistInboundImages saves each inbound channel image as a browsable artifact
// entry (M822), keyed to the run correlation, with the vision caption (if the
// sidecar ran) attached. Best-effort: a decode/store failure for one image is
// logged-by-omission, never fatal to the run. Returns the new entry ids.
func persistInboundImages(k *kernelruntime.Kernel, msg channel.UnifiedMessage, corr, caption string) []string {
	idx := k.ArtifactIndex()
	if idx == nil || len(msg.Images) == 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	var ids []string
	for n, du := range msg.Images {
		mime, data, ok := decodeDataURL(du)
		if !ok || len(data) == 0 {
			continue
		}
		e, err := idx.PutEntry(artifact.Entry{
			Kind:    "image",
			Source:  msg.ChannelKind,
			Sender:  msg.Sender,
			Corr:    corr,
			Mime:    mime,
			Name:    fmt.Sprintf("%s-image-%d%s", msg.ChannelKind, n+1, extForMime(mime)),
			Caption: caption,
		}, data, now)
		if err == nil {
			ids = append(ids, e.ID)
		}
	}
	return ids
}

// decodeDataURL parses a data: URL (data:<mime>[;base64],<payload>) into its mime
// and decoded bytes. ok=false for anything that isn't a data URL. Base64 is the
// only encoding channels produce for images; a non-base64 payload is returned raw.
func decodeDataURL(s string) (mime string, data []byte, ok bool) {
	if !strings.HasPrefix(s, "data:") {
		return "", nil, false
	}
	rest := s[len("data:"):]
	comma := strings.IndexByte(rest, ',')
	if comma < 0 {
		return "", nil, false
	}
	meta, payload := rest[:comma], rest[comma+1:]
	mime = meta
	base64Encoded := false
	if i := strings.IndexByte(meta, ';'); i >= 0 {
		mime = meta[:i]
		base64Encoded = strings.Contains(meta[i:], "base64")
	}
	if base64Encoded {
		b, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return "", nil, false
		}
		return mime, b, true
	}
	return mime, []byte(payload), true
}

// extForMime maps the common image mimes to a file extension for the artifact's
// display name; unknown types get no extension.
func extForMime(mime string) string {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "audio/ogg", "audio/opus":
		return ".ogg"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/mp4", "audio/m4a", "audio/x-m4a":
		return ".m4a"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	default:
		return ""
	}
}

// collectChannels reports the configured messaging channels for `agt status`
// (M141), read-only from the same env the buildX functions consume. A channel is
// listed when its token is set; Inbound reflects whether it can actually receive
// and act on commands (Telegram always can; Slack/Discord need a listen addr plus
// the inbound secret/public key), so a half-configured webhook channel shows up
// as outbound-only rather than silently looking active.
// collectChannels derives `agt status`'s configured-channel list from the
// channel manifest registry (LD-7). It used to be a hand-maintained per-kind
// predicate list that had silently drifted to cover 11 of the 34 registered
// kinds; deriving from Manifest.RequiredEnv/AddrEnv/AllowlistEnv/InboundEnv
// means a newly registered channel is status-visible with no edit here.
func collectChannels() []controlplane.ChannelInfo {
	env := func(name string) string { return strings.TrimSpace(os.Getenv(name)) }
	var out []controlplane.ChannelInfo
	for _, m := range channel.Manifests() {
		configured := len(m.RequiredEnv) > 0
		for _, e := range m.RequiredEnv {
			if env(e) == "" {
				configured = false
				break
			}
		}
		// The generic webhook is usable outbound-only with just an outbound
		// URL — the one kind whose "configured" predicate is an OR the
		// manifest's all-required semantics can't express.
		if !configured && m.Kind == "webhook" && env("AGEZT_WEBHOOK_OUTBOUND_URL") != "" {
			configured = true
		}
		if !configured {
			continue
		}
		inbound := m.Duplex
		for _, e := range m.InboundEnv {
			if env(e) == "" {
				inbound = false
				break
			}
		}
		info := controlplane.ChannelInfo{Kind: m.Kind, Inbound: inbound}
		if m.AddrEnv != "" {
			info.Addr = env(m.AddrEnv)
		}
		if m.AllowlistEnv != "" {
			info.Allowlist = len(splitNonEmpty(env(m.AllowlistEnv)))
		}
		out = append(out, info)
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

// chanInstance is one configured account-instance of a channel kind (multi-account).
type chanInstance struct {
	key  string // channel.InstanceKey(kind, label): bare kind for the default, "kind#label" otherwise
	desc string
	ch   channel.Channel
	sink pulse.BriefSink
}

// wireInstances converts channelwire's built instances (the factory-migrated
// channels, Phase 2.1) into the daemon's chanInstance shape so the existing
// startInstances / allInsts / registerInstances wiring stays untouched.
func wireInstances(insts []channelwire.Instance) []chanInstance {
	var out []chanInstance
	for _, in := range insts {
		out = append(out, chanInstance{key: in.Key, desc: in.Desc, ch: in.Channel, sink: in.Sink})
	}
	return out
}

// startInstances starts each instance's read loop and logs it; logs a single
// "disabled" line for the kind when none are configured.
func startInstances(ctx context.Context, stdout io.Writer, kind, label, disabledHint string, insts []chanInstance) {
	if len(insts) == 0 {
		if disabledHint != "" {
			fmt.Fprintf(stdout, "  %-16s : %s\n", label, disabledHint)
		}
		return
	}
	for _, in := range insts {
		go in.ch.Start(ctx)
		who := in.key
		if who == kind {
			who = "default"
		}
		fmt.Fprintf(stdout, "  %-16s : %s [%s]\n", label, in.desc, who)
	}
}

// instanceSinks collects the non-nil brief sinks across instance groups.
func instanceSinks(groups ...[]chanInstance) []pulse.BriefSink {
	var out []pulse.BriefSink
	for _, g := range groups {
		for _, in := range g {
			if in.sink != nil {
				out = append(out, in.sink)
			}
		}
	}
	return out
}

// registerInstances maps every instance into liveChannels by its instance key.
func registerInstances(live map[string]channel.Channel, groups ...[]chanInstance) {
	for _, g := range groups {
		for _, in := range g {
			live[in.key] = in.ch
		}
	}
}

// instanceMatch returns the instance keys a send target addresses: an exact
// "kind#label" key, or every "kind"/"kind#*" key when target is a bare kind
// (fan-out across all accounts of that kind). For a single-account kind this is
// exactly one key — identical to the pre-multi-account behavior.
func instanceMatch(keys []string, target string) []string {
	if strings.Contains(target, "#") {
		for _, k := range keys {
			if k == target {
				return []string{target}
			}
		}
		return nil
	}
	var out []string
	for _, k := range keys {
		if base, _, _ := strings.Cut(k, "#"); base == target {
			out = append(out, k)
		}
	}
	return out
}

// liveChannelKeys returns the instance keys of the live channel map (used to
// record per-account live state for the Channels UI).
func liveChannelKeys(live map[string]channel.Channel) []string {
	out := make([]string, 0, len(live))
	for k := range live {
		out = append(out, k)
	}
	return out
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

func startWorkboardSweepTicker(ctx context.Context, k *kernelruntime.Kernel, stdout io.Writer) string {
	raw := os.Getenv(brand.EnvPrefix + "WORKBOARD_SWEEP_EVERY")
	if raw == "" {
		return ""
	}
	every, err := time.ParseDuration(raw)
	if err != nil || every <= 0 {
		fmt.Fprintf(stdout, "  workboard sweep  : invalid AGEZT_WORKBOARD_SWEEP_EVERY %q (%v) - on-demand only\n", raw, err)
		return ""
	}
	staleAfter := 10 * time.Minute
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WORKBOARD_STALE_AFTER")); spec != "" {
		if parsed, perr := time.ParseDuration(spec); perr == nil && parsed > 0 {
			staleAfter = parsed
		} else {
			fmt.Fprintf(stdout, "  workboard sweep  : invalid AGEZT_WORKBOARD_STALE_AFTER %q (%v) - using %s\n", spec, perr, staleAfter)
		}
	}
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				corr := "workboard-sweep-" + ulid.New()
				tasks, err := k.SweepStaleWorkboardClaims(corr, "workboard-sweeper", staleAfter, 100)
				if err != nil {
					fmt.Fprintf(stdout, "workboard sweep failed: %v\n", err)
					continue
				}
				if len(tasks) > 0 {
					fmt.Fprintf(stdout, "workboard sweep reclaimed %d stale claim(s)\n", len(tasks))
				}
			}
		}
	}()
	return "every " + every.String() + " (stale after " + staleAfter.String() + ")"
}

// startBrainDistillTicker starts a periodic brain-distillation pass when
// AGEZT_BRAIN_DISTILL_EVERY is a valid positive duration, on the daemon ctx
// (so halt/shutdown stop it). Returns a banner description, or "" when no
// timer is configured. Mirrors the reflection ticker.
func startBrainDistillTicker(ctx context.Context, k *kernelruntime.Kernel, stdout io.Writer) string {
	raw := os.Getenv(brand.EnvPrefix + "BRAIN_DISTILL_EVERY")
	if raw == "" {
		return ""
	}
	every, err := time.ParseDuration(raw)
	if err != nil || every <= 0 {
		fmt.Fprintf(stdout, "  brain distill    : invalid AGEZT_BRAIN_DISTILL_EVERY %q (%v) — on-demand only\n", raw, err)
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
				corr := "brain-distill-" + ulid.New()
				if _, err := k.DistillBrain(ctx, corr); err != nil {
					fmt.Fprintf(stdout, "brain-distill pass failed: %v\n", err)
				}
			}
		}
	}()
	return "every " + every.String()
}

// startProfileDistillTicker runs the operator-profile synthesis on a low daily
// cadence (M1000) when the profile feature is on, so AGEZT learns who the
// operator is without being asked. Default 24h; AGEZT_USER_PROFILE_EVERY overrides
// (operators can also schedule the profile_distill system task at a custom cadence
// or run `agt memory profile`). Returns a banner description, or "" when off.
func startProfileDistillTicker(ctx context.Context, k *kernelruntime.Kernel, on bool, stdout io.Writer) string {
	if !on {
		return ""
	}
	every := 24 * time.Hour
	if raw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "USER_PROFILE_EVERY")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			every = d
		} else {
			fmt.Fprintf(stdout, "  user profile     : invalid AGEZT_USER_PROFILE_EVERY %q (%v) — using 24h\n", raw, err)
		}
	}
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				corr := "profile-distill-" + ulid.New()
				if _, err := k.DistillProfile(ctx, corr); err != nil {
					fmt.Fprintf(stdout, "profile-distill pass failed: %v\n", err)
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

// buildAlertNotify starts the alert → channel notifier (M782) when
// AGEZT_ALERT_NOTIFY is on and at least one channel is configured. Knobs:
//
//	AGEZT_ALERT_NOTIFY           1/on/true enables (default off — opt-in)
//	AGEZT_ALERT_NOTIFY_LEVEL     "critical" = criticals only; default warning+
//	AGEZT_ALERT_NOTIFY_COOLDOWN  per-alert (kind+run) repeat suppression, default 5m
//	AGEZT_ALERT_NOTIFY_MAX       global cap per 10-minute window, default 12
func buildAlertNotify(ctx context.Context, k *kernelruntime.Kernel, sink pulse.BriefSink) string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ALERT_NOTIFY"))) {
	case "1", "on", "true", "yes":
	default:
		return "disabled (set AGEZT_ALERT_NOTIFY=1 to push warning/critical alerts to channels)"
	}
	if sink == nil {
		return "enabled but NO channel configured — configure Telegram/Slack/… first"
	}
	cfg := alerter.Config{MinLevel: alerter.ParseLevel(os.Getenv(brand.EnvPrefix + "ALERT_NOTIFY_LEVEL"))}
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ALERT_NOTIFY_COOLDOWN")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.Cooldown = d
		}
	}
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ALERT_NOTIFY_MAX")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxPerWindow = n
		}
	}
	// Mute window (M815): hold warnings during a daily quiet window (criticals
	// always break through). Reuses Pulse's "START-END" 24h form, e.g. "0-7".
	cfg.Mute = pulse.ParseQuietHours(os.Getenv(brand.EnvPrefix + "ALERT_NOTIFY_MUTE"))
	// Per-source routing (M815): drop noisy categories (run/egress/budget/
	// provider/kernel) outright while keeping the rest.
	cfg.MuteSources = alerter.ParseMuteSources(os.Getenv(brand.EnvPrefix + "ALERT_NOTIFY_MUTE_SOURCES"))
	if !alerter.Start(ctx, k.Bus(), sink, cfg) {
		return "disabled"
	}
	extra := ""
	if cfg.Mute.Enabled {
		extra += fmt.Sprintf("; muted %s (criticals still break through)", cfg.Mute.Spec())
	}
	if len(cfg.MuteSources) > 0 {
		srcs := make([]string, 0, len(cfg.MuteSources))
		for s := range cfg.MuteSources {
			srcs = append(srcs, s)
		}
		sort.Strings(srcs)
		extra += "; sources muted: " + strings.Join(srcs, ",")
	}
	return fmt.Sprintf("on (level≥%s → channels; repeats suppressed; flood-capped%s)", cfg.MinLevel, extra)
}

// standingTrustCeiling computes the effective trust ceiling for a standing-order
// firing (M999): the MORE restrictive (lower edict level) of the order's explicit
// max_trust and the level implied by its initiative mode (inform_only→L0, ask→L1).
// Returns (level, false) when neither caps — the firing runs uncapped, the
// pre-M999 default for orders with empty mode and no max_trust.
//
// Fail-safe for act_or_ask (VULN-003): an order whose mode is the explicit
// autonomous dial "act_or_ask" but which leaves max_trust blank would otherwise
// fire UNCAPPED (L4) — the most permissive setting silently meaning "no clamp",
// despite the operator having chosen a mode whose very name implies bounded
// autonomy. Because such a firing is unattended and its trigger payload can be
// attacker-influenced (VULN-004), it must not run uncapped by omission. We default
// it to L2 (ask-first), mirroring the seeded guardian-initiative responder. An
// operator who genuinely wants uncapped act_or_ask autonomy opts in explicitly by
// setting max_trust=L4. Empty/unset mode is left untouched so pre-M999 and
// non-initiative orders keep their existing behaviour.
func standingTrustCeiling(in standing.Initiative) (edict.TrustLevel, bool) {
	ceil := edict.LevelAllow
	have := false
	if lvl, err := edict.ParseTrustLevel(in.MaxTrust); err == nil {
		ceil, have = lvl, true
	}
	if modeLvl, capped := in.Mode.MaxAutonomyTrust(); capped {
		if lvl, err := edict.ParseTrustLevel(modeLvl); err == nil {
			if !have || lvl < ceil {
				ceil = lvl
			}
			have = true
		}
	}
	if !have && in.Mode == standing.InitiativeActOrAsk {
		ceil, have = edict.LevelAskFirst, true
	}
	return ceil, have
}

// buildStandingRunner starts the event-trigger half of standing orders
// (SPEC-16 §4): when a journal event matches an enabled order's event trigger,
// the order's governed plan is launched as a run (bounded by its budget ceiling)
// and a standing.fired event is journaled. Cron triggers are handled by the
// schedule engine, not here.
func buildStandingRunner(ctx context.Context, k *kernelruntime.Kernel, brief func(ctx context.Context, kind, text string)) (string, func(id string) bool) {
	fire := func(fctx context.Context, o standing.Order, subject string, triggerPayload map[string]any) {
		// A fired order launches a full governed run (provider/tool/plugin code) and
		// then briefs over the network — any of which can panic. This goroutine is
		// dispatched with a bare `go fire(...)` by the runner/cron loop, so its own
		// recover() does NOT cover us; without this defer a single bad run would take
		// down the whole daemon. Contain the panic to this order and journal it as a
		// standing.error so it stays diagnosable (`agt journal`).
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "standing order %q panicked: %v\n", o.Name, r)
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
		// Run AS a named agent (M790): resolve the order's roster profile up
		// front — an unknown or paused agent journals a standing.error instead
		// of silently running as the default identity (mirrors `agt run --agent`).
		var prof *roster.Profile
		if slug := strings.TrimSpace(o.Agent); slug != "" {
			p, ok := k.Roster().Get(slug)
			if !ok || p.Retired || !p.Enabled {
				reason := "unknown agent " + slug
				if ok {
					reason = "agent " + p.Slug + " is paused"
					if p.Retired {
						reason = "agent " + p.Slug + " is retired — revive it first"
					}
				}
				_, _ = k.Bus().Publish(event.Spec{
					Subject: "standing." + o.ID,
					Kind:    event.KindStandingError,
					Actor:   "standing",
					Payload: map[string]any{"id": o.ID, "name": o.Name, "trigger_subject": subject, "agent": slug, "reason": reason},
				})
				return
			}
			if !p.AllowsDirectCall() {
				manager := strings.TrimSpace(p.ParentAgent)
				if manager == "" {
					manager = strings.TrimSpace(p.OwnerAgent)
				}
				hint := "route the work through its parent/owner agent"
				if manager != "" {
					hint = "wake " + manager + " or delegate through it"
				}
				_, _ = k.Bus().Publish(event.Spec{
					Subject: "standing." + o.ID,
					Kind:    event.KindStandingError,
					Actor:   "standing",
					Payload: map[string]any{
						"id":              o.ID,
						"name":            o.Name,
						"trigger_subject": subject,
						"agent":           p.Slug,
						"reason":          "agent " + p.Slug + " is a managed sub-agent and cannot be fired directly by a standing order; " + hint,
					},
				})
				return
			}
			prof = &p
		}
		// Ground the run in the order's scope (SPEC-16 §4): the agent is told what
		// this standing order watches.
		intent = standing.ScopedIntent(o, intent)
		intent = standing.TriggeredIntent(intent, subject, triggerPayload)
		firedPayload := map[string]any{"id": o.ID, "name": o.Name, "trigger_subject": subject, "intent": intent}
		if len(triggerPayload) > 0 {
			firedPayload["trigger_payload"] = triggerPayload
		}
		if prof != nil {
			firedPayload["agent"] = prof.Slug // who this firing runs AS (M790)
			// Carry the same autonomy runbook schedule.fired does, so a standing
			// wake is traceable as event payload -> status -> detail -> activity.
			firedPayload["autonomy_runbook"] = agentAutonomyRunbookPayload(*prof)
		}
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "standing." + o.ID,
			Kind:          event.KindStandingFired,
			Actor:         "standing",
			CorrelationID: corr,
			Payload:       firedPayload,
		})
		rctx := fctx
		if prof != nil {
			// Soul → system, model + fallbacks → chain, memory scope (M790).
			rctx = kernelruntime.WithAgentProfile(rctx, *prof)
			// The profile's per-run ceiling is the DEFAULT; the order's own wins.
			if o.Initiative.BudgetPerRunMc <= 0 && prof.MaxCostMc > 0 {
				rctx = kernelruntime.WithMaxCost(rctx, prof.MaxCostMc)
			}
		}
		rctx = kernelruntime.WithWakeContext(rctx, kernelruntime.WakeContext{
			Source:         "standing",
			Reason:         "event",
			StandingID:     o.ID,
			StandingName:   o.Name,
			TriggerSubject: subject,
		})
		if o.Initiative.BudgetPerRunMc > 0 {
			rctx = kernelruntime.WithMaxCost(rctx, o.Initiative.BudgetPerRunMc)
		}
		// Cap autonomous action (SPEC-16 §4, M999): the effective ceiling is the MORE
		// restrictive of the order's max_trust and the trust implied by its initiative
		// MODE (inform_only→L0/no-tools, ask→L1/approval-each). A normally auto-allowed
		// tool is downgraded to Ask/Deny within this run. Before M999 the mode was
		// stored but never enforced — only max_trust gated; now the mode is a real dial.
		if lvl, ok := standingTrustCeiling(o.Initiative); ok {
			rctx = kernelruntime.WithTrustCeiling(rctx, lvl)
		}
		// Do-it-for-sure firings (M655): when the order carries an assure budget,
		// each firing runs-verifies-retries until the plan is judged complete (or
		// the budget is spent) — symmetric with assured schedules, so an
		// event/cron-triggered order actually gets its task done.
		var answer string
		if o.Assure > 0 {
			answer, _, _ = k.RunAssured(rctx, corr, intent, o.Assure)
		} else if prof != nil && prof.RetryPolicy != nil && prof.RetryPolicy.MaxAttempts > 1 {
			answer, _ = k.RunWithRetry(rctx, corr, intent, *prof.RetryPolicy)
		} else {
			answer, _ = k.RunWith(rctx, corr, intent)
		}
		// Brief the result to the order's configured channel (SPEC-16 §4 briefing).
		if text, ok := standing.BriefText(o, answer); ok && brief != nil {
			brief(fctx, o.BriefingChan, text)
		}
	}
	// fireNow launches an order on demand (M765), through the same governed fire path
	// the triggers use — so "run now" from the console/CLI behaves exactly like a real
	// firing. Returns false for an unknown id. Works even if auto-triggers are off.
	fireNow := func(id string) bool {
		o, ok := k.Standing().Get(id)
		if !ok {
			return false
		}
		go fire(ctx, o, "manual", nil)
		return true
	}

	evOK := standing.StartRunner(ctx, k.Bus(), k.Standing(), standing.RunnerConfig{}, fire)
	cronOK := standing.StartCron(ctx, k.Standing(), nil, fire)
	if !evOK && !cronOK {
		return "disabled", fireNow
	}
	return fmt.Sprintf("on (event + cron triggers; %d order(s) defined)", k.Standing().Count()), fireNow
}

// buildResumer re-dispatches runs that were interrupted by a restart (M1002). On
// boot, any ticket left in the resume store is a run that did not finish cleanly
// (a clean finish deletes its ticket) — self-update, stop/start, or hard kill.
// Each resumable ticket is re-dispatched through the SAME governed entry point it
// used, seeded with its saved conversation so a Kind=run continues where it left
// off. The resumer owns the ticket lifecycle for the runs it launches: it marks
// them owned (so the inner RunWith/wrapper neither recreates the ticket — which
// would reset the crash-loop counter — nor deletes it) and finalizes on return.
//
// Safety rails: a ticket that used an un-reconstructable per-run override, whose
// agent is gone, or that has exceeded AGEZT_RESUME_MAX_ATTEMPTS is quarantined
// (moved aside for postmortem) rather than re-dispatched, so a poison run can
// never wedge boot. The attempt counter is incremented and fsynced BEFORE
// dispatch, so a resume that hard-crashes the daemon still records the attempt.
func buildResumer(ctx context.Context, k *kernelruntime.Kernel) string {
	store := k.ResumeStore()
	if store == nil {
		return "disabled"
	}
	tickets, err := store.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resume: list tickets: %v\n", err)
		return "error"
	}
	if len(tickets) == 0 {
		return "none pending"
	}
	maxAttempts := 3
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "RESUME_MAX_ATTEMPTS")); v != "" {
		if n, aerr := strconv.Atoi(v); aerr == nil && n > 0 {
			maxAttempts = n
		}
	}

	quarantine := func(t *resume.Ticket, reason string) {
		_ = store.Quarantine(t.Corr)
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "run.resume.quarantined",
			Kind:          event.KindAnomalyDetected,
			Actor:         "resume",
			CorrelationID: t.Corr,
			Payload:       map[string]any{"corr": t.Corr, "agent": t.AgentSlug, "attempts": t.Attempts, "reason": reason, "severity": "warning"},
		})
	}

	resumed := 0
	for _, t := range tickets {
		if !t.Resumable {
			quarantine(t, "non-resumable (per-run override cannot be reconstructed)")
			continue
		}
		if t.Attempts >= maxAttempts {
			quarantine(t, fmt.Sprintf("exceeded resume attempt cap (%d)", maxAttempts))
			continue
		}
		// Resolve the agent this run ran AS. If it named an agent that is now gone
		// or disabled, quarantine rather than silently resume under the default
		// identity (which would run with the wrong persona/tools/ceilings).
		var prof *roster.Profile
		if slug := strings.TrimSpace(t.AgentSlug); slug != "" {
			p, ok := k.Roster().Get(slug)
			if !ok || p.Retired || !p.Enabled {
				quarantine(t, "agent "+slug+" is gone, retired, or disabled")
				continue
			}
			prof = &p
		}
		// Record the attempt durably BEFORE dispatch: a resume that hard-crashes the
		// daemon must still have counted, or the crash-loop guard never trips.
		if _, aerr := store.IncrementAttempt(t.Corr); aerr != nil {
			fmt.Fprintf(os.Stderr, "resume: increment attempt for %s: %v\n", t.Corr, aerr)
			continue
		}

		// Rebuild the run context from the ticket's resolved fields. The resumer
		// OWNS the ticket, so mark it owned to stop the inner RunWith recreating it.
		rctx := kernelruntime.WithResumeOwned(ctx, t.Kind)
		if prof != nil {
			rctx = kernelruntime.WithAgentProfile(rctx, *prof)
		}
		rctx = kernelruntime.WithWakeContext(rctx, kernelruntime.WakeContext{
			Source:         strutil.FirstNonEmpty(t.WakeSource, "resume"),
			Reason:         "resumed",
			ScheduleID:     t.WakeScheduleID,
			StandingID:     t.WakeStandingID,
			StandingName:   t.WakeStandingName,
			TriggerSubject: t.WakeTriggerSubject,
		})
		// Governance invariant (M1002): a tightened trust ceiling MUST be re-applied
		// so a resumed run never silently regains authority.
		if t.TrustCeiling != nil {
			rctx = kernelruntime.WithTrustCeiling(rctx, edict.TrustLevel(*t.TrustCeiling))
		}
		if t.MaxCostMc > 0 {
			rctx = kernelruntime.WithMaxCost(rctx, t.MaxCostMc)
		}
		if t.RunTimeoutMs > 0 {
			rctx = kernelruntime.WithRunTimeout(rctx, time.Duration(t.RunTimeoutMs)*time.Millisecond)
		}
		// Continue the interrupted conversation for a message-bearing run; assured/
		// retry re-run from the top (cheap re-verification), so they carry no seed.
		if t.Kind == resume.KindRun && len(t.Messages) > 0 {
			rctx = kernelruntime.WithResumeSeed(rctx, t.Messages, t.Iter)
		}

		resumed++
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "run.resumed",
			Kind:          event.KindInfo,
			Actor:         "resume",
			CorrelationID: t.Corr,
			Payload:       map[string]any{"corr": t.Corr, "agent": t.AgentSlug, "kind": t.Kind, "iter": t.Iter, "attempt": t.Attempts + 1, "seeded": len(t.Messages) > 0},
		})
		go func() {
			// Contain a panic to this run — a bad resumed run must not take down boot.
			defer func() {
				if r := recover(); r != nil {
					fmt.Fprintf(os.Stderr, "resume: run %s panicked: %v\n", t.Corr, r)
					k.ResumeFinalize(t.Corr, fmt.Errorf("resume panic: %v", r))
				}
			}()
			var rerr error
			switch t.Kind {
			case resume.KindAssured:
				budget := t.AssureBudget
				if budget <= 0 {
					budget = 1
				}
				_, _, rerr = k.RunAssured(rctx, t.Corr, t.Intent, budget)
			case resume.KindRetry:
				if prof != nil && prof.RetryPolicy != nil && prof.RetryPolicy.MaxAttempts > 1 {
					_, rerr = k.RunWithRetry(rctx, t.Corr, t.Intent, *prof.RetryPolicy)
				} else {
					_, rerr = k.RunWith(rctx, t.Corr, t.Intent)
				}
			default:
				_, rerr = k.RunWith(rctx, t.Corr, t.Intent)
			}
			// The resumer owns the ticket: clear it on a clean/failed terminal, keep
			// it if a NEW shutdown interrupted the resumed run.
			k.ResumeFinalize(t.Corr, rerr)
		}()
	}
	quarantined := len(tickets) - resumed
	if resumed == 0 {
		return fmt.Sprintf("none resumable (%d quarantined)", quarantined)
	}
	if quarantined > 0 {
		return fmt.Sprintf("%d run(s) resumed, %d quarantined", resumed, quarantined)
	}
	return fmt.Sprintf("%d run(s) resumed", resumed)
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
	total := "unbounded"
	if l.MaxTotal > 0 {
		total = fmt.Sprintf("≤%d", l.MaxTotal)
	}
	return fmt.Sprintf("depth≤%d, fan-out %s, total %s, spend %s", l.MaxDepth, fanout, total, spend)
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
		ent, ok := store.Get(id)
		if !ok {
			return fmt.Errorf("schedule %s: not found", id)
		}
		var prof *roster.Profile
		if slug := strings.TrimSpace(ent.Agent); slug != "" {
			p, ok := k.Roster().Get(slug)
			if !ok {
				return fmt.Errorf("schedule %s: unknown agent %s", id, slug)
			}
			if p.Retired {
				return fmt.Errorf("schedule %s: agent %s is retired — revive it first", id, p.Slug)
			}
			if !p.Enabled {
				return fmt.Errorf("schedule %s: agent %s is paused", id, p.Slug)
			}
			if !p.AllowsDirectCall() {
				manager := strings.TrimSpace(p.ParentAgent)
				if manager == "" {
					manager = strings.TrimSpace(p.OwnerAgent)
				}
				hint := "route the work through its parent/owner agent"
				if manager != "" {
					hint = "wake " + manager + " or delegate through it"
				}
				return fmt.Errorf("schedule %s: agent %s is a managed sub-agent and cannot be scheduled directly; %s", id, p.Slug, hint)
			}
			prof = &p
		}
		mctx := scheduledRunContext(runCtx, model, prof)
		mctx = kernelruntime.WithWakeContext(mctx, kernelruntime.WakeContext{
			Source:     "schedule",
			Reason:     ent.Target,
			ScheduleID: id,
		})
		if ent.Target == cadence.TargetWorkflow {
			var payload any
			if len(ent.Payload) > 0 {
				if err := json.Unmarshal(ent.Payload, &payload); err != nil {
					return fmt.Errorf("schedule %s: workflow payload: %w", id, err)
				}
			}
			_, _ = k.Bus().Publish(event.Spec{
				Subject:       "schedule.fired",
				Kind:          event.KindScheduleFired,
				Actor:         "schedule",
				CorrelationID: corr,
				Payload:       scheduleFiredEventPayload(id, intent, model, ent, prof),
			})
			return runScheduledTrackedTarget(mctx, k, corr, ent, intent, func(ctx context.Context) (string, error) {
				res, err := k.RunWorkflow(ctx, corr, ent.Workflow, payload)
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("workflow %s completed (%d nodes)", ent.Workflow, len(res.Executed)), nil
			})
		}
		if ent.Target == cadence.TargetSystemTask {
			_, _ = k.Bus().Publish(event.Spec{
				Subject:       "schedule.fired",
				Kind:          event.KindScheduleFired,
				Actor:         "schedule",
				CorrelationID: corr,
				Payload:       scheduleFiredEventPayload(id, intent, model, ent, prof),
			})
			return runScheduledTrackedTarget(mctx, k, corr, ent, intent, func(ctx context.Context) (string, error) {
				if err := systemtasks.Run(ctx, k, corr, id, ent.SystemTask); err != nil {
					return "", err
				}
				return "system task " + ent.SystemTask + " completed", nil
			})
		}
		if ent.Target == cadence.TargetTool {
			payload := ent.Payload
			if len(payload) == 0 {
				payload = json.RawMessage(`{}`)
			}
			_, _ = k.Bus().Publish(event.Spec{
				Subject:       "schedule.fired",
				Kind:          event.KindScheduleFired,
				Actor:         "schedule",
				CorrelationID: corr,
				Payload:       scheduleFiredEventPayload(id, intent, model, ent, prof),
			})
			return runScheduledTrackedTarget(mctx, k, corr, ent, intent, func(ctx context.Context) (string, error) {
				res, err := k.RunTool(ctx, corr, "schedule-"+id, ent.Tool, payload)
				if err != nil {
					return "", err
				}
				if res.IsError {
					return "", fmt.Errorf("tool %s failed: %s", ent.Tool, res.Output)
				}
				if strings.TrimSpace(res.Output) == "" {
					return "tool " + ent.Tool + " completed", nil
				}
				return res.Output, nil
			})
		}
		if ent.Target != cadence.TargetIntent {
			return fmt.Errorf("schedule %s: unknown target %q", id, ent.Target)
		}
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "schedule.fired",
			Kind:          event.KindScheduleFired,
			Actor:         "schedule",
			CorrelationID: corr,
			// schedule_id (M55) attributes the firing to its schedule entry, so
			// `agt schedule fires --id <sched>` can filter and `agt schedule list`
			// can show a schedule's last outcome.
			Payload: scheduleFiredEventPayload(id, intent, model, ent, prof),
		})
		// Do-it-for-sure firings (M654): when the entry carries an assure budget,
		// each firing runs-verifies-retries until the task is judged complete (or
		// the budget is spent), so an unattended schedule/continuous loop actually
		// gets its task done rather than firing once and hoping.
		var ans string
		var err error
		if ent.Assure > 0 {
			ans, _, err = k.RunAssured(mctx, corr, intent, ent.Assure)
		} else if prof != nil && prof.RetryPolicy != nil && prof.RetryPolicy.MaxAttempts > 1 {
			ans, err = k.RunWithRetry(mctx, corr, intent, *prof.RetryPolicy)
		} else {
			ans, err = k.RunWith(mctx, corr, intent)
		}
		// Deliver the scheduled run's answer to the operator's channels when
		// AGEZT_SCHEDULE_NOTIFY is on (M152): a proactive morning digest reaches
		// you instead of sitting silently in the journal. Only on success with a
		// non-empty answer; off entirely when onAnswer is nil.
		if err == nil && onAnswer != nil {
			onAnswer(runCtx, id, ans)
		}
		return err
	}
	eng := cadence.NewEngine(store, run, 0, stdout)
	// Injection tripwire (M886): a suspicious scheduled intent journals an
	// anomaly.detected warning on every firing (it still fires — default-allow).
	eng.Bus = k.Bus()
	// Backstop each firing with a deadline so a single hung run can't permanently
	// stall its schedule (its in-flight guard would never clear). Default 1h is
	// generous for any reasonable agentic run; AGEZT_SCHEDULE_RUN_TIMEOUT overrides
	// (a value of 0/"off" disables the backstop). Must be set before Start.
	eng.RunTimeout = time.Hour
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "SCHEDULE_RUN_TIMEOUT")); v != "" {
		if strings.EqualFold(v, "off") || v == "0" {
			eng.RunTimeout = 0
		} else if d, err := time.ParseDuration(v); err == nil && d > 0 {
			eng.RunTimeout = d
		}
	}
	k.SetScheduleEngine(eng)
	eng.Start(ctx)

	entries := store.List()
	if len(entries) == 0 {
		return "active (no schedules yet — add with `agt schedule add`)"
	}
	return cadence.Describe(entries)
}

func scheduledRunContext(runCtx context.Context, model string, prof *roster.Profile) context.Context {
	mctx := runCtx
	if prof != nil {
		mctx = kernelruntime.WithAgentProfile(mctx, *prof)
		if prof.MaxCostMc > 0 {
			mctx = kernelruntime.WithMaxCost(mctx, prof.MaxCostMc)
		}
	}
	model = strings.TrimSpace(model)
	if model != "" {
		mctx = kernelruntime.WithModel(mctx, model)
		mctx = kernelruntime.WithModelChain(mctx, []string{model})
	}
	return mctx
}

func scheduleFiredEventPayload(id, intent, model string, ent cadence.Entry, profs ...*roster.Profile) map[string]any {
	payload := map[string]any{
		"schedule_id": id,
		"intent":      intent,
		"model":       model,
		"target":      ent.Target,
		"agent":       ent.Agent,
	}
	if len(profs) > 0 && profs[0] != nil {
		payload["autonomy_runbook"] = agentAutonomyRunbookPayload(*profs[0])
	}
	switch ent.Target {
	case cadence.TargetWorkflow:
		payload["workflow"] = ent.Workflow
		payload["executor"] = "workflow"
		payload["uses_llm"] = true
	case cadence.TargetSystemTask:
		payload["system_task"] = ent.SystemTask
		if info, ok := systemtasks.Info(ent.SystemTask); ok {
			payload["executor"] = info.Executor
			payload["category"] = info.Category
			payload["effect_class"] = info.EffectClass
			payload["uses_llm"] = info.UsesLLM
		} else {
			payload["executor"] = "daemon"
			payload["uses_llm"] = false
		}
	case cadence.TargetTool:
		payload["tool"] = ent.Tool
		payload["executor"] = "tool"
		payload["uses_llm"] = false
	default:
		payload["executor"] = "agent"
		payload["uses_llm"] = true
	}
	return payload
}

// agentAutonomyRunbookPayload attaches the machine-readable wake contract to
// autonomous wake evidence (schedule.fired and standing.fired) when the firing
// resolves a concrete agent profile. It delegates to the canonical roster builder
// so operator, schedule, standing, and delegated wakes all carry an
// identically-shaped runbook through the journal.
func agentAutonomyRunbookPayload(p roster.Profile) map[string]any {
	return roster.AutonomyRunbook(p)
}

func runScheduledTrackedTarget(ctx context.Context, k *kernelruntime.Kernel, corr string, ent cadence.Entry, intent string, run func(context.Context) (string, error)) error {
	sf := schedulePayloadForEntry(ent, intent)
	action := controlplaneScheduleAction(sf)
	receivedPayload := map[string]any{
		"schedule_id":      ent.ID,
		"intent":           action,
		"scheduled_intent": intent,
		"target":           ent.Target,
		"agent":            ent.Agent,
		"workflow":         ent.Workflow,
		"system_task":      ent.SystemTask,
		"tool":             ent.Tool,
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "schedule.task",
		Kind:          event.KindTaskReceived,
		Actor:         "schedule",
		CorrelationID: corr,
		Payload:       receivedPayload,
	})

	answer, err := run(ctx)
	if err != nil {
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "schedule.task",
			Kind:          event.KindTaskFailed,
			Actor:         "schedule",
			CorrelationID: corr,
			Payload: map[string]any{
				"schedule_id": ent.ID,
				"target":      ent.Target,
				"reason":      "error",
				"error":       err.Error(),
			},
		})
		return err
	}
	k.CompleteAgentLifecycle(ctx, corr)
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "schedule.task",
		Kind:          event.KindTaskCompleted,
		Actor:         "schedule",
		CorrelationID: corr,
		Payload: map[string]any{
			"schedule_id": ent.ID,
			"target":      ent.Target,
			"answer":      truncateScheduledAnswer(answer),
			"iters":       0,
		},
	})
	return nil
}

type scheduledTargetPayload struct {
	ScheduleID string
	Intent     string
	Target     string
	Agent      string
	Workflow   string
	SystemTask string
	Tool       string
}

func schedulePayloadForEntry(ent cadence.Entry, intent string) scheduledTargetPayload {
	return scheduledTargetPayload{
		ScheduleID: ent.ID,
		Intent:     intent,
		Target:     ent.Target,
		Agent:      ent.Agent,
		Workflow:   ent.Workflow,
		SystemTask: ent.SystemTask,
		Tool:       ent.Tool,
	}
}

func controlplaneScheduleAction(p scheduledTargetPayload) string {
	switch p.Target {
	case "workflow":
		if p.Workflow != "" {
			return "run workflow " + p.Workflow
		}
	case "system_task":
		if p.SystemTask != "" {
			return "run system task " + p.SystemTask
		}
	case "tool":
		if p.Tool != "" {
			return "run tool " + p.Tool
		}
	}
	if p.Agent != "" && p.Intent != "" {
		return "wake " + p.Agent + ": " + p.Intent
	}
	if p.Intent != "" {
		return p.Intent
	}
	return p.ScheduleID
}

func truncateScheduledAnswer(s string) string {
	const max = 4096
	if len(s) <= max {
		return s
	}
	return s[:max] + "...[truncated]"
}

// startUpdateChecker runs the background update checker goroutine (M860).
// It fires on the configured CheckInterval. When an update is found, it
// auto-applies after the daemon goes idle (drain). The journal receives an
// event so the update is auditable. The watchdog is signalled to restart
// with the new binary.
// startUpdateChecker → boot_ops.go

// bootStep is one step of runDaemon's tool late-bind + seed/banner phase
// (Phase 2.6 3a). run performs the step: it may print its own (multi-line or
// conditional) banner output directly, or return a single ready-to-print banner
// line as desc ("" = nothing to print). A returned error aborts the daemon only
// when fatal is set; best-effort steps surface their own failures inside run
// and return nil. The table keeps the sequence scannable and gives a single
// place to time steps later.
type bootStep struct {
	name  string
	run   func() (desc string, err error)
	fatal bool
}

// pulseObserverAdmin adapts the live pulse engine to controlplane.PulseObservers
// (Phase 2.6 3b-i): the daemon owns the DiskUsage func, the warden and the state
// store the observer constructors need, so the control plane stays decoupled
// from kernel/pulse. Wired only when the engine is running (pulse enabled).
type pulseObserverAdmin struct {
	eng  *pulse.Engine
	ward warden.Engine
	st   *state.FileStore
}

// AddDiskObserver registers a runtime disk-space watch (M767).
func (a pulseObserverAdmin) AddDiskObserver(path string, minPct float64) (string, bool) {
	return a.eng.AddObserver(pulse.NewDiskObserver(path, minPct, pulse.DiskUsage)), true
}

// AddProbeObserver registers a runtime command-probe watch (M768) — the command
// runs through the warden each beat, like any agent shell call.
func (a pulseObserverAdmin) AddProbeObserver(name string, argv []string) (string, bool) {
	return a.eng.AddObserver(pulse.NewProbeObserver(name, argv, a.ward, a.st)), true
}

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
	// Self-health observer (M628): the daemon watches its OWN run/tool
	// reliability and briefs the operator when its health transitions
	// (healthy↔degraded↔critical) — proactive self-monitoring, not just the
	// reactive Analyst. On by default (the whole point is to watch unprompted);
	// AGEZT_PULSE_HEALTH=off disables it, =<float> overrides the tool-error-rate
	// degrade threshold (default 0.30).
	if hv := os.Getenv(brand.EnvPrefix + "PULSE_HEALTH"); !strings.EqualFold(hv, "off") {
		degradeAt := 0.0 // observer falls back to its default
		if f, err := strconv.ParseFloat(hv, 64); err == nil && f > 0 {
			degradeAt = f
		}
		obs = append(obs, pulse.NewHealthObserver(healthStatFromJournal(k), degradeAt, 0))
		parts = append(parts, "self:health")
	}
	useLLM := strings.EqualFold(os.Getenv(brand.EnvPrefix+"PULSE_LLM"), "on")
	// Autonomy level (M999): off|ask|act, default act. `act` makes Pulse EMIT a
	// pulse.initiative.act event on actionable observations; an autonomous run still
	// requires an ENABLED standing order bound to it (the seeded responder ships
	// disabled), so a fresh install is bold-by-default yet dormant until opt-in.
	initiative := pulse.ParseInitiative(os.Getenv(brand.EnvPrefix + "PULSE_INITIATIVE"))

	eng := pulse.New(pulse.Config{
		Bus:        k.Bus(),
		State:      k.State(),
		Warden:     ward,
		Provider:   k.Provider(),
		Model:      model,
		Relevance:  k.World(), // world-model relevance signal (SPEC-05 §3.4)
		Observers:  obs,
		Dial:       dial,
		Initiative: initiative,
		Cadence:    cadence,
		QuietHours: qh,
		UseLLM:     useLLM,
		Sink:       briefSink(stdout, extraSink),
	})
	observers := "no observers configured"
	if len(parts) > 0 {
		observers = strings.Join(parts, ",")
	}
	return eng, fmt.Sprintf("dial=%s initiative=%s cadence=%s observers=[%s]", dial, initiative, cadence, observers)
}

// healthStatFromJournal returns a pulse.HealthStatFunc that samples the
// daemon's recent reliability from the tail of its own journal: tool.invoked /
// tool.result(error) for tool reliability, and task.completed / task.failed for
// run reliability. It reads only the last healthWindow events so the scan is
// cheap and the assessment reflects RECENT behaviour, not all-time history.
func healthStatFromJournal(k *kernelruntime.Kernel) pulse.HealthStatFunc {
	const healthWindow = 2000
	return func(context.Context) (pulse.HealthStat, error) {
		j := k.Journal()
		if j == nil {
			return pulse.HealthStat{}, nil
		}
		evs, err := j.Tail(healthWindow)
		if err != nil {
			return pulse.HealthStat{}, err
		}
		var st pulse.HealthStat
		for _, e := range evs {
			switch e.Kind {
			case event.KindToolInvoked:
				st.ToolCalls++
			case event.KindToolResult:
				var p struct {
					Error bool `json:"error"`
				}
				_ = json.Unmarshal(e.Payload, &p)
				if p.Error {
					st.ToolErrors++
				}
			case event.KindTaskCompleted:
				st.Runs++
			case event.KindTaskFailed:
				st.Runs++
				st.FailedRuns++
			}
		}
		return st, nil
	}
}

// wireArtifactIndexer subscribes to the bus and indexes every offloaded tool
// output (M827): a tool.result event with a raw_ref means the agent stored a
// large output in the blob store, so we add a metadata index entry pointing at
// that ref (kind=tool-output, source=run, the tool name, the run correlation).
// The file manager then lists run outputs alongside inbound images. Best-effort:
// an index failure is silently skipped — it must never disturb a run. The
// subscription lives on the daemon ctx and ends when the daemon stops.
func wireArtifactIndexer(ctx context.Context, k *kernelruntime.Kernel) {
	idx := k.ArtifactIndex()
	if idx == nil {
		return
	}
	sub, err := k.Bus().Subscribe(">", 256)
	if err != nil {
		return
	}
	go func() {
		defer sub.Cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub.C:
				if !ok {
					return
				}
				if ev.Kind != event.KindToolResult {
					continue
				}
				var p struct {
					RawRef      string `json:"raw_ref"`
					Tool        string `json:"tool"`
					OutputBytes int64  `json:"output_bytes"`
				}
				if json.Unmarshal(ev.Payload, &p) != nil || p.RawRef == "" {
					continue
				}
				name := p.Tool
				if name == "" {
					name = "tool"
				}
				_, _ = idx.IndexRef(p.RawRef, artifact.Entry{
					Kind:   "tool-output",
					Source: "run",
					Name:   fmt.Sprintf("%s-output.txt", name),
					Mime:   "text/plain",
					Corr:   ev.CorrelationID,
					Size:   p.OutputBytes,
				}, time.Now().UnixMilli())
			}
		}
	}()
}

// netguardPublish returns the per-tool egress-block audit publisher handed to
// toolreg.KernelDeps.NetguardPublish: Set.Configure calls it once per
// netguard-guarded tool instance, and the returned callback journals a refused
// dial (SSRF / metadata attempt) as a netguard.blocked event (M109). A nil bus
// returns nil so Configure skips the wiring (harmless no-op, e.g. in tests).
func netguardPublish(b *bus.Bus) func(tool string) func(ip, reason string) {
	if b == nil {
		return nil
	}
	return func(tool string) func(ip, reason string) {
		return func(ip, reason string) {
			_, _ = b.Publish(event.Spec{
				Subject: "netguard.block",
				Kind:    event.KindNetguardBlocked,
				Actor:   tool,
				Payload: map[string]any{"ip": ip, "reason": reason, "tool": tool},
			})
		}
	}
}

// boardSubjectSlug sanitises a board topic into one subject segment (M656):
// lowercased, with any run of characters that aren't [a-z0-9_-] collapsed to a
// single dash, so "Acil Müdahale!" → "acil-m-dahale" and the event subject
// "board.<slug>" stays a single, well-formed segment a standing trigger can match.
// An empty/all-symbol topic degrades to "untopiced" so the subject is never
// "board." with a trailing dot.
func boardSubjectSlug(topic string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(topic)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return "untopiced"
	}
	return s
}

// voiceTranscriberShim adapts the runtime Voice adapter (Transcribe(ctx, audio,
// filename)) to the webui.Transcriber seam (Transcribe(ctx, filename, audio)) so
// the console mic and Voice-mode STT flow through whatever provider the voice
// adapter is configured for — ElevenLabs / Deepgram included, not just OpenAI.
type voiceTranscriberShim struct{ v kernelruntime.Voice }

func (s voiceTranscriberShim) Transcribe(ctx context.Context, filename string, audio []byte) (string, error) {
	return s.v.Transcribe(ctx, audio, filename)
}

// sttTranscriberFromEnv builds the speech-to-text client from AGEZT_STT_* (or a
// fallback OPENAI_API_KEY), or returns nil when no STT endpoint is configured.
// Shared by the Web UI mic button (/api/transcribe, M689) and the OpenAI-
// compatible /v1/audio/transcriptions route — one place decides "is STT on?".
// Returns the concrete *stt.Client so callers can nil-check the pointer before
// handing it to a Set*Transcriber (avoiding a typed-nil interface).
func sttTranscriberFromEnv() *stt.Client {
	key := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "STT_API_KEY"))
	if key == "" {
		key = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	url := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "STT_API_URL"))
	if key == "" && url == "" {
		return nil
	}
	return stt.New(stt.Config{
		APIURL: url,
		APIKey: key,
		Model:  strings.TrimSpace(os.Getenv(brand.EnvPrefix + "STT_MODEL")),
	})
}

// injectConfig bridges the Config Center's config store + vault into the process
// environment at startup so the existing os.Getenv consumers read operator edits
// unchanged (M693). Precedence: a value already in the real environment WINS
// (operator's .env/shell); the store/vault only fill gaps. Returns the schema env
// vars that were pinned by the real environment (computed BEFORE injection, so our
// own Setenv calls aren't mistaken for operator pins) for the Config Center to
// show read-only. AGEZT_CONFIG=off disables the bridge entirely.
func injectConfig(baseDir string, vault *creds.Store, stdout io.Writer) map[string]bool {
	pinned := map[string]bool{}
	// Pin across the FULL merged surface (built-in + registered) so a skill's
	// registered field is also marked read-only when the operator pins it in .env.
	for _, sec := range settings.NewRegistry(baseDir).Sections() {
		for _, f := range sec.Fields {
			if os.Getenv(f.Env) != "" {
				pinned[f.Env] = true
			}
		}
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv(brand.EnvPrefix+"CONFIG")), "off") {
		return pinned
	}
	store := settings.NewStore(baseDir)
	if err := store.Load(); err != nil {
		fmt.Fprintf(stdout, "  config store     : load failed (%v) — environment only\n", err)
		return pinned
	}
	injected := 0
	for name, val := range store.All() {
		if val != "" && os.Getenv(name) == "" {
			_ = os.Setenv(name, val)
			injected++
		}
	}
	// Channel/config SECRETS live in the vault under their AGEZT_* name; inject
	// those too. Provider API keys are NON-AGEZT_ and resolved via the cred chain,
	// so they need no env injection.
	for _, name := range vault.Names() {
		if strings.HasPrefix(name, brand.EnvPrefix) && os.Getenv(name) == "" {
			if v := vault.Get(name); v != "" {
				_ = os.Setenv(name, v)
				injected++
			}
		}
	}
	if injected > 0 {
		fmt.Fprintf(stdout, "  config store     : %d setting(s) applied from %s\n", injected, store.Path)
	}
	return pinned
}

// workspaceRoot resolves the directory the file and shell tools share:
// $AGEZT_WORKSPACE, or <baseDir>/workspace by default. Used by buildTools (to
// scope the tools) and by the kernel Config (to tell the model where it is via
// the M609 environment preamble), so the two never drift.
func workspaceRoot(baseDir string) string {
	if ws := os.Getenv(brand.EnvPrefix + "WORKSPACE"); ws != "" {
		return ws
	}
	return filepath.Join(baseDir, "workspace")
}

// buildTools + councilSeatName → boot_tools.go

func selectAskPolicy() (edict.AskPolicy, string) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "APPROVAL_MODE"))) {
	case "deny":
		return edict.AskDeny, "AskDeny (strict; only L4 calls run)"
	case "prompt", "ask":
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

func wardenOptionsFromEnv() (warden.Options, string) {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WARDEN_DOCKER")))
	if raw != "1" && raw != "true" && raw != "yes" && raw != "on" {
		return warden.Options{}, ""
	}
	runtimeName := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WARDEN_DOCKER_RUNTIME"))
	if runtimeName == "" {
		runtimeName = "docker"
	}
	image := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WARDEN_DOCKER_IMAGE"))
	if image == "" {
		image = "python:3.12-slim"
	}
	network := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WARDEN_DOCKER_NETWORK"))
	if network == "" {
		network = "none"
	}
	return warden.Options{
		Container: warden.ContainerOptions{
			Enabled: true,
			Runtime: runtimeName,
			Image:   image,
			Network: network,
		},
	}, fmt.Sprintf("; container=%s image=%s network=%s", runtimeName, image, network)
}

func selectAutoApproveCapabilities() (map[string]bool, string) {
	raw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "AUTO_APPROVE_CAPS"))
	switch strings.ToLower(raw) {
	case "off", "0", "false", "no", "none":
		return nil, "off (set " + brand.EnvPrefix + "AUTO_APPROVE_CAPS=all or a comma list)"
	case "", "all", "1", "true", "yes", "on":
		caps := map[string]bool{}
		for _, c := range edict.AllCapabilities() {
			caps[string(c)] = true
		}
		return caps, fmt.Sprintf("on (%d known capabilities; hard-deny/SSRF/budget guards still apply)", len(caps))
	default:
		caps := map[string]bool{}
		var unknown []string
		for _, item := range splitNonEmpty(raw) {
			if edict.KnownCapability(item) {
				caps[item] = true
			} else {
				unknown = append(unknown, item)
			}
		}
		if len(caps) == 0 {
			return nil, fmt.Sprintf("off (no known capabilities in %sAUTO_APPROVE_CAPS=%q)", brand.EnvPrefix, raw)
		}
		desc := fmt.Sprintf("on (%d selected capabilities)", len(caps))
		if len(unknown) > 0 {
			desc += fmt.Sprintf("; ignored unknown: %s", strings.Join(unknown, ", "))
		}
		return caps, desc
	}
}

// keep import honest
var _ = event.GenesisHash

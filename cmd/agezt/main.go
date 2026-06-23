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
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/alerter"
	"github.com/agezt/agezt/kernel/anomaly"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/market"
	kernelmemory "github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/netguard"
	"github.com/agezt/agezt/kernel/openaiapi"
	"github.com/agezt/agezt/kernel/plugin"
	"github.com/agezt/agezt/kernel/pulse"
	"github.com/agezt/agezt/kernel/redact"
	"github.com/agezt/agezt/kernel/restapi"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/kernel/stt"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/kernel/tunnel"
	"github.com/agezt/agezt/kernel/ulid"
	"github.com/agezt/agezt/kernel/update"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/kernel/webhook"
	"github.com/agezt/agezt/kernel/webui"
	"github.com/agezt/agezt/kernel/workflow"
	"github.com/agezt/agezt/plugins/builtinchannels"
	"github.com/agezt/agezt/plugins/builtinguardians"
	"github.com/agezt/agezt/plugins/builtinmarket"
	"github.com/agezt/agezt/plugins/builtinskills"
	"github.com/agezt/agezt/plugins/channels/chatwebhook"
	"github.com/agezt/agezt/plugins/channels/dingtalk"
	"github.com/agezt/agezt/plugins/channels/discord"
	"github.com/agezt/agezt/plugins/channels/email"
	"github.com/agezt/agezt/plugins/channels/feishu"
	"github.com/agezt/agezt/plugins/channels/homeassistant"
	"github.com/agezt/agezt/plugins/channels/imessage"
	"github.com/agezt/agezt/plugins/channels/irc"
	linechan "github.com/agezt/agezt/plugins/channels/line"
	"github.com/agezt/agezt/plugins/channels/mastodon"
	"github.com/agezt/agezt/plugins/channels/matrix"
	"github.com/agezt/agezt/plugins/channels/nextcloudtalk"
	"github.com/agezt/agezt/plugins/channels/nostr"
	"github.com/agezt/agezt/plugins/channels/onebot"
	"github.com/agezt/agezt/plugins/channels/push"
	signalchan "github.com/agezt/agezt/plugins/channels/signal"
	"github.com/agezt/agezt/plugins/channels/slack"
	"github.com/agezt/agezt/plugins/channels/sms"
	"github.com/agezt/agezt/plugins/channels/teams"
	"github.com/agezt/agezt/plugins/channels/telegram"
	webhookchan "github.com/agezt/agezt/plugins/channels/webhook"
	"github.com/agezt/agezt/plugins/channels/wecom"
	"github.com/agezt/agezt/plugins/channels/whatsapp"
	"github.com/agezt/agezt/plugins/channels/whatsappgw"
	"github.com/agezt/agezt/plugins/channels/zalo"
	"github.com/agezt/agezt/plugins/providers/compat"
	"github.com/agezt/agezt/plugins/providers/embed"
	"github.com/agezt/agezt/plugins/providers/image"
	"github.com/agezt/agezt/plugins/providers/rerank"
	"github.com/agezt/agezt/plugins/providers/voice"
	"github.com/agezt/agezt/plugins/tools/acpagent"
	artifactstool "github.com/agezt/agezt/plugins/tools/artifacts"
	boardtool "github.com/agezt/agezt/plugins/tools/boardtool"
	"github.com/agezt/agezt/plugins/tools/browser"
	"github.com/agezt/agezt/plugins/tools/codeexec"
	"github.com/agezt/agezt/plugins/tools/coding"
	conductortool "github.com/agezt/agezt/plugins/tools/conductor"
	configtool "github.com/agezt/agezt/plugins/tools/config"
	counciltool "github.com/agezt/agezt/plugins/tools/council"
	dbtool "github.com/agezt/agezt/plugins/tools/db"
	"github.com/agezt/agezt/plugins/tools/fetch"
	filetool "github.com/agezt/agezt/plugins/tools/file"
	"github.com/agezt/agezt/plugins/tools/forgetool"
	hatool "github.com/agezt/agezt/plugins/tools/homeassistant"
	httptool "github.com/agezt/agezt/plugins/tools/http"
	"github.com/agezt/agezt/plugins/tools/introspecttool"
	"github.com/agezt/agezt/plugins/tools/mcptool"
	"github.com/agezt/agezt/plugins/tools/notify"
	"github.com/agezt/agezt/plugins/tools/overseertool"
	"github.com/agezt/agezt/plugins/tools/peer"
	"github.com/agezt/agezt/plugins/tools/runstool"
	scheduletool "github.com/agezt/agezt/plugins/tools/schedule"
	"github.com/agezt/agezt/plugins/tools/sendmedia"
	"github.com/agezt/agezt/plugins/tools/shell"
	skilltool "github.com/agezt/agezt/plugins/tools/skilltool"
	standingtool "github.com/agezt/agezt/plugins/tools/standingtool"
	"github.com/agezt/agezt/plugins/tools/websearch"
	"github.com/agezt/agezt/plugins/tools/workflowtool"
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
func runUpdate(stdout, stderr io.Writer) int {
	// Wire the same base-dir resolution as the daemon so `agezt update` works
	// even when no daemon is running (the service is embedded in the binary).
	baseDir, err := paths.BaseDir()
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.Binary, err)
		return 1
	}

	args := os.Args[2:] // skip "agezt update"
	apply := len(args) > 0 && args[0] == "--apply"

	// Connect to the running daemon's control plane.
	cl, err := controlplane.NewClient(baseDir)
	if err != nil {
		fmt.Fprintf(stderr, "%s: controlplane: %v\n", brand.Binary, err)
		return 1
	}
	defer cl.Close()

	if apply {
		// First check to get the available update.
		check, err := cl.UpdateCheck(context.Background())
		if err != nil {
			fmt.Fprintf(stderr, "%s: update check: %v\n", brand.Binary, err)
			return 1
		}
		if check.Update == nil {
			fmt.Fprintf(stdout, "%s: already up to date (%s)\n", brand.Binary, check.Current)
			return 0
		}
		fmt.Fprintf(stdout, "%s: applying update %s (from %s)\n", brand.Binary, check.Update.Version, check.Current)
		result, err := cl.UpdateApply(context.Background(), check.Update.Version, check.Update.SHA256, check.Update.URL, check.Update.Notes)
		if err != nil {
			fmt.Fprintf(stderr, "%s: update apply: %v\n", brand.Binary, err)
			return 1
		}
		if result.Error != "" {
			fmt.Fprintf(stderr, "%s: update failed: %s\n", brand.Binary, result.Error)
			return 1
		}
		fmt.Fprintf(stdout, "%s: update applied, daemon will restart shortly\n", brand.Binary)
		return 0
	}

	// Check-only (default).
	check, err := cl.UpdateCheck(context.Background())
	if err != nil {
		fmt.Fprintf(stderr, "%s: update check: %v\n", brand.Binary, err)
		return 1
	}
	if check.Update == nil {
		fmt.Fprintf(stdout, "%s: up to date (%s)\n", brand.Binary, check.Current)
	} else {
		fmt.Fprintf(stdout, "%s: update available: %s (current: %s)\n", brand.Binary, check.Update.Version, check.Current)
		if check.Update.Notes != "" {
			fmt.Fprintf(stdout, "\n%s\n", check.Update.Notes)
		}
	}
	return 0
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
	// wins; the store/vault only fill gaps. Must run BEFORE buildGovernor + channel
	// construction read the env. configPinned (schema vars set in the real env) is
	// handed to the control plane so the Config Center can show them read-only.
	configPinned := injectConfig(baseDir, credStore, stdout)
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

	// Make ChatGPT ("Sign in with ChatGPT") discoverable in Models; it only
	// registers as a live provider once the operator signs in.
	seedChatGPTCatalog(catStore)
	cat, _ = catStore.Load()

	gov, govDesc, model, err := buildGovernor(cat, credLookup, baseDir)
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
	edictOpts := edict.Options{AskPolicy: askPolicy, HardDeny: hardDeny}
	// Master permissive switch (M611): AGEZT_ALLOW_ALL=1 sets EVERY governed
	// capability to L4 (allow) so nothing is denied or prompts — a single-operator
	// dev convenience ("default everything allowed, restrict later"). The built-in
	// catastrophe hard-deny rails (fork-bomb, dd-to-raw-device) deliberately stay,
	// since they guard against self-destruction rather than gate normal tools, and
	// are no-ops on Windows anyway. Loud banner so this is never silent.
	permissive := os.Getenv(brand.EnvPrefix+"ALLOW_ALL") == "1"
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

	tools, pluginManifest, pluginToolCaps, toolsDesc, err := buildTools(baseDir, stderr, ward)
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
	var sendMediaTool *sendmedia.Tool
	if len(notifyTargets) > 0 {
		notifyTool = notify.New() // unbound; Bind wires the sender once channels exist
		tools["notify"] = notifyTool
		// send_media: the attachment-carrying sibling of notify (same allowlist
		// pinning), so the agent can push an image/voice/file artifact to the
		// operator. Registered unbound here; Bind wires it once channels exist.
		sendMediaTool = sendmedia.New()
		tools["send_media"] = sendMediaTool
	}

	// Self-scheduling tool (`schedule`, M634): the agent arranges its OWN future
	// runs in the daemon's cadence store. Registered here (before the kernel
	// starts, like notify) and Bound to the live store after the kernel opens.
	// Always available — the schedule store is the kernel's and always exists.
	scheduleTool := scheduletool.New()
	tools["schedule"] = scheduleTool

	// Run-introspection tool (`runs`, M644): the agent recalls its OWN past runs
	// from the journal. Registered now, Bound to the live journal after the kernel
	// opens. Always available — the journal is the kernel's and always exists.
	runsTool := runstool.New()
	tools["runs"] = runsTool

	// Standing-order tool (`standing`, M645): the agent creates durable event/cron
	// trigger rules that wake an agent later. Registered now, Bound to the kernel
	// after it opens (the kernel satisfies the tool's journaled standing-CRUD surface).
	standingToolInst := standingtool.New()
	tools["standing"] = standingToolInst

	// Board tool (`board`, M647): the shared, persistent message board every agent
	// can post to and read from, so they can coordinate and talk to each other.
	// Registered now, Bound to its on-disk store under the daemon base dir after
	// the kernel opens.
	boardToolInst := boardtool.New()
	tools["board"] = boardToolInst

	// Skill tool (`skill`, M648): the agent modifies ITSELF — authoring, promoting,
	// and retiring its own reusable procedures through Forge. Registered now, Bound
	// to the kernel's Forge after it opens.
	skillToolInst := skilltool.New()
	tools["skill"] = skillToolInst

	// Introspection tool (`introspect`, M682): the agent reads the daemon's OWN
	// live state — a real health overview plus schedule/standing detail — in one
	// call, so a "summarise AGEZT's health" task can see everything instead of
	// guessing. Registered now, Bound to the live kernel after it opens.
	introspectToolInst := introspecttool.New()
	tools["introspect"] = introspectToolInst

	// Overseer tool (`overseer`, M850): the brain/overseer agent supervises and
	// intervenes on the fleet — list/cancel runs, halt/resume the daemon, pause/
	// retire/revive agents, triage open help. Registered now, Bound after open.
	overseerToolInst := overseertool.New()
	tools["overseer"] = overseerToolInst

	// Tool-forge tool (`tool_forge`, M794): the agent builds its OWN tools —
	// drafts a script, tests it in the code_exec sandbox, and once the operator
	// promotes it every run can call it as forge_<name>. Registered now, Bound
	// to the live kernel after it opens.
	forgeToolInst := forgetool.New()
	tools["tool_forge"] = forgeToolInst

	// MCP self-install tool (`mcp`, M796): the agent extends its own toolbox —
	// registering and ATTACHING MCP servers at runtime (Edict mcp.install, Ask
	// by default). Registered now, Bound to the live kernel after it opens.
	mcpToolInst := mcptool.New()
	tools["mcp"] = mcpToolInst

	// Workflow tool (`workflow`, M802): the agent authors and runs durable
	// workflows in the SAME store the console canvas edits (Edict
	// workflow.manage, AskFirst by default; tool nodes inside a run re-gate
	// per call). Registered now, Bound to the live kernel after it opens.
	workflowToolInst := workflowtool.New()
	tools["workflow"] = workflowToolInst

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
	// How many tool calls a run must make before it's worth an auto-distillation
	// pass (M993). Higher = fewer, more meaningful auto-memories — simple/short
	// runs no longer each spawn distilled notes. Default 6 (was 4); override with
	// AGEZT_MEMORY_DISTILL_MIN_TOOLS (0 or negative falls back to the default).
	distillMinTools := 6
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "MEMORY_DISTILL_MIN_TOOLS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			distillMinTools = n
		}
	}
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
	// Host-environment preamble (M609): on by default — the model needs to know
	// its OS/shell/workspace to act correctly (esp. on Windows). AGEZT_ENV_INJECT=off
	// disables it for operators who pin everything via a custom system prompt.
	envInjectOn := !strings.EqualFold(os.Getenv(brand.EnvPrefix+"ENV_INJECT"), "off")
	// Multi-agent delegation (P6-MULTI-01): the `delegate` tool lets a lead
	// agent spawn bounded sub-agents. On by default; AGEZT_SUBAGENT=off disables
	// it, AGEZT_SUBAGENT_DEPTH sets how deep delegation may nest (default 1).
	subAgentOn := !strings.EqualFold(os.Getenv(brand.EnvPrefix+"SUBAGENT"), "off")
	// Default depth 3 (M843): a lead agent can decompose a task, delegate the parts
	// to sub-agents, and THOSE sub-agents can delegate further — a real leader/worker
	// tree, not just one flat layer. The owner wants agents to "split tasks and run
	// more agents", and the default-allow posture favours capability; the tree-total
	// rail below keeps deep delegation bounded. AGEZT_SUBAGENT_DEPTH overrides.
	subAgentDepth := 3
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
	// AGEZT_SUBAGENT_MAX_TOTAL caps the total number of sub-agents in one
	// delegation TREE across all depths (M629) — the rail that makes
	// AGEZT_SUBAGENT_DEPTH>1 safe, since depth×fan-out alone can't bound a
	// tree's overall size. 0 / absent = unbounded; a positive value refuses the
	// (N+1)th spawn anywhere in the tree.
	subAgentTotal := 0
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "SUBAGENT_MAX_TOTAL")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			subAgentTotal = n
		}
	}
	// With deep delegation on by default (depth>1), give the tree a generous but
	// finite size rail when the operator hasn't set one (M843) — depth×fan-out
	// alone can't bound a tree, so an unbounded total + deep recursion risks a
	// fork-bomb. 48 total sub-agents is far more than any real leader/worker task
	// needs while still preventing runaway. depth==1 stays unbounded (unchanged);
	// AGEZT_SUBAGENT_MAX_TOTAL overrides.
	if subAgentTotal == 0 && subAgentDepth > 1 {
		subAgentTotal = 48
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

	// AGEZT_OBSERVATION_DELTAS=on (or 1) makes repeated identical tool/input
	// observations return a compact delta to the model while the journal keeps
	// the raw output. Off by default for compatibility.
	obsDeltasRaw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "OBSERVATION_DELTAS"))
	observationDeltas := strings.EqualFold(obsDeltasRaw, "on") || obsDeltasRaw == "1"
	// AGEZT_EPISTEMIC_ESCALATION=on routes otherwise-allowed tool proposals
	// through HITL when the runtime's external calibration gate sees matching
	// historical failures, low effect confidence, temporal sensitivity, or novel
	// dynamic tool conditions. Off by default; signals are still journaled.
	epistemicEscalationRaw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "EPISTEMIC_ESCALATION"))
	epistemicEscalation := strings.EqualFold(epistemicEscalationRaw, "on") || epistemicEscalationRaw == "1"
	// AGEZT_INTENT_REGRET_GATING=on routes otherwise-allowed tool proposals
	// through HITL when the user's utterance is underdetermined and the proposed
	// action has high wrong-action regret. Off by default; intent frames are
	// still journaled.
	intentRegretGatingRaw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "INTENT_REGRET_GATING"))
	intentRegretGating := strings.EqualFold(intentRegretGatingRaw, "on") || intentRegretGatingRaw == "1"
	// AGEZT_PROMPT_INJECTION_GUARD selects the guard posture for effectful actions
	// proposed within the causal window of directive-like untrusted web/file/API
	// content: unset/anything → on (HITL approval), "warn" → allow + journal a
	// banner, "off"/"0" → no active intervention. The observation boundary and
	// audit metadata remain enabled in all modes.
	promptInjectionMode := kernelruntime.ParsePromptInjectionMode(os.Getenv(brand.EnvPrefix + "PROMPT_INJECTION_GUARD"))
	disableHeuristicBypassRaw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "DISABLE_HEURISTIC_BYPASS"))
	disableHeuristicBypass := strings.EqualFold(disableHeuristicBypassRaw, "on") || disableHeuristicBypassRaw == "1"

	// AGEZT_SKILL_SHADOWEVAL=on judges the shadow skills relevant to a completed
	// run against what actually happened (SPEC-05 §5.2). Off by default — it spends
	// extra provider calls per run, so the operator opts in.
	shadowEval := strings.EqualFold(os.Getenv(brand.EnvPrefix+"SKILL_SHADOWEVAL"), "on")

	// Provider embeddings for memory recall (M901, DECISIONS C5 opt-in): when
	// AGEZT_EMBED_URL + AGEZT_EMBED_MODEL are both set, recall ranks by TRUE
	// semantic similarity from an OpenAI-compatible /v1/embeddings endpoint —
	// a local Ollama ("http://localhost:11434" + nomic-embed-text, zero cost,
	// no key) or a hosted API (api.openai.com/v1 + text-embedding-3-small +
	// AGEZT_EMBED_KEY). Unset (default) keeps the local feature-hash embedder.
	// Recall falls back to local on any embedder failure, so a wrong URL
	// degrades quality, never availability.
	var memEmbedder kernelmemory.Embedder
	if embedURL := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "EMBED_URL")); embedURL != "" {
		embedModel := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "EMBED_MODEL"))
		if embedModel == "" {
			fmt.Fprintf(stderr, "%s: %sEMBED_URL is set but %sEMBED_MODEL is empty — provider embeddings disabled\n", brand.Binary, brand.EnvPrefix, brand.EnvPrefix)
		} else {
			memEmbedder = embed.New(embedURL, embedModel, strings.TrimSpace(os.Getenv(brand.EnvPrefix+"EMBED_KEY")))
		}
	}

	// Voice adapter (STT + TTS) over an OpenAI-compatible endpoint, same shape as
	// the embeddings adapter. Each half is independent: set AGEZT_STT_URL +
	// AGEZT_STT_MODEL to let agents transcribe inbound audio, and/or AGEZT_TTS_URL
	// + AGEZT_TTS_MODEL to let them synthesize spoken replies. Local (faster-
	// whisper / Kokoro behind an OpenAI shim) or hosted (api.openai.com/v1 +
	// AGEZT_STT_KEY / AGEZT_TTS_KEY). Unset → no voice tool is registered.
	voiceAdapter := &voice.Adapter{}
	if sttURL := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "STT_URL")); sttURL != "" {
		if sttModel := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "STT_MODEL")); sttModel == "" {
			fmt.Fprintf(stderr, "%s: %sSTT_URL is set but %sSTT_MODEL is empty — transcription disabled\n", brand.Binary, brand.EnvPrefix, brand.EnvPrefix)
		} else {
			voiceAdapter.STT = &voice.STTClient{BaseURL: sttURL, Model: sttModel, APIKey: strings.TrimSpace(os.Getenv(brand.EnvPrefix + "STT_KEY"))}
		}
	}
	if ttsURL := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TTS_URL")); ttsURL != "" {
		if ttsModel := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TTS_MODEL")); ttsModel == "" {
			fmt.Fprintf(stderr, "%s: %sTTS_URL is set but %sTTS_MODEL is empty — synthesis disabled\n", brand.Binary, brand.EnvPrefix, brand.EnvPrefix)
		} else {
			voiceAdapter.TTS = &voice.TTSClient{BaseURL: ttsURL, Model: ttsModel, Voice: strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TTS_VOICE")), APIKey: strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TTS_KEY"))}
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
	if imgURL := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "IMAGE_URL")); imgURL != "" {
		if imgModel := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "IMAGE_MODEL")); imgModel == "" {
			fmt.Fprintf(stderr, "%s: %sIMAGE_URL is set but %sIMAGE_MODEL is empty — image generation disabled\n", brand.Binary, brand.EnvPrefix, brand.EnvPrefix)
		} else {
			imageCfg = image.New(imgURL, imgModel, strings.TrimSpace(os.Getenv(brand.EnvPrefix+"IMAGE_KEY")))
		}
	}

	// Reranking (M997): when AGEZT_RERANK_URL + AGEZT_RERANK_MODEL are set, the
	// `rerank` tool is registered, reordering candidate documents via a
	// Cohere/Jina-style /rerank endpoint. Unset → no tool.
	var rerankCfg kernelruntime.Reranker
	if rrURL := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "RERANK_URL")); rrURL != "" {
		if rrModel := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "RERANK_MODEL")); rrModel == "" {
			fmt.Fprintf(stderr, "%s: %sRERANK_URL is set but %sRERANK_MODEL is empty — reranking disabled\n", brand.Binary, brand.EnvPrefix, brand.EnvPrefix)
		} else {
			rerankCfg = rerank.New(rrURL, rrModel, strings.TrimSpace(os.Getenv(brand.EnvPrefix+"RERANK_KEY")))
		}
	}

	cfg := kernelruntime.Config{
		BaseDir:          baseDir,
		Provider:         gov, // Governor implements agent.Provider
		Tools:            tools,
		Plugins:          pluginManifest,
		ToolCapabilities: pluginToolCaps, // M900: manifest-declared policy axes

		Model:                      model,
		System:                     os.Getenv(brand.EnvPrefix + "SYSTEM_PROMPT"),
		Warden:                     ward,
		Edict:                      edictEng,
		Catalog:                    cat,
		MemoryInject:               memOn,
		MemoryTool:                 memOn,
		MemoryDistill:              memOn,
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
		MarketTool:                 true, // agents can discover + install capability packs mid-task
		SubAgentMaxDepth:           subAgentDepth,
		SubAgentMaxFanout:          subAgentFanout,
		SubAgentMaxSpendMicrocents: subAgentSpendCap,
		SubAgentMaxTotal:           subAgentTotal,
	}
	// Script-tool forge runner (M794): forged tools execute through the same
	// code_exec sandbox (warden isolation, scrubbed env). Only wired when the
	// sandbox is available — without it the forge reports itself unavailable.
	if ce, ok := tools["code_exec"].(*codeexec.Tool); ok {
		cfg.ScriptRunner = ce
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
	// Per-run tool-round cap (M824): AGEZT_MAX_ITER sets how many tool-call rounds
	// a single run may take before it stops with max_iters. Defaults to the agent
	// package's DefaultMaxIter. A malformed or non-positive value is a hard startup
	// error (fast feedback). Raise it for deep agentic tasks; the chat's "Continue"
	// affordance resumes a run that still hit the cap.
	maxIterDesc := fmt.Sprintf("%d per run (default; set %sMAX_ITER to change)", agent.DefaultMaxIter, brand.EnvPrefix)
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "MAX_ITER")); spec != "" {
		n, perr := strconv.Atoi(spec)
		if perr != nil || n <= 0 {
			fmt.Fprintf(stderr, "%s: %sMAX_ITER: want a positive integer, got %q\n", brand.Binary, brand.EnvPrefix, spec)
			return 1
		}
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
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "MAX_AUTO_CONTINUE")); spec != "" {
		n, perr := strconv.Atoi(spec)
		if perr != nil {
			fmt.Fprintf(stderr, "%s: %sMAX_AUTO_CONTINUE: want an integer, got %q\n", brand.Binary, brand.EnvPrefix, spec)
			return 1
		}
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
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "AUTO_CONTINUE_WAIT")); spec != "" {
		d, perr := time.ParseDuration(spec)
		if perr != nil || d < 0 {
			fmt.Fprintf(stderr, "%s: %sAUTO_CONTINUE_WAIT: want a non-negative duration, got %q\n", brand.Binary, brand.EnvPrefix, spec)
			return 1
		}
		cfg.AutoContinueWait = d
	}

	// In-turn parallel tool dispatch (M880): AGEZT_PARALLEL_TOOLS caps how many
	// tool calls from ONE assistant turn execute concurrently. Defaults to the
	// agent package's DefaultMaxParallelTools; 1 disables (strictly sequential).
	// Malformed or non-positive is a hard startup error (fast feedback).
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "PARALLEL_TOOLS")); spec != "" {
		n, perr := strconv.Atoi(spec)
		if perr != nil || n <= 0 {
			fmt.Fprintf(stderr, "%s: %sPARALLEL_TOOLS: want a positive integer, got %q\n", brand.Binary, brand.EnvPrefix, spec)
			return 1
		}
		cfg.MaxParallelTools = n
	}

	// Tool discovery (CH-03): AGEZT_TOOL_DISCOVERY_MAX=N trims each provider
	// request to the N most relevant tool schemas using the built-in lexical
	// selector. Off by default so existing deployments keep offering every tool;
	// malformed or negative is a hard startup error.
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TOOL_DISCOVERY_MAX")); spec != "" {
		n, perr := strconv.Atoi(spec)
		if perr != nil || n < 0 {
			fmt.Fprintf(stderr, "%s: %sTOOL_DISCOVERY_MAX: want a non-negative integer, got %q\n", brand.Binary, brand.EnvPrefix, spec)
			return 1
		}
		cfg.ToolDiscoveryMax = n
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
		prov, _, model2, auth, err := selectPrimary(c, freshLookup, baseDir)
		if err != nil {
			return fmt.Errorf("select primary: %w", err)
		}
		// Demote the stale "unconfigured" sentinel before installing the real one
		// (M816). When the daemon booted with no AGEZT_PROVIDER, buildGovernor
		// registered the unconfigured sentinel as the PRIMARY. Registry.Replace
		// only swaps an entry of the SAME name, so replacing "unconfigured" with
		// "deepseek" would APPEND deepseek behind the sentinel — leaving the
		// sentinel at primary[0], still refusing every run (the first-run-wizard
		// case: add a key + set AGEZT_PROVIDER, reload, but runs still error
		// "no provider configured"). Remove the sentinel entirely — unlike the old
		// mock there is no fallback role for it. gov.Replace below rebuilds the
		// primary/fallback slices from the registry, so order matters: mutate the
		// registry first, then Replace. If the reload still resolves to the
		// sentinel (operator added a key but no AGEZT_PROVIDER), we keep it.
		reg := gov.Registry()
		if prov.Name() != unconfiguredProviderName {
			reg.Remove(unconfiguredProviderName) // no-op when absent
		}
		reconcileAlternateProviders(reg, c, freshLookup, prov.Name(), baseDir)
		if err := gov.Replace(&governor.ProviderInfo{
			Name:     prov.Name(),
			Provider: prov,
			AuthMode: auth,
			Models:   catalogModelIDs(c, prov.Name()),
		}); err != nil {
			return fmt.Errorf("registry replace: %w", err)
		}
		// Hot-swap the live default model to match the freshly-selected provider
		// (M816). Without this the governor would route to the new provider while
		// runs still carried the OLD model id — e.g. after the first-run wizard
		// switches mock→deepseek, requests would carry model "mock" and the real
		// provider rejects it. k is non-nil whenever Reload runs (control plane
		// only dispatches it post-Open); guard anyway for safety.
		if k != nil {
			k.SetModel(model2)
		}
		return nil
	}

	// Vision sidecar picker (M821): returns a keyed vision-capable model id the
	// governor can route to, or ("", false) if none. Eligibility mirrors
	// buildGovernor's registered set (supported family + credentialed) so the
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
		lookup, _ := buildAWSCredChain(credStore.Lookup)
		return cat.VisionCapableAmong(func(provID string) bool {
			e := cat.Providers[provID]
			return e != nil && compat.IsSupportedFamily(e.Family()) && e.HasCredentials(lookup)
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
		lookup, _ := buildAWSCredChain(credStore.Lookup)
		// Accept a bare ("model") or provider-qualified ("provider/model") id.
		want := modelID
		if i := strings.IndexByte(modelID, '/'); i >= 0 {
			want = modelID[i+1:]
		}
		for _, p := range cat.ProviderList() {
			e := cat.Providers[p.ID]
			if e == nil || !compat.IsSupportedFamily(e.Family()) || !e.HasCredentials(lookup) {
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
		lookup, _ := buildAWSCredChain(credStore.Lookup)
		models := cat.BestModelsAcross(func(provID string) bool {
			e := cat.Providers[provID]
			return e != nil && compat.IsSupportedFamily(e.Family()) && e.HasCredentials(lookup)
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

	var openErr error
	k, openErr = kernelruntime.Open(cfg)
	if openErr != nil {
		fmt.Fprintf(stderr, "%s: open runtime: %v\n", brand.Binary, openErr)
		return 1
	}
	defer k.Close()

	// Bind the kernel into the config tool now that it exists, so live-apply
	// fields (provider/model) rebuild the provider in place via Reload().
	if ct, ok := tools["config"].(*configtool.Tool); ok {
		ct.SetKernel(k)
	}
	// Inject the artifact index into the fetch tool (M831) now that the kernel
	// owns it, so downloaded files are saved as browsable artifacts.
	if fe, ok := tools["fetch"].(*fetch.Tool); ok {
		fe.SetIndex(k.ArtifactIndex())
	}
	// Inject the artifact index into the artifacts tool (M832) so the agent can
	// list/read/delete the files it has saved.
	if af, ok := tools["artifacts"].(*artifactstool.Tool); ok {
		af.SetIndex(k.ArtifactIndex())
	}
	// Inject the data lake into the db tool (M834).
	if dbt, ok := tools["db"].(*dbtool.Tool); ok {
		dbt.SetStore(k.DataLake())
	}
	// Inject the kernel into the council tool (M837) as the deliberation runner.
	if ct, ok := tools["council"].(*counciltool.Tool); ok {
		ct.SetRunner(k)
	}
	// Inject the kernel into the conductor tool (M997) as the orchestration runner.
	if cond, ok := tools["conductor"].(*conductortool.Tool); ok {
		cond.SetRunner(k)
	}

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

	srv := controlplane.NewServer(k, baseDir)
	srv.SetConfigEnvPinned(configPinned) // M693: Config Center marks env-pinned fields read-only
	if boardErr == nil {
		// Board writes over the control plane (M937): `agt`/Go-SDK board_send,
		// board_ack go through the shared instance and fire the same notifier.
		srv.SetBoard(boardStore, boardNotify)
	}
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
	// Self-update engine (M860): wired when AGEZT_UPDATE_ENDPOINT or
	// AGEZT_UPDATE_GITHUB_OWNER/REPO is set. When not configured, update
	// commands report "update is disabled" rather than erroring.
	var updateSvc *update.Service
	if endpoint := os.Getenv(brand.EnvPrefix + "UPDATE_ENDPOINT"); endpoint != "" {
		updateSvc = update.New(update.Config{
			Source:   update.SourceEndpoint,
			Endpoint: endpoint,
			BaseDir:  baseDir,
			DrainTimeout: func() time.Duration {
				if t := os.Getenv(brand.EnvPrefix + "UPDATE_DRAIN_TIMEOUT"); t != "" {
					if d, err := time.ParseDuration(t); err == nil {
						return d
					}
				}
				return 30 * time.Second
			}(),
			CheckInterval: func() time.Duration {
				if t := os.Getenv(brand.EnvPrefix + "UPDATE_CHECK_INTERVAL"); t != "" {
					if d, err := time.ParseDuration(t); err == nil && d > 0 {
						return d
					}
				}
				return 0 // disabled by default
			}(),
		})
	} else if owner := os.Getenv(brand.EnvPrefix + "UPDATE_GITHUB_OWNER"); owner != "" {
		repo := os.Getenv(brand.EnvPrefix + "UPDATE_GITHUB_REPO")
		if repo == "" {
			repo = brand.Binary // default repo to the binary name
		}
		updateSvc = update.New(update.Config{
			Source:        update.SourceGitHub,
			GitHubOwner:   owner,
			GitHubRepo:    repo,
			BaseDir:       baseDir,
			DrainTimeout:  30 * time.Second,
			CheckInterval: 0,
		})
	}
	if updateSvc != nil {
		srv.SetUpdateService(updateSvc)
	}
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
	// First-run nudge (M816): when the governor degraded to the offline mock
	// (no credentialed provider), make the fix impossible to miss — point at
	// both the CLI wizard and the Web UI, which auto-opens its setup screen.
	if model == "mock" {
		fmt.Fprintf(stdout, "  ⚠ setup needed   : no provider key yet — run `%s quickstart`, or open the Web UI (URL below) to add one\n", brand.CLI)
	}
	if adv := modelAdvisory(cat, model); adv != "" {
		fmt.Fprintf(stdout, "  model advisory   : ⚠ %s\n", adv)
	}
	fmt.Fprintf(stdout, "  credentials      : %s\n", credDesc)
	fmt.Fprintf(stdout, "  redaction        : %s\n", redactDesc)
	fmt.Fprintf(stdout, "  tools            : %s\n", toolsDesc)
	fmt.Fprintf(stdout, "  policy engine    : edict (allow-by-default — every capability on unless you opt out; %s)\n", askPolicyDesc)
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

	// Telegram channel (SPEC-04 §1) — duplex when AGEZT_TELEGRAM_TOKEN is
	// set. Built before Pulse so its brief sink can tee with the log sink.
	// Telegram channel (SPEC-04 §1) — multi-account: the default instance plus any
	// "#label" accounts (several bots) are all built and started.
	tgInsts := buildAccounts(ctx, k, "telegram", buildTelegramInstance)
	startInstances(ctx, stdout, "telegram", "telegram", "disabled (set AGEZT_TELEGRAM_TOKEN)", tgInsts)

	// Slack channel (SPEC-04 §1) — duplex when AGEZT_SLACK_TOKEN is set. Serves
	// the Events API endpoint for inbound (HMAC-verified) and chat.postMessage for
	// outbound; briefs tee to it like Telegram.
	slInsts := buildAccounts(ctx, k, "slack", buildSlackInstance)
	startInstances(ctx, stdout, "slack", "slack", "disabled (set AGEZT_SLACK_TOKEN)", slInsts)

	// Discord channel (SPEC-04 §1) — duplex when AGEZT_DISCORD_TOKEN is set.
	// Serves the Interactions endpoint for inbound slash commands (Ed25519-verified)
	// and posts via the bot token for outbound; briefs tee to it like the others.
	dcInsts := buildAccounts(ctx, k, "discord", buildDiscordInstance)
	startInstances(ctx, stdout, "discord", "discord", "disabled (set AGEZT_DISCORD_TOKEN)", dcInsts)

	// Generic webhook channel (SPEC-04 §1) — vendor-neutral duplex. Any external
	// system POSTs a signed JSON message and gets the agent's reply synchronously;
	// briefs/`agt send` tee to a configured outbound URL. Enabled when a secret
	// (inbound) or an outbound URL is set.
	whInsts := buildAccountsLegacy(ctx, k, "webhook", buildWebhook)
	startInstances(ctx, stdout, "webhook", "webhook channel", "disabled (set AGEZT_WEBHOOK_SECRET + AGEZT_WEBHOOK_ADDR)", whInsts)

	// Email channel (SPEC-04 §1) — outbound-only over SMTP. Briefs/`agt send` mail
	// the allowlisted recipients. Enabled when AGEZT_EMAIL_SMTP_ADDR is set.
	// Email channel (SPEC-04 §1) — multi-account: the default instance plus any
	// "#label" accounts (several mailboxes, each its own SMTP) are all built.
	emInsts := buildAccounts(ctx, k, "email", buildEmailInstance)
	startInstances(ctx, stdout, "email", "email channel", "disabled (set AGEZT_EMAIL_SMTP_ADDR + AGEZT_EMAIL_FROM)", emInsts)

	// Matrix channel (SPEC-04 §1) — duplex over the open Matrix Client-Server API
	// when AGEZT_MATRIX_HOMESERVER + AGEZT_MATRIX_TOKEN are set. Long-polls /sync
	// for inbound and PUTs m.room.message for outbound; briefs tee to the
	// allowlisted rooms like the others.
	mxInsts := buildAccounts(ctx, k, "matrix", buildMatrixInstance)
	startInstances(ctx, stdout, "matrix", "matrix channel", "disabled (set AGEZT_MATRIX_HOMESERVER + AGEZT_MATRIX_TOKEN)", mxInsts)

	// IRC channel (SPEC-04 §1) — two-way over a persistent socket to any ircd.
	ircInsts := buildAccountsLegacy(ctx, k, "irc", buildIRC)
	startInstances(ctx, stdout, "irc", "irc channel", "disabled (set AGEZT_IRC_SERVER + AGEZT_IRC_NICK)", ircInsts)

	// Twitch chat (SPEC-04 §1) — IRC over Twitch's server; reuses the IRC channel.
	twInsts := buildAccountsLegacy(ctx, k, "twitch", buildTwitch)
	startInstances(ctx, stdout, "twitch", "twitch channel", "disabled (set AGEZT_TWITCH_USERNAME + AGEZT_TWITCH_TOKEN)", twInsts)

	// WhatsApp via a self-hosted gateway (WAHA/Evolution) — the easy WhatsApp path.
	wgInsts := buildAccountsLegacy(ctx, k, "whatsappgw", buildWhatsAppGateway)
	startInstances(ctx, stdout, "whatsappgw", "whatsapp gateway", "disabled (set AGEZT_WHATSAPPGW_URL)", wgInsts)

	// iMessage via a self-hosted BlueBubbles server — the Mac-bridge iMessage path.
	imInsts := buildAccountsLegacy(ctx, k, "imessage", buildIMessage)
	startInstances(ctx, stdout, "imessage", "imessage channel", "disabled (set AGEZT_IMESSAGE_URL)", imInsts)

	// LINE two-way (official Messaging API) — supersedes the outbound-only push
	// LINE when a channel secret is set.
	lnInsts := buildAccountsLegacy(ctx, k, "line", buildLine)
	startInstances(ctx, stdout, "line", "line channel", "", lnInsts)

	// Two-way Google Chat / Mattermost (incoming webhook out + webhook in) —
	// supersede the outbound-only push entries when an inbound addr is set.
	gcInsts := buildAccountsLegacy(ctx, k, "googlechat", func(c context.Context, kk *kernelruntime.Kernel) (*chatwebhook.Channel, pulse.BriefSink, string) {
		return buildChatWebhook(c, kk, chatwebhook.KindGoogleChat, "GOOGLECHAT")
	})
	startInstances(ctx, stdout, "googlechat", "googlechat (2way)", "", gcInsts)
	mmInsts := buildAccountsLegacy(ctx, k, "mattermost", func(c context.Context, kk *kernelruntime.Kernel) (*chatwebhook.Channel, pulse.BriefSink, string) {
		return buildChatWebhook(c, kk, chatwebhook.KindMattermost, "MATTERMOST")
	})
	startInstances(ctx, stdout, "mattermost", "mattermost (2way)", "", mmInsts)

	// DingTalk / Feishu / WeCom two-way (China enterprise platforms).
	dtInsts := buildAccountsLegacy(ctx, k, "dingtalk", buildDingTalk)
	startInstances(ctx, stdout, "dingtalk", "dingtalk (2way)", "", dtInsts)
	fsInsts := buildAccountsLegacy(ctx, k, "feishu", buildFeishu)
	startInstances(ctx, stdout, "feishu", "feishu (2way)", "", fsInsts)
	wcInsts := buildAccountsLegacy(ctx, k, "wecom", buildWeCom)
	startInstances(ctx, stdout, "wecom", "wecom (2way)", "", wcInsts)

	// QQ / WeChat via a OneBot v11 gateway; Zalo via the Official Account API.
	qqInsts := buildAccountsLegacy(ctx, k, "qq", func(c context.Context, kk *kernelruntime.Kernel) (*onebot.Channel, pulse.BriefSink, string) {
		return buildOneBot(c, kk, "qq", "QQ")
	})
	startInstances(ctx, stdout, "qq", "qq channel", "", qqInsts)
	wxInsts := buildAccountsLegacy(ctx, k, "wechat", func(c context.Context, kk *kernelruntime.Kernel) (*onebot.Channel, pulse.BriefSink, string) {
		return buildOneBot(c, kk, "wechat", "WECHAT")
	})
	startInstances(ctx, stdout, "wechat", "wechat channel", "", wxInsts)
	zlInsts := buildAccountsLegacy(ctx, k, "zalo", buildZalo)
	startInstances(ctx, stdout, "zalo", "zalo channel", "", zlInsts)
	nctInsts := buildAccountsLegacy(ctx, k, "nextcloudtalk", buildNextcloudTalk)
	startInstances(ctx, stdout, "nextcloudtalk", "nextcloud talk", "", nctInsts)
	maInsts := buildAccountsLegacy(ctx, k, "mastodon", buildMastodon)
	startInstances(ctx, stdout, "mastodon", "mastodon channel", "", maInsts)
	noInsts := buildAccountsLegacy(ctx, k, "nostr", buildNostr)
	startInstances(ctx, stdout, "nostr", "nostr channel", "", noInsts)

	// SMS channel (SPEC-04 §1) — duplex over Twilio Programmable Messaging when
	// AGEZT_SMS_ACCOUNT_SID + AGEZT_SMS_AUTH_TOKEN are set. Inbound is a signed
	// Twilio webhook (needs AGEZT_SMS_ADDR); outbound texts go via the REST API
	// (needs AGEZT_SMS_FROM); briefs tee to the allowlisted numbers.
	smInsts := buildAccountsLegacy(ctx, k, "sms", buildSMS)
	startInstances(ctx, stdout, "sms", "sms channel", "disabled (set AGEZT_SMS_ACCOUNT_SID + AGEZT_SMS_AUTH_TOKEN)", smInsts)

	// WhatsApp channel (SPEC-04 §1) — duplex over Meta's WhatsApp Cloud API when
	// AGEZT_WHATSAPP_APP_SECRET + AGEZT_WHATSAPP_ACCESS_TOKEN are set. Inbound is a
	// signed Meta webhook (needs AGEZT_WHATSAPP_ADDR); outbound goes via the Graph
	// API (needs AGEZT_WHATSAPP_PHONE_NUMBER_ID); briefs tee to the allowlist.
	waInsts := buildAccounts(ctx, k, "whatsapp", buildWhatsAppInstance)
	startInstances(ctx, stdout, "whatsapp", "whatsapp channel", "disabled (set AGEZT_WHATSAPP_APP_SECRET + AGEZT_WHATSAPP_ACCESS_TOKEN)", waInsts)

	// Home Assistant channel (SPEC-04 §1) — outbound to HA's notify API when
	// AGEZT_HOMEASSISTANT_URL + AGEZT_HOMEASSISTANT_TOKEN are set. Briefs/`agt send`
	// land as phone pushes / TTS / persistent notifications on the allowlisted
	// notify services. Outbound-only (drive FROM HA via the webhook channel).
	haInsts := buildAccountsLegacy(ctx, k, "homeassistant", buildHomeAssistant)
	startInstances(ctx, stdout, "homeassistant", "homeassistant ch", "disabled (set AGEZT_HOMEASSISTANT_URL + AGEZT_HOMEASSISTANT_TOKEN)", haInsts)

	// Teams channel (SPEC-04 §1) — outbound to Microsoft Teams Incoming Webhooks
	// when AGEZT_TEAMS_WEBHOOKS is set (name=url,name2=url2). Briefs/`agt send`
	// post a card to the named Teams channel. Outbound-only.
	tmInsts := buildAccountsLegacy(ctx, k, "teams", buildTeams)
	startInstances(ctx, stdout, "teams", "teams channel", "disabled (set AGEZT_TEAMS_WEBHOOKS=name=url,...)", tmInsts)

	// Signal channel (SPEC-04 §1) — duplex via an operator-run signal-cli-rest-api
	// when AGEZT_SIGNAL_API_URL + AGEZT_SIGNAL_NUMBER are set. Long-polls
	// /v1/receive for inbound and POSTs /v2/send for outbound; briefs tee to the
	// allowlisted numbers like the others.
	sgInsts := buildAccountsLegacy(ctx, k, "signal", buildSignal)
	startInstances(ctx, stdout, "signal", "signal channel", "disabled (set AGEZT_SIGNAL_API_URL + AGEZT_SIGNAL_NUMBER)", sgInsts)

	// Push-notification channels (SPEC-04 §1): a family of simple outbound
	// destinations — ntfy, Pushover, Gotify, Pushbullet, Google Chat, Mattermost —
	// each enabled by its own env and exposed as a distinct channel. Briefs/`agt
	// send` POST to the service. Outbound-only.
	pushChans, pushSink, pushDesc := buildPushChannels(ctx, k)
	for _, pc := range pushChans {
		go pc.Start(ctx)
	}
	if len(pushChans) > 0 {
		fmt.Fprintf(stdout, "  push channels    : %s\n", pushDesc)
	} else {
		fmt.Fprintf(stdout, "  push channels    : disabled (ntfy/pushover/gotify/pushbullet/googlechat/mattermost)\n")
	}

	// Every configured channel's brief sink, teed: Pulse briefs and (M782)
	// alert notifications share the same delivery surface.
	// All channels are multi-account now: one brief sink per instance, plus the
	// push family (its own internal multi-destination sink).
	allInsts := [][]chanInstance{
		tgInsts, emInsts, slInsts, dcInsts, mxInsts, waInsts,
		whInsts, ircInsts, twInsts, wgInsts, imInsts, lnInsts, gcInsts, mmInsts,
		dtInsts, fsInsts, wcInsts, qqInsts, wxInsts, zlInsts, nctInsts, maInsts, noInsts,
		smInsts, haInsts, tmInsts, sgInsts,
	}
	channelSinks := combineSinks(append(instanceSinks(allInsts...), pushSink)...)

	// Pulse — the proactive heart (SPEC-03). On by default; the resident
	// engine runs on the daemon ctx so `agt halt`/SIGTERM/`agt shutdown`
	// stop it with everything else. AGEZT_PULSE=off disables it. When a channel
	// is configured, briefs tee to it (closes the Jarvis loop).
	if eng, pulseDesc := buildPulse(k, ward, model, stdout, channelSinks); eng != nil {
		eng.Start(ctx)
		srv.SetPulse(eng)
		// Runtime disk watches (M767): build the observer here (the daemon owns the
		// DiskUsage func) and register it on the live engine.
		srv.SetDiskWatch(func(path string, minPct float64) (string, bool) {
			return eng.AddObserver(pulse.NewDiskObserver(path, minPct, pulse.DiskUsage)), true
		})
		// Runtime command-probe watches (M768): the agent runs the command each beat
		// (warden-gated, like any shell call) and alerts when its pass/fail flips.
		srv.SetProbeWatch(func(name string, argv []string) (string, bool) {
			return eng.AddObserver(pulse.NewProbeObserver(name, argv, ward, k.State())), true
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

	// Brain distillation (M804). Always available via `agt memory
	// consolidate`; set AGEZT_BRAIN_DISTILL_EVERY (e.g. 24h) to run the
	// consolidation pass on a timer — the standing "sleep cycle" that merges
	// accumulated near-duplicate memories. Mirrors the reflection ticker.
	if bdDesc := startBrainDistillTicker(ctx, k, stdout); bdDesc != "" {
		fmt.Fprintf(stdout, "  brain distill    : %s\n", bdDesc)
	} else {
		fmt.Fprintf(stdout, "  brain distill    : on-demand (agt memory consolidate; set AGEZT_BRAIN_DISTILL_EVERY for a timer)\n")
	}

	// Web UI (SPEC-07) — the SSE Live Monitor + read panels over the same
	// bus/control plane the CLI uses. Off unless AGEZT_WEB_ADDR is set;
	// runs on the daemon ctx (halt/shutdown stop it), localhost + token.
	if webDesc := buildWebUI(ctx, k, baseDir, stdout); webDesc != "" {
		fmt.Fprintf(stdout, "  web ui           : %s\n", webDesc)
	} else {
		fmt.Fprintf(stdout, "  web ui           : disabled (AGEZT_WEB_ADDR=off; unset it to serve on 127.0.0.1:8787)\n")
	}

	// Tunnel (SPEC-07) — expose a local HTTP service (the Web UI, else the REST
	// API) to the public internet via a supervised cloudflared/ngrok/custom binary.
	// Off unless AGEZT_TUNNEL or AGEZT_TUNNEL_CMD is set; the operator opts in
	// explicitly since this makes the service publicly reachable.
	if tunDesc := buildTunnel(ctx, stdout); tunDesc != "" {
		fmt.Fprintf(stdout, "  tunnel           : %s\n", tunDesc)
	} else {
		fmt.Fprintf(stdout, "  tunnel           : disabled (set AGEZT_TUNNEL=cloudflared|ngrok or AGEZT_TUNNEL_CMD)\n")
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
	if restDesc := buildRESTAPI(ctx, k, tenantReg, &draining, restBoard, boardNotify, updateSvc, stdout); restDesc != "" {
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
	// Every channel is multi-account: register every instance by its instance key
	// ("telegram", "telegram#bot2", "email#work", …).
	registerInstances(liveChannels, allInsts...)
	for _, pc := range pushChans {
		liveChannels[pc.Name()] = pc
	}
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
	// Bind the proactive media-messaging tool (`send_media`) to the same live
	// channels + operator allowlist, plus the artifact store so it can resolve a
	// ref to bytes. Recipients stay pinned to the allowlist (the agent supplies
	// only the artifact ref + optional caption).
	if sendMediaTool != nil {
		sendMediaTool.Bind(channelSendMedia, notifyTargets, func(ref string) ([]byte, error) {
			if a := k.Artifacts(); a != nil {
				return a.Get(ref)
			}
			return nil, fmt.Errorf("artifact store unavailable")
		})
		fmt.Fprintf(stdout, "  send_media tool  : enabled (the agent can send images/voice/files to the operator)\n")
	}

	// Bind the self-scheduling tool to the live cadence store (M634), now that the
	// kernel (and its store) exist. The store is the same one the schedule engine
	// ticks, so an agent-created schedule fires like any operator-added one.
	if sched := k.Schedules(); sched != nil {
		scheduleTool.Bind(sched)
		scheduleTool.BindAgentLookup(k.Roster().Get)
		fmt.Fprintf(stdout, "  schedule tool    : enabled (the agent can schedule its own future runs)\n")
	}

	// Bind the run-introspection tool to the live journal (M644).
	if j := k.Journal(); j != nil {
		runsTool.Bind(j)
	}

	// Bind the standing-order tool to the kernel (M645) — it satisfies the tool's
	// journaled AddStanding / RemoveStanding / Standing() surface.
	standingToolInst.Bind(k)

	// Bind the shared message board (M647): the SAME store instance the control
	// plane and REST mailbox write through (opened once, before the servers).
	// Each post journals a board.posted event (M656) via boardNotify, so a
	// standing order can trigger on a topic — or on board.dm.<recipient> for an
	// addressed message (M788) — and one agent's post wakes another. The posting
	// run's correlation ties into `agt why`.
	if boardErr != nil {
		fmt.Fprintf(stderr, "%s: board tool unavailable: %v\n", brand.Binary, boardErr)
	} else {
		boardToolInst.BindStore(boardStore)
		boardToolInst.OnPost(boardNotify)
		fmt.Fprintf(stdout, "  board tool       : enabled (agents share a persistent message board)\n")
	}

	// Bind the skill tool to the kernel's Forge (M648), so the agent can author and
	// manage its OWN skills through the same journaled, reversible state machine.
	if fg := k.Forge(); fg != nil {
		skillToolInst.Bind(fg)
		fmt.Fprintf(stdout, "  skill tool       : enabled (the agent can author and manage its own skills)\n")
		// Register the built-in channel manifests (Telegram, WhatsApp, …) so the
		// Channels wizard can list + configure every shipped channel uniformly.
		builtinchannels.RegisterAll()

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
		fmt.Fprintf(stdout, "  marketplace      : enabled (built-in Official + synced remotes; `agt market`)\n")
	}

	// Bind the introspection tool to the live kernel (M682), so the agent can read
	// the daemon's own health/schedules/standing-orders in one call.
	introspectToolInst.Bind(introspecttool.NewKernelSource(k))
	fmt.Fprintf(stdout, "  introspect tool  : enabled (the agent can read the daemon's own live state)\n")

	// Bind the overseer tool to the live kernel (M850), so a brain/overseer agent
	// can supervise and intervene on the fleet — cancel runs, halt/resume, pause/
	// retire/revive agents, triage open help. baseDir locates the board it reads.
	overseerToolInst.Bind(overseertool.NewKernelSource(k, baseDir))
	fmt.Fprintf(stdout, "  overseer tool    : enabled (a brain agent can supervise & intervene on the fleet)\n")

	// Seed the built-in guardian agents (M961): the daemon's internal self-healing
	// fleet (health / doctor / stuck / budget / routing-429 / code), each a System-marked
	// agent with an event or cadence trigger, wielding the tools bound just above.
	// Idempotent by slug (an operator who pauses/removes one is respected);
	// best-effort — a seed failure never blocks startup.
	if guards, gerr := builtinguardians.SeedAll(builtinguardians.NewKernelHost(k), ""); gerr != nil {
		fmt.Fprintf(stderr, "  built-in guardians: partial (%v)\n", gerr)
	} else {
		created := 0
		for _, g := range guards {
			if g.Created {
				created++
			}
		}
		if created > 0 {
			fmt.Fprintf(stdout, "  built-in guardians: seeded %d (health, doctor, stuck, budget, routing, code)\n", created)
		} else if len(guards) > 0 {
			fmt.Fprintf(stdout, "  built-in guardians: present (%d)\n", len(guards))
		}
	}

	// Bind the code-execution tool to the live bus (M683) so each run journals a
	// code.executed event. The tool itself was constructed in buildTools (it needed
	// the warden + base dir); we reach it through the returned tools map.
	if ce, ok := tools["code_exec"].(*codeexec.Tool); ok {
		ce.Bind(k.Bus())
		// Let the Conductor's Verifier role (M997) actually RUN a worker's code
		// through the same sandbox. nil-safe: when code_exec is absent the
		// Verifier falls back to LLM critique.
		k.SetConductorExec(ce)
		fmt.Fprintf(stdout, "  code_exec tool   : enabled (the agent can write & run code: %s)\n", strings.Join(ce.Languages(), ", "))
	}

	// Bind the tool-forge tool to the live kernel (M794), so the agent can draft
	// and test its own script tools through the journaled scripttool.* lifecycle.
	// Going live still takes an operator promote (`agt toolforge promote`).
	forgeToolInst.Bind(k)
	fmt.Fprintf(stdout, "  tool_forge tool  : enabled (the agent can build its own tools; operator promotes)\n")

	// Bind the workflow tool (M802): agents author/run workflows themselves;
	// everything lands in the journaled workflow.* lifecycle the operator sees.
	workflowToolInst.Bind(k)
	fmt.Fprintf(stdout, "  workflow tool    : enabled (the agent can author & run workflows)\n")

	// Bind the MCP self-install tool (M796) and auto-attach every ENABLED
	// registered server. Per-server failures are reported, never fatal — one
	// broken server must not take the daemon down.
	mcpToolInst.Bind(k)
	if registered := k.MCPStore().Count(); registered > 0 {
		attached, failures := k.AttachEnabledMCPServers(ctx)
		fmt.Fprintf(stdout, "  mcp servers      : %d attached of %d registered\n", len(attached), registered)
		for name, aerr := range failures {
			fmt.Fprintf(stdout, "    %s: attach failed: %v\n", name, aerr)
		}
	} else {
		fmt.Fprintf(stdout, "  mcp self-install : enabled (the agent can attach MCP servers at runtime; Edict mcp.install gates it)\n")
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
		if wh := splitNonEmpty(os.Getenv(brand.EnvPrefix + "WEBHOOK_CHANNELS")); len(wh) > 0 {
			briefTargets["webhook"] = wh
		}
	}
	standingBrief := func(bctx context.Context, kind, text string) {
		for _, recip := range briefTargets[kind] {
			_ = channelSend(bctx, kind, recip, text)
		}
	}
	standingDesc, fireStandingNow := buildStandingRunner(ctx, k, standingBrief)
	srv.SetStandingFire(fireStandingNow)
	fmt.Fprintf(stdout, "  standing orders  : %s\n", standingDesc)
	fmt.Fprintf(stdout, "  auto repair      : %s\n", wireAutoRepair(ctx, k, baseDir, boardStore, boardNotify))

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
func buildTelegramInstance(ctx context.Context, k *kernelruntime.Kernel, label string, get func(string) string) (channel.Channel, pulse.BriefSink, string) {
	token := strings.TrimSpace(get(brand.EnvPrefix + "TELEGRAM_TOKEN"))
	if token == "" {
		return nil, nil, ""
	}
	chatIDs := splitNonEmpty(get(brand.EnvPrefix + "TELEGRAM_CHAT_ID"))
	allow := channel.NewAllowlist(chatIDs)

	handler := makeChannelHandler(k)
	ch := telegram.New(telegram.Config{
		Token:     token,
		BaseURL:   strings.TrimSpace(get(brand.EnvPrefix + "TELEGRAM_API_BASE")), // empty → public Bot API
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
func buildSlackInstance(ctx context.Context, k *kernelruntime.Kernel, label string, get func(string) string) (channel.Channel, pulse.BriefSink, string) {
	token := strings.TrimSpace(get(brand.EnvPrefix + "SLACK_TOKEN"))
	if token == "" {
		return nil, nil, ""
	}
	secret := strings.TrimSpace(get(brand.EnvPrefix + "SLACK_SIGNING_SECRET"))
	addr := strings.TrimSpace(get(brand.EnvPrefix + "SLACK_ADDR"))
	channelIDs := splitNonEmpty(get(brand.EnvPrefix + "SLACK_CHANNELS"))

	handler := makeChannelHandler(k)
	ch := slack.New(slack.Config{
		Token:         token,
		SigningSecret: secret,
		Addr:          addr,
		BaseURL:       strings.TrimSpace(get(brand.EnvPrefix + "SLACK_API_BASE")), // empty → public Web API
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

// buildEmailInstance constructs an email channel account when AGEZT_EMAIL_SMTP_ADDR
// is set. Outbound over SMTP; two-way when an inbox (IMAP/POP3) is also configured.
//
//	AGEZT_EMAIL_SMTP_ADDR     SMTP server host:port (e.g. smtp.example.com:587), enables
//	AGEZT_EMAIL_FROM          sender address
//	AGEZT_EMAIL_USERNAME      SMTP AUTH username (with PASSWORD); empty → no auth
//	AGEZT_EMAIL_PASSWORD      SMTP AUTH password
//	AGEZT_EMAIL_RECIPIENTS    comma-separated allowlist of addresses (mail targets + inbound senders)
//	AGEZT_EMAIL_INBOX_ADDR    IMAP/POP3 server host:port — enables two-way (poll for new mail)
//	AGEZT_EMAIL_INBOX_PROTOCOL "imap" (default) or "pop3"
//	AGEZT_EMAIL_INBOX_USERNAME/PASSWORD  mailbox creds (default to the SMTP ones)
//	AGEZT_EMAIL_INBOX_TLS     "tls" (default) | "starttls" | "none"
//	AGEZT_EMAIL_INBOX_POLL    poll interval seconds (default 60)
func buildEmailInstance(ctx context.Context, k *kernelruntime.Kernel, label string, get func(string) string) (channel.Channel, pulse.BriefSink, string) {
	addr := strings.TrimSpace(get(brand.EnvPrefix + "EMAIL_SMTP_ADDR"))
	if addr == "" {
		return nil, nil, ""
	}
	from := strings.TrimSpace(get(brand.EnvPrefix + "EMAIL_FROM"))
	recipients := splitNonEmpty(get(brand.EnvPrefix + "EMAIL_RECIPIENTS"))
	inboxAddr := strings.TrimSpace(get(brand.EnvPrefix + "EMAIL_INBOX_ADDR"))
	pollSecs := 0
	if v := strings.TrimSpace(get(brand.EnvPrefix + "EMAIL_INBOX_POLL")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			pollSecs = n
		}
	}

	var handler channel.InboundHandler
	if inboxAddr != "" {
		handler = makeChannelHandler(k)
	}
	ch := email.New(email.Config{
		Addr:          addr,
		From:          from,
		Username:      strings.TrimSpace(get(brand.EnvPrefix + "EMAIL_USERNAME")),
		Password:      get(brand.EnvPrefix + "EMAIL_PASSWORD"),
		Allowlist:     channel.NewAllowlist(recipients),
		Bus:           k.Bus(),
		InboxAddr:     inboxAddr,
		InboxProtocol: strings.TrimSpace(get(brand.EnvPrefix + "EMAIL_INBOX_PROTOCOL")),
		InboxUsername: strings.TrimSpace(get(brand.EnvPrefix + "EMAIL_INBOX_USERNAME")),
		InboxPassword: get(brand.EnvPrefix + "EMAIL_INBOX_PASSWORD"),
		InboxTLS:      strings.TrimSpace(get(brand.EnvPrefix + "EMAIL_INBOX_TLS")),
		PollSecs:      pollSecs,
		Handler:       handler,
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

	dir := "outbound"
	if inboxAddr != "" {
		dir = "two-way (inbox " + inboxAddr + ")"
	}
	switch {
	case from == "":
		return ch, sink, "configured but NO from address (set AGEZT_EMAIL_FROM)"
	case len(recipients) == 0:
		return ch, sink, fmt.Sprintf("%s via %s, NO recipients (set AGEZT_EMAIL_RECIPIENTS)", dir, addr)
	default:
		return ch, sink, fmt.Sprintf("%s via %s, %d recipient(s)", dir, addr, len(recipients))
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
func buildDiscordInstance(ctx context.Context, k *kernelruntime.Kernel, label string, get func(string) string) (channel.Channel, pulse.BriefSink, string) {
	token := strings.TrimSpace(get(brand.EnvPrefix + "DISCORD_TOKEN"))
	if token == "" {
		return nil, nil, ""
	}
	pubKey := strings.TrimSpace(get(brand.EnvPrefix + "DISCORD_PUBLIC_KEY"))
	appID := strings.TrimSpace(get(brand.EnvPrefix + "DISCORD_APP_ID"))
	addr := strings.TrimSpace(get(brand.EnvPrefix + "DISCORD_ADDR"))
	channelIDs := splitNonEmpty(get(brand.EnvPrefix + "DISCORD_CHANNELS"))

	handler := makeChannelHandler(k)
	ch := discord.New(discord.Config{
		Token:         token,
		PublicKey:     pubKey,
		ApplicationID: appID,
		Addr:          addr,
		BaseURL:       strings.TrimSpace(get(brand.EnvPrefix + "DISCORD_API_BASE")), // empty → public API
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

// buildMatrix constructs the in-process Matrix channel when AGEZT_MATRIX_HOMESERVER
// and AGEZT_MATRIX_TOKEN are set, plus a Pulse brief sink to the allowlisted rooms.
// Returns (nil, nil, "") when unconfigured. Mirrors buildTelegram: long-polls /sync
// for inbound, PUTs m.room.message for outbound.
func buildMatrixInstance(ctx context.Context, k *kernelruntime.Kernel, label string, get func(string) string) (channel.Channel, pulse.BriefSink, string) {
	homeserver := strings.TrimSpace(get(brand.EnvPrefix + "MATRIX_HOMESERVER"))
	token := strings.TrimSpace(get(brand.EnvPrefix + "MATRIX_TOKEN"))
	if homeserver == "" || token == "" {
		return nil, nil, ""
	}
	roomIDs := splitNonEmpty(get(brand.EnvPrefix + "MATRIX_ROOMS"))

	handler := makeChannelHandler(k)
	ch := matrix.New(matrix.Config{
		Homeserver: homeserver,
		Token:      token,
		Allowlist:  channel.NewAllowlist(roomIDs),
		Bus:        k.Bus(),
		Handler:    handler,
	})

	// Pulse briefs → the allowlisted rooms. Nil sink when no room configured (the
	// bot can still receive commands once a room is allowlisted).
	var sink pulse.BriefSink
	if len(roomIDs) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, id := range roomIDs {
				if err := ch.Send(ctx, channel.Outbound{ChannelID: id, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	desc := fmt.Sprintf("listening, allowlist=%d room(s)", len(roomIDs))
	if len(roomIDs) == 0 {
		desc = "listening, NO allowlist (outbound-only; set AGEZT_MATRIX_ROOMS to allow commands)"
	}
	return ch, sink, desc
}

// buildIRC constructs the two-way IRC channel when AGEZT_IRC_SERVER +
// AGEZT_IRC_NICK are set. It joins AGEZT_IRC_CHANNELS and acts on inbound from
// allowlisted sources (the joined channels, plus any AGEZT_IRC_ALLOWLIST nicks/
// channels); Pulse briefs tee to the joined channels.
//
//	AGEZT_IRC_SERVER     host:port (e.g. irc.libera.chat:6697)   (required)
//	AGEZT_IRC_NICK       the bot's nick                          (required)
//	AGEZT_IRC_CHANNELS   comma-separated channels to join (#foo) — allowed by default
//	AGEZT_IRC_PASSWORD   optional server password (PASS)
//	AGEZT_IRC_TLS        "true" to force TLS (auto for :6697)
//	AGEZT_IRC_ALLOWLIST  extra allowed sources (nicks for DMs / channels)
func buildIRC(ctx context.Context, k *kernelruntime.Kernel) (*irc.Channel, pulse.BriefSink, string) {
	server := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "IRC_SERVER"))
	nick := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "IRC_NICK"))
	if server == "" || nick == "" {
		return nil, nil, ""
	}
	chans := splitNonEmpty(os.Getenv(brand.EnvPrefix + "IRC_CHANNELS"))
	// The joined channels are allowed by default; extra nicks/channels widen it.
	allowed := append([]string(nil), chans...)
	allowed = append(allowed, splitNonEmpty(os.Getenv(brand.EnvPrefix+"IRC_ALLOWLIST"))...)
	useTLS := strings.EqualFold(strings.TrimSpace(os.Getenv(brand.EnvPrefix+"IRC_TLS")), "true") || strings.HasSuffix(server, ":6697")

	ch := irc.New(irc.Config{
		Server:    server,
		TLS:       useTLS,
		Nick:      nick,
		Password:  strings.TrimSpace(os.Getenv(brand.EnvPrefix + "IRC_PASSWORD")),
		Channels:  chans,
		Allowlist: channel.NewAllowlist(allowed),
		Bus:       k.Bus(),
		Handler:   makeChannelHandler(k),
	})

	var sink pulse.BriefSink
	if len(chans) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, c := range chans {
				if err := ch.Send(ctx, channel.Outbound{ChannelID: c, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	desc := fmt.Sprintf("%s as %s, %d channel(s)", server, nick, len(chans))
	if len(chans) == 0 {
		desc = fmt.Sprintf("%s as %s, NO channels (set AGEZT_IRC_CHANNELS)", server, nick)
	}
	return ch, sink, desc
}

// buildTwitch constructs a Twitch chat channel when AGEZT_TWITCH_USERNAME +
// AGEZT_TWITCH_TOKEN are set. Twitch chat is IRC, so this reuses the IRC channel
// pinned to Twitch's server with an "oauth:" PASS; it joins AGEZT_TWITCH_CHANNELS
// (lowercase #channel) and acts on inbound from those channels by default.
//
//	AGEZT_TWITCH_USERNAME  the bot account's login name        (required)
//	AGEZT_TWITCH_TOKEN     OAuth token ("oauth:" prefix added) (required)
//	AGEZT_TWITCH_CHANNELS  comma-separated #channels to join — allowed by default
//	AGEZT_TWITCH_ALLOWLIST extra allowed sources (nicks / channels)
func buildTwitch(ctx context.Context, k *kernelruntime.Kernel) (*irc.Channel, pulse.BriefSink, string) {
	user := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TWITCH_USERNAME"))
	token := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TWITCH_TOKEN"))
	if user == "" || token == "" {
		return nil, nil, ""
	}
	chans := splitNonEmpty(os.Getenv(brand.EnvPrefix + "TWITCH_CHANNELS"))
	allowed := append([]string(nil), chans...)
	allowed = append(allowed, splitNonEmpty(os.Getenv(brand.EnvPrefix+"TWITCH_ALLOWLIST"))...)
	pass := token
	if !strings.HasPrefix(pass, "oauth:") {
		pass = "oauth:" + pass
	}

	ch := irc.New(irc.Config{
		Kind:      "twitch",
		Server:    "irc.chat.twitch.tv:6697",
		TLS:       true,
		Nick:      strings.ToLower(user),
		Password:  pass,
		Channels:  chans,
		Allowlist: channel.NewAllowlist(allowed),
		Bus:       k.Bus(),
		Handler:   makeChannelHandler(k),
	})

	var sink pulse.BriefSink
	if len(chans) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, c := range chans {
				if err := ch.Send(ctx, channel.Outbound{ChannelID: c, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	desc := fmt.Sprintf("as %s, %d channel(s)", user, len(chans))
	if len(chans) == 0 {
		desc = fmt.Sprintf("as %s, NO channels (set AGEZT_TWITCH_CHANNELS)", user)
	}
	return ch, sink, desc
}

// buildWhatsAppGateway constructs the easy-path WhatsApp channel — a self-hosted
// WAHA or Evolution API gateway (QR login, no Meta) — when AGEZT_WHATSAPPGW_URL
// is set. Outbound always; inbound (two-way) when AGEZT_WHATSAPPGW_ADDR points
// the gateway's webhook at this daemon. Briefs tee to the allowlisted numbers.
//
//	AGEZT_WHATSAPPGW_URL      gateway base URL, e.g. http://localhost:3000  (required)
//	AGEZT_WHATSAPPGW_BACKEND  "waha" (default) or "evolution"
//	AGEZT_WHATSAPPGW_SESSION  WAHA session / Evolution instance (default "default")
//	AGEZT_WHATSAPPGW_KEY      gateway API key
//	AGEZT_WHATSAPPGW_NUMBERS  comma-separated allowed sender numbers
//	AGEZT_WHATSAPPGW_ADDR     host:port to serve the inbound webhook (two-way)
//	AGEZT_WHATSAPPGW_PATH     inbound route (default /whatsappgw)
//	AGEZT_WHATSAPPGW_SECRET   optional shared secret the gateway must echo inbound
func buildWhatsAppGateway(ctx context.Context, k *kernelruntime.Kernel) (*whatsappgw.Channel, pulse.BriefSink, string) {
	base := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WHATSAPPGW_URL"))
	if base == "" {
		return nil, nil, ""
	}
	numbers := splitNonEmpty(os.Getenv(brand.EnvPrefix + "WHATSAPPGW_NUMBERS"))
	addr := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WHATSAPPGW_ADDR"))
	backend := strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WHATSAPPGW_BACKEND")))

	ch := whatsappgw.New(whatsappgw.Config{
		Backend:   backend,
		BaseURL:   base,
		Session:   strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WHATSAPPGW_SESSION")),
		APIKey:    strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WHATSAPPGW_KEY")),
		Allowlist: channel.NewAllowlist(numbers),
		Bus:       k.Bus(),
		Handler:   makeChannelHandler(k),
		Addr:      addr,
		Path:      strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WHATSAPPGW_PATH")),
		Secret:    strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WHATSAPPGW_SECRET")),
	})

	var sink pulse.BriefSink
	if len(numbers) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, n := range numbers {
				if err := ch.Send(ctx, channel.Outbound{ChannelID: n, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	be := backend
	if be == "" {
		be = "waha"
	}
	desc := fmt.Sprintf("%s via %s, allowlist=%d number(s)", base, be, len(numbers))
	if addr == "" {
		desc += " (outbound-only; set AGEZT_WHATSAPPGW_ADDR for two-way)"
	}
	return ch, sink, desc
}

// buildIMessage constructs the iMessage channel — a self-hosted BlueBubbles
// server (a Mac bridge; https://bluebubbles.app) — when AGEZT_IMESSAGE_URL is
// set. Outbound always; inbound (two-way) when AGEZT_IMESSAGE_ADDR points the
// BlueBubbles webhook back at this channel.
//
//	AGEZT_IMESSAGE_URL        BlueBubbles server URL, e.g. http://localhost:1234  (required)
//	AGEZT_IMESSAGE_PASSWORD   BlueBubbles server password
//	AGEZT_IMESSAGE_METHOD     "private-api" (default) or "apple-script"
//	AGEZT_IMESSAGE_ADDRESSES  comma-separated allowed sender addresses (phone/email)
//	AGEZT_IMESSAGE_ADDR       host:port to serve the inbound webhook (two-way)
//	AGEZT_IMESSAGE_PATH       inbound route (default /imessage)
//	AGEZT_IMESSAGE_SECRET     optional shared secret the webhook must echo (X-Webhook-Secret)
func buildIMessage(ctx context.Context, k *kernelruntime.Kernel) (*imessage.Channel, pulse.BriefSink, string) {
	base := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "IMESSAGE_URL"))
	if base == "" {
		return nil, nil, ""
	}
	addresses := splitNonEmpty(os.Getenv(brand.EnvPrefix + "IMESSAGE_ADDRESSES"))
	addr := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "IMESSAGE_ADDR"))

	ch := imessage.New(imessage.Config{
		BaseURL:   base,
		Password:  strings.TrimSpace(os.Getenv(brand.EnvPrefix + "IMESSAGE_PASSWORD")),
		Method:    strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "IMESSAGE_METHOD"))),
		Allowlist: channel.NewAllowlist(addresses),
		Bus:       k.Bus(),
		Handler:   makeChannelHandler(k),
		Addr:      addr,
		Path:      strings.TrimSpace(os.Getenv(brand.EnvPrefix + "IMESSAGE_PATH")),
		Secret:    strings.TrimSpace(os.Getenv(brand.EnvPrefix + "IMESSAGE_SECRET")),
	})

	var sink pulse.BriefSink
	if len(addresses) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, a := range addresses {
				if err := ch.Send(ctx, channel.Outbound{ChannelID: a, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	desc := fmt.Sprintf("%s via BlueBubbles, allowlist=%d address(es)", base, len(addresses))
	if addr == "" {
		desc += " (outbound-only; set AGEZT_IMESSAGE_ADDR for two-way)"
	}
	return ch, sink, desc
}

// twoWayLineConfigured reports whether the dedicated two-way LINE channel should
// own the "line" name (a channel secret is set) — in which case buildPushChannels
// yields LINE to it to avoid a double registration.
func twoWayLineConfigured() bool {
	return strings.TrimSpace(os.Getenv(brand.EnvPrefix+"LINE_SECRET")) != ""
}

// buildLine constructs the two-way LINE channel (official Messaging API) when a
// channel secret is set. Outbound push uses AGEZT_LINE_TOKEN; inbound (two-way)
// is served when AGEZT_LINE_ADDR is set and verified with AGEZT_LINE_SECRET.
//
//	AGEZT_LINE_TOKEN    channel access token (Bearer for reply/push)
//	AGEZT_LINE_SECRET   channel secret (verifies inbound signature; enables two-way)  (required)
//	AGEZT_LINE_USERS    comma-separated allowed sender userIds
//	AGEZT_LINE_TO       recipient id for Pulse briefs
//	AGEZT_LINE_ADDR     host:port to serve LINE's webhook (two-way)
//	AGEZT_LINE_PATH     inbound route (default /line)
func buildLine(ctx context.Context, k *kernelruntime.Kernel) (*linechan.Channel, pulse.BriefSink, string) {
	if !twoWayLineConfigured() {
		return nil, nil, ""
	}
	users := splitNonEmpty(os.Getenv(brand.EnvPrefix + "LINE_USERS"))
	addr := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "LINE_ADDR"))

	ch := linechan.New(linechan.Config{
		Secret:      strings.TrimSpace(os.Getenv(brand.EnvPrefix + "LINE_SECRET")),
		AccessToken: strings.TrimSpace(os.Getenv(brand.EnvPrefix + "LINE_TOKEN")),
		Allowlist:   channel.NewAllowlist(users),
		Bus:         k.Bus(),
		Handler:     makeChannelHandler(k),
		Addr:        addr,
		Path:        strings.TrimSpace(os.Getenv(brand.EnvPrefix + "LINE_PATH")),
	})

	var sink pulse.BriefSink
	if to := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "LINE_TO")); to != "" {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			return ch.Send(ctx, channel.Outbound{ChannelID: to, Text: formatBrief(b), Priority: channel.PriorityNotify})
		})
	}

	desc := fmt.Sprintf("LINE Messaging API, allowlist=%d user(s)", len(users))
	if addr == "" {
		desc += " (outbound-only; set AGEZT_LINE_ADDR for two-way)"
	}
	return ch, sink, desc
}

// twoWayChatConfigured reports whether the two-way chat-webhook channel for a
// push kind (prefix "GOOGLECHAT" / "MATTERMOST") should own that name — an
// inbound addr is set — so buildPushChannels yields the outbound-only entry.
func twoWayChatConfigured(prefix string) bool {
	return strings.TrimSpace(os.Getenv(brand.EnvPrefix+prefix+"_ADDR")) != ""
}

// buildChatWebhook constructs a two-way Google Chat / Mattermost channel when its
// inbound addr is set. Outbound + replies use the incoming-webhook URL.
//
//	AGEZT_<PREFIX>_WEBHOOK  incoming-webhook URL (outbound + replies)
//	AGEZT_<PREFIX>_ADDR     host:port to serve the inbound webhook (enables two-way)
//	AGEZT_<PREFIX>_TOKEN    verification token (Mattermost outgoing-webhook token / Google Chat ?token=)
//	AGEZT_<PREFIX>_USERS    comma-separated allowed senders (usernames / emails)
//	AGEZT_<PREFIX>_PATH     inbound route (default /<kind>)
func buildChatWebhook(ctx context.Context, k *kernelruntime.Kernel, kind, prefix string) (*chatwebhook.Channel, pulse.BriefSink, string) {
	if !twoWayChatConfigured(prefix) {
		return nil, nil, ""
	}
	get := func(s string) string { return strings.TrimSpace(os.Getenv(brand.EnvPrefix + prefix + s)) }
	users := splitNonEmpty(os.Getenv(brand.EnvPrefix + prefix + "_USERS"))
	webhook := get("_WEBHOOK")

	ch := chatwebhook.New(chatwebhook.Config{
		Kind:       kind,
		WebhookURL: webhook,
		Token:      get("_TOKEN"),
		Allowlist:  channel.NewAllowlist(users),
		Bus:        k.Bus(),
		Handler:    makeChannelHandler(k),
		Addr:       get("_ADDR"),
		Path:       get("_PATH"),
	})

	var sink pulse.BriefSink
	if webhook != "" {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			return ch.Send(ctx, channel.Outbound{Text: formatBrief(b), Priority: channel.PriorityNotify})
		})
	}

	desc := fmt.Sprintf("%s two-way, allowlist=%d sender(s)", kind, len(users))
	if webhook == "" {
		desc += " (inbound-only; set AGEZT_" + prefix + "_WEBHOOK for replies/briefs)"
	}
	return ch, sink, desc
}

// buildDingTalk constructs the two-way DingTalk channel when AGEZT_DINGTALK_ADDR
// is set. Replies go to each message's sessionWebhook; briefs use the custom
// robot webhook (AGEZT_DINGTALK_WEBHOOK).
//
//	AGEZT_DINGTALK_WEBHOOK  custom-robot webhook (outbound briefs / agt send)
//	AGEZT_DINGTALK_SECRET   robot secret (verifies inbound timestamp+sign)
//	AGEZT_DINGTALK_USERS    comma-separated allowed senderStaffId / nick
//	AGEZT_DINGTALK_ADDR     host:port to serve the inbound webhook (enables two-way)
//	AGEZT_DINGTALK_PATH     inbound route (default /dingtalk)
func buildDingTalk(ctx context.Context, k *kernelruntime.Kernel) (*dingtalk.Channel, pulse.BriefSink, string) {
	addr := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "DINGTALK_ADDR"))
	if addr == "" {
		return nil, nil, ""
	}
	users := splitNonEmpty(os.Getenv(brand.EnvPrefix + "DINGTALK_USERS"))
	webhook := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "DINGTALK_WEBHOOK"))
	ch := dingtalk.New(dingtalk.Config{
		WebhookURL: webhook,
		Secret:     strings.TrimSpace(os.Getenv(brand.EnvPrefix + "DINGTALK_SECRET")),
		Allowlist:  channel.NewAllowlist(users),
		Bus:        k.Bus(),
		Handler:    makeChannelHandler(k),
		Addr:       addr,
		Path:       strings.TrimSpace(os.Getenv(brand.EnvPrefix + "DINGTALK_PATH")),
	})
	var sink pulse.BriefSink
	if webhook != "" {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			return ch.Send(ctx, channel.Outbound{Text: formatBrief(b), Priority: channel.PriorityNotify})
		})
	}
	return ch, sink, fmt.Sprintf("DingTalk robot, allowlist=%d sender(s)", len(users))
}

// buildFeishu constructs the two-way Feishu / Lark channel when AGEZT_FEISHU_ADDR
// is set. Replies + briefs use the IM API (tenant_access_token from app id/secret).
//
//	AGEZT_FEISHU_APP_ID        app id
//	AGEZT_FEISHU_APP_SECRET    app secret
//	AGEZT_FEISHU_VERIFY_TOKEN  event verification token
//	AGEZT_FEISHU_CHAT          chat_id for proactive briefs
//	AGEZT_FEISHU_USERS         comma-separated allowed sender open_ids
//	AGEZT_FEISHU_ADDR          host:port to serve the inbound webhook (enables two-way)
//	AGEZT_FEISHU_PATH          inbound route (default /feishu)
func buildFeishu(ctx context.Context, k *kernelruntime.Kernel) (*feishu.Channel, pulse.BriefSink, string) {
	addr := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "FEISHU_ADDR"))
	appID := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "FEISHU_APP_ID"))
	if addr == "" || appID == "" {
		return nil, nil, ""
	}
	users := splitNonEmpty(os.Getenv(brand.EnvPrefix + "FEISHU_USERS"))
	chat := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "FEISHU_CHAT"))
	ch := feishu.New(feishu.Config{
		AppID:       appID,
		AppSecret:   strings.TrimSpace(os.Getenv(brand.EnvPrefix + "FEISHU_APP_SECRET")),
		VerifyToken: strings.TrimSpace(os.Getenv(brand.EnvPrefix + "FEISHU_VERIFY_TOKEN")),
		DefaultChat: chat,
		Allowlist:   channel.NewAllowlist(users),
		Bus:         k.Bus(),
		Handler:     makeChannelHandler(k),
		Addr:        addr,
		Path:        strings.TrimSpace(os.Getenv(brand.EnvPrefix + "FEISHU_PATH")),
	})
	var sink pulse.BriefSink
	if chat != "" {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			return ch.Send(ctx, channel.Outbound{ChannelID: chat, Text: formatBrief(b), Priority: channel.PriorityNotify})
		})
	}
	return ch, sink, fmt.Sprintf("Feishu app, allowlist=%d user(s)", len(users))
}

// buildWeCom constructs the two-way WeCom (WeChat Work) channel when
// AGEZT_WECOM_ADDR is set. Inbound is an AES-encrypted callback; replies use the
// app message-send API (access_token from corp id/secret).
//
//	AGEZT_WECOM_CORP_ID      corp id
//	AGEZT_WECOM_CORP_SECRET  app secret
//	AGEZT_WECOM_AGENT_ID     app agent id
//	AGEZT_WECOM_TOKEN        callback token
//	AGEZT_WECOM_AES_KEY      callback EncodingAESKey
//	AGEZT_WECOM_USERS        comma-separated allowed user ids
//	AGEZT_WECOM_ADDR         host:port to serve the inbound callback (enables two-way)
//	AGEZT_WECOM_PATH         inbound route (default /wecom)
func buildWeCom(ctx context.Context, k *kernelruntime.Kernel) (*wecom.Channel, pulse.BriefSink, string) {
	addr := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WECOM_ADDR"))
	corp := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WECOM_CORP_ID"))
	if addr == "" || corp == "" {
		return nil, nil, ""
	}
	users := splitNonEmpty(os.Getenv(brand.EnvPrefix + "WECOM_USERS"))
	ch := wecom.New(wecom.Config{
		CorpID:     corp,
		CorpSecret: strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WECOM_CORP_SECRET")),
		AgentID:    strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WECOM_AGENT_ID")),
		Token:      strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WECOM_TOKEN")),
		AESKey:     strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WECOM_AES_KEY")),
		Allowlist:  channel.NewAllowlist(users),
		Bus:        k.Bus(),
		Handler:    makeChannelHandler(k),
		Addr:       addr,
		Path:       strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WECOM_PATH")),
	})
	var sink pulse.BriefSink
	if len(users) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, u := range users {
				if err := ch.Send(ctx, channel.Outbound{ChannelID: u, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}
	return ch, sink, fmt.Sprintf("WeCom app, allowlist=%d user(s)", len(users))
}

// buildOneBot constructs a QQ / WeChat channel over a OneBot v11 gateway when its
// inbound addr is set. QQ and WeChat have no first-party bot API; a self-hosted
// gateway (go-cqhttp / NapCat / Lagrange for QQ; wcf / wechatbot for WeChat)
// speaks OneBot. kind is the channel name; prefix is the env namespace.
//
//	AGEZT_<PREFIX>_GATEWAY  gateway HTTP API base, e.g. http://localhost:5700
//	AGEZT_<PREFIX>_TOKEN    gateway access token (bearer)
//	AGEZT_<PREFIX>_SECRET   HMAC-SHA1 secret verifying inbound X-Signature
//	AGEZT_<PREFIX>_USERS    comma-separated allowed user ids
//	AGEZT_<PREFIX>_ADDR     host:port to serve the inbound webhook (enables two-way)
//	AGEZT_<PREFIX>_PATH     inbound route (default /<kind>)
func buildOneBot(ctx context.Context, k *kernelruntime.Kernel, kind, prefix string) (*onebot.Channel, pulse.BriefSink, string) {
	get := func(s string) string { return strings.TrimSpace(os.Getenv(brand.EnvPrefix + prefix + s)) }
	addr := get("_ADDR")
	if addr == "" {
		return nil, nil, ""
	}
	users := splitNonEmpty(os.Getenv(brand.EnvPrefix + prefix + "_USERS"))
	ch := onebot.New(onebot.Config{
		Kind:        kind,
		APIBase:     get("_GATEWAY"),
		AccessToken: get("_TOKEN"),
		Secret:      get("_SECRET"),
		Allowlist:   channel.NewAllowlist(users),
		Bus:         k.Bus(),
		Handler:     makeChannelHandler(k),
		Addr:        addr,
		Path:        get("_PATH"),
	})
	var sink pulse.BriefSink
	if len(users) > 0 && get("_GATEWAY") != "" {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, u := range users {
				if err := ch.Send(ctx, channel.Outbound{ChannelID: "private:" + u, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}
	return ch, sink, fmt.Sprintf("%s via OneBot gateway, allowlist=%d user(s)", kind, len(users))
}

// buildZalo constructs the two-way Zalo channel (Official Account API) when
// AGEZT_ZALO_ADDR is set. Replies + briefs use the OA message API.
//
//	AGEZT_ZALO_APP_ID   OA app id (part of the inbound signature)
//	AGEZT_ZALO_TOKEN    OA access token (sends)
//	AGEZT_ZALO_SECRET   OA secret key (verifies the inbound signature)
//	AGEZT_ZALO_USERS    comma-separated allowed user ids
//	AGEZT_ZALO_ADDR     host:port to serve the inbound webhook (enables two-way)
//	AGEZT_ZALO_PATH     inbound route (default /zalo)
func buildZalo(ctx context.Context, k *kernelruntime.Kernel) (*zalo.Channel, pulse.BriefSink, string) {
	addr := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ZALO_ADDR"))
	if addr == "" {
		return nil, nil, ""
	}
	users := splitNonEmpty(os.Getenv(brand.EnvPrefix + "ZALO_USERS"))
	ch := zalo.New(zalo.Config{
		AppID:       strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ZALO_APP_ID")),
		AccessToken: strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ZALO_TOKEN")),
		Secret:      strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ZALO_SECRET")),
		Allowlist:   channel.NewAllowlist(users),
		Bus:         k.Bus(),
		Handler:     makeChannelHandler(k),
		Addr:        addr,
		Path:        strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ZALO_PATH")),
	})
	var sink pulse.BriefSink
	if len(users) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, u := range users {
				if err := ch.Send(ctx, channel.Outbound{ChannelID: u, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}
	return ch, sink, fmt.Sprintf("Zalo OA, allowlist=%d user(s)", len(users))
}

// buildNextcloudTalk constructs the Nextcloud Talk channel when AGEZT_NEXTCLOUDTALK_URL
// + AGEZT_NEXTCLOUDTALK_SECRET are set. Inbound (signed bot webhook) is served when
// AGEZT_NEXTCLOUDTALK_ADDR is also set; outbound replies + Pulse briefs go to the
// allowlisted conversation tokens (AGEZT_NEXTCLOUDTALK_TOKENS).
//
//	AGEZT_NEXTCLOUDTALK_URL     Nextcloud base URL, e.g. https://cloud.example.com (required)
//	AGEZT_NEXTCLOUDTALK_SECRET  shared bot secret; signs/verifies (required)
//	AGEZT_NEXTCLOUDTALK_ADDR    host:port for the inbound webhook (inbound)
//	AGEZT_NEXTCLOUDTALK_PATH    inbound route (default /nextcloudtalk)
//	AGEZT_NEXTCLOUDTALK_TOKENS  comma-separated allowlist of conversation tokens
func buildNextcloudTalk(ctx context.Context, k *kernelruntime.Kernel) (*nextcloudtalk.Channel, pulse.BriefSink, string) {
	server := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "NEXTCLOUDTALK_URL"))
	secret := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "NEXTCLOUDTALK_SECRET"))
	if server == "" || secret == "" {
		return nil, nil, ""
	}
	tokens := splitNonEmpty(os.Getenv(brand.EnvPrefix + "NEXTCLOUDTALK_TOKENS"))
	ch := nextcloudtalk.New(nextcloudtalk.Config{
		ServerURL: server,
		Secret:    secret,
		Allowlist: channel.NewAllowlist(tokens),
		Bus:       k.Bus(),
		Handler:   makeChannelHandler(k),
		Addr:      strings.TrimSpace(os.Getenv(brand.EnvPrefix + "NEXTCLOUDTALK_ADDR")),
		Path:      strings.TrimSpace(os.Getenv(brand.EnvPrefix + "NEXTCLOUDTALK_PATH")),
	})
	var sink pulse.BriefSink
	if len(tokens) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, t := range tokens {
				if err := ch.Send(ctx, channel.Outbound{ChannelID: t, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}
	return ch, sink, fmt.Sprintf("Nextcloud Talk, allowlist=%d token(s)", len(tokens))
}

// twoWayMastodonConfigured reports whether the dedicated two-way Mastodon channel
// should own the "mastodon" name — true when an acct allowlist is set, signalling
// the operator wants the agent to answer mentions (not just post).
func twoWayMastodonConfigured() bool {
	return strings.TrimSpace(os.Getenv(brand.EnvPrefix+"MASTODON_USERS")) != ""
}

// buildMastodon constructs the two-way Mastodon channel when AGEZT_MASTODON_USERS
// is set (alongside SERVER + TOKEN). It polls mention notifications and replies as
// threaded statuses; outbound briefs post standalone statuses.
//
//	AGEZT_MASTODON_SERVER  instance base URL (required)
//	AGEZT_MASTODON_TOKEN   access token, read:notifications + write:statuses (required)
//	AGEZT_MASTODON_USERS   comma-separated acct handles allowed to drive the agent (enables two-way)
//	AGEZT_MASTODON_POLL    poll interval seconds (default 60)
func buildMastodon(ctx context.Context, k *kernelruntime.Kernel) (*mastodon.Channel, pulse.BriefSink, string) {
	if !twoWayMastodonConfigured() {
		return nil, nil, ""
	}
	server := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "MASTODON_SERVER"))
	token := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "MASTODON_TOKEN"))
	if server == "" || token == "" {
		return nil, nil, ""
	}
	users := splitNonEmpty(os.Getenv(brand.EnvPrefix + "MASTODON_USERS"))
	poll := 0
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "MASTODON_POLL")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			poll = n
		}
	}
	ch := mastodon.New(mastodon.Config{
		Server:    server,
		Token:     token,
		Allowlist: channel.NewAllowlist(users),
		Bus:       k.Bus(),
		Handler:   makeChannelHandler(k),
		PollSecs:  poll,
	})
	sink := pulse.SinkFunc(func(b pulse.Brief) error {
		return ch.Send(ctx, channel.Outbound{Text: formatBrief(b), Priority: channel.PriorityNotify})
	})
	return ch, sink, fmt.Sprintf("Mastodon (two-way), allowlist=%d acct(s)", len(users))
}

// buildNostr constructs the Nostr channel when AGEZT_NOSTR_PRIVKEY + AGEZT_NOSTR_RELAYS
// are set. It connects to the relays, answers kind-1 mentions of the agent's
// pubkey from allowlisted authors, and posts briefs as standalone notes.
//
//	AGEZT_NOSTR_PRIVKEY  64-char hex secret key (required)
//	AGEZT_NOSTR_RELAYS   comma-separated wss:// relay URLs (required)
//	AGEZT_NOSTR_AUTHORS  comma-separated author pubkeys (hex) allowed to drive the agent
func buildNostr(ctx context.Context, k *kernelruntime.Kernel) (*nostr.Channel, pulse.BriefSink, string) {
	priv := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "NOSTR_PRIVKEY"))
	relays := splitNonEmpty(os.Getenv(brand.EnvPrefix + "NOSTR_RELAYS"))
	if priv == "" || len(relays) == 0 {
		return nil, nil, ""
	}
	authors := splitNonEmpty(os.Getenv(brand.EnvPrefix + "NOSTR_AUTHORS"))
	// Authors may be hex or npub… — normalize to hex so the allowlist matches the
	// hex pubkey on inbound events.
	norm := make([]string, 0, len(authors))
	for _, a := range authors {
		if h, derr := nostr.DecodePubkey(a); derr == nil {
			norm = append(norm, h)
		}
	}
	ch, err := nostr.New(nostr.Config{
		PrivKeyHex: priv,
		Relays:     relays,
		Allowlist:  channel.NewAllowlist(norm),
		Bus:        k.Bus(),
		Handler:    makeChannelHandler(k),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: nostr channel disabled: %v\n", brand.Binary, err)
		return nil, nil, ""
	}
	sink := pulse.SinkFunc(func(b pulse.Brief) error {
		return ch.Send(ctx, channel.Outbound{Text: formatBrief(b), Priority: channel.PriorityNotify})
	})
	return ch, sink, fmt.Sprintf("Nostr, %d relay(s), allowlist=%d author(s)", len(relays), len(authors))
}

// buildSMS constructs the Twilio SMS channel when AGEZT_SMS_ACCOUNT_SID +
// AGEZT_SMS_AUTH_TOKEN are set. Inbound (signed Twilio webhook) is served when
// AGEZT_SMS_ADDR is also set; outbound texts + Pulse briefs to the allowlisted
// numbers (AGEZT_SMS_NUMBERS) need AGEZT_SMS_FROM.
//
//	AGEZT_SMS_ACCOUNT_SID  Twilio Account SID  (required)
//	AGEZT_SMS_AUTH_TOKEN   Twilio auth token   (required; signs inbound + REST)
//	AGEZT_SMS_FROM         Twilio number to send from, E.164 (outbound)
//	AGEZT_SMS_ADDR         host:port for the inbound webhook (inbound)
//	AGEZT_SMS_PATH         inbound route (default /sms)
//	AGEZT_SMS_PUBLIC_URL   exact public URL Twilio POSTs to (signature check behind a tunnel)
//	AGEZT_SMS_NUMBERS      comma-separated allowlist of sender numbers
func buildSMS(ctx context.Context, k *kernelruntime.Kernel) (*sms.Channel, pulse.BriefSink, string) {
	sid := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "SMS_ACCOUNT_SID"))
	token := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "SMS_AUTH_TOKEN"))
	if sid == "" || token == "" {
		return nil, nil, ""
	}
	from := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "SMS_FROM"))
	addr := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "SMS_ADDR"))
	path := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "SMS_PATH"))
	publicURL := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "SMS_PUBLIC_URL"))
	numbers := splitNonEmpty(os.Getenv(brand.EnvPrefix + "SMS_NUMBERS"))

	ch := sms.New(sms.Config{
		Addr:       addr,
		Path:       path,
		AccountSID: sid,
		AuthToken:  token,
		From:       from,
		PublicURL:  publicURL,
		Allowlist:  channel.NewAllowlist(numbers),
		Bus:        k.Bus(),
		Handler:    makeChannelHandler(k),
	})

	// Pulse briefs → the allowlisted numbers (needs an outbound From).
	var sink pulse.BriefSink
	if from != "" && len(numbers) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, id := range numbers {
				if err := ch.Send(ctx, channel.Outbound{ChannelID: id, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	switch {
	case addr == "":
		return ch, sink, fmt.Sprintf("outbound-only (set AGEZT_SMS_ADDR for inbound), allowlist=%d number(s)", len(numbers))
	default:
		p := path
		if p == "" {
			p = sms.DefaultPath
		}
		return ch, sink, fmt.Sprintf("inbound at %s%s, allowlist=%d number(s)", addr, p, len(numbers))
	}
}

// buildWhatsApp constructs the WhatsApp Cloud API channel when
// AGEZT_WHATSAPP_APP_SECRET + AGEZT_WHATSAPP_ACCESS_TOKEN are set. Inbound (signed
// Meta webhook) is served when AGEZT_WHATSAPP_ADDR is also set; outbound + Pulse
// briefs to the allowlisted numbers need AGEZT_WHATSAPP_PHONE_NUMBER_ID.
//
//	AGEZT_WHATSAPP_APP_SECRET       Meta app secret (required; signs inbound)
//	AGEZT_WHATSAPP_ACCESS_TOKEN     Graph API bearer token (required; outbound)
//	AGEZT_WHATSAPP_PHONE_NUMBER_ID  business phone-number id (outbound endpoint)
//	AGEZT_WHATSAPP_VERIFY_TOKEN     token echoed in Meta's GET verify handshake
//	AGEZT_WHATSAPP_ADDR             host:port for the inbound webhook (inbound)
//	AGEZT_WHATSAPP_PATH             inbound route (default /whatsapp)
//	AGEZT_WHATSAPP_NUMBERS          comma-separated allowlist of sender numbers
func buildWhatsAppInstance(ctx context.Context, k *kernelruntime.Kernel, label string, get func(string) string) (channel.Channel, pulse.BriefSink, string) {
	appSecret := strings.TrimSpace(get(brand.EnvPrefix + "WHATSAPP_APP_SECRET"))
	accessToken := strings.TrimSpace(get(brand.EnvPrefix + "WHATSAPP_ACCESS_TOKEN"))
	if appSecret == "" || accessToken == "" {
		return nil, nil, ""
	}
	phoneID := strings.TrimSpace(get(brand.EnvPrefix + "WHATSAPP_PHONE_NUMBER_ID"))
	verifyToken := strings.TrimSpace(get(brand.EnvPrefix + "WHATSAPP_VERIFY_TOKEN"))
	addr := strings.TrimSpace(get(brand.EnvPrefix + "WHATSAPP_ADDR"))
	path := strings.TrimSpace(get(brand.EnvPrefix + "WHATSAPP_PATH"))
	numbers := splitNonEmpty(get(brand.EnvPrefix + "WHATSAPP_NUMBERS"))

	ch := whatsapp.New(whatsapp.Config{
		Addr:          addr,
		Path:          path,
		VerifyToken:   verifyToken,
		AppSecret:     appSecret,
		AccessToken:   accessToken,
		PhoneNumberID: phoneID,
		Allowlist:     channel.NewAllowlist(numbers),
		Bus:           k.Bus(),
		Handler:       makeChannelHandler(k),
	})

	var sink pulse.BriefSink
	if phoneID != "" && len(numbers) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, id := range numbers {
				if err := ch.Send(ctx, channel.Outbound{ChannelID: id, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	switch {
	case addr == "":
		return ch, sink, fmt.Sprintf("outbound-only (set AGEZT_WHATSAPP_ADDR for inbound), allowlist=%d number(s)", len(numbers))
	default:
		p := path
		if p == "" {
			p = whatsapp.DefaultPath
		}
		return ch, sink, fmt.Sprintf("inbound at %s%s, allowlist=%d number(s)", addr, p, len(numbers))
	}
}

// buildHomeAssistant constructs the outbound Home Assistant channel when
// AGEZT_HOMEASSISTANT_URL + AGEZT_HOMEASSISTANT_TOKEN are set. Pulse briefs +
// `agt send` go to the allowlisted notify services (AGEZT_HOMEASSISTANT_SERVICES).
//
//	AGEZT_HOMEASSISTANT_URL       HA base, e.g. http://homeassistant.local:8123
//	AGEZT_HOMEASSISTANT_TOKEN     long-lived access token
//	AGEZT_HOMEASSISTANT_SERVICES  comma-separated notify service allowlist
//	                              (e.g. mobile_app_phone,persistent_notification)
func buildHomeAssistant(ctx context.Context, k *kernelruntime.Kernel) (*homeassistant.Channel, pulse.BriefSink, string) {
	baseURL := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "HOMEASSISTANT_URL"))
	token := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "HOMEASSISTANT_TOKEN"))
	if baseURL == "" || token == "" {
		return nil, nil, ""
	}
	services := splitNonEmpty(os.Getenv(brand.EnvPrefix + "HOMEASSISTANT_SERVICES"))

	ch := homeassistant.New(homeassistant.Config{
		BaseURL:   baseURL,
		Token:     token,
		Allowlist: channel.NewAllowlist(services),
		Bus:       k.Bus(),
	})

	var sink pulse.BriefSink
	if len(services) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, svc := range services {
				if err := ch.Send(ctx, channel.Outbound{ChannelID: svc, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	desc := fmt.Sprintf("outbound → %s, allowlist=%d service(s)", baseURL, len(services))
	if len(services) == 0 {
		desc = fmt.Sprintf("outbound → %s, NO allowlist (set AGEZT_HOMEASSISTANT_SERVICES to notify)", baseURL)
	}
	return ch, sink, desc
}

// buildTeams constructs the outbound Microsoft Teams channel when
// AGEZT_TEAMS_WEBHOOKS is set. Pulse briefs + `agt send` post a card to the named
// Teams Incoming Webhooks.
//
//	AGEZT_TEAMS_WEBHOOKS  name=url,name2=url2 — named Teams Incoming Webhook URLs;
//	                      `agt send --channel teams --to <name>` selects one.
func buildTeams(ctx context.Context, k *kernelruntime.Kernel) (*teams.Channel, pulse.BriefSink, string) {
	hooks := parseNamedWebhooks(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TEAMS_WEBHOOKS")))
	if len(hooks) == 0 {
		return nil, nil, ""
	}
	ch := teams.New(teams.Config{Webhooks: hooks, Bus: k.Bus()})

	names := ch.Names()
	sink := pulse.SinkFunc(func(b pulse.Brief) error {
		var firstErr error
		for _, name := range names {
			if err := ch.Send(ctx, channel.Outbound{ChannelID: name, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	})

	return ch, sink, fmt.Sprintf("outbound → %d Teams webhook(s)", len(names))
}

// buildSignal constructs the in-process Signal channel when AGEZT_SIGNAL_API_URL
// and AGEZT_SIGNAL_NUMBER are set, plus a Pulse brief sink to the allowlisted
// numbers. Returns (nil, nil, "") when unconfigured. Talks to an operator-run
// signal-cli-rest-api: long-polls /v1/receive for inbound, POSTs /v2/send for
// outbound (mirrors buildMatrix).
//
//	AGEZT_SIGNAL_API_URL     signal-cli-rest-api base URL (required), e.g. http://127.0.0.1:8080
//	AGEZT_SIGNAL_NUMBER      the registered Signal number this bot is, E.164 (required)
//	AGEZT_SIGNAL_RECIPIENTS  comma-separated allowlist of sender numbers (+ brief recipients)
//	AGEZT_SIGNAL_TOKEN       optional bearer token (a reverse proxy fronting the API)
//	AGEZT_SIGNAL_POLL_SECS   /v1/receive long-poll seconds (default 10)
func buildSignal(ctx context.Context, k *kernelruntime.Kernel) (*signalchan.Channel, pulse.BriefSink, string) {
	apiURL := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "SIGNAL_API_URL"))
	number := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "SIGNAL_NUMBER"))
	if apiURL == "" || number == "" {
		return nil, nil, ""
	}
	recipients := splitNonEmpty(os.Getenv(brand.EnvPrefix + "SIGNAL_RECIPIENTS"))
	token := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "SIGNAL_TOKEN"))
	poll := 0
	if v := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "SIGNAL_POLL_SECS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			poll = n
		}
	}

	ch := signalchan.New(signalchan.Config{
		APIURL:          apiURL,
		Number:          number,
		Token:           token,
		Allowlist:       channel.NewAllowlist(recipients),
		Bus:             k.Bus(),
		Handler:         makeChannelHandler(k),
		PollTimeoutSecs: poll,
	})

	// Pulse briefs → the allowlisted numbers. Nil sink when none configured (the
	// bot can still receive commands once a number is allowlisted, and operators
	// can still `agt send --channel signal --to <number>`).
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

	desc := fmt.Sprintf("listening, allowlist=%d number(s)", len(recipients))
	if len(recipients) == 0 {
		desc = "listening, NO allowlist (outbound-only; set AGEZT_SIGNAL_RECIPIENTS to allow commands)"
	}
	return ch, sink, desc
}

// buildPushChannels constructs every configured push-notification channel (ntfy,
// Pushover, Gotify, Pushbullet, Google Chat, Mattermost). Each is enabled by its
// own env and added as a distinct outbound channel; a combined Pulse sink fans a
// brief out to all of them. Returns nil when none are configured.
func buildPushChannels(ctx context.Context, k *kernelruntime.Kernel) ([]*push.Channel, pulse.BriefSink, string) {
	env := func(s string) string { return strings.TrimSpace(os.Getenv(brand.EnvPrefix + s)) }
	var chans []*push.Channel
	add := func(cfg push.Config) {
		cfg.Bus = k.Bus()
		if ch, err := push.New(cfg); err == nil {
			chans = append(chans, ch)
		}
	}
	if t := env("NTFY_TOPIC"); t != "" {
		add(push.Config{Kind: push.KindNtfy, Server: env("NTFY_SERVER"), Topic: t, Token: env("NTFY_TOKEN")})
	}
	if env("PUSHOVER_TOKEN") != "" {
		add(push.Config{Kind: push.KindPushover, Token: env("PUSHOVER_TOKEN"), User: env("PUSHOVER_USER")})
	}
	if env("GOTIFY_TOKEN") != "" {
		add(push.Config{Kind: push.KindGotify, Server: env("GOTIFY_SERVER"), Token: env("GOTIFY_TOKEN")})
	}
	if env("PUSHBULLET_TOKEN") != "" {
		add(push.Config{Kind: push.KindPushbullet, Token: env("PUSHBULLET_TOKEN")})
	}
	if u := env("GOOGLECHAT_WEBHOOK"); u != "" && !twoWayChatConfigured("GOOGLECHAT") {
		// Two-way Google Chat (AGEZT_GOOGLECHAT_ADDR) owns the name; see buildChatWebhook.
		add(push.Config{Kind: push.KindGoogleChat, URL: u})
	}
	if u := env("MATTERMOST_WEBHOOK"); u != "" && !twoWayChatConfigured("MATTERMOST") {
		add(push.Config{Kind: push.KindMattermost, URL: u})
	}
	if u := env("ROCKETCHAT_WEBHOOK"); u != "" {
		add(push.Config{Kind: push.KindRocketChat, URL: u})
	}
	if env("MASTODON_TOKEN") != "" && !twoWayMastodonConfigured() {
		// When an allowlist is set the dedicated two-way Mastodon channel owns the
		// "mastodon" name (it polls mentions + posts), so skip this outbound entry.
		add(push.Config{Kind: push.KindMastodon, Server: env("MASTODON_SERVER"), Token: env("MASTODON_TOKEN")})
	}
	if env("LINE_TOKEN") != "" && !twoWayLineConfigured() {
		// When AGEZT_LINE_SECRET is set the dedicated two-way LINE channel owns
		// the "line" name (see buildLine), so skip the outbound-only push entry.
		add(push.Config{Kind: push.KindLine, Token: env("LINE_TOKEN"), Target: env("LINE_TO")})
	}
	if env("ZULIP_APIKEY") != "" {
		add(push.Config{Kind: push.KindZulip, Server: env("ZULIP_SERVER"), User: env("ZULIP_EMAIL"), Token: env("ZULIP_APIKEY"), Target: env("ZULIP_STREAM"), Topic: env("ZULIP_TOPIC")})
	}
	// Feishu/DingTalk/WeCom: the dedicated two-way channels own the name when
	// their inbound addr is configured (see buildFeishu/buildDingTalk/buildWeCom).
	if u := env("FEISHU_WEBHOOK"); u != "" && (env("FEISHU_ADDR") == "" || env("FEISHU_APP_ID") == "") {
		add(push.Config{Kind: push.KindFeishu, URL: u})
	}
	if u := env("DINGTALK_WEBHOOK"); u != "" && env("DINGTALK_ADDR") == "" {
		add(push.Config{Kind: push.KindDingTalk, URL: u})
	}
	if u := env("WECOM_WEBHOOK"); u != "" && (env("WECOM_ADDR") == "" || env("WECOM_CORP_ID") == "") {
		add(push.Config{Kind: push.KindWeCom, URL: u})
	}
	if u := env("SYNOLOGY_WEBHOOK"); u != "" {
		add(push.Config{Kind: push.KindSynology, URL: u})
	}
	if len(chans) == 0 {
		return nil, nil, ""
	}
	names := make([]string, 0, len(chans))
	for _, ch := range chans {
		names = append(names, ch.Name())
	}
	sink := pulse.SinkFunc(func(b pulse.Brief) error {
		var firstErr error
		for _, ch := range chans {
			if err := ch.Send(ctx, channel.Outbound{Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	})
	return chans, sink, fmt.Sprintf("outbound → %s", strings.Join(names, ", "))
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
	if env("MATRIX_HOMESERVER") != "" && env("MATRIX_TOKEN") != "" {
		out = append(out, controlplane.ChannelInfo{
			Kind:      "matrix",
			Inbound:   true, // long-polls /sync whenever configured
			Addr:      env("MATRIX_HOMESERVER"),
			Allowlist: len(splitNonEmpty(os.Getenv(brand.EnvPrefix + "MATRIX_ROOMS"))),
		})
	}
	if env("SMS_ACCOUNT_SID") != "" && env("SMS_AUTH_TOKEN") != "" {
		addr := env("SMS_ADDR")
		out = append(out, controlplane.ChannelInfo{
			Kind:      "sms",
			Inbound:   addr != "", // inbound webhook served when an addr is set
			Addr:      addr,
			Allowlist: len(splitNonEmpty(os.Getenv(brand.EnvPrefix + "SMS_NUMBERS"))),
		})
	}
	if env("WHATSAPP_APP_SECRET") != "" && env("WHATSAPP_ACCESS_TOKEN") != "" {
		addr := env("WHATSAPP_ADDR")
		out = append(out, controlplane.ChannelInfo{
			Kind:      "whatsapp",
			Inbound:   addr != "", // inbound webhook served when an addr is set
			Addr:      addr,
			Allowlist: len(splitNonEmpty(os.Getenv(brand.EnvPrefix + "WHATSAPP_NUMBERS"))),
		})
	}
	if env("HOMEASSISTANT_URL") != "" && env("HOMEASSISTANT_TOKEN") != "" {
		out = append(out, controlplane.ChannelInfo{
			Kind:      "homeassistant",
			Inbound:   false, // outbound-only (notify API)
			Addr:      env("HOMEASSISTANT_URL"),
			Allowlist: len(splitNonEmpty(os.Getenv(brand.EnvPrefix + "HOMEASSISTANT_SERVICES"))),
		})
	}
	if env("TEAMS_WEBHOOKS") != "" {
		out = append(out, controlplane.ChannelInfo{
			Kind:      "teams",
			Inbound:   false, // outbound-only (incoming webhooks)
			Allowlist: len(parseNamedWebhooks(env("TEAMS_WEBHOOKS"))),
		})
	}
	if env("SIGNAL_API_URL") != "" && env("SIGNAL_NUMBER") != "" {
		out = append(out, controlplane.ChannelInfo{
			Kind:      "signal",
			Inbound:   true, // long-polls /v1/receive whenever configured
			Addr:      env("SIGNAL_API_URL"),
			Allowlist: len(splitNonEmpty(os.Getenv(brand.EnvPrefix + "SIGNAL_RECIPIENTS"))),
		})
	}
	return out
}

// parseNamedWebhooks parses a "name=url,name2=url2" spec into a name→url map.
// Each entry splits on the FIRST '=' (URLs may contain '='); blank names/urls
// are dropped.
func parseNamedWebhooks(spec string) map[string]string {
	out := map[string]string{}
	for _, entry := range strings.Split(spec, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		eq := strings.IndexByte(entry, '=')
		if eq <= 0 {
			continue
		}
		name := strings.TrimSpace(entry[:eq])
		url := strings.TrimSpace(entry[eq+1:])
		if name != "" && url != "" {
			out[name] = url
		}
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
	key  string // instanceKey(kind, label): bare kind for the default, "kind#label" otherwise
	desc string
	ch   channel.Channel
	sink pulse.BriefSink
}

// instanceKey addresses a channel instance: the bare kind for the default
// (back-compat) instance, "kind#label" for a labelled one.
func instanceKey(kind, label string) string { return channel.InstanceKey(kind, label) }

// channelLabels returns the configured NON-default instance labels for a channel
// kind, discovered from the process env (the daemon injects store+vault keys,
// including "#label" suffixed ones, at boot via injectConfig).
func channelLabels(kind string) []string {
	baseEnvs := settings.SectionEnvs(kind)
	if len(baseEnvs) == 0 {
		return nil
	}
	env := os.Environ()
	keys := make([]string, 0, len(env))
	for _, kv := range env {
		if k, _, ok := strings.Cut(kv, "="); ok {
			keys = append(keys, k)
		}
	}
	return settings.AccountLabels(keys, baseEnvs)
}

// buildAccounts builds every configured instance (the default plus each labelled
// account) of a channel kind via a per-instance builder. The builder MUST return
// a nil channel.Channel interface (not a typed nil) when its instance is
// unconfigured, so unconfigured instances are skipped cleanly.
func buildAccounts(ctx context.Context, k *kernelruntime.Kernel, kind string,
	build func(context.Context, *kernelruntime.Kernel, string, func(string) string) (channel.Channel, pulse.BriefSink, string)) []chanInstance {
	var out []chanInstance
	for _, label := range append([]string{""}, channelLabels(kind)...) {
		ch, sink, desc := build(ctx, k, label, settings.FieldGetter(label))
		if ch == nil {
			continue
		}
		out = append(out, chanInstance{key: instanceKey(kind, label), desc: desc, ch: ch, sink: sink})
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

// overlayEnv temporarily sets each base env to its "#label" value for a labelled
// instance, returning a restore func; for the default instance ("") it's a no-op.
// This lets the legacy buildXxx functions (which read os.Getenv directly) build a
// labelled account without rewriting them — safe because builds are synchronous
// and each channel reads its config into a struct before Start runs.
func overlayEnv(baseEnvs []string, label string) func() {
	if label == "" {
		return func() {}
	}
	type saved struct {
		key, val string
		had      bool
	}
	prev := make([]saved, 0, len(baseEnvs))
	for _, base := range baseEnvs {
		old, had := os.LookupEnv(base)
		prev = append(prev, saved{base, old, had})
		if v, ok := os.LookupEnv(base + "#" + label); ok {
			_ = os.Setenv(base, v)
		} else {
			_ = os.Unsetenv(base)
		}
	}
	return func() {
		for _, s := range prev {
			if s.had {
				_ = os.Setenv(s.key, s.val)
			} else {
				_ = os.Unsetenv(s.key)
			}
		}
	}
}

// buildAccountsLegacy builds the default + every "#label" instance of a channel
// whose buildXxx still reads os.Getenv directly, via overlayEnv. A typed-nil
// return (unconfigured instance) is skipped.
func buildAccountsLegacy[T channel.Channel](ctx context.Context, k *kernelruntime.Kernel, kind string,
	f func(context.Context, *kernelruntime.Kernel) (T, pulse.BriefSink, string)) []chanInstance {
	baseEnvs := settings.SectionEnvs(kind)
	var out []chanInstance
	for _, label := range append([]string{""}, channelLabels(kind)...) {
		restore := overlayEnv(baseEnvs, label)
		ch, sink, desc := f(ctx, k)
		restore()
		if rv := reflect.ValueOf(ch); rv.Kind() == reflect.Ptr && rv.IsNil() {
			continue
		}
		out = append(out, chanInstance{key: instanceKey(kind, label), desc: desc, ch: ch, sink: sink})
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
	// Default-ON (M817): the web console is the product surface, so a bare
	// `agezt` serves it without ceremony. AGEZT_WEB_ADDR overrides the bind
	// address; the explicit opt-OUT keywords disable it (mirrors the owner's
	// allow-by-default posture — you turn it off, you don't turn it on).
	addr := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WEB_ADDR"))
	defaulted := false
	switch strings.ToLower(addr) {
	case "off", "disabled", "none", "no", "0", "false":
		return ""
	case "":
		addr = "127.0.0.1:8787"
		defaulted = true
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
	if err != nil && defaulted {
		// The default port is taken (a second daemon, or another app on 8787).
		// Don't leave the console dark over a port clash — fall back to an
		// OS-assigned free port on loopback so a bare `agezt` ALWAYS gets a UI.
		fmt.Fprintf(stdout, "  web ui           : %s busy — using a free port instead\n", addr)
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		fmt.Fprintf(stdout, "  web ui           : disabled (listen %s: %v)\n", addr, err)
		return ""
	}
	wsrv := webui.New(k.Bus(), client, token)
	// Optional console password (M817 → M933): when AGEZT_WEB_PASSWORD is set, a
	// token-less visit shows the login screen and the password opens the console
	// (alternative door); the tokened banner URL keeps working alone. Wired as a
	// LIVE source — re-read from the env per gate decision — so setting the
	// password from the Setup wizard / Config Center (whose live-apply path
	// updates the env) takes effect without a restart. It's a SECRET in the
	// schema: the value lives in the vault; injectConfig bridged it into the env
	// at boot. AGEZT_WEB_PASSWORD_STRICT=on restores M817 compose semantics
	// (token AND password on every data route) for consoles exposed beyond
	// loopback (e.g. a tunnel).
	wsrv.SetPasswordFn(func() string { return strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WEB_PASSWORD")) })
	wsrv.SetPasswordStrict(strings.EqualFold(os.Getenv(brand.EnvPrefix+"WEB_PASSWORD_STRICT"), "on"))
	passwordOn := strings.TrimSpace(os.Getenv(brand.EnvPrefix+"WEB_PASSWORD")) != ""
	// Wire speech-to-text for the chat mic button (M689) when an STT endpoint is
	// configured. Guard on the concrete pointer so a nil never becomes a non-nil
	// interface (which would make /api/transcribe think STT is configured).
	if t := sttTranscriberFromEnv(); t != nil {
		wsrv.SetTranscriber(t)
		fmt.Fprintf(stdout, "  voice input      : enabled (chat mic → speech-to-text)\n")
	}
	srv := newGuardedHTTPServer(wsrv.Handler())
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
	if passwordOn {
		desc += "  (password login enabled at http://" + ln.Addr().String() + "/)"
	}
	if !isLoopback(addr) {
		desc += "  [WARNING: not loopback — reachable beyond localhost]"
	}
	return desc
}

// buildTunnel starts a public tunnel to a local HTTP service when AGEZT_TUNNEL
// (cloudflared|ngrok) or AGEZT_TUNNEL_CMD (a custom command) is set. It targets
// AGEZT_TUNNEL_TARGET, else the Web UI addr, else the REST addr. The supervised
// binary's public URL is printed to the daemon log once it connects. Returns ""
// (disabled) when no tunnel is configured.
//
//	AGEZT_TUNNEL         provider preset: cloudflared | ngrok
//	AGEZT_TUNNEL_CMD     explicit command (whitespace-split), overrides the preset
//	AGEZT_TUNNEL_TARGET  local URL to expose (default: the Web UI, else REST, addr)
func buildTunnel(ctx context.Context, stdout io.Writer) string {
	provider := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TUNNEL"))
	cmdStr := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TUNNEL_CMD"))
	if provider == "" && cmdStr == "" {
		return ""
	}

	target := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TUNNEL_TARGET"))
	if target == "" {
		if web := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "WEB_ADDR")); web != "" {
			target = addrToURL(web)
		} else if rest := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "REST_ADDR")); rest != "" {
			target = addrToURL(rest)
		}
	}

	cfg := tunnel.Config{
		Provider:  provider,
		TargetURL: target,
		OnURL: func(u string) {
			fmt.Fprintf(stdout, "  tunnel public URL: %s  [the service is now reachable on the public internet]\n", u)
		},
	}
	if cmdStr != "" {
		cfg.Command = strings.Fields(cmdStr)
	}

	tun, err := tunnel.New(cfg)
	if err != nil {
		fmt.Fprintf(stdout, "  tunnel           : disabled (%v)\n", err)
		return ""
	}
	go tun.Start(ctx)

	what := "custom command"
	if cmdStr == "" {
		what = provider
	}
	desc := fmt.Sprintf("%s → exposing %s (public URL prints here once connected)", what, target)
	if target == "" {
		desc = fmt.Sprintf("%s (custom command; no local target derived)", what)
	}
	return desc
}

// addrToURL turns a listen addr (host:port, or :port) into a loopback http URL.
func addrToURL(addr string) string {
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	return "http://" + addr
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
	// Fast path: the Governor keeps a bounded in-memory per-correlation usage
	// index, so usage for a just-completed run is O(1) instead of an O(journal)
	// scan per API response (which a client hammering the API could amplify into a
	// DoS). The journal scan below stays the authoritative fallback for any
	// correlation not in the bounded index, so the reported numbers are identical.
	if ur, ok := e.k.Provider().(interface {
		UsageFor(string) (int, int, bool)
	}); ok {
		if in, out, ok := ur.UsageFor(corr); ok && (in != 0 || out != 0) {
			return in, out, true
		}
	}
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
	// Speech-to-text upload (POST /v1/audio/transcriptions) — wired when an STT
	// endpoint is configured (a key, or a custom URL for a local whisper server).
	// Same source of truth as the Web UI mic button (M689).
	if t := sttTranscriberFromEnv(); t != nil {
		api.SetTranscriber(t)
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
func buildRESTAPI(ctx context.Context, k *kernelruntime.Kernel, reg *tenant.Registry, draining *atomic.Bool, boardStore *board.Store, boardNotify func(board.Message, string), updateSvc *update.Service, stdout io.Writer) string {
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
	// Mailbox (M937): the shared message board for SDK apps — the same store
	// instance the `board` tool writes, so external sends wake standing orders
	// exactly like agent sends. Nil when the board failed to open.
	if boardStore != nil {
		rest.SetMailbox(boardStore, boardNotify)
	}
	// Self-update engine (M860): wired when AGEZT_UPDATE_ENDPOINT or
	// AGEZT_UPDATE_GITHUB_OWNER/REPO is set. Nil when not configured;
	// the update handlers report that.
	if updateSvc != nil {
		rest.SetUpdateService(updateSvc)
	}
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
		// Cap autonomous action at the order's max_trust ceiling (SPEC-16 §4): a
		// normally auto-allowed tool is downgraded to Ask/Deny within this run.
		if lvl, perr := edict.ParseTrustLevel(o.Initiative.MaxTrust); perr == nil {
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
				if err := runScheduledSystemTask(ctx, k, corr, id, ent.SystemTask); err != nil {
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
		if info, ok := scheduledSystemTaskInfo(ent.SystemTask); ok {
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

func scheduledSystemTaskInfo(name string) (cadence.SystemTaskInfo, bool) {
	name = strings.TrimSpace(name)
	for _, info := range cadence.SystemTaskInfos() {
		if info.Name == name {
			return info, true
		}
	}
	return cadence.SystemTaskInfo{}, false
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

func runScheduledSystemTask(ctx context.Context, k *kernelruntime.Kernel, corr, scheduleID, task string) error {
	switch strings.TrimSpace(task) {
	case cadence.SystemTaskCatalogSync:
		return runScheduledCatalogSync(ctx, k, corr, scheduleID)
	case cadence.SystemTaskArtifactCollect:
		return runScheduledArtifactCollect(ctx, k, corr, scheduleID)
	case cadence.SystemTaskMemoryClean:
		return runScheduledMemoryClean(ctx, k, corr, scheduleID)
	case cadence.SystemTaskMemoryTidy:
		return runScheduledMemoryTidy(ctx, k, corr, scheduleID)
	case cadence.SystemTaskLogClean:
		return runScheduledLogClean(ctx, k, corr, scheduleID)
	case cadence.SystemTaskGraveyardScan:
		return runScheduledGraveyardScan(ctx, k, corr, scheduleID)
	default:
		return fmt.Errorf("schedule %s: unknown system task %q", scheduleID, task)
	}
}

// graveyardRetentionDays is the retention window for the graveyard scan: retired
// agents older than this are reported as removal-eligible. 0 (the default) means
// keep-forever — the scan still reports graveyard size but flags nothing eligible.
// This task is NOTIFY-ONLY: it never archives or deletes (removal stays an explicit
// operator action), so a misconfigured window can only over-report, never destroy.
func graveyardRetentionDays() int {
	raw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "GRAVEYARD_RETENTION_DAYS"))
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func runScheduledGraveyardScan(ctx context.Context, k *kernelruntime.Kernel, corr, scheduleID string) error {
	_ = ctx
	retentionDays := graveyardRetentionDays()
	nowMS := time.Now().UnixMilli()
	cutoffMS := int64(0)
	if retentionDays > 0 {
		cutoffMS = nowMS - int64(retentionDays)*24*3600*1000
	}
	graveyard := 0
	eligible := make([]string, 0)
	for _, p := range k.Roster().List() {
		if !p.Retired {
			continue
		}
		graveyard++
		if cutoffMS > 0 && p.RetiredMS > 0 && p.RetiredMS <= cutoffMS {
			eligible = append(eligible, p.Slug)
		}
	}
	sort.Strings(eligible)
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "schedule.system_task.graveyard_scan",
		Kind:          event.KindInfo,
		Actor:         "schedule",
		CorrelationID: corr,
		Payload: map[string]any{
			"schedule_id":     scheduleID,
			"system_task":     cadence.SystemTaskGraveyardScan,
			"retention_days":  retentionDays,
			"graveyard_count": graveyard,
			"eligible_count":  len(eligible),
			"eligible":        eligible,
			// Explicit: this task only reports — it performs no removal.
			"action": "report_only",
		},
	})
	return nil
}

func runScheduledArtifactCollect(ctx context.Context, k *kernelruntime.Kernel, corr, scheduleID string) error {
	_ = ctx
	idx := k.ArtifactIndex()
	if idx == nil {
		return fmt.Errorf("schedule %s: artifact index unavailable", scheduleID)
	}
	const olderThanDays = 30
	cutoff := time.Now().Add(-time.Duration(olderThanDays) * 24 * time.Hour).UnixMilli()
	collected, bytes := idx.Collect(cutoff)
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "schedule.system_task.artifact_collect",
		Kind:          event.KindInfo,
		Actor:         "schedule",
		CorrelationID: corr,
		Payload: map[string]any{
			"schedule_id":     scheduleID,
			"system_task":     cadence.SystemTaskArtifactCollect,
			"older_than_days": olderThanDays,
			"cutoff_ms":       cutoff,
			"collected":       collected,
			"bytes":           bytes,
		},
	})
	return nil
}

func runScheduledMemoryClean(ctx context.Context, k *kernelruntime.Kernel, corr, scheduleID string) error {
	_ = ctx
	if k.Memory() == nil {
		return fmt.Errorf("schedule %s: memory unavailable", scheduleID)
	}
	report, err := k.Memory().CleanLowValue(corr, false)
	if err != nil {
		return err
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "schedule.system_task.memory_clean",
		Kind:          event.KindInfo,
		Actor:         "schedule",
		CorrelationID: corr,
		Payload: map[string]any{
			"schedule_id": scheduleID,
			"system_task": cadence.SystemTaskMemoryClean,
			"scanned":     report.Scanned,
			"rejected":    report.Rejected,
			"removed":     report.Removed,
		},
	})
	return nil
}

func runScheduledMemoryTidy(ctx context.Context, k *kernelruntime.Kernel, corr, scheduleID string) error {
	_ = ctx
	if k.Memory() == nil {
		return fmt.Errorf("schedule %s: memory unavailable", scheduleID)
	}
	collapsed, err := k.Memory().DedupeDistilled(corr, false)
	if err != nil {
		return err
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "schedule.system_task.memory_tidy",
		Kind:          event.KindInfo,
		Actor:         "schedule",
		CorrelationID: corr,
		Payload: map[string]any{
			"schedule_id": scheduleID,
			"system_task": cadence.SystemTaskMemoryTidy,
			"collapsed":   collapsed,
		},
	})
	return nil
}

func runScheduledLogClean(ctx context.Context, k *kernelruntime.Kernel, corr, scheduleID string) error {
	_ = ctx
	j := k.Journal()
	if j == nil {
		return fmt.Errorf("schedule %s: journal unavailable", scheduleID)
	}
	var events int64
	var oldestMS int64
	var latestMS int64
	if err := j.Range(func(e *event.Event) error {
		events++
		if oldestMS == 0 || (e.TSUnixMS > 0 && e.TSUnixMS < oldestMS) {
			oldestMS = e.TSUnixMS
		}
		if e.TSUnixMS > latestMS {
			latestMS = e.TSUnixMS
		}
		return nil
	}); err != nil {
		return err
	}
	headSeq, headHash := j.Head()
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "schedule.system_task.log_clean",
		Kind:          event.KindInfo,
		Actor:         "schedule",
		CorrelationID: corr,
		Payload: map[string]any{
			"schedule_id":       scheduleID,
			"system_task":       cadence.SystemTaskLogClean,
			"events_scanned":    events,
			"oldest_unix_ms":    oldestMS,
			"latest_unix_ms":    latestMS,
			"head_seq":          headSeq,
			"head_hash":         headHash,
			"physical_deletion": false,
			"effect_class":      "log_maintenance",
		},
	})
	return nil
}

func runScheduledCatalogSync(ctx context.Context, k *kernelruntime.Kernel, corr, scheduleID string) error {
	url := envOrDefaultLocal(brand.EnvPrefix+"CATALOG_URL", catalog.DefaultSyncURL)
	syncer := catalog.NewSyncer()
	syncer.URL = url
	raw, cat, res, err := syncer.Sync(ctx)
	if err != nil {
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "catalog.sync",
			Kind:          event.KindCatalogSyncFailed,
			Actor:         "schedule",
			CorrelationID: corr,
			Payload:       map[string]any{"url": url, "schedule_id": scheduleID, "system_task": cadence.SystemTaskCatalogSync, "error": err.Error()},
		})
		return err
	}
	if err := k.CatalogStore().SaveAPI(raw, url); err != nil {
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "catalog.sync",
			Kind:          event.KindCatalogSyncFailed,
			Actor:         "schedule",
			CorrelationID: corr,
			Payload:       map[string]any{"url": url, "schedule_id": scheduleID, "system_task": cadence.SystemTaskCatalogSync, "error": "save: " + err.Error()},
		})
		return fmt.Errorf("save: %w", err)
	}
	freshCat, providersReloaded, provErr := k.Reload()
	if freshCat == nil {
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "catalog.sync",
			Kind:          event.KindCatalogSyncFailed,
			Actor:         "schedule",
			CorrelationID: corr,
			Payload:       map[string]any{"url": url, "schedule_id": scheduleID, "system_task": cadence.SystemTaskCatalogSync, "error": "reload: " + provErr.Error()},
		})
		return fmt.Errorf("reload: %w", provErr)
	}
	payload := map[string]any{
		"url":                url,
		"schedule_id":        scheduleID,
		"system_task":        cadence.SystemTaskCatalogSync,
		"bytes":              res.Bytes,
		"provider_count":     res.ProviderCount,
		"model_count":        res.ModelCount,
		"duration_ms":        res.Duration.Milliseconds(),
		"providers_reloaded": providersReloaded,
		"effect_class":       "config_update",
	}
	if provErr != nil {
		payload["provider_reload_error"] = provErr.Error()
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "catalog.sync",
		Kind:          event.KindCatalogSynced,
		Actor:         "schedule",
		CorrelationID: corr,
		Payload:       payload,
	})
	_ = cat
	return nil
}

func envOrDefaultLocal(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
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

// startUpdateChecker runs the background update checker goroutine (M860).
// It fires on the configured CheckInterval. When an update is found, it
// auto-applies after the daemon goes idle (drain). The journal receives an
// event so the update is auditable. The watchdog is signalled to restart
// with the new binary.
func startUpdateChecker(ctx context.Context, k *kernelruntime.Kernel, svc *update.Service, stdout, stderr io.Writer) {
	ticker := time.NewTicker(svc.CheckInterval())
	defer ticker.Stop()

	// Do an immediate first check rather than waiting for the first interval.
	check := func() {
		result, err := svc.Check(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "%s: auto-update check: %v\n", brand.Binary, err)
			return
		}
		if result.Update == nil {
			return // already up to date
		}

		info := result.Update
		fmt.Fprintf(stdout, "%s: auto-update: %s available (current: %s)\n", brand.Binary, info.Version, result.Current)

		// Publish journal event for auditability.
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "update.available",
			Kind:    event.KindInfo,
			Actor:   "update-checker",
			Payload: map[string]any{
				"current_version": result.Current,
				"new_version":     info.Version,
				"url":             info.URL,
			},
		})

		// Drain and apply.
		fmt.Fprintf(stdout, "%s: auto-update: draining daemon for %s\n", brand.Binary, info.Version)
		_, activeRuns := k.DrainAndHalt(svc.DrainTimeout())
		if activeRuns > 0 {
			fmt.Fprintf(stderr, "%s: auto-update: drain timeout (%d runs still active)\n", brand.Binary, activeRuns)
			// Don't apply — leave the daemon running, let the operator investigate.
			return
		}

		err = svc.Apply(ctx, info, func(context.Context, time.Duration) update.DrainResult {
			// Already drained above; no-op drain for Apply.
			return update.DrainResult{}
		})
		if err != nil {
			_, _ = k.Bus().Publish(event.Spec{
				Subject: "update.failed",
				Kind:    event.KindAnomalyDetected,
				Actor:   "update-checker",
				Payload: map[string]any{
					"version": info.Version,
					"error":   err.Error(),
				},
			})
			fmt.Fprintf(stderr, "%s: auto-update failed: %v (daemon stays running)\n", brand.Binary, err)
			return
		}

		// Success. Publish event and signal shutdown so watchdog spawns new binary.
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "update.applied",
			Kind:    event.KindInfo,
			Actor:   "update-checker",
			Payload: map[string]any{"version": info.Version},
		})
		fmt.Fprintf(stdout, "%s: auto-update: %s applied, restarting\n", brand.Binary, info.Version)
		// Signal graceful shutdown — watchdog will restart with new binary.
		// The watchdog is already watching for exit; it will re-spawn.
		os.Exit(0) // clean exit; watchdog handles restart
	}

	// Immediate first check.
	check()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
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

// providerMiddleware builds the opt-in provider middleware stack from the
// environment (M997). It is empty by default, so every provider is registered
// unwrapped and behaviour is unchanged. Operators opt in to:
//   - DefaultParams: AGEZT_GEN_TEMPERATURE / AGEZT_GEN_TOP_P / AGEZT_GEN_REASONING_EFFORT
//     supply per-call sampling defaults filled in only where a request left them unset.
//   - ExtractReasoning: AGEZT_EXTRACT_REASONING=on pulls inline <think>…</think> out of
//     the answer into ReasoningContent (for inline-reasoning models on OpenAI-compatible /
//     Ollama gateways that don't use a dedicated reasoning field).
//   - SimulateStreaming: AGEZT_SIMULATE_STREAMING=on lets non-streaming providers present
//     a single-chunk stream for a uniform UI.
func providerMiddleware() []agent.Middleware {
	envOn := func(suffix string) bool {
		v := strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + suffix)))
		return v == "1" || v == "on" || v == "true" || v == "yes"
	}
	var mws []agent.Middleware

	var defaults agent.Params
	if s := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "GEN_TEMPERATURE")); s != "" {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			defaults.Temperature = &f
		}
	}
	if s := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "GEN_TOP_P")); s != "" {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			defaults.TopP = &f
		}
	}
	defaults.ReasoningEffort = strings.TrimSpace(os.Getenv(brand.EnvPrefix + "GEN_REASONING_EFFORT"))
	if !defaults.IsZero() {
		mws = append(mws, agent.DefaultParamsMiddleware(defaults))
	}
	if envOn("EXTRACT_REASONING") {
		mws = append(mws, agent.ExtractReasoningMiddleware("<think>", "</think>"))
	}
	if envOn("SIMULATE_STREAMING") {
		mws = append(mws, agent.SimulateStreamingMiddleware())
	}
	return mws
}

// buildGovernor constructs the routing layer: one primary provider plus every
// other credentialed catalog provider as a model-routable alternate. Returns the
// Governor (also serves as agent.Provider), a human-readable banner description,
// and the run model for the kernel config ("" when none is configured).
//
// The daemon has NO default provider, NO credential auto-pick, NO offline mock
// fallback, and NO default model (owner rule: "hiçbir default provider/model").
//
// **Provider selection (catalog-driven):**
//
//	$AGEZT_PROVIDER=<catalog-id>    → e.g. "anthropic", "ollama-local",
//	                                  "groq", "openai" — any provider in the
//	                                  synced catalog. The ONLY way to select a
//	                                  primary. An unknown id is a hard error.
//	(unset)                          → UNCONFIGURED: a sentinel primary that
//	                                  fails every LLM call with an actionable
//	                                  "configure a provider" error. The daemon,
//	                                  Web UI, and Setup still run.
//	$AGEZT_MODEL=<model-id>         → the run model. If unset, runs resolve their
//	                                  model from per-task routing or a fallback
//	                                  chain; with neither, the governor returns
//	                                  ErrNoModelConfigured.
func buildGovernor(cat *catalog.Catalog, lookup func(string) string, baseDir string) (*governor.Governor, string, string, error) {
	reg := governor.NewRegistry()
	mw := providerMiddleware() // M997: opt-in; empty by default → providers registered unwrapped
	primary, primaryDesc, model, authMode, err := selectPrimary(cat, lookup, baseDir)
	if err != nil {
		return nil, "", "", err
	}
	primaryName := primary.Name()
	if err := reg.Register(&governor.ProviderInfo{
		Name:     primaryName,
		Provider: agent.Wrap(primary, mw...),
		AuthMode: authMode,
		Models:   catalogModelIDs(cat, primaryName),
	}); err != nil {
		return nil, "", "", fmt.Errorf("register primary: %w", err)
	}
	// Track which catalog providers actually got registered — the eligible
	// set for cross-provider down-routing (M40). Keyed by catalog provider id,
	// so it matches catalog lookups. (For the "unconfigured" sentinel this is a
	// non-catalog name that simply won't match, which is fine.)
	registered := map[string]bool{primaryName: true}

	// Register every OTHER credentialed + supported catalog provider as a
	// model-routable alternate (SPEC-15 §1): a request naming one of their
	// models is routed to that provider (per-request model routing), while the
	// primary stays the default. Build failures are skipped, never fatal — a
	// misconfigured alternate must not stop the daemon. Each compat provider's
	// Name() is its unique catalog id (wrapNamed), so there are no collisions.
	extraProviders := 0
	for _, entry := range cat.ProviderList() {
		if entry.ID == primaryName {
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
			Provider: agent.Wrap(p, mw...),
			AuthMode: auth,
			Models:   catalogModelIDs(cat, entry.ID),
		}); err != nil {
			continue // duplicate name or similar — skip gracefully
		}
		registered[entry.ID] = true
		extraProviders++
	}

	// ChatGPT ("Sign in with ChatGPT") registers as a subscription alternate when
	// signed in (and not already the primary) — its models route to the Responses
	// backend adapter.
	if registerChatGPTAlternate(reg, baseDir, primaryName, false) {
		registered["chatgpt"] = true
		extraProviders++
	}

	// No offline mock fallback: the daemon never silently answers with a mock
	// (owner rule). When the primary fails and no fallback chain / alternate
	// serves the request, the governor surfaces the real error.
	fallbackDesc := ""

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

	// Per-task-type model fallback CHAINS (M703): task → ordered model ids tried
	// in turn. Supersedes TASK_MODEL_OVERRIDES for the same task. Editable live
	// via the Routing UI / control plane (persisted back into this env var).
	var taskModelChains governor.TaskModelChains
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TASK_MODEL_CHAINS")); spec != "" {
		parsed, err := governor.ParseTaskModelChainsEnv(spec)
		if err != nil {
			return nil, "", "", fmt.Errorf("AGEZT_TASK_MODEL_CHAINS: %w", err)
		}
		taskModelChains = parsed
	}

	// Named reusable fallback chains (M963): a registry of "@name → [models]"
	// referenced anywhere a model is chosen, plus an optional default chain for
	// runs that resolve to none. Editable live via the Chains UI (persisted back
	// into these env vars).
	var fallbackChains map[string][]string
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "FALLBACK_CHAINS")); spec != "" {
		parsed, err := governor.ParseFallbackChainsEnv(spec)
		if err != nil {
			return nil, "", "", fmt.Errorf("AGEZT_FALLBACK_CHAINS: %w", err)
		}
		fallbackChains = parsed
	}
	defaultChain := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "DEFAULT_CHAIN"))

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

	// Opt-in LLM response cache (M888): AGEZT_LLM_CACHE_TTL=<duration> serves
	// an IDENTICAL completion request from memory within the TTL — no provider
	// call, no spend. Off when unset (an LLM is not a pure function; chat
	// regenerate wants fresh samples). Malformed = hard startup error.
	var respCacheTTL time.Duration
	if spec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "LLM_CACHE_TTL")); spec != "" {
		d, derr := time.ParseDuration(spec)
		if derr != nil || d < 0 {
			return nil, "", "", fmt.Errorf("%sLLM_CACHE_TTL: want a non-negative Go duration (e.g. 5m), got %q", brand.EnvPrefix, spec)
		}
		respCacheTTL = d
	}

	gov, err := governor.New(governor.Config{
		Registry:                reg,
		ResponseCacheTTL:        respCacheTTL,
		DailyCeilingMicrocents:  ceiling,
		RateLimitPerMin:         ratePerMin,
		TaskRoutes:              taskRoutes,
		TaskRouteRequires:       taskRequires,
		TaskModelOverrides:      taskModels,
		TaskModelChains:         taskModelChains,
		FallbackChains:          fallbackChains,
		DefaultChain:            defaultChain,
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
		ModelStrictToolArgsNative: cat.StrictToolArgsNative,
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

// reconcileAlternateProviders mirrors buildGovernor's alternate registration on
// the hot-reload path (M928). buildGovernor registers every other credentialed+
// supported catalog provider so per-task model chains route each model to its
// serving provider — but the reload path used to swap only the primary. After
// the first-run flow (boot catalog-less → mock primary, then catalog sync +
// vault keys + reload) the other keyed providers stayed unregistered, so a
// chain model like "glm-5.1" was sent to whatever the primary happened to be
// until a daemon restart. This drops alternates that lost eligibility (key
// revoked / provider gone from the catalog) and (re-)registers the eligible
// set with their catalog Models so per-request model routing works. The caller
// installs the primary LAST via gov.Replace, which also rebuilds the routing
// chains over the reconciled registry. Build failures are skipped, never
// fatal — a misconfigured alternate must not stop the reload (same rule as
// boot). Fallback entries (the offline mock) are never touched.
func reconcileAlternateProviders(reg *governor.Registry, c *catalog.Catalog, lookup func(string) string, primaryName, baseDir string) {
	eligible := map[string]bool{primaryName: true}
	for _, entry := range c.ProviderList() {
		if entry.ID == primaryName {
			continue
		}
		if !compat.IsSupportedFamily(entry.Family()) || !entry.HasCredentials(lookup) {
			continue
		}
		p, _, _, auth, err := buildFromCatalog(entry, "", lookup)
		if err != nil {
			continue
		}
		if err := reg.Replace(&governor.ProviderInfo{
			Name:     p.Name(),
			Provider: p,
			AuthMode: auth,
			Models:   catalogModelIDs(c, entry.ID),
		}); err != nil {
			continue
		}
		eligible[entry.ID] = true
	}
	// ChatGPT subscription alternate (re-)registered when signed in.
	if registerChatGPTAlternate(reg, baseDir, primaryName, true) {
		eligible["chatgpt"] = true
	}
	for _, info := range reg.All() {
		if info.IsFallback || eligible[info.Name] {
			continue
		}
		reg.Remove(info.Name)
	}
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
// the resolved run model id (may be ""), the auth-mode tag for the
// Governor's registry, and an error.
//
// Selection:
//
//  1. AGEZT_PROVIDER=<catalog id> → look up in cat; compat.Build it. The ONLY
//     way to select a real primary; an unknown id is a hard error.
//  2. AGEZT_PROVIDER unset        → the "unconfigured" sentinel primary. No
//     auto-pick, no mock. The daemon boots so Setup/routing can be configured,
//     but LLM runs fail fast with an actionable error.
//
// The run model comes from AGEZT_MODEL when set; otherwise it is left empty and
// resolved per-run from routing / a fallback chain (or ErrNoModelConfigured).
func selectPrimary(cat *catalog.Catalog, lookup func(string) string, baseDir string) (agent.Provider, string, string, governor.AuthMode, error) {
	// AGEZT_PROVIDER and AGEZT_MODEL are *config*, not credentials —
	// always read from env directly (operators may want a one-off
	// override that doesn't sit in the vault).
	want := strings.ToLower(strings.TrimSpace(os.Getenv(brand.EnvPrefix + "PROVIDER")))
	modelOverride := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "MODEL"))

	// ChatGPT ("Sign in with ChatGPT") is not a compat catalog provider — it uses
	// the OAuth token store + Responses adapter, so build it directly.
	if want == "chatgpt" {
		prov, desc, auth, ok := buildChatGPTPrimary(baseDir, modelOverride)
		if !ok {
			return nil, "", "", "", fmt.Errorf(
				"%sPROVIDER=chatgpt but not signed in — use Setup → Providers → Sign in with ChatGPT first", brand.EnvPrefix)
		}
		return prov, desc, modelOverride, auth, nil
	}

	// Explicit catalog id is the ONLY way to select a primary. The daemon has no
	// default provider, never auto-picks from credentials, and has no offline mock
	// fallback (owner rule: "hiçbir default provider/model"). An unknown id is a
	// hard error so a typo is loud, not silently degraded.
	if want != "" {
		entry, ok := cat.Providers[want]
		if !ok {
			return nil, "", "", "", fmt.Errorf(
				"%sPROVIDER=%q not in catalog; run `agt catalog sync` then `agt catalog list`",
				brand.EnvPrefix, want)
		}
		return buildFromCatalog(entry, modelOverride, lookup)
	}

	// AGEZT_PROVIDER unset → boot UNCONFIGURED. The daemon, Web UI, and Setup all
	// run so the operator can add a provider + key and configure routing/chains,
	// but any LLM call fails fast with an actionable error (unconfiguredProvider).
	// No credential auto-pick, no mock.
	return unconfiguredProvider{},
		"unconfigured (no " + brand.EnvPrefix + "PROVIDER set — add a provider + key in Setup → Providers; LLM runs fail until then)",
		"", governor.AuthLocal, nil
}

// buildFromCatalog finalises a catalog entry into a wire Provider.
// Shared by both the explicit-id path and the auto-pick path.
// `lookup` is the chained vault+env credential resolver from runDaemon.
func buildFromCatalog(entry *catalog.Provider, modelOverride string, lookup func(string) string) (agent.Provider, string, string, governor.AuthMode, error) {
	// The daemon has NO default run model. AGEZT_MODEL, when set, is the model
	// every run uses unless per-task routing or a fallback chain overrides it.
	// When AGEZT_MODEL is empty the returned run model stays "" — so cfg.Model is
	// empty and the governor refuses any run that doesn't resolve a model via
	// routing/chain (ErrNoModelConfigured), per the owner's no-default rule.
	//
	// compat.Build still needs *a* concrete, catalog-valid model id to construct
	// the provider wire, so when AGEZT_MODEL is empty we fall back to the first
	// catalog model as an INERT construction placeholder. It is never surfaced as
	// a run default (cfg.Model stays "") and is never reached at call time (the
	// governor guard + per-provider model-required errors fire first).
	runModel := modelOverride
	constructModel := modelOverride
	if constructModel == "" {
		constructModel = compat.FirstModelID(entry)
	}
	if constructModel == "" {
		return nil, "", "", "", fmt.Errorf("provider %q in catalog has no models; set %sMODEL", entry.ID, brand.EnvPrefix)
	}
	prov, _, err := compat.Build(entry, constructModel, lookup)
	if err != nil {
		return nil, "", "", "", err
	}
	auth := governor.AuthAPIKey
	if len(entry.Env) == 0 {
		auth = governor.AuthLocal
	}
	modelDesc := runModel
	if modelDesc == "" {
		modelDesc = "(unset — resolved from routing/fallback chain per run)"
	}
	desc := fmt.Sprintf("%s(catalog; family=%s, model=%s)", entry.ID, entry.Family(), modelDesc)
	return prov, desc, runModel, auth, nil
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
	if ws, ok := tools["web_search"].(*websearch.Tool); ok {
		ws.OnBlock = publish("web_search")
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

// every command through the supplied Warden engine.
// workspaceRoot resolves the directory the file and shell tools share:
// $AGEZT_WORKSPACE, or <baseDir>/workspace by default. Used by buildTools (to
// scope the tools) and by the kernel Config (to tell the model where it is via
// the M609 environment preamble), so the two never drift.
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

func workspaceRoot(baseDir string) string {
	if ws := os.Getenv(brand.EnvPrefix + "WORKSPACE"); ws != "" {
		return ws
	}
	return filepath.Join(baseDir, "workspace")
}

func buildTools(baseDir string, stderr io.Writer, ward warden.Engine) (map[string]agent.Tool, []kernelruntime.PluginInfo, map[string]string, string, error) {
	out := map[string]agent.Tool{}
	var registered []string
	// Manifest of external plugins that successfully spawned.
	// Surfaced to the kernel via Config.Plugins so the control
	// plane can serve `agt plugin list`. Stays nil when no
	// AGEZT_PLUGINS entries are configured.
	var manifestEntries []kernelruntime.PluginInfo
	// Declared tool capabilities from plugin manifests (M900): prefixed tool
	// name → Edict capability the plugin claims to belong to. Validated by the
	// kernel at Open (unknown axes dropped). Stays nil without plugins.
	var toolCaps map[string]string

	// Workspace root — both the file tool (scoped to it) and the shell tool (runs
	// in it, M609) use this so `dir`/`ls` (shell) and `file read x` (file) agree
	// on what "here" is.
	wsRoot := workspaceRoot(baseDir)

	// shell — always registered, routed through Warden. Effective
	// isolation profile depends on host OS (M1.c: always ProfileNone
	// with the request journaled as a downgrade on non-Linux). Runs in the
	// shared workspace root (M609) so it sees the same files as the file tool.
	sh := shell.NewWithWarden(ward)
	sh.WorkDir = wsRoot
	out["shell"] = sh
	registered = append(registered, "shell(warden=requested-namespace)")

	// file — scoped to the same workspace root.
	ft, err := filetool.New(wsRoot)
	if err != nil {
		return nil, nil, nil, "", fmt.Errorf("file tool: %w", err)
	}
	out["file"] = ft
	registered = append(registered, "file(root="+ft.Root()+")")

	// config — the agent's window into the Config Center: read/write/register
	// settings (same registry + vault as the `agt config` CLI and HTTP routes).
	// The kernel is bound after runtime.Open (see bindConfigTool) so live-apply
	// fields (provider/model) can rebuild the provider in place.
	out["config"] = configtool.New(baseDir)
	registered = append(registered, "config()")

	// http — default-ALLOW (M818, owner law: every capability open unless you opt
	// out). Any PUBLIC host is reachable out of the box; the opt-OUT is a non-empty
	// $AGEZT_HTTP_ALLOWED_HOSTS (comma-separated), which RESTRICTS the tool to just
	// those hosts. The SSRF egress guard (loopback / private / cloud-metadata
	// refused) is the hard floor and stays on regardless — relaxed only by the
	// explicit AGEZT_HTTP_ALLOW_* flags below. So "open" means the public internet,
	// not a pivot into co-located admin surfaces.
	ht := httptool.New()
	httpRestricted := false
	if hostsCSV := os.Getenv(brand.EnvPrefix + "HTTP_ALLOWED_HOSTS"); strings.TrimSpace(hostsCSV) != "" {
		for h := range strings.SplitSeq(hostsCSV, ",") {
			if h = strings.TrimSpace(h); h != "" {
				ht.AllowedHosts = append(ht.AllowedHosts, h)
			}
		}
		httpRestricted = len(ht.AllowedHosts) > 0
	}
	if !httpRestricted {
		ht.AllowAll = true // default-allow: no allowlist pinned ⇒ any public host
	}
	// Master permissive switch (M611): AGEZT_ALLOW_ALL=1 implies the full open
	// posture for the network tools too — any host, including loopback and the
	// private network — so "everything allowed" really means everything. It also
	// overrides a pinned allowlist back to open.
	allowAll := os.Getenv(brand.EnvPrefix+"ALLOW_ALL") == "1"
	if allowAll || os.Getenv(brand.EnvPrefix+"HTTP_ALLOW_ALL") == "1" {
		ht.AllowAll = true
		httpRestricted = false
	}
	// Egress guard (M16): by default the http tool refuses internal/metadata
	// addresses even for allowlisted/AllowAll hosts. Relax per range for local use.
	egress := "guarded"
	if allowAll || os.Getenv(brand.EnvPrefix+"HTTP_ALLOW_LOOPBACK") == "1" {
		ht.AllowLoopback = true
		egress = "loopback-ok"
	}
	if allowAll || os.Getenv(brand.EnvPrefix+"HTTP_ALLOW_PRIVATE") == "1" {
		ht.AllowPrivate = true
		if egress == "loopback-ok" {
			egress = "loopback+private-ok"
		} else {
			egress = "private-ok"
		}
		fmt.Fprintln(stderr, "WARNING: AGEZT_HTTP_ALLOW_PRIVATE=1 lets the http tool reach the private network.")
	}
	out["http"] = ht
	if httpRestricted {
		registered = append(registered, fmt.Sprintf("http(hosts=%d, egress=%s)", len(ht.AllowedHosts), egress))
	} else {
		registered = append(registered, fmt.Sprintf("http(any host, egress=%s)", egress))
	}

	// browser.read — same allowlist pattern as http (uses AGEZT_BROWSER_*
	// env vars; deliberately separate from http's allowlist so operators
	// can grant browser-read access to a wider domain set than POSTs).
	// Same default-ALLOW posture as http (M818): any public host out of the box;
	// a non-empty AGEZT_BROWSER_ALLOWED_HOSTS is the opt-OUT that restricts it.
	br := browser.New()
	browserRestricted := false
	if hostsCSV := os.Getenv(brand.EnvPrefix + "BROWSER_ALLOWED_HOSTS"); strings.TrimSpace(hostsCSV) != "" {
		for h := range strings.SplitSeq(hostsCSV, ",") {
			if h = strings.TrimSpace(h); h != "" {
				br.AllowedHosts = append(br.AllowedHosts, h)
			}
		}
		browserRestricted = len(br.AllowedHosts) > 0
	}
	if !browserRestricted {
		br.AllowAll = true // default-allow
	}
	if allowAll || os.Getenv(brand.EnvPrefix+"BROWSER_ALLOW_ALL") == "1" {
		br.AllowAll = true
		browserRestricted = false
	}
	// Egress guard (M16): browser.read refuses internal/metadata addresses by
	// default, even for allowlisted/AllowAll hosts. Relax per range for local use.
	if allowAll || os.Getenv(brand.EnvPrefix+"BROWSER_ALLOW_LOOPBACK") == "1" {
		br.AllowLoopback = true
	}
	if allowAll || os.Getenv(brand.EnvPrefix+"BROWSER_ALLOW_PRIVATE") == "1" {
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
	if browserRestricted {
		registered = append(registered, fmt.Sprintf("browser.read(hosts=%d)", len(br.AllowedHosts)))
	} else {
		registered = append(registered, "browser.read(any host)")
	}

	// web_search — keyword search against a public engine, returning result
	// titles/URLs/snippets. The capability that lets the agent DISCOVER a URL
	// (then fetch it with http/browser.read), not just fetch one it was handed.
	// The engine host is fixed, so there's no host allowlist; the egress guard
	// still refuses internal/metadata addresses (relaxed under ALLOW_ALL for
	// parity with the other network tools). Always registered — no secret needed.
	ws := websearch.New()
	if allowAll {
		ws.AllowLoopback = true
		ws.AllowPrivate = true
	}
	out["web_search"] = ws
	registered = append(registered, "web_search(duckduckgo)")

	// fetch — download a URL's bytes and save them as a browsable artifact (M831),
	// so the agent can keep an image/PDF/file it finds (it shows up in Files). Same
	// SSRF-guarded egress as the other network tools; the artifact index is injected
	// after the kernel opens. Always registered.
	fe := fetch.New()
	if allowAll {
		fe.AllowLoopback = true
		fe.AllowPrivate = true
	}
	out["fetch"] = fe
	registered = append(registered, "fetch(url→artifact)")

	// artifacts — list/read/delete the files the agent has saved (fetch downloads,
	// offloaded tool outputs, inbound images), so a file from one run is usable in a
	// later one (M832). A read/list/delete view over the artifact index injected
	// after the kernel opens. Always registered.
	af := artifactstool.New()
	out["artifacts"] = af
	registered = append(registered, "artifacts(list/read/delete)")

	// db — the Personal Data Lake (M834): agent-built, multi-agent-shared structured
	// collections (expenses, tasks, notes, contacts, …) the human can also browse in
	// the Data view. The lake is injected after the kernel opens. Always registered.
	dbt := dbtool.New()
	out["db"] = dbt
	registered = append(registered, "db(data-lake)")

	// council — the Council of Elders (M837): convene a multi-model panel for a
	// hard decision and get a reconciled consensus. The kernel runner is injected
	// after Open. Always registered.
	ct := counciltool.New()
	out["council"] = ct
	registered = append(registered, "council(consensus panel)")

	// conductor — the Conductor (M997): an asymmetric, verify-driven panel
	// (Thinker/Worker/Verifier) that runs the worker's code and loops until it
	// passes. The kernel runner + code-exec backend are injected after Open.
	// Always registered.
	cond := conductortool.New()
	out["conductor"] = cond
	registered = append(registered, "conductor(verify-driven panel)")

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

	// code_exec — the agent writes & runs Python/JS/TS in a sandboxed workspace
	// under <baseDir>/sandbox (M683). Routed through the same warden as shell, with
	// a scrubbed env (no secrets), per-call ephemeral dirs (or named persistent
	// projects), and resource caps. Registered when at least one runtime is present,
	// UNLESS AGEZT_SANDBOX=off. Network is on by default; AGEZT_SANDBOX_NO_NET=1
	// forces it off. Bound to the kernel bus after it opens (for the code.executed
	// event), via the returned tools map in the daemon run path.
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(brand.EnvPrefix+"SANDBOX")), "off") {
		if rt := codeexec.DetectRuntimes(); len(rt) > 0 {
			netOn := os.Getenv(brand.EnvPrefix+"SANDBOX_NO_NET") != "1"
			ce := codeexec.NewWithWarden(ward, filepath.Join(baseDir, "sandbox"), rt, netOn)
			out["code_exec"] = ce
			netTag := "net=on"
			if !netOn {
				netTag = "net=off"
			}
			registered = append(registered, fmt.Sprintf("code_exec(langs=%s, %s)", strings.Join(ce.Languages(), "/"), netTag))
		}
	}

	// acp_agent — external ACP-agent bridge (SPEC-15 §3, the inverse of `agt
	// acp`). It drives an external agent speaking the Agent Client Protocol over
	// stdio (Gemini CLI, Claude Code's adapter, Codex, …) over JSON-RPC and relays
	// its answer. Registered when AGEZT_ACP_AGENT_CMD sets a default command OR any
	// catalog ACP agent is installed on the host (discovery via kernel/acpcatalog),
	// so a run can delegate to any installed agent by slug without configuration.
	acpCmd := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ACP_AGENT_CMD"))
	if at := acpagent.New(acpCmd, acpagent.AbsCwd(wsRoot)); at != nil {
		out["acp_agent"] = at
		registered = append(registered, "acp_agent(external agent)")
	}

	// homeassistant — read entity state + call services on the operator's Home
	// Assistant. Shares the channel's AGEZT_HOMEASSISTANT_URL/_TOKEN (same
	// instance), but is gated by its OWN allowlists so the outbound notify channel
	// can be configured without auto-exposing an actionable control surface:
	//   AGEZT_HOMEASSISTANT_TOOL_READ      — entity read allowlist (get_states)
	//   AGEZT_HOMEASSISTANT_TOOL_SERVICES  — service call allowlist (call_service)
	//   AGEZT_HOMEASSISTANT_TOOL_ALLOW_ALL_SERVICES=1 — bypass the service allowlist (DANGEROUS)
	// Both allowlists are fail-closed; the tool registers only when URL+TOKEN are
	// set AND at least one axis is enabled (so bare channel config exposes nothing).
	{
		haURL := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "HOMEASSISTANT_URL"))
		haTok := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "HOMEASSISTANT_TOKEN"))
		read := splitNonEmpty(os.Getenv(brand.EnvPrefix + "HOMEASSISTANT_TOOL_READ"))
		services := splitNonEmpty(os.Getenv(brand.EnvPrefix + "HOMEASSISTANT_TOOL_SERVICES"))
		if os.Getenv(brand.EnvPrefix+"HOMEASSISTANT_TOOL_ALLOW_ALL_SERVICES") == "1" {
			services = append(services, "*")
			fmt.Fprintln(stderr, "WARNING: AGEZT_HOMEASSISTANT_TOOL_ALLOW_ALL_SERVICES=1 lets the agent call ANY Home Assistant service.")
		}
		if haURL != "" && haTok != "" && (len(read) > 0 || len(services) > 0) {
			hat := hatool.New()
			hat.BaseURL = haURL
			hat.Token = haTok
			hat.ReadEntities = read
			hat.AllowedServices = services
			out["homeassistant"] = hat
			registered = append(registered, "homeassistant("+hat.Capabilities()+")")
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
				return nil, nil, nil, "", fmt.Errorf("AGEZT_PEERS: %w", err)
			}
			// Per-tenant peer sets (M219): a tenant's runs route against its own peers,
			// falling back to the global set. Parsed/validated up front like AGEZT_PEERS.
			tenantPeers, terr := peer.ParseTenantPeers(tenantSpec)
			if terr != nil {
				return nil, nil, nil, "", fmt.Errorf("AGEZT_TENANT_PEERS: %w", terr)
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
				return nil, nil, nil, "", fmt.Errorf("AGEZT_PLUGIN_PINS: %w", err)
			}
			pins = parsed
		}
		// Tool allowlist (M1.hh) — same hard-error semantics as pins.
		var allowedTools plugin.ToolAllowlistSpec
		if allowSpec := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "PLUGIN_TOOLS")); allowSpec != "" {
			parsed, err := plugin.ParseToolAllowlistSpec(allowSpec)
			if err != nil {
				return nil, nil, nil, "", fmt.Errorf("AGEZT_PLUGIN_TOOLS: %w", err)
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
			return nil, nil, nil, "", fmt.Errorf("AGEZT_PLUGINS: %w", err)
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
			declaredCaps := p.ToolCapabilities(prefix + ".") // M900: manifest-declared policy axes
			loadedCount := 0
			for name, tool := range pluginTools {
				if _, conflict := out[name]; conflict {
					fmt.Fprintf(stderr, "WARNING: plugin %q tool %q conflicts with existing tool — keeping in-process version\n", prefix, name)
					continue
				}
				out[name] = tool
				loadedCount++
				if cap, ok := declaredCaps[name]; ok {
					if toolCaps == nil {
						toolCaps = map[string]string{}
					}
					toolCaps[name] = cap
				}
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

	return out, manifestEntries, toolCaps, strings.Join(registered, ", "), nil
}

// councilSeatName labels the i-th council seat (M837): the first three get
// distinct elder names, the rest a numbered fallback.
func councilSeatName(i int) string {
	names := []string{"Elder Alpha", "Elder Beta", "Elder Gamma"}
	if i < len(names) {
		return names[i]
	}
	return fmt.Sprintf("Elder %d", i+1)
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

// alwaysFailProvider is a test shim (used by reconcile_providers_test.go) that
// errors on every call with a non-cancel/non-budget error so shouldFallback
// returns true. It forces the Governor's fallback chain to engage.
type alwaysFailProvider struct{ name string }

func (p *alwaysFailProvider) Name() string { return p.name }
func (p *alwaysFailProvider) Complete(ctx context.Context, _ agent.CompletionRequest) (*agent.CompletionResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("demo-shim: simulated primary failure")
}

// unconfiguredProviderName is the Name() of the sentinel primary registered when
// no LLM provider is configured. The reload path keys off it to swap in a real
// provider once the operator configures one.
const unconfiguredProviderName = "unconfigured"

// unconfiguredProvider is the daemon's primary when NO LLM provider is
// configured (AGEZT_PROVIDER unset). The daemon ships with no default provider
// or model (owner rule: "hiçbir default provider/model"), so a fresh install
// boots with this sentinel: the daemon, Web UI, and Setup all run, but any LLM
// call fails fast with an actionable message telling the operator to add a
// provider + key and a model (via AGEZT_MODEL or a routing/fallback chain). It
// is swapped for a real provider by the reload path once one is configured.
type unconfiguredProvider struct{}

func (unconfiguredProvider) Name() string { return unconfiguredProviderName }
func (unconfiguredProvider) Complete(ctx context.Context, _ agent.CompletionRequest) (*agent.CompletionResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("no LLM provider configured — add a provider and API key (Setup → Providers, or set %sPROVIDER) and a model (%sMODEL, a per-task route, or a fallback chain)", brand.EnvPrefix, brand.EnvPrefix)
}

// keep import honest
var _ = event.GenesisHash

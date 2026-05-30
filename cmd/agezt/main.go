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
	"syscall"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/openaiapi"
	"github.com/agezt/agezt/kernel/plugin"
	"github.com/agezt/agezt/kernel/pulse"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/ulid"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/kernel/webui"
	"github.com/agezt/agezt/plugins/channels/telegram"
	"github.com/agezt/agezt/plugins/providers/anthropic"
	"github.com/agezt/agezt/plugins/providers/compat"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/browser"
	filetool "github.com/agezt/agezt/plugins/tools/file"
	httptool "github.com/agezt/agezt/plugins/tools/http"
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
	edictEng := edict.New(edict.Options{AskPolicy: askPolicy})

	tools, pluginManifest, toolsDesc, err := buildTools(baseDir, stderr, ward)
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

	cfg := kernelruntime.Config{
		BaseDir:               baseDir,
		Provider:              gov, // Governor implements agent.Provider
		Tools:                 tools,
		Plugins:               pluginManifest,
		Model:                 model,
		System:                os.Getenv(brand.EnvPrefix + "SYSTEM_PROMPT"),
		Warden:                ward,
		Edict:                 edictEng,
		Catalog:               cat,
		MemoryInject:          memOn,
		MemoryTool:            memOn,
		MemoryDistill:         memOn,
		MemoryTopK:            5,
		MemoryDistillMinTools: 4,
		WorldInject:           worldOn,
		WorldTool:             worldOn,
		WorldTopK:             5,
		SkillInject:           skillOn,
		SkillTopK:             3,
		SkillForge:            forgeOn,
		SkillForgeMinTools:    4,
		SubAgentTool:          subAgentOn,
		SubAgentMaxDepth:      subAgentDepth,
	}
	cfg.OnReload = func() error {
		// Re-load vault (catalog already refreshed by Kernel.Reload).
		if err := credStore.Load(); err != nil {
			return fmt.Errorf("credentials vault: %w", err)
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := controlplane.NewServer(k, baseDir)
	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(stderr, "%s: start control plane: %v\n", brand.Binary, err)
		return 1
	}
	defer srv.Stop()

	fmt.Fprintf(stdout, "%s %s — daemon ready (protocol v%d)\n", brand.Name, brand.Version, brand.ProtocolVersion)
	fmt.Fprintf(stdout, "  base dir         : %s\n", baseDir)
	fmt.Fprintf(stdout, "  governor         : %s\n", govDesc)
	fmt.Fprintf(stdout, "  credentials      : %s\n", credDesc)
	fmt.Fprintf(stdout, "  tools            : %s\n", toolsDesc)
	fmt.Fprintf(stdout, "  policy engine    : edict (defaults from DECISIONS F3; %s)\n", askPolicyDesc)
	fmt.Fprintf(stdout, "  warden           : %s\n", wardDesc)
	fmt.Fprintf(stdout, "  control plane    : %s\n", srv.Addr())
	fmt.Fprintf(stdout, "  knowledge        : memory %s · world model %s (%d entities) · skills %s/forge %s (%d active)\n",
		onOff(memOn), onOff(worldOn), k.World().Count(), onOff(skillOn), onOff(forgeOn), k.Forge().Count())

	// Telegram channel (SPEC-04 §1) — duplex when AGEZT_TELEGRAM_TOKEN is
	// set. Built before Pulse so its brief sink can tee with the log sink.
	tgChan, tgSink, tgDesc := buildTelegram(ctx, k)
	if tgChan != nil {
		go tgChan.Start(ctx)
		fmt.Fprintf(stdout, "  telegram         : %s\n", tgDesc)
	} else {
		fmt.Fprintf(stdout, "  telegram         : disabled (set AGEZT_TELEGRAM_TOKEN)\n")
	}

	// Pulse — the proactive heart (SPEC-03). On by default; the resident
	// engine runs on the daemon ctx so `agt halt`/SIGTERM/`agt shutdown`
	// stop it with everything else. AGEZT_PULSE=off disables it. When
	// Telegram is configured, briefs tee to it (closes the Jarvis loop).
	if eng, pulseDesc := buildPulse(k, ward, model, stdout, tgSink); eng != nil {
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

	// OpenAI-compatible API (P7-API-01) — POST /v1/chat/completions + GET
	// /v1/models so any OpenAI client drives Agezt through the same tool-loop +
	// Edict + journal. Off unless AGEZT_API_ADDR is set; loopback + token.
	if apiDesc := buildOpenAIAPI(ctx, k, stdout); apiDesc != "" {
		fmt.Fprintf(stdout, "  openai api       : %s\n", apiDesc)
	} else {
		fmt.Fprintf(stdout, "  openai api       : disabled (set AGEZT_API_ADDR, e.g. 127.0.0.1:8799)\n")
	}

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
	cancel()
	// Give in-flight runs a moment to react to halt.
	deadline := time.Now().Add(2 * time.Second)
	for k != nil && !k.IsHalted() && time.Now().Before(deadline) {
		k.Halt()
		time.Sleep(50 * time.Millisecond)
	}
	return 0
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

	handler := func(hctx context.Context, msg channel.UnifiedMessage, corr string) (string, error) {
		return k.RunWith(hctx, corr, msg.Text)
	}
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
	srv := &http.Server{Handler: webui.New(k.Bus(), client, token).Handler()}
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
func (e kernelAPIEngine) RunModel(ctx context.Context, corr, intent, model string) (string, error) {
	// Honour the requested model for this run (empty → kernel default).
	return e.k.RunWith(kernelruntime.WithModel(ctx, model), corr, intent)
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

// buildOpenAIAPI starts the OpenAI-compatible HTTP resident when AGEZT_API_ADDR
// is set, mirroring buildWebUI's lifecycle (daemon ctx, graceful shutdown,
// minted token, loopback warning). Returns the banner description or "".
func buildOpenAIAPI(ctx context.Context, k *kernelruntime.Kernel, stdout io.Writer) string {
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
	srv := &http.Server{Handler: openaiapi.New(kernelAPIEngine{k}, k.Bus(), token).Handler()}
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
	primaryName := primary.Name()
	if demoFail {
		primaryName = primaryName + "-failshim"
		primary = &alwaysFailProvider{name: primaryName}
		primaryDesc = "[demo-shim:always-fail] " + primaryDesc
	}
	if err := reg.Register(&governor.ProviderInfo{
		Name:     primaryName,
		Provider: primary,
		AuthMode: authMode,
	}); err != nil {
		return nil, "", "", fmt.Errorf("register primary: %w", err)
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

	gov, err := governor.New(governor.Config{
		Registry:               reg,
		DailyCeilingMicrocents: ceiling,
		TaskRoutes:             taskRoutes,
		TaskRouteRequires:      taskRequires,
		TaskModelOverrides:     taskModels,
		TaskBudgets:            taskBudgets,
	})
	if err != nil {
		return nil, "", "", err
	}
	desc := fmt.Sprintf("primary=%s%s, daily_ceiling=$%.2f",
		primaryDesc, fallbackDesc, float64(ceiling)/1e9)
	if len(taskRoutes) > 0 {
		desc += fmt.Sprintf(", task_routes=%d", len(taskRoutes))
	}
	if len(taskBudgets) > 0 {
		desc += fmt.Sprintf(", task_budgets=%d", len(taskBudgets))
	}
	return gov, desc, model, nil
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
	out["http"] = ht
	if ht.AllowAll {
		registered = append(registered, "http(allow_all=true)")
	} else {
		registered = append(registered, fmt.Sprintf("http(hosts=%d)", len(ht.AllowedHosts)))
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
		var usedPrefixes []string

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for entry := range strings.SplitSeq(spec, ",") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			prefix, cmdLine, ok := strings.Cut(entry, "=")
			if !ok {
				fmt.Fprintf(stderr, "WARNING: AGEZT_PLUGINS entry %q missing '=' — expected '<prefix>=<path>'\n", entry)
				continue
			}
			parts := strings.Fields(cmdLine)
			if len(parts) == 0 {
				fmt.Fprintf(stderr, "WARNING: AGEZT_PLUGINS entry %q has empty path\n", entry)
				continue
			}
			usedPrefixes = append(usedPrefixes, prefix)
			cfg := plugin.Config{
				Path: parts[0],
				Args: parts[1:],
				Logger: func(line string) {
					fmt.Fprintf(stderr, "[plugin:%s] %s\n", prefix, line)
				},
				PinnedHash:   pins[prefix],         // empty if no pin configured for this prefix
				AllowedTools: allowedTools[prefix], // nil if no allowlist for this prefix
			}
			p, err := plugin.Spawn(ctx, cfg)
			if err != nil {
				fmt.Fprintf(stderr, "WARNING: plugin %q (%s) failed to start: %v\n", prefix, parts[0], err)
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
				Path:         parts[0],
				Args:         append([]string(nil), parts[1:]...),
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
func newDemoMock() agent.Provider {
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

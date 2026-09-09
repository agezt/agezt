// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/datalake"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/governor"
	imagetool "github.com/agezt/agezt/kernel/imagetool"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/kernel/mcp"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/okr"
	"github.com/agezt/agezt/kernel/reflect"
	"github.com/agezt/agezt/kernel/roster"
	reranktool "github.com/agezt/agezt/kernel/reranktool"
	"github.com/agezt/agezt/kernel/resume"
	"github.com/agezt/agezt/kernel/scheduler"
	"github.com/agezt/agezt/kernel/seat"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/kernel/state"
	"github.com/agezt/agezt/kernel/taste"
	"github.com/agezt/agezt/kernel/toolforge"
	voicetool "github.com/agezt/agezt/kernel/voicetool"
	"github.com/agezt/agezt/kernel/warden"
	"github.com/agezt/agezt/kernel/workboard"
	"github.com/agezt/agezt/kernel/workflow"
	"github.com/agezt/agezt/kernel/worldmodel"
	"github.com/agezt/agezt/kernel/agentgw"
	"github.com/agezt/agezt/kernel/runtime/accessors"
	"github.com/agezt/agezt/kernel/runtime/lifecycle"
	"github.com/agezt/agezt/kernel/runtime/runexec"
)

// This file is the kernel's composition root: Open + Close + closeAll +
// DefaultShutdownDrainTimeout. Pulled out of the runtime.go god file as
// the second step of the Day 9 split (after lifecycle.go).
//
// Open is intentionally long (~330 lines) because it is a single linear
// "wire every store, register every tool, build the Kernel" script. The
// store-opening blocks could in principle be split into per-store
// helpers, but that introduces many small functions and obscures the
// boot order. We accept the long function in exchange for one place
// that tells the boot story top to bottom; future extractions (e.g. a
// `compose/storeopen.go` per subsystem) can follow if it ever needs
// real review.

// DefaultShutdownDrainTimeout is how long Close waits for cancelled in-flight
// runs to actually return before tearing down their stores (M883).
const DefaultShutdownDrainTimeout = 5 * time.Second

// Open initialises the journal, state, and bus under cfg.BaseDir and
// returns a ready-to-use Kernel.
func Open(cfg Config) (*Kernel, error) {
	if cfg.BaseDir == "" {
		return nil, errors.New("runtime: BaseDir required")
	}
	if cfg.Provider == nil {
		return nil, errors.New("runtime: Provider required")
	}

	// Every store opened below registers its Close here; fail() unwinds them in
	// reverse order. The previous hand-copied close cascades had already
	// diverged (late failure paths leaked the journal handle), so this is the
	// one place unwind order lives.
	var closers []interface{ Close() error }
	fail := func(prefix string, err error) (*Kernel, error) {
		for i := len(closers) - 1; i >= 0; i-- {
			_ = closers[i].Close()
		}
		return nil, fmt.Errorf("%s: %w", prefix, err)
	}

	j, err := journal.Open(filepath.Join(cfg.BaseDir, "journal"), journal.Options{})
	if err != nil {
		return nil, fmt.Errorf("runtime: journal: %w", err)
	}
	closers = append(closers, j)
	st, err := state.Open(filepath.Join(cfg.BaseDir, "state"))
	if err != nil {
		return fail("runtime: state", err)
	}
	closers = append(closers, st)
	eng := cfg.Edict
	if eng == nil {
		eng = edict.New(edict.Options{})
	}
	kbus := bus.New(j)
	w := cfg.Warden
	if w == nil {
		w = warden.New(kbus)
	}
	apr := cfg.Approvals
	if apr == nil {
		apr = approval.New(approval.Config{Bus: kbus, Timeout: cfg.ApprovalTimeout})
	}
	sched := scheduler.New(scheduler.Config{Bus: kbus, Monitor: scheduler.ContextInvariantMonitor})

	catDir := cfg.CatalogDir
	if catDir == "" {
		catDir = filepath.Join(cfg.BaseDir, "catalog")
	}
	mstore, err := memory.Open(filepath.Join(cfg.BaseDir, "memory"))
	if err != nil {
		return fail("runtime: memory", err)
	}
	closers = append(closers, mstore)
	mgr := memory.NewManager(mstore, kbus)
	if cfg.MemoryEmbedder != nil {
		mgr.SetEmbedder(cfg.MemoryEmbedder) // M884: provider embeddings opt-in
	}

	wstore, err := worldmodel.Open(filepath.Join(cfg.BaseDir, "worldmodel"))
	if err != nil {
		return fail("runtime: worldmodel", err)
	}
	closers = append(closers, wstore)
	wgraph := worldmodel.NewGraph(wstore, kbus)

	skstore, err := skill.Open(filepath.Join(cfg.BaseDir, "skills"))
	if err != nil {
		return fail("runtime: skills", err)
	}
	closers = append(closers, skstore)
	forge := skill.NewForge(skstore, kbus)
	// Wire the on-disk bundle store so skills can ship reference files + scripts
	// (agentskills.io shape, M847). Best-effort: a bundle-store failure leaves
	// skills body-only rather than failing daemon start.
	if bundles, berr := skill.OpenBundles(filepath.Join(cfg.BaseDir, "skills")); berr == nil {
		forge.SetBundles(bundles)
	}

	schedStore, err := cadence.OpenStore(filepath.Join(cfg.BaseDir, "cadence"))
	if err != nil {
		return fail("runtime: cadence", err)
	}

	// Content-addressed artifact store (SPEC-04 §3.6): the agent loop offloads
	// oversized tool outputs here so the journal stays small. Store-only — no bus.
	artStore, err := artifact.Open(filepath.Join(cfg.BaseDir, "artifacts"))
	if err != nil {
		return fail("runtime: artifacts", err)
	}
	// Metadata index over the blob store (M822) — browsable/deletable entries
	// (inbound images, tool outputs). Failure here is non-fatal to the blob store
	// but we surface it so the operator knows the file-manager won't populate.
	artIndex, err := artifact.OpenIndex(artStore, filepath.Join(cfg.BaseDir, "artifacts"))
	if err != nil {
		return fail("runtime: artifact index", err)
	}

	// Personal Data Lake (M834): file-based structured collections agents build
	// and share. Pure on-disk (no handle to close), so its error path just unwinds
	// the prior stores like the others.
	lake, err := datalake.Open(cfg.BaseDir, func() int64 { return time.Now().UnixMilli() })
	if err != nil {
		return fail("runtime: data lake", err)
	}
	// Seed the built-in Personal Data Lake collections (M835) — expenses, calendar,
	// tasks, notes, habits, bookmarks, contacts. Idempotent (EnsureCollection skips
	// existing ones) and best-effort: a seed hiccup must not block boot, and the
	// next start retries.
	_, _ = lake.SeedBuiltins("system")

	ststore, err := standing.Open(filepath.Join(cfg.BaseDir, "standing"))
	if err != nil {
		return fail("runtime: standing", err)
	}

	// Durable in-flight-run tickets (M1002): opened only when resume is enabled so
	// a disabled daemon writes nothing. The store just prepares a directory, so a
	// failure here is unusual but still unwinds the stores opened above.
	var rsstore *resume.Store
	if cfg.ResumeEnabled {
		rsstore, err = resume.Open(filepath.Join(cfg.BaseDir, "resume"), cfg.ResumeSnapshotMaxBytes)
		if err != nil {
			return fail("runtime: resume", err)
		}
	}

	rstore, err := roster.Open(filepath.Join(cfg.BaseDir, "roster"))
	if err != nil {
		return fail("runtime: roster", err)
	}

	tfstore, err := toolforge.Open(filepath.Join(cfg.BaseDir, "toolforge"))
	if err != nil {
		return fail("runtime: toolforge", err)
	}

	mcpstore, err := mcp.OpenStore(filepath.Join(cfg.BaseDir, "mcp"))
	if err != nil {
		return fail("runtime: mcp", err)
	}

	wfstore, err := workflow.OpenStore(filepath.Join(cfg.BaseDir, "workflows"))
	if err != nil {
		return fail("runtime: workflows", err)
	}

	wbstore, err := workboard.OpenStore(filepath.Join(cfg.BaseDir, "workboard"))
	if err != nil {
		return fail("runtime: workboard", err)
	}

	okrstore, err := okr.OpenStore(filepath.Join(cfg.BaseDir, "okr"))
	if err != nil {
		return fail("runtime: okr", err)
	}

	tastestore, err := taste.OpenStore(filepath.Join(cfg.BaseDir, "taste"))
	if err != nil {
		return fail("runtime: taste", err)
	}

	seatstore, err := seat.OpenStore(filepath.Join(cfg.BaseDir, "seats"))
	if err != nil {
		return fail("runtime: seats", err)
	}

	// Reflection holds no store of its own — it folds the journal and tunes
	// the world graph, then journals its report (SPEC-05 §6). Default decay
	// knobs; the daemon may override via the optional periodic trigger.
	reflectEng := reflect.New(j, wgraph, kbus, reflect.Config{})

	// The agent's effective tool set is the configured tools plus the
	// in-process memory/world tools (when enabled). Built once, exposed via
	// Tools() so `agt tool list` reflects what the loop actually sees.
	effTools := make(map[string]agent.Tool, len(cfg.Tools)+2)
	maps.Copy(effTools, cfg.Tools)
	if cfg.MemoryTool {
		effTools["memory"] = mgr.Tool()
	}
	if cfg.WorldTool {
		effTools["world"] = wgraph.Tool()
	}
	// The sub-agent tool's runner needs the finished *Kernel, which doesn't
	// exist yet; register the tool now and wire its runner just after k is
	// built (effTools is the same map k.tools holds).
	var subTool *subAgentTool
	var awaitTool *subAgentAwaitTool
	if cfg.SubAgentTool {
		subTool = newSubAgentTool()
		effTools["delegate"] = subTool
		// The collect half of async delegation (M881): delegate(async=true)
		// returns a spawn_id; delegate_await blocks until that child finishes.
		awaitTool = newSubAgentAwaitTool()
		effTools["delegate_await"] = awaitTool
	}
	// The market tool needs k.Market (wired by the daemon after Open); register
	// it now and bind its lazy getter just after k is built (same map k holds).
	var mktTool *market.Tool
	if cfg.MarketTool {
		mktTool = market.NewTool()
		effTools["market"] = mktTool
	}
	// The voice tool needs the kernel's artifact store (bound just after k is
	// built) to persist synthesized audio.
	var vTool *voicetool.Tool
	if cfg.Voice != nil {
		vTool = voicetool.New(cfg.Voice)
		effTools["voice"] = vTool
	}
	// image_generate needs the artifact store too (bound just after k is built).
	var imgTool *imagetool.Tool
	if cfg.ImageGenerator != nil {
		imgTool = imagetool.New(cfg.ImageGenerator)
		effTools["image_generate"] = imgTool
	}
	if cfg.Reranker != nil {
		effTools["rerank"] = reranktool.New(cfg.Reranker)
	}

	catStore := catalog.NewStore(catDir)
	cat := cfg.Catalog
	if cat == nil {
		loaded, err := catStore.Load()
		if err != nil {
			return fail("runtime: catalog load", err)
		}
		cat = loaded
	}
	governor.SetCatalog(cat)

	k := &Kernel{
		cfg:          cfg,
		journal:      j,
		state:        st,
		bus:          kbus,
		edict:        eng,
		warden:       w,
		approvals:    apr,
		scheduler:    sched,
		catalogStore: catStore,
		catalog:      cat,
		memory:       mgr,
		memoryDir:    mstore,
		world:        wgraph,
		worldDir:     wstore,
		forge:        forge,
		skillDir:     skstore,
		standing:     ststore,
		resume:       rsstore,
		roster:       rstore,
		toolForge:    tfstore,
		mcpStore:     mcpstore,
		mcpConns:     make(map[string]mcp.Conn),
		workflows:    wfstore,
		workboard:    wbstore,
		okr:          okrstore,
		taste:        tastestore,
		seat:         seatstore,
		artifacts:    artStore,
		artIndex:     artIndex,
		lake:         lake,
		reflect:      reflectEng,
		schedules:    schedStore,
		tools:        effTools,
		system:       cfg.System,
		model:        cfg.Model,
		runs:         make(map[string]context.CancelFunc),
		fanout:       make(map[string]int),
		tree:         make(map[string]int),
		steers:       make(map[string]*runControl),
		spawns:       make(map[string]*spawnHandle),
		toolCaps:     validatedToolCaps(cfg.ToolCapabilities), // M900
		startTime:    time.Now(),
	}
	if subTool != nil {
		subTool.run = k.runSubAgent
		subTool.spawn = k.runSubAgentAsync // M881: non-blocking delegation
	}
	if awaitTool != nil {
		awaitTool.await = k.awaitSubAgent
	}
	if mktTool != nil {
		mktTool.Manager = k.Market
	}
	if imgTool != nil {
		imgTool.SaveArtifact = k.artifacts.Put
	}
	if vTool != nil {
		vTool.SaveArtifact = k.artifacts.Put
	}

	// Config Center for agent SDK config access (M???)
	configCenter, err := configcenter.Open(configcenter.DefaultConfig(cfg.BaseDir))
	if err != nil {
		return fail("runtime: configcenter", err)
	}
	closers = append(closers, configCenter)
	// Wire approval registry for HITL support
	if apr != nil {
		configCenter.SetApprovalRegistry(apr)
	}
	k.configCenter = configCenter

	// Agent Gateway for subprocess communication (Agent SDK)
	gwCfg := agentgw.DefaultGatewayConfig(cfg.BaseDir)
	// Token signing key: a per-install secret persisted under the base dir (or
	// $AGEZT_AGENTGW_TOKEN_SECRET), shared with the `agt` CLI. Never the old
	// hardcoded "change-me-in-production" constant.
	secret, err := agentgw.ResolveTokenSecret(cfg.BaseDir)
	if err != nil {
		return fail("runtime: resolve agentgw token secret", err)
	}
	gwCfg.TokenSecret = secret
	// Override socket path from environment if set (useful for Windows TCP testing)
	if sockPath := os.Getenv("AGEZT_AGENTGW_SOCKET"); sockPath != "" {
		gwCfg.SocketPath = sockPath
	}
	agentGW := agentgw.NewGateway(gwCfg)
	agentGW.Attach(kbus, mgr, rstore)
	agentGW.SetConfigCenter(configCenter)
	agentGW.SetAuditJournal(j) // wire the audit trail (was a nil no-op)
	k.agentGW = agentGW

	// Start the gateway listener in background
	go func() {
		if err := agentGW.Listen(context.Background()); err != nil {
			slog.Error("runtime: agentgw listen", "error", err)
		}
	}()

	// Wire the lifecycle sub-manager (Day 12 split) last so it sees
	// the fully-built *Kernel. The Manager only holds a KernelAPI
	// reference; it does not own the kernel's lifecycle state.
	k.lifecycle = lifecycle.New(k)

	// Wire the accessors sub-manager (Day 13 split). Same pattern:
	// the Accessor holds a KernelAPI reference; it does not own any
	// state of its own.
	k.accessors = accessors.New(k)

	// Wire the runexec sub-manager (Day 22 split). The Runner holds
	// a KernelAPI reference; Day 23 will move the 260-line RunWith
	// body here. For now Runner just delegates back to *Kernel.
	k.runexec = runexec.New(k)

	return k, nil
}

var _ runexec.KernelAPI = (*Kernel)(nil)

// Close stops the bus, then closes state and the journal. Pending runs are
// cancelled via Halt, then given a bounded drain window (M883) so a run
// mid-journal-write finishes cleanly instead of racing store teardown.
func (k *Kernel) Close() error {
	k.Suspend("close") // M1002: classify in-flight runs as resumable before Halt cancels them
	k.Halt()           // cancel any in-flight runs first
	// Drain: cancelled runs still need to unwind — publish their terminal
	// task.failed, release fan-out tallies, return from tools that honour the
	// cancel late. Wait bounded; a run wedged in a cancel-ignoring tool must
	// not block shutdown forever.
	drain := k.cfg.ShutdownDrainTimeout
	if drain == 0 {
		drain = DefaultShutdownDrainTimeout
	}
	if drain > 0 {
		settled := make(chan struct{})
		go func() {
			k.runWG.Wait()
			close(settled)
		}()
		t := time.NewTimer(drain)
		select {
		case <-settled:
			t.Stop()
		case <-t.C:
			// Best-effort breadcrumb: the journal is still open here, so the
			// abandonment is auditable. The wedged goroutine dies with the
			// process.
			_, _ = k.bus.Publish(event.Spec{
				Subject: "kernel.shutdown",
				Kind:    event.KindAnomalyDetected,
				Actor:   "kernel",
				Payload: map[string]any{
					"anomaly":  "shutdown_drain_timeout",
					"waited":   drain.String(),
					"detail":   "in-flight runs did not settle after Halt; closing stores anyway",
					"severity": "warning",
				},
			})
		}
	}
	k.closeMCPConns() // detach every live MCP server (kills the children)
	k.bus.Close()
	// Close every store even if an earlier one errors — the previous short-circuit
	// returned on the first error and leaked the remaining handles, notably the
	// journal's OS file descriptor (a held handle blocks a re-Open of the dir on
	// Windows). errors.Join reports all failures. (M477)
	return closeAll(
		k.state.Close,
		k.memoryDir.Close,
		k.worldDir.Close,
		k.skillDir.Close,
		k.journal.Close,
		func() error { return k.agentGW.Close() },
	)
}

// closeAll invokes every close func (none skipped) and joins their errors.
func closeAll(closers ...func() error) error {
	errs := make([]error, 0, len(closers))
	for _, c := range closers {
		errs = append(errs, c())
	}
	return errors.Join(errs...)
}

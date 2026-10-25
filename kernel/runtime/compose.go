// SPDX-License-Identifier: MIT

package runtime

// Provenance: Runtime composition: types + Open (the composition root). Code
//             extracted from compose.go during the Day-79 god-file split. Public API
//             unchanged.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"time"

	"github.com/agezt/agezt/kernel/agentgw"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/contract/catalogapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolphaseapi"
	"github.com/agezt/agezt/kernel/contract/worldmodelapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
	"github.com/agezt/agezt/kernel/modules/extensions/mcp"
	"github.com/agezt/agezt/kernel/modules/knowledge/memory"
	"github.com/agezt/agezt/kernel/modules/skills/skill"
	"github.com/agezt/agezt/kernel/platform/toolpipeline"
	"github.com/agezt/agezt/kernel/reflect"
	"github.com/agezt/agezt/kernel/resume"
	"github.com/agezt/agezt/kernel/runtime/lifecycle"
	"github.com/agezt/agezt/kernel/runtime/runexec"
	"github.com/agezt/agezt/kernel/scheduler"
	"github.com/agezt/agezt/kernel/state"
	"github.com/agezt/agezt/kernel/toolexec"
	"github.com/agezt/agezt/kernel/warden"
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

	mstore, err := memory.Open(filepath.Join(cfg.BaseDir, "memory"))
	if err != nil {
		return fail("runtime: memory", err)
	}
	closers = append(closers, mstore)
	mgr := memory.NewManager(mstore, kbus)
	if cfg.MemoryEmbedder != nil {
		mgr.SetEmbedder(cfg.MemoryEmbedder) // M884: provider embeddings opt-in
	}

	// The world-model graph is NOT opened here (W3.54) — the FILE is the
	// composition root's, and the kernel holds the contract's ports. The
	// daemon opens the file (worldmodel/worldmodel.json) and hands over the
	// store; the daemon also hands over the BUILDER (it names the module),
	// and the kernel calls it with that store and its own publisher — so the
	// graph journals through the kernel's bus without either side naming the
	// other's implementation.
	//
	// A host that was given neither holds a nil port, and the world surfaces
	// report that the graph is unavailable rather than calling into nothing.
	var wgraph worldmodelapi.Graph
	if cfg.WorldStore != nil && cfg.WorldGraph != nil {
		wgraph = cfg.WorldGraph(cfg.WorldStore, worldPublisher{b: kbus})
	}

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

	// The typed schedule library is NOT opened here (W3.49) — it is the
	// composition root's file, and the kernel holds the contract's host port
	// (cfg.Schedules). A host that was given none holds a nil port: the
	// schedule surfaces report that the library is unavailable rather than
	// calling into nothing.

	// Content-addressed artifact store (SPEC-04 §3.6) and the metadata index
	// over it (M822) are NOT opened here — they are the composition root's
	// files (W3.43), and the host holds the contract's ports: cfg.Artifacts
	// (the blob store) and cfg.ArtifactIndex (the browsable, deletable
	// entries). A host that was given neither holds nil ports and says "not
	// available" rather than calling into nothing.
	//
	// The journal owns the bytes it names: a tool.result's raw_ref points at
	// an offloaded output, and the journal keeps that name forever. The
	// collector must not remove a blob the journal still references, so the
	// index asks the journal (through the guard) before it collects anything.
	// The guard is installed HERE because the guard is the kernel's — the
	// journal is the kernel's — and the port is what the index is.
	if cfg.ArtifactIndex != nil {
		cfg.ArtifactIndex.SetRefGuard(&journalRefGuard{j: j})
	}

	// Personal Data Lake (M834): file-based structured collections agents build
	// and share. The lake is NOT opened here — it is the composition root's
	// file (W3.42), and the host holds the contract's port: cfg.Lake. The
	// daemon opens it, seeds the built-in collections (M835) and hands it
	// over; a host that was given no lake holds a nil port and says "not
	// available" rather than calling into nothing.

	// The standing-order library is NOT opened here (W3.50) — it is the
	// composition root's file, and the kernel holds the contract's store port
	// (cfg.Standing). A host that was given none holds a nil port: the
	// standing surfaces report that the library is unavailable rather than
	// calling into nothing. The daemon's own residents (the event runner and
	// the cron ticker) are built over that very store after Open.

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

	mcpstore, err := mcp.OpenStore(filepath.Join(cfg.BaseDir, "mcp"))
	if err != nil {
		return fail("runtime: mcp", err)
	}

	// The workflow store is NOT opened here (W3.48): the store file is the
	// composition root's business (cmd/agezt), and the kernel holds the
	// contract's read port. The ENGINE is the host's too — it is built over
	// the store and over the host's own ports, so the daemon hands it over
	// after Open.

	// The task board is NOT opened here (W3.47): the store file is the
	// composition root's business (cmd/agezt), and the kernel holds the
	// contract's host port. A kernel opened without one refuses every board
	// operation instead of calling into nothing.

	// The objective library is NOT opened here (W3.53) — it is the composition
	// root's file, and the kernel holds the contract's store port
	// (cfg.OKRStore). A host that was given none holds a nil port and the
	// objective surfaces report that the library is unavailable rather than
	// calling into nothing.

	// The curated exemplar library is NOT opened here (W3.52) — it is the
	// composition root's file, and the kernel holds the contract's store port
	// (cfg.TasteStore). A host that was given none holds a nil port and
	// injects no exemplars into a run rather than calling into nothing. The
	// daemon opens the file (taste/taste.json) and hands it over.

	// The seat catalog is NOT opened here: the store is the composition root's
	// business (the daemon owns the file), and the host holds the contract's
	// port — cfg.Seats. A host that was given nothing holds a nil port and says
	// "not available" rather than calling into nothing.

	// Reflection holds no store of its own — it folds the journal and tunes
	// the world graph, then journals its report (SPEC-05 §6). Default decay
	// knobs; the daemon may override via the optional periodic trigger.
	reflectEng := reflect.New(j, wgraph, kbus, reflect.Config{})

	// The agent's effective tool set is the configured tools plus the
	// in-process memory/world tools (when enabled). Built once, exposed via
	// Tools() so `agt tool list` reflects what the loop actually sees.
	effTools := make(map[string]toolapi.Tool, len(cfg.Tools)+2)
	maps.Copy(effTools, cfg.Tools)
	if cfg.MemoryTool {
		effTools["memory"] = mgr.Tool()
	}
	// The world tool is registered only when the host gave a graph: a nil
	// port means no world model on this daemon, and a tool that calls into
	// nothing would answer the agent with a panic (W3.54).
	if cfg.WorldTool && wgraph != nil {
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
	// The market tool is NOT built here (W3.46): it lives in the market
	// module, and the kernel is a module — it may not name a module's
	// implementation. The composition root (cmd/agezt) builds it over the
	// contract's marketplace port and hands it over as an ordinary tool in
	// cfg.Tools, which is why the kernel reaches no market package.
	//
	// The voice, image and rerank tools are NOT built here (W3.44): they live
	// in their own packages, and the kernel is a module — it may not name a
	// module's implementation. The composition root (cmd/agezt) builds them
	// from the daemon's own adapter seams and hands them over as ordinary
	// tools in cfg.Tools, which is why the kernel reaches no tool package.

	// The catalog's directory is NOT opened here (W3.51) — it is the
	// composition root's file, and the kernel holds the contract's store
	// port (cfg.CatalogStore). A host that was given none holds a nil port:
	// the catalog's write surfaces refuse rather than writing into nothing,
	// and the snapshot is the empty catalog — which is exactly what a daemon
	// that has never synced has.
	catStore := cfg.CatalogStore
	cat := cfg.Catalog
	if cat == nil {
		if catStore != nil {
			loaded, err := catStore.Load()
			if err != nil {
				return fail("runtime: catalog load", err)
			}
			cat = loaded
		} else {
			cat = catalogapi.NewEmpty()
		}
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
		catalogSync:  cfg.CatalogSyncer,
		memory:       mgr,
		memoryDir:    mstore,
		world:        wgraph,
		worldStore:   cfg.WorldStore,
		forge:        forge,
		skillDir:     skstore,
		standing:     cfg.Standing,
		resume:       rsstore,
		roster:       rstore,
		toolForge:    cfg.ToolForge,
		mcpStore:     mcpstore,
		mcpConns:     make(map[string]mcp.Conn),
		workflows:    cfg.Workflows,
		workboard:    cfg.Workboard,
		okr:          cfg.OKRStore,
		taste:        cfg.TasteStore,
		seat:         cfg.Seats,
		artifacts:    cfg.Artifacts,
		artIndex:     cfg.ArtifactIndex,
		lake:         cfg.Lake,
		reflect:      reflectEng,
		schedules:    cfg.Schedules,
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

	// Config Center for agent SDK config access (M???)
	configCenter, err := configcenter.Open(configcenter.DefaultConfig(cfg.BaseDir))
	if err != nil {
		return fail("runtime: configcenter", err)
	}
	closers = append(closers, configCenter)
	configCenter.SetBus(kbus) // config reads are part of the audit chain
	// Wire approval registry for HITL support
	if apr != nil {
		configCenter.SetApprovalRegistry(apr)
	}
	// Secret-rated values live in the vault, not the entry files. Migration of
	// plaintext secrets left by older versions happens here. A vault failure
	// degrades (the values stay in their 0600 files) instead of failing boot.
	if cfg.ConfigVault != nil {
		if n, err := configCenter.UseVault(cfg.ConfigVault); err != nil {
			slog.Warn("config center: secrets not moved into the vault", "error", err, "moved", n)
		} else if n > 0 {
			slog.Info("config center: moved plaintext secrets into the vault", "count", n)
		}
	}
	k.configCenter = configCenter

	newInvoker := cfg.NewToolInvoker
	if newInvoker == nil {
		newInvoker = toolexec.NewInvoker
	}
	k.toolInvoker = newInvoker(toolpipeline.Dependencies{Tools: k, Policy: k, Events: k, Noise: k})
	if k.toolInvoker == nil {
		kbus.Close()
		return fail("runtime: tool invoker", errors.New("factory returned nil"))
	}
	if phases, ok := k.toolInvoker.(toolphaseapi.Phases); ok {
		k.toolPhases = phases
	} else {
		// Older custom one-shot factories never handled loop admission. Preserve
		// that contract with one host-bound canonical phase service per kernel.
		k.toolPhases = toolpipeline.NewInvoker(toolpipeline.Dependencies{Tools: k, Policy: k, Events: k, Noise: k}).(toolphaseapi.Phases)
	}

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

	// Wire the runexec sub-manager (Day 22 split). The Runner holds
	// a KernelAPI reference; Day 23 will move the 260-line RunWith
	// body here. For now Runner just delegates back to *Kernel.
	k.runexec = runexec.New(k)

	return k, nil
}

var _ runexec.KernelAPI = (*Kernel)(nil)

// Close stops the bus, then closes state and the journal. Pending runs are

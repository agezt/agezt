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
	"path/filepath"
	"time"

	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/contract/approvalapi"
	"github.com/agezt/agezt/kernel/contract/catalogapi"
	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/contract/skillapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolphaseapi"
	"github.com/agezt/agezt/kernel/contract/worldmodelapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/journal"
	extapi "github.com/agezt/agezt/kernel/modules/extensions/api"
	"github.com/agezt/agezt/kernel/modules/runs/lifecycle"
	"github.com/agezt/agezt/kernel/modules/runs/prompt"
	"github.com/agezt/agezt/kernel/modules/runs/registry"
	"github.com/agezt/agezt/kernel/modules/runs/resume"
	"github.com/agezt/agezt/kernel/modules/runs/steer"
	"github.com/agezt/agezt/kernel/platform/toolpipeline"
	"github.com/agezt/agezt/kernel/reflect"
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

	// The memory store is NOT opened here (W3.57) — the FILE is the
	// composition root's, and the kernel holds the contract's ports. The
	// daemon opens the file (memory/memory.json) and hands over the store;
	// the daemon also hands over the BUILDER (it names the module), and the
	// kernel calls it with that store and its own publisher — so every
	// mutation journals through the kernel's bus without either side naming
	// the other's implementation.
	//
	// A host that was given neither holds a nil port: the reads answer "no
	// memory" and the writes refuse, rather than calling into nothing.
	var mgr memoryapi.Manager
	if cfg.MemoryStore != nil && cfg.MemoryBuilder != nil {
		mgr = cfg.MemoryBuilder(memoryapi.Deps{
			Store:     cfg.MemoryStore,
			Publisher: memoryPublisher{b: kbus},
			Embedder:  cfg.MemoryEmbedder,  // M884: provider embeddings opt-in
			Generator: cfg.MemoryGenerator, // the structured-output path (W3.57)
		})
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

	// The skill library is NOT opened here (W3.55) — the FILE is the
	// composition root's, and the kernel holds the contract's ports. The
	// daemon opens the file (skills/skills.json) and hands over the store;
	// the daemon also hands over the BUILDER (it names the module), and the
	// kernel calls it with that store and its own publisher — so every
	// transition journals through the kernel's bus without either side naming
	// the other's implementation.
	//
	// A host that was given neither holds a nil port, and the skill surfaces
	// report that the library is unavailable rather than calling into nothing.
	var forge skillapi.Curator
	if cfg.SkillStore != nil && cfg.SkillCurator != nil {
		forge = cfg.SkillCurator(cfg.SkillStore, skillPublisher{b: kbus})
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

	// The roster's file is the COMPOSITION ROOT's (W3.56) — it is the daemon's
	// business, and the kernel is a module that may not name one. The kernel
	// holds `cfg.Roster`, the contract's store port, and the daemon opens
	// roster/roster.json and hands it over.
	//
	// A host that was given none holds a nil port: the roster reads answer
	// "no agents" and the roster writes refuse, rather than calling into
	// nothing.

	// The MCP registry is NOT opened here (W3.58): the registry's FILE is the
	// composition root's business (cmd/agezt), and the kernel holds the
	// contract's store port (cfg.MCPStore). A host that was given none holds a
	// nil port: the registry reads answer "no servers" and the registry
	// writes refuse, rather than calling into nothing.
	//
	// The registry's LIFECYCLE is the extensions module's (W3.71) — the module
	// keeps the operations, the live attachments and the bridge, and the host
	// keeps the names. The daemon hands over the BUILDER (it names the
	// module), and the kernel calls it with the store, the two dialers and
	// its own publisher — so every lifecycle transition journals through the
	// kernel's bus without either side naming the other's implementation.
	//
	// A host that was given neither holds no service: the registry writes
	// refuse and the reads answer "no servers".
	var mcpSvc extapi.MCPRegistryService
	if cfg.MCPStore != nil && cfg.MCPBuilder != nil {
		mcpSvc = cfg.MCPBuilder(extapi.MCPRegistryDependencies{
			Registry:   cfg.MCPStore,
			Dialer:     cfg.MCPDialer,
			HTTPDialer: cfg.MCPHTTPDialer,
			Publish:    mcpPublisher{b: kbus}.Publish,
		})
	}

	// The script-tool FORGE's lifecycle is the extensions module's (W3.72) —
	// the module keeps the operations, and the daemon keeps the names. The
	// daemon hands over the BUILDER (it names the module), and the kernel
	// calls it with the store, the sandbox runner, the operator's approval
	// registry and its own publisher — so every lifecycle transition journals
	// through the kernel's bus without either side naming the other's
	// implementation.
	//
	// A host that was given neither holds no service: the forge writes
	// refuse and the reads answer "no tools".
	var forgeSvc extapi.ForgeService
	if cfg.ToolForge != nil && cfg.ForgeBuilder != nil {
		forgeSvc = cfg.ForgeBuilder(extapi.ForgeDependencies{
			Store:        cfg.ToolForge,
			ScriptRunner: cfg.ScriptRunner,
			AutoPromote:  cfg.AutoPromoteScriptTools,
			Approve: func(ctx context.Context, spec approvalapi.SubmitSpec) approvalapi.Outcome {
				if apr == nil {
					return approvalapi.Outcome{}
				}
				return apr.Submit(ctx, spec)
			},
			Publish: mcpPublisher{b: kbus}.Publish,
		})
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
	if cfg.MemoryTool && mgr != nil {
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
		memoryStore:  cfg.MemoryStore,
		world:        wgraph,
		worldStore:   cfg.WorldStore,
		forge:        forge,
		skillStore:   cfg.SkillStore,
		standing:     cfg.Standing,
		resume:       rsstore,
		roster:       cfg.Roster,
		toolForge:    cfg.ToolForge,
		mcpStore:     cfg.MCPStore,
		mcp:          mcpSvc,
		scriptForge:  forgeSvc,
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
		registry:     registry.New(),
		fanout:       make(map[string]int),
		tree:         make(map[string]int),
		steers:       make(map[string]*steer.Control),
		spawns:       make(map[string]*spawnHandle),
		toolCaps:     validatedToolCaps(cfg.ToolCapabilities), // M900
		startTime:    time.Now(),
	}

	// The steering SERVICE is the runs module's (W3.76b): the module owns the
	// steering and the interventions, and the daemon hands it the doors — the
	// live runs, the bus and the cancel. The module never names the daemon.
	k.steerSvc = steer.NewService(steer.Dependencies{
		Lookup: func(corr string) (*steer.Control, bool) {
			rc := k.controlFor(corr)
			return rc, rc != nil
		},
		Publish: func(spec event.Spec) { _, _ = kbus.Publish(spec) },
		Cancel:  k.CancelRun,
		Now:     time.Now,
	})
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

	// The agent gateway is NOT built here (W3.67): it is an ADAPTER (L5) — the
	// transport the agent SDK's subprocesses talk to — and the kernel is a
	// module (L3) that may not hold one. The composition root (cmd/agezt)
	// builds it over the kernel's own ports (bus, memory, roster, config
	// center, journal) and hands it over as nothing at all: the daemon owns
	// the gateway, starts it and closes it. A kernel opened without a daemon
	// has no gateway, and the SDK subprocess path is simply not served.

	// Wire the RUNS MODULE's prompt service (W3.76e): it assembles what a run
	// knows before it starts. The module reads the four stores through the
	// contract rows the daemon already holds (memory, taste, world, skills)
	// and reaches the daemon for the three things it cannot do itself (the
	// live identity, the bus, the clock).
	k.prompt = prompt.New(promptConfig(cfg), promptHost{k}, prompt.Deps{
		Memory: k.memory,
		Taste:  k.taste,
		World:  k.world,
		Skills: k.forge,
	})

	// Wire the run-lifecycle manager last so it sees the fully-built
	// *Kernel. The Manager is the RUNS MODULE's (W3.76d): it owns the
	// halt/resume/cancel surface over the module's live-run registry, and
	// reaches the daemon only for the two side effects it cannot perform
	// itself (Suspend, PublishBus).
	k.lifecycle = lifecycle.New(k.registry, k)

	// Wire the runexec sub-manager (Day 22 split). The Runner holds
	// a KernelAPI reference; Day 23 will move the 260-line RunWith
	// body here. For now Runner just delegates back to *Kernel.
	k.runexec = runexec.New(k)

	return k, nil
}

var _ runexec.KernelAPI = (*Kernel)(nil)

// Close stops the bus, then closes state and the journal. Pending runs are

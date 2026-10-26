// SPDX-License-Identifier: MIT

package main

import (
	"path/filepath"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/contract/skillapi"
	"github.com/agezt/agezt/kernel/contract/worldmodelapi"
	"github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/kernel/modules/agents/reaper"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
	"github.com/agezt/agezt/kernel/modules/artifacts/artifact"
	"github.com/agezt/agezt/kernel/modules/artifacts/datalake"
	"github.com/agezt/agezt/kernel/modules/extensions/mcp"
	"github.com/agezt/agezt/kernel/modules/extensions/toolforge"
	"github.com/agezt/agezt/kernel/modules/knowledge/memory"
	"github.com/agezt/agezt/kernel/modules/knowledge/taste"
	"github.com/agezt/agezt/kernel/modules/knowledge/worldmodel"
	"github.com/agezt/agezt/kernel/modules/providers/catalog"
	"github.com/agezt/agezt/kernel/modules/reasoning"
	"github.com/agezt/agezt/kernel/modules/skills/skill"
	"github.com/agezt/agezt/kernel/modules/work/okr"
	"github.com/agezt/agezt/kernel/modules/work/seat"
	"github.com/agezt/agezt/kernel/modules/work/workboard"
	"github.com/agezt/agezt/kernel/modules/workflows/workflow"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/standing"
)

// openAppKernel is shared by primary and tenant composition. Every Open binds
// the application entry to that kernel's own policy, audit and lookup ports.
//
// It also binds two modules — the reasoning module (the Council, the Conductor
// and the deep-research harness) and the agents module's reaper (the read-only
// fleet health scan). A module cannot be imported by the kernel itself (a
// module may import another module's api package and nothing else — archcheck's
// principle P5), so the composition root builds them here, over the kernel's own
// ports, and injects them.
//
// The seat catalog is bound the same way: the daemon opens the store (the file
// is the daemon's business) and hands the kernel the contract's port. A store
// that cannot be opened is a startup error, exactly as it was when the kernel
// opened it itself.
func openAppKernel(cfg kernelruntime.Config) (*kernelruntime.Kernel, error) {
	cfg.NewToolInvoker = apptools.NewInvoker
	if cfg.Seats == nil {
		seats, err := seat.OpenStore(filepath.Join(cfg.BaseDir, "seats"))
		if err != nil {
			return nil, err
		}
		cfg.Seats = seats
	}
	if cfg.Lake == nil {
		lake, err := datalake.Open(cfg.BaseDir, func() int64 { return time.Now().UnixMilli() })
		if err != nil {
			return nil, err
		}
		// Seed the built-in Personal Data Lake collections (M835) — expenses,
		// calendar, tasks, notes, habits, bookmarks, contacts. Idempotent
		// (EnsureCollection skips existing ones) and best-effort: a seed hiccup
		// must not block boot, and the next start retries.
		//
		// The seeding happens HERE, while the daemon still holds the concrete
		// lake: seeding is not part of the port the host is given.
		_, _ = lake.SeedBuiltins("system")
		cfg.Lake = lake
	}
	if cfg.Artifacts == nil {
		// Content-addressed artifact store (SPEC-04 §3.6) and the metadata
		// index over it (M822). The daemon opens both and hands the kernel the
		// contract's ports (W3.43): the files are the daemon's business.
		//
		// The journal's ref guard is NOT installed here — the journal is the
		// kernel's, so the kernel installs its own guard on the index it was
		// given.
		store, err := artifact.Open(filepath.Join(cfg.BaseDir, "artifacts"))
		if err != nil {
			return nil, err
		}
		index, err := artifact.OpenIndex(store, filepath.Join(cfg.BaseDir, "artifacts"))
		if err != nil {
			return nil, err
		}
		cfg.Artifacts = store
		cfg.ArtifactIndex = index
	}
	if cfg.Workflows == nil {
		// The workflow store (W3.48) — the contract's host port. The daemon
		// opens the file (workflows/workflows.json) and hands the kernel the
		// port; the agent-facing tool reads the same rows through the
		// narrower read side.
		//
		// A store that cannot be opened is a STARTUP ERROR: the workflow
		// library is part of the daemon, not a convenience.
		store, err := workflow.OpenStore(filepath.Join(cfg.BaseDir, "workflows"))
		if err != nil {
			return nil, err
		}
		cfg.Workflows = store
	}
	if cfg.Schedules == nil {
		// The typed schedule library (W3.49) — the contract's host port. The
		// daemon opens the file (cadence/schedules.json) and hands the kernel
		// the port; the live resident is built over that same store by the
		// composition root once the kernel exists.
		//
		// A library that cannot be opened is a STARTUP ERROR: the schedule
		// library is part of the daemon, not a convenience.
		store, err := cadence.OpenStore(filepath.Join(cfg.BaseDir, "cadence"))
		if err != nil {
			return nil, err
		}
		cfg.Schedules = store
	}
	if cfg.Standing == nil {
		// The standing-order library (W3.50) — the contract's store port. The
		// daemon opens the file (standing/standing.json) and hands the kernel
		// the port; the daemon's residents (the event runner and the cron
		// ticker) are built over that same store by the composition root once
		// the kernel exists.
		//
		// A library that cannot be opened is a STARTUP ERROR: the standing
		// library is part of the daemon, not a convenience.
		store, err := standing.Open(filepath.Join(cfg.BaseDir, "standing"))
		if err != nil {
			return nil, err
		}
		cfg.Standing = store
	}
	if cfg.CatalogStore == nil {
		// The catalog directory (W3.51) — the contract's store port. The
		// daemon opens the file (catalog/api.json + local.json + custom.json)
		// and hands the kernel the port; the kernel reloads the snapshot
		// through it and never opens a directory of its own.
		//
		// A catalog that cannot be read is a STARTUP ERROR: the catalog is
		// part of the daemon, not a convenience.
		store := catalog.NewStore(filepath.Join(cfg.BaseDir, "catalog"))
		if _, err := store.Load(); err != nil {
			return nil, err
		}
		cfg.CatalogStore = store
	}
	if cfg.CatalogSyncer == nil {
		// The catalog syncer (W3.51) — the contract's syncer port. The daemon
		// builds it over the netguard-screened client and hands the kernel the
		// port; the kernel never fetches anything itself.
		cfg.CatalogSyncer = catalog.NewSyncer()
	}
	if cfg.TasteStore == nil {
		// The curated exemplar library (W3.52) — the contract's store port. The
		// daemon opens the file (taste/taste.json) and hands the kernel the
		// port; the kernel selects the exemplars that apply to a run through
		// that port and never opens a library of its own.
		//
		// The library lives under the BASE DIR, so a tenant's kernel — which
		// has its own base dir — opens its own exemplars.
		//
		// A library that cannot be opened is a STARTUP ERROR: the exemplar
		// library is part of the daemon, not a convenience.
		store, err := taste.OpenStore(filepath.Join(cfg.BaseDir, "taste"))
		if err != nil {
			return nil, err
		}
		cfg.TasteStore = store
	}
	if cfg.Roster == nil {
		// The roster (W3.56) — the contract's store port. The daemon opens the
		// file (roster/roster.json) and hands the kernel the port; the kernel
		// drives the agent lifecycle through it, and never opens a roster of
		// its own.
		//
		// The roster lives under the BASE DIR, so a tenant's kernel — which has
		// its own base dir — keeps its own agents.
		//
		// A roster that cannot be opened is a STARTUP ERROR: the agents are
		// part of the daemon, not a convenience.
		store, err := roster.Open(filepath.Join(cfg.BaseDir, "roster"))
		if err != nil {
			return nil, err
		}
		cfg.Roster = store
	}
	if cfg.OKRStore == nil {
		// The objective library (W3.53) — the contract's store port. The daemon
		// opens the file (okr/okr.json) and hands the kernel the port; the
		// kernel creates goals, links tasks to key results and flips a goal's
		// status through it, and never opens a library of its own.
		//
		// The library lives under the BASE DIR, so a tenant's kernel — which
		// has its own base dir — opens its own objectives.
		//
		// A library that cannot be opened is a STARTUP ERROR: the objective
		// library is part of the daemon, not a convenience.
		store, err := okr.OpenStore(filepath.Join(cfg.BaseDir, "okr"))
		if err != nil {
			return nil, err
		}
		cfg.OKRStore = store
	}
	if cfg.WorldStore == nil {
		// The world-model file (W3.54) — the contract's store port. The daemon
		// opens the file (worldmodel/worldmodel.json) and hands the kernel the
		// port; the kernel never opens a graph of its own.
		//
		// The file lives under the BASE DIR, so a tenant's kernel — which has
		// its own base dir — opens its own world model.
		//
		// A file that cannot be opened is a STARTUP ERROR: the world model is
		// part of the daemon, not a convenience.
		store, err := worldmodel.Open(filepath.Join(cfg.BaseDir, "worldmodel"))
		if err != nil {
			return nil, err
		}
		cfg.WorldStore = store
	}
	if cfg.WorldGraph == nil {
		// The world-model BUILDER (W3.54) — the daemon names the module, the
		// kernel does not. The kernel calls the builder with the store it was
		// given and its own publisher, so the graph journals through the
		// kernel's bus without either side naming the other's implementation.
		cfg.WorldGraph = func(store worldmodelapi.Store, pub worldmodelapi.Publisher) worldmodelapi.Graph {
			return worldmodel.NewGraph(store, pub)
		}
	}
	if cfg.SkillStore == nil {
		// The skill library's file (W3.55) — the contract's store port. The
		// daemon opens the file (skills/skills.json) and hands the kernel the
		// port; the kernel never opens a library of its own.
		//
		// The file lives under the BASE DIR, so a tenant's kernel — which has
		// its own base dir — curates its own skills.
		//
		// A library that cannot be opened is a STARTUP ERROR: the skill
		// library is part of the daemon, not a convenience.
		store, err := skill.Open(filepath.Join(cfg.BaseDir, "skills"))
		if err != nil {
			return nil, err
		}
		cfg.SkillStore = store
	}
	if cfg.SkillCurator == nil {
		// The skill curator's BUILDER (W3.55) — the daemon names the module,
		// the kernel does not. The kernel calls the builder with the store it
		// was given and its own publisher, so every transition journals
		// through the kernel's bus without either side naming the other's
		// implementation.
		cfg.SkillCurator = func(store skillapi.Store, pub skillapi.Publisher) skillapi.Curator {
			f := skill.NewForge(store, pub)
			// Wire the on-disk bundle store so skills can ship reference files
			// + scripts (agentskills.io shape, M847). Best-effort: a
			// bundle-store failure leaves skills body-only rather than failing
			// daemon start.
			if bundles, berr := skill.OpenBundles(filepath.Join(cfg.BaseDir, "skills")); berr == nil {
				f.SetBundles(bundles)
			}
			return f
		}
	}
	if cfg.MemoryStore == nil {
		// The memory's FILE (W3.57) — the contract's store port. The daemon
		// opens the file (memory/memory.json) and hands the kernel the port;
		// the kernel never opens a memory of its own.
		//
		// The file lives under the BASE DIR, so a tenant's kernel — which has
		// its own base dir — keeps its own memory.
		//
		// A store that cannot be opened is a STARTUP ERROR: the memory is
		// part of the daemon, not a convenience.
		store, err := memory.Open(filepath.Join(cfg.BaseDir, "memory"))
		if err != nil {
			return nil, err
		}
		cfg.MemoryStore = store
	}
	if cfg.MemoryBuilder == nil {
		// The memory manager's BUILDER (W3.57) — the daemon names the module,
		// the kernel does not. The kernel calls the builder with the store it
		// was given, its own publisher, the embedder and the structured-output
		// path, so every mutation journals through the kernel's bus without
		// either side naming the other's implementation.
		cfg.MemoryBuilder = func(deps memoryapi.Deps) memoryapi.Manager {
			return memory.NewManager(deps)
		}
	}
	if cfg.MemoryGenerator == nil {
		// The structured-output path (W3.57) — it lives in another module,
		// so it is INJECTED rather than named: the knowledge module may not
		// reach into the runs module.
		cfg.MemoryGenerator = agent.GenerateObject
	}
	if cfg.MCPStore == nil {
		// The MCP registry's FILE (W3.58) — the contract's store port. The
		// daemon opens the file (mcp/servers.json) and hands the kernel the
		// port; the kernel never opens a registry of its own.
		//
		// The file lives under the BASE DIR, so a tenant's kernel — which has
		// its own base dir — keeps its own registry.
		//
		// A store that cannot be opened is a STARTUP ERROR: the registry is
		// part of the daemon, not a convenience.
		store, err := mcp.OpenStore(filepath.Join(cfg.BaseDir, "mcp"))
		if err != nil {
			return nil, err
		}
		cfg.MCPStore = store
	}
	if cfg.MCPDialer == nil {
		// The MCP DIALERS (W3.58) — the implementations live in the module and
		// the kernel may not name them, so the daemon injects them. The kernel
		// refuses an attach when no dialer is configured rather than calling
		// into nothing.
		cfg.MCPDialer = mcp.Dial
	}
	if cfg.MCPHTTPDialer == nil {
		cfg.MCPHTTPDialer = mcp.DialHTTP
	}
	if cfg.Workboard == nil {
		// The durable task board (W3.47) — the contract's host port. The
		// daemon opens the file (workboard/workboard.json) and hands the
		// kernel the port; the kernel drives the lifecycle through it, and the
		// agent-facing tool keeps the narrower read port.
		//
		// A store that cannot be opened is a STARTUP ERROR: the board is part
		// of the daemon, not a convenience.
		store, err := workboard.OpenStore(filepath.Join(cfg.BaseDir, "workboard"))
		if err != nil {
			return nil, err
		}
		cfg.Workboard = store
	}
	if cfg.ToolForge == nil {
		// The script-tool store (M794) — the contract's port (W3.45). The
		// daemon opens the file (toolforge/scripttools.json) and hands the
		// kernel the port; the kernel drives the lifecycle through it, and the
		// tool registry receives the narrower read port.
		//
		// A store that cannot be opened is a STARTUP ERROR: the forge is part
		// of the daemon, not a convenience.
		store, err := toolforge.Open(filepath.Join(cfg.BaseDir, "toolforge"))
		if err != nil {
			return nil, err
		}
		cfg.ToolForge = store
	}
	// The voice, image and rerank tools (W3.44): the kernel may not name a tool
	// package, so the composition root builds them over the daemon's own
	// adapter seams and hands them over as ordinary tools. They are built HERE,
	// after the store is open, because the two that persist bytes bind it.
	cfg.Tools = mediaTools(cfg.Tools, cfg)
	// The market tool (W3.46): the same rule — the kernel may not name the
	// market module, so the composition root builds the tool over the
	// contract's marketplace port and hands it over as an ordinary tool.
	mktTool := market.NewTool()
	cfg.Tools = withMarketTool(cfg.Tools, mktTool)
	k, err := kernelruntime.Open(cfg)
	if err != nil {
		return nil, err
	}
	// The tool resolves the KERNEL's marketplace port lazily: the daemon wires
	// the manager (SetMarket) after Open, and a tenant's kernel never gets one
	// — so a tenant's market tool reports the marketplace as unavailable
	// instead of borrowing the primary's manager.
	mktTool.Manager = k.Market
	// The workflow ENGINE is the daemon's too (W3.48): it is a module's
	// implementation, and the kernel is not allowed to name one. It is built
	// HERE — over the store the daemon holds and over the host's own ports
	// (the kernel's governed paths) — and handed over as the contract's
	// engine port.
	//
	// The engine is built AFTER Open, because the ports are the kernel's own:
	// a workflow run must keep the host's policy checks, its audit arc and
	// its spend attribution.
	k.SetWorkflowEngine(workflow.NewEngine(cfg.Workflows, k.WorkflowPorts()))
	k.SetReasoning(reasoning.New(k.ReasoningDependencies()))
	k.SetReaperScanner(reaper.New(k.ReaperPorts()))
	return k, nil
}

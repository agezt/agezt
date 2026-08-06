// SPDX-License-Identifier: MIT

// Kernel-bound zero-arg specs (Phase 2.2 PR 4): the tools that used to be
// constructed as captured locals in cmd/agezt/main.go (scheduleTool,
// runsTool, …) and .Bind()-ed to the live kernel hundreds of lines later. All
// of them are always-on, take no construction arguments, and their entire
// late wiring needed nothing but the open kernel (+ baseDir for overseer) —
// exactly the Configure phase. Each hook closes over the concrete instance its
// own Build produced. The notify / send_media / board tools are NOT here: they
// need live channels / the shared board store and stay in main.go until the
// LateDeps PR.
package builtintools

import (
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/toolreg"
	"github.com/agezt/agezt/plugins/tools/forgetool"
	"github.com/agezt/agezt/plugins/tools/introspecttool"
	"github.com/agezt/agezt/plugins/tools/mcptool"
	"github.com/agezt/agezt/plugins/tools/overseertool"
	"github.com/agezt/agezt/plugins/tools/runstool"
	scheduletool "github.com/agezt/agezt/plugins/tools/schedule"
	skilltool "github.com/agezt/agezt/plugins/tools/skilltool"
	standingtool "github.com/agezt/agezt/plugins/tools/standingtool"
	"github.com/agezt/agezt/plugins/tools/workboardtool"
	"github.com/agezt/agezt/plugins/tools/workflowtool"
)

// specSchedule — self-scheduling (`schedule`, M634): the agent arranges its OWN
// future runs in the daemon's cadence store. Configure binds the live store —
// the same one the schedule engine ticks, so an agent-created schedule fires
// like any operator-added one — plus the roster lookup for per-agent runs.
func specSchedule() toolreg.Spec {
	var st *scheduletool.Tool
	return toolreg.Spec{
		Name: "schedule",
		Build: func(toolreg.BuildDeps) (toolreg.Built, error) {
			st = scheduletool.New()
			return toolreg.Built{Tool: st}, nil
		},
		Configure: func(_ agent.Tool, d toolreg.KernelDeps) error {
			if d.K == nil {
				return nil
			}
			if sched := d.K.Schedules(); sched != nil {
				st.Bind(sched)
				st.BindAgentLookup(d.K.Roster().Get)
			}
			return nil
		},
	}
}

// specRuns — run introspection (`runs`, M644): the agent recalls its OWN past
// runs from the journal. Configure binds the kernel's live journal.
func specRuns() toolreg.Spec {
	var rt *runstool.Tool
	return toolreg.Spec{
		Name: "runs",
		Build: func(toolreg.BuildDeps) (toolreg.Built, error) {
			rt = runstool.New()
			return toolreg.Built{Tool: rt}, nil
		},
		Configure: func(_ agent.Tool, d toolreg.KernelDeps) error {
			if d.Journal != nil {
				rt.Bind(d.Journal)
			}
			return nil
		},
	}
}

// specStanding — standing orders (`standing`, M645): the agent creates durable
// event/cron trigger rules that wake an agent later. Configure binds the kernel
// (it satisfies the tool's journaled standing-CRUD surface).
func specStanding() toolreg.Spec {
	var st *standingtool.Tool
	return toolreg.Spec{
		Name: "standing",
		Build: func(toolreg.BuildDeps) (toolreg.Built, error) {
			st = standingtool.New()
			return toolreg.Built{Tool: st}, nil
		},
		Configure: func(_ agent.Tool, d toolreg.KernelDeps) error {
			if d.K != nil {
				st.Bind(d.K)
			}
			return nil
		},
	}
}

// specSkill — self-modification (`skill`, M648): the agent authors, promotes,
// and retires its own reusable procedures through the kernel's Forge — the same
// journaled, reversible state machine the operator drives.
func specSkill() toolreg.Spec {
	var sk *skilltool.Tool
	return toolreg.Spec{
		Name: "skill",
		Build: func(toolreg.BuildDeps) (toolreg.Built, error) {
			sk = skilltool.New()
			return toolreg.Built{Tool: sk}, nil
		},
		Configure: func(_ agent.Tool, d toolreg.KernelDeps) error {
			if d.K == nil {
				return nil
			}
			if fg := d.K.Forge(); fg != nil {
				sk.Bind(fg)
			}
			return nil
		},
	}
}

// specIntrospect — daemon self-inspection (`introspect`, M682): the agent reads
// the daemon's OWN live state — a real health overview plus schedule/standing
// detail — in one call, so a "summarise AGEZT's health" task can see everything
// instead of guessing.
func specIntrospect() toolreg.Spec {
	var it *introspecttool.Tool
	return toolreg.Spec{
		Name: "introspect",
		Build: func(toolreg.BuildDeps) (toolreg.Built, error) {
			it = introspecttool.New()
			return toolreg.Built{Tool: it}, nil
		},
		Configure: func(_ agent.Tool, d toolreg.KernelDeps) error {
			if d.K != nil {
				it.Bind(introspecttool.NewKernelSource(d.K))
			}
			return nil
		},
	}
}

// specOverseer — fleet supervision (`overseer`, M850): the brain/overseer agent
// supervises and intervenes on the fleet — list/cancel runs, halt/resume the
// daemon, pause/retire/revive agents, triage open help. BaseDir locates the
// board it reads.
func specOverseer() toolreg.Spec {
	var ot *overseertool.Tool
	return toolreg.Spec{
		Name: "overseer",
		Build: func(toolreg.BuildDeps) (toolreg.Built, error) {
			ot = overseertool.New()
			return toolreg.Built{Tool: ot}, nil
		},
		Configure: func(_ agent.Tool, d toolreg.KernelDeps) error {
			if d.K != nil {
				ot.Bind(overseertool.NewKernelSource(d.K, d.BaseDir))
			}
			return nil
		},
	}
}

// specToolForge — self-tooling (`tool_forge`, M794): the agent builds its OWN
// tools — drafts a script, tests it in the code_exec sandbox, and once promoted
// every run can call it as forge_<name>. Configure binds the live kernel's
// journaled scripttool.* lifecycle.
func specToolForge() toolreg.Spec {
	var ft *forgetool.Tool
	return toolreg.Spec{
		Name: "tool_forge",
		Build: func(toolreg.BuildDeps) (toolreg.Built, error) {
			ft = forgetool.New()
			return toolreg.Built{Tool: ft}, nil
		},
		Configure: func(_ agent.Tool, d toolreg.KernelDeps) error {
			if d.K != nil {
				ft.Bind(d.K)
			}
			return nil
		},
	}
}

// specMCP — MCP self-install (`mcp`, M796): the agent extends its own toolbox —
// registering and ATTACHING MCP servers at runtime (Edict mcp.install, Ask by
// default). The boot-time auto-attach of already-enabled servers stays in
// main.go (it needs the daemon context).
func specMCP() toolreg.Spec {
	var mt *mcptool.Tool
	return toolreg.Spec{
		Name: "mcp",
		Build: func(toolreg.BuildDeps) (toolreg.Built, error) {
			mt = mcptool.New()
			return toolreg.Built{Tool: mt}, nil
		},
		Configure: func(_ agent.Tool, d toolreg.KernelDeps) error {
			if d.K != nil {
				mt.Bind(d.K)
			}
			return nil
		},
	}
}

// specWorkflow — workflow authoring (`workflow`, M802): the agent authors and
// runs durable workflows in the SAME store the console canvas edits (Edict
// workflow.manage, AskFirst by default; tool nodes inside a run re-gate per
// call).
func specWorkflow() toolreg.Spec {
	var wt *workflowtool.Tool
	return toolreg.Spec{
		Name: "workflow",
		Build: func(toolreg.BuildDeps) (toolreg.Built, error) {
			wt = workflowtool.New()
			return toolreg.Built{Tool: wt}, nil
		},
		Configure: func(_ agent.Tool, d toolreg.KernelDeps) error {
			if d.K != nil {
				wt.Bind(d.K)
			}
			return nil
		},
	}
}

// specWorkboard — durable typed work items (`workboard`): agents create and
// move tasks through the same journaled runtime path the CLI and control plane
// use, instead of hiding state in chat.
func specWorkboard() toolreg.Spec {
	var wb *workboardtool.Tool
	return toolreg.Spec{
		Name: "workboard",
		Build: func(toolreg.BuildDeps) (toolreg.Built, error) {
			wb = workboardtool.New()
			return toolreg.Built{Tool: wb}, nil
		},
		Configure: func(_ agent.Tool, d toolreg.KernelDeps) error {
			if d.K != nil {
				wb.Bind(d.K)
			}
			return nil
		},
	}
}

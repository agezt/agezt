// SPDX-License-Identifier: MIT

// Set*-injection specs (Phase 2.2 PR 3): the always-on tools whose post-Open
// dependency injection (SetKernel / SetIndex / SetStore / SetRunner) used to be
// hand-wired through string-keyed downcasts in cmd/agezt/main.go, plus
// code_exec whose ScriptRunner pre-Open hook and bus/Conductor wiring lived
// there too. Each spec's hooks close over the concrete instance its OWN Build
// produced, so no type assertion is ever needed and a wrong-name lookup is a
// compile error, not a silently-skipped `if _, ok :=` block.
package builtintools

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/toolreg"
	artifactstool "github.com/agezt/agezt/plugins/tools/artifacts"
	"github.com/agezt/agezt/plugins/tools/codeexec"
	conductortool "github.com/agezt/agezt/plugins/tools/conductor"
	configtool "github.com/agezt/agezt/plugins/tools/config"
	counciltool "github.com/agezt/agezt/plugins/tools/council"
	dbtool "github.com/agezt/agezt/plugins/tools/db"
	research "github.com/agezt/agezt/plugins/tools/research"
)

// specConfig — the agent's window into the Config Center: read/write/register
// settings (same registry + vault as the `agt config` CLI and HTTP routes).
// The kernel is injected after runtime.Open so live-apply fields
// (provider/model) can rebuild the provider in place via Reload().
func specConfig() toolreg.Spec {
	var ct *configtool.Tool
	return toolreg.Spec{
		Name: "config",
		Build: func(d toolreg.BuildDeps) (toolreg.Built, error) {
			ct = configtool.New(d.BaseDir)
			return toolreg.Built{Tool: ct, Desc: "config()"}, nil
		},
		Configure: func(_ agent.Tool, d toolreg.KernelDeps) error {
			if d.K != nil {
				ct.SetKernel(d.K)
			}
			return nil
		},
	}
}

// specArtifacts — list/read/delete the files the agent has saved (fetch
// downloads, offloaded tool outputs, inbound images), so a file from one run is
// usable in a later one (M832). A read/list/delete view over the artifact index
// injected after the kernel opens. Always registered.
func specArtifacts() toolreg.Spec {
	var af *artifactstool.Tool
	return toolreg.Spec{
		Name: "artifacts",
		Build: func(toolreg.BuildDeps) (toolreg.Built, error) {
			af = artifactstool.New()
			return toolreg.Built{Tool: af, Desc: "artifacts(list/read/delete)"}, nil
		},
		Configure: func(_ agent.Tool, d toolreg.KernelDeps) error {
			if d.Artifacts != nil {
				af.SetIndex(d.Artifacts)
			}
			return nil
		},
	}
}

// specDB — the Personal Data Lake (M834): agent-built, multi-agent-shared
// structured collections (expenses, tasks, notes, contacts, …) the human can
// also browse in the Data view. The lake is injected after the kernel opens.
// Always registered.
func specDB() toolreg.Spec {
	var dbt *dbtool.Tool
	return toolreg.Spec{
		Name: "db",
		Build: func(toolreg.BuildDeps) (toolreg.Built, error) {
			dbt = dbtool.New()
			return toolreg.Built{Tool: dbt, Desc: "db(data-lake)"}, nil
		},
		Configure: func(_ agent.Tool, d toolreg.KernelDeps) error {
			if d.Lake != nil {
				dbt.SetStore(d.Lake)
			}
			return nil
		},
	}
}

// specCouncil — the Council of Elders (M837): convene a multi-model panel for a
// hard decision and get a reconciled consensus. The kernel is injected after
// Open as the deliberation runner. Always registered.
func specCouncil() toolreg.Spec {
	var ct *counciltool.Tool
	return toolreg.Spec{
		Name: "council",
		Build: func(toolreg.BuildDeps) (toolreg.Built, error) {
			ct = counciltool.New()
			return toolreg.Built{Tool: ct, Desc: "council(consensus panel)"}, nil
		},
		Configure: func(_ agent.Tool, d toolreg.KernelDeps) error {
			if d.K != nil {
				ct.SetRunner(d.K)
			}
			return nil
		},
	}
}

// specConductor — the Conductor (M997): an asymmetric, verify-driven panel
// (Thinker/Worker/Verifier) that runs the worker's code and loops until it
// passes. The kernel is injected after Open as the orchestration runner (the
// code-exec backend is wired by specCodeExec's Configure). Always registered.
func specConductor() toolreg.Spec {
	var cond *conductortool.Tool
	return toolreg.Spec{
		Name: "conductor",
		Build: func(toolreg.BuildDeps) (toolreg.Built, error) {
			cond = conductortool.New()
			return toolreg.Built{Tool: cond, Desc: "conductor(verify-driven panel)"}, nil
		},
		Configure: func(_ agent.Tool, d toolreg.KernelDeps) error {
			if d.K != nil {
				cond.SetRunner(d.K)
			}
			return nil
		},
	}
}

// specResearch — the deep-research harness (M1001): decompose a question,
// gather independent web sources via web_search + browser.read, and synthesize
// a citation-grounded report. The kernel is injected after Open as the harness
// runner. Always registered; the underlying searches/fetches are each gated by
// their own capability inside RunTool.
func specResearch() toolreg.Spec {
	var rt *research.Tool
	return toolreg.Spec{
		Name: "research",
		Build: func(toolreg.BuildDeps) (toolreg.Built, error) {
			rt = research.New()
			return toolreg.Built{Tool: rt, Desc: "research(deep-research harness)"}, nil
		},
		Configure: func(_ agent.Tool, d toolreg.KernelDeps) error {
			if d.K != nil {
				rt.SetRunner(d.K)
			}
			return nil
		},
	}
}

// specCodeExec — the agent writes & runs Python/JS/TS in a sandboxed workspace
// under <baseDir>/sandbox (M683). Routed through the same warden as shell, with
// a scrubbed env (no secrets), per-call ephemeral dirs (or named persistent
// projects), and resource caps. Registered when at least one runtime is
// present, UNLESS AGEZT_SANDBOX=off. Network is on by default;
// AGEZT_SANDBOX_NO_NET=1 forces it off.
//
// Lifecycle hooks (all close over the concrete instance, and only run when
// Build actually registered the tool):
//   - PreOpen: the script-tool forge (M794) executes forged tools through the
//     same sandbox — cfg.ScriptRunner is set before runtime.Open so the forge
//     is live from the first run. Without the sandbox the forge reports itself
//     unavailable.
//   - Configure: the artifact index (scripts export durable files under
//     .agezt-artifacts/), the bus (each run journals a code.executed event,
//     M683), and the Conductor's Verifier backend (M997) — nil-safe on the
//     kernel side: when code_exec is absent the Verifier falls back to LLM
//     critique.
func specCodeExec() toolreg.Spec {
	var ce *codeexec.Tool
	return toolreg.Spec{
		Name: "code_exec",
		Build: func(d toolreg.BuildDeps) (toolreg.Built, error) {
			if strings.EqualFold(strings.TrimSpace(d.Get(brand.EnvPrefix+"SANDBOX")), "off") {
				return toolreg.Built{}, nil
			}
			rt := codeexec.DetectRuntimes()
			if len(rt) == 0 {
				return toolreg.Built{}, nil
			}
			netOn := d.Get(brand.EnvPrefix+"SANDBOX_NO_NET") != "1"
			ce = codeexec.NewWithWarden(d.Warden, filepath.Join(d.BaseDir, "sandbox"), rt, netOn)
			netTag := "net=on"
			if !netOn {
				netTag = "net=off"
			}
			desc := fmt.Sprintf("code_exec(langs=%s, %s)", strings.Join(ce.Languages(), "/"), netTag)
			return toolreg.Built{Tool: ce, Desc: desc}, nil
		},
		PreOpen: func(_ agent.Tool, cfg *runtime.Config) {
			cfg.ScriptRunner = ce
		},
		Configure: func(_ agent.Tool, d toolreg.KernelDeps) error {
			if d.Artifacts != nil {
				ce.SetIndex(d.Artifacts)
			}
			if d.Bus != nil {
				ce.Bind(d.Bus)
			}
			if d.K != nil {
				d.K.SetConductorExec(ce)
			}
			return nil
		},
	}
}

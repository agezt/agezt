// SPDX-License-Identifier: MIT

// Env-gated / workspace-scoped specs (Phase 2.2 PR 6): the last inline blocks
// of cmd/agezt's buildTools — the always-on workspace pair (shell, file) and
// the operator-opt-in externals (coding, acp_agent, homeassistant,
// remote_run). Construction reads the environment through BuildDeps.Get; a
// malformed operator spec (remote_run's peer lists, file's checkpoint init)
// returns a Build error = hard boot failure, exactly as before.
package builtintools

import (
	"fmt"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/toolreg"
	"github.com/agezt/agezt/plugins/tools/acpagent"
	"github.com/agezt/agezt/plugins/tools/coding"
	filetool "github.com/agezt/agezt/plugins/tools/file"
	hatool "github.com/agezt/agezt/plugins/tools/homeassistant"
	"github.com/agezt/agezt/plugins/tools/peer"
	"github.com/agezt/agezt/plugins/tools/shell"
)

// specShell — always registered, routed through Warden. Effective isolation
// profile depends on host OS (M1.c: always ProfileNone with the request
// journaled as a downgrade on non-Linux). Runs in the shared workspace root
// (M609) so it sees the same files as the file tool.
func specShell() toolreg.Spec {
	return toolreg.Spec{
		Name: "shell",
		Build: func(d toolreg.BuildDeps) (toolreg.Built, error) {
			sh := shell.NewWithWarden(d.Warden)
			sh.WorkDir = d.WorkspaceRoot
			sh.BaseDir = d.BaseDir
			return toolreg.Built{Tool: sh, Desc: "shell(warden=requested-namespace)"}, nil
		},
	}
}

// specFile — scoped to the same workspace root as shell, with the checkpoint
// store under the daemon base dir. A checkpoint-init failure is a hard boot
// error (the tool would otherwise silently lose its undo surface).
func specFile() toolreg.Spec {
	return toolreg.Spec{
		Name: "file",
		Build: func(d toolreg.BuildDeps) (toolreg.Built, error) {
			ft, err := filetool.NewWithCheckpoint(d.WorkspaceRoot, d.BaseDir)
			if err != nil {
				return toolreg.Built{}, err
			}
			return toolreg.Built{Tool: ft, Desc: "file(root=" + ft.Root() + ")"}, nil
		},
	}
}

// specCoding — external coding-agent bridge (P6-CODE). Registered only when
// AGEZT_CODING_CMD is set (the command that runs Claude Code / Codex / Aider /
// any agent). It operates on a git worktree of the workspace and returns a
// diff; it never merges. Off by default.
func specCoding() toolreg.Spec {
	return toolreg.Spec{
		Name: "coding",
		Build: func(d toolreg.BuildDeps) (toolreg.Built, error) {
			codingCmd := strings.TrimSpace(d.Get(brand.EnvPrefix + "CODING_CMD"))
			if codingCmd == "" {
				return toolreg.Built{}, nil
			}
			ct := coding.New(codingCmd, coding.AbsRepo(d.WorkspaceRoot))
			if ct == nil {
				return toolreg.Built{}, nil
			}
			return toolreg.Built{Tool: ct, Desc: "coding(external agent)"}, nil
		},
	}
}

// specACPAgent — external ACP-agent bridge (SPEC-15 §3, the inverse of `agt
// acp`). It drives an external agent speaking the Agent Client Protocol over
// stdio (Gemini CLI, Claude Code's adapter, Codex, …) over JSON-RPC and relays
// its answer. Registered when AGEZT_ACP_AGENT_CMD sets a default command OR
// any catalog ACP agent is installed on the host (discovery via
// kernel/acpcatalog), so a run can delegate to any installed agent by slug
// without configuration.
func specACPAgent() toolreg.Spec {
	return toolreg.Spec{
		Name: "acp_agent",
		Build: func(d toolreg.BuildDeps) (toolreg.Built, error) {
			acpCmd := strings.TrimSpace(d.Get(brand.EnvPrefix + "ACP_AGENT_CMD"))
			at := acpagent.New(acpCmd, acpagent.AbsCwd(d.WorkspaceRoot))
			if at == nil {
				return toolreg.Built{}, nil
			}
			return toolreg.Built{Tool: at, Desc: "acp_agent(external agent)"}, nil
		},
	}
}

// specHomeAssistant — read entity state + call services on the operator's Home
// Assistant. Shares the channel's AGEZT_HOMEASSISTANT_URL/_TOKEN (same
// instance), but is gated by its OWN allowlists so the outbound notify channel
// can be configured without auto-exposing an actionable control surface:
//
//	AGEZT_HOMEASSISTANT_TOOL_READ      — entity read allowlist (get_states)
//	AGEZT_HOMEASSISTANT_TOOL_SERVICES  — service call allowlist (call_service)
//	AGEZT_HOMEASSISTANT_TOOL_ALLOW_ALL_SERVICES=1 — bypass the service allowlist (DANGEROUS)
//
// Both allowlists are fail-closed; the tool registers only when URL+TOKEN are
// set AND at least one axis is enabled (so bare channel config exposes
// nothing).
func specHomeAssistant() toolreg.Spec {
	return toolreg.Spec{
		Name: "homeassistant",
		Build: func(d toolreg.BuildDeps) (toolreg.Built, error) {
			haURL := strings.TrimSpace(d.Get(brand.EnvPrefix + "HOMEASSISTANT_URL"))
			haTok := strings.TrimSpace(d.Get(brand.EnvPrefix + "HOMEASSISTANT_TOKEN"))
			read := splitHosts(nil, d.Get(brand.EnvPrefix+"HOMEASSISTANT_TOOL_READ"))
			services := splitHosts(nil, d.Get(brand.EnvPrefix+"HOMEASSISTANT_TOOL_SERVICES"))
			if d.Get(brand.EnvPrefix+"HOMEASSISTANT_TOOL_ALLOW_ALL_SERVICES") == "1" {
				services = append(services, "*")
				fmt.Fprintln(d.Stderr, "WARNING: AGEZT_HOMEASSISTANT_TOOL_ALLOW_ALL_SERVICES=1 lets the agent call ANY Home Assistant service.")
			}
			if haURL == "" || haTok == "" || (len(read) == 0 && len(services) == 0) {
				return toolreg.Built{}, nil
			}
			hat := hatool.New()
			hat.BaseURL = haURL
			hat.Token = haTok
			hat.ReadEntities = read
			hat.AllowedServices = services
			return toolreg.Built{Tool: hat, Desc: "homeassistant(" + hat.Capabilities() + ")"}, nil
		},
	}
}

// specRemoteRun — mesh delegation to a peer Agezt node over its REST API (M8).
// Registered only when AGEZT_PEERS is set (name=url|token,…). A malformed spec
// is a hard startup error so a misconfigured mesh is caught early. Per-tenant
// peer sets (M219): a tenant's runs route against its own peers, falling back
// to the global set — parsed/validated up front like AGEZT_PEERS.
func specRemoteRun() toolreg.Spec {
	return toolreg.Spec{
		Name: "remote_run",
		Build: func(d toolreg.BuildDeps) (toolreg.Built, error) {
			peerSpec := strings.TrimSpace(d.Get(brand.EnvPrefix + "PEERS"))
			tenantSpec := strings.TrimSpace(d.Get(brand.EnvPrefix + "TENANT_PEERS"))
			if peerSpec == "" && tenantSpec == "" {
				return toolreg.Built{}, nil
			}
			peers, err := peer.ParsePeers(peerSpec)
			if err != nil {
				return toolreg.Built{}, fmt.Errorf("%sPEERS: %w", brand.EnvPrefix, err)
			}
			tenantPeers, terr := peer.ParseTenantPeers(tenantSpec)
			if terr != nil {
				return toolreg.Built{}, fmt.Errorf("%sTENANT_PEERS: %w", brand.EnvPrefix, terr)
			}
			pt := peer.NewWithTenants(peers, tenantPeers)
			if pt == nil {
				return toolreg.Built{}, nil
			}
			desc := fmt.Sprintf("remote_run(%d peer(s))", len(peers))
			if len(tenantPeers) > 0 {
				desc = fmt.Sprintf("remote_run(%d peer(s), %d tenant override(s))", len(peers), len(tenantPeers))
			}
			return toolreg.Built{Tool: pt, Desc: desc}, nil
		},
	}
}

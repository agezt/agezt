// SPDX-License-Identifier: MIT

// Package edict is the policy engine + trust ladder
// (TASKS P1-EDICT-01..03; DECISIONS F3/F4).
//
// Two layers:
//
//  1. Hard-deny rules (DECISIONS F4) — pattern-matched against tool input;
//     ALWAYS deny regardless of trust level, never overridable. fork-bombs,
//     rm -rf /, mkfs, shutdown/reboot, audit-disable attempts.
//
//  2. Trust ladder (DECISIONS F3) — per-capability level L0..L4.
//     L0 deny · L1-L3 ask · L4 allow. "Ask" levels are resolved by the
//     engine's AskPolicy: AskAllow (default) treats Ask as Allow + WouldAsk=true
//     so the journal captures the would-have-been-prompt; AskDeny treats Ask as
//     Deny (strict mode); AskPrompt routes a live human approval via the
//     runtime's approval.Registry, blocking the call until an operator decides.
//
// Every Decide call is intended to be journaled as a policy.decision event
// by the runtime; the engine itself does not journal so it stays a pure,
// easily-testable function.
package edict

import (
)

// Capability identifies a class of action governed by policy. Capability
// strings are stable; downstream loggers/UIs depend on them.
type Capability string

const (
	CapShell        Capability = "shell"
	CapFileRead     Capability = "file.read"
	CapFileWrite    Capability = "file.write"
	CapFileDelete   Capability = "file.delete"
	CapFileList     Capability = "file.list"
	CapHTTPGet      Capability = "http.get"
	CapHTTPPost     Capability = "http.post"
	CapProviderCall Capability = "provider.call"
	// CapDelegate gates the `delegate` tool (spawn a sub-agent, P6-MULTI-01).
	// The delegation itself is allowed by default — it has no external side
	// effect of its own; the sub-agent's actual tool calls are each gated
	// through this same engine, so safety is enforced where the action happens.
	CapDelegate Capability = "delegate"
	// CapCoding gates the `coding` tool (delegate to an external coding agent
	// in an isolated git worktree, P6-CODE). It runs an external process that
	// writes files, so it is Ask-first by default — but the change lands only
	// in a throwaway worktree and is returned as a diff, never merged.
	CapCoding Capability = "coding"
	// CapACPAgent gates the `acp_agent` tool (delegate to an external agent over
	// the Agent Client Protocol, SPEC-15 §3). It spawns an external agent that
	// can act in its own sandbox, so it is Ask-first by default like coding.
	CapACPAgent Capability = "acp_agent"
	// CapRemoteRun gates the `remote_run` tool (delegate a task to a peer Agezt
	// node over its REST API, M8 mesh). It ships a task to an external node — an
	// outward, side-effecting action — so it is Ask-first by default.
	CapRemoteRun Capability = "remote_run"
	// CapNotify gates the `notify` tool (M143): the agent proactively sends a short
	// message to the operator over a configured channel mid-run. It is outward, but
	// the destinations are pinned to the operator's OWN pre-configured allowlist —
	// the agent supplies only the text, never the recipient — so there is no
	// arbitrary-exfiltration surface. Allowed by default (the agent talking to its
	// owner); an operator can still raise its level or deny it like any capability.
	CapNotify Capability = "notify"
	// CapHomeAssistantRead gates the `homeassistant` tool's get_states operation:
	// reading smart-home entity state. Read-only and filtered to an entity
	// allowlist, so Allow by default — the agent can answer "is the light on?"
	// without a prompt for every read.
	CapHomeAssistantRead Capability = "homeassistant.read"
	// CapHomeAssistantCall gates the `homeassistant` tool's call_service operation:
	// ACTUATING the physical world (lights, locks, climate). Ask-first by default —
	// turning something on/off in the operator's home warrants confirmation, even
	// though it is already constrained to a service allowlist.
	CapHomeAssistantCall Capability = "homeassistant.call"

	// CapBrowserRead gates the `browser.read` tool: fetching a web page and
	// returning its visible text. A network read, so ask-first by default (folds
	// to allow under AskAllow) — symmetric with CapHTTPGet; the tool's own host
	// allowlist is the second gate. Previously UNREGISTERED, which made
	// browser.read permanently default-denied and ungrantable via the policy
	// surface (M613).
	CapBrowserRead Capability = "browser.read"
	// CapBrowserAction gates `browser.action`: launching a real browser and
	// performing user-like clicks/fills/keypresses. It has a higher blast radius
	// than browser.read because actions can mutate remote state, so operators can
	// govern it separately from plain page reads.
	CapBrowserAction Capability = "browser.action"
	// CapMemory gates the `memory` tool: persisting/recalling durable knowledge
	// in the operator's own local store. Low risk, Allow by default. Previously
	// unregistered → default-denied (M613).
	CapMemory Capability = "memory"
	// CapWorld gates the `world` tool: reading/growing the local world-model
	// graph. Low risk, Allow by default. Previously unregistered → default-denied
	// (M613).
	CapWorld Capability = "world"
	// CapWebSearch gates the `web_search` tool: running a keyword query against a
	// public search engine and returning result titles/URLs/snippets. A network
	// read with no operator-controlled target host (the engine is fixed), so it
	// is ask-first by default — symmetric with CapBrowserRead/CapHTTPGet (M627).
	CapWebSearch Capability = "web.search"
	// CapResearch gates the `research` tool: the deep-research harness that
	// composes web_search + browser.read + provider calls into a multi-source,
	// citation-grounded report. Every underlying network read and model call is
	// still gated by its own capability inside RunTool, so this axis governs the
	// orchestration itself (fan-out + model spend). Symmetric with CapWebSearch:
	// a network read with fixed engine host, ask-first in spirit but LevelAllow
	// under the max-autonomy default posture (M1001).
	CapResearch Capability = "research"
	// CapSchedule gates the `schedule` tool: the agent arranging its OWN future
	// runs (one-shot / recurring / daily) in the daemon's cadence store. A
	// genuine autonomy grant — a scheduled intent fires later through the full
	// governed loop — so it is ask-first by default (M634).
	CapSchedule Capability = "schedule"
	// CapRunsRead gates the `runs` tool: the agent reading its OWN past runs from
	// the journal (recent runs / stats / search). A read of local activity the
	// operator already owns — low risk, Allow by default (M644).
	CapRunsRead Capability = "runs.read"
	// CapStanding gates the `standing` tool: the agent creating durable event/cron
	// wake rules for governed future runs. It does not mint agent identities, but
	// it does set up unattended behaviour, so it is ask-first by default (M645).
	CapStanding Capability = "standing"
	// CapBoard gates the `board` tool: agents posting to and reading from the
	// shared, persistent message board so they can coordinate and talk to each
	// other. A local shared note-store like memory — low risk, Allow by
	// default (M647).
	CapBoard Capability = "board"
	// CapWorkboard gates the `workboard` tool: agents creating and moving durable
	// typed task records. It is local coordination state, not external execution,
	// and every mutation is journaled as workboard.task.*; operators can still
	// tighten this axis independently from the free-form board mailbox.
	CapWorkboard Capability = "workboard"
	// CapSkill gates the `skill` tool: the agent modifying ITSELF — authoring,
	// promoting, and retiring its own reusable procedures through Forge. A genuine
	// self-modification grant (a learned, active skill shapes future planning), but
	// every transition is journaled and reversible (`agt skill revert`) and a new
	// skill starts as a draft outside the retrieval pool — so ask-first by
	// default (M648).
	CapSkill Capability = "skill"
	// CapIntrospect gates the `introspect` tool: the agent reading the daemon's
	// OWN live state in one call — health overview (uptime, halted, active runs,
	// counts), plus detailed listings of schedules and standing orders. A
	// read-only reflection of state the operator already owns, no mutation and no
	// network — low risk, Allow by default (M682), so a "summarise AGEZT's health"
	// task can actually see everything instead of guessing.
	CapIntrospect Capability = "introspect"
	// CapOversee gates the `overseer` tool: a privileged "brain" agent supervising
	// and INTERVENING on the rest of the system — cancel a runaway run, halt or
	// resume the whole daemon, pause / retire / revive other agents, triage the
	// open help requests (M850). High blast radius (it can stop everything), but
	// every action goes through the kernel's own journaled, reversible methods —
	// the same controls an operator has. Allow by default per the default-allow
	// posture, with its OWN knob so an owner can opt OUT of autonomous oversight
	// without disabling delegation.
	CapOversee Capability = "oversee"
	// CapCodeExec gates the `code_exec` tool: the agent WRITING and RUNNING
	// arbitrary code (Python / Node / Deno) to compute, scrape, and build things
	// (M683). A high-blast-radius capability — code can read/write the sandbox
	// workspace and (by default) reach the network — but every run is sandboxed
	// (scrubbed env so secrets never leak, work confined to <baseDir>/sandbox,
	// resource-capped, Deno fs-jailed on every OS) and journaled (code.executed +
	// warden.exec). The owner runs it Allow by default for full autonomy; operators
	// who want confirmation set it to ask/deny in the policy center.
	CapCodeExec Capability = "code.exec"

	// CapToolForge gates the AUTHORING ops of the `tool_forge` tool (M794):
	// the agent drafting, editing, listing, and inspecting its own script
	// tools. Like CapSkill, a genuine self-modification grant — but a draft
	// is never live (promotion is operator-driven through the control plane,
	// not a tool op), every transition is journaled (scripttool.*), and
	// quarantine is an instant kill switch — so ask-first by default.
	// op=test EXECUTES the draft in the sandbox and maps to CapCodeExec.
	CapToolForge Capability = "tool.forge"

	// CapMCPInstall gates the SELF-INSTALL ops of the `mcp` tool (M796): the
	// agent registering, ATTACHING (spawning an arbitrary external process),
	// detaching, or removing an MCP server at runtime. The strongest
	// self-extension grant in the system — an attached server's tools are
	// whatever IT advertises — so Ask on every call by default. The child
	// gets a scrubbed env (no AGEZT_*/secrets) and every transition is
	// journaled (mcp.*); detach is the instant kill switch.
	CapMCPInstall Capability = "mcp.install"
	// CapMCP gates every CALL of a bridged mcp_<server>_<tool> tool: code
	// the daemon didn't ship, talking to a process the operator (or an
	// approved agent) attached. Ask-first by default — vet the first use
	// per session, then flow.
	CapMCP Capability = "mcp.call"

	// CapMarket gates the `market` tool: an agent discovering and installing
	// capability packs (skills + MCP servers + CLI tools) from the marketplace
	// mid-task. Self-extension like CapMCPInstall, but a superset — a pack can
	// carry MCP servers AND skills AND host tools — so it gets its own axis
	// rather than borrowing the MCP-specific one and misreporting itself in the
	// audit trail. Search and install share the axis: an operator either lets
	// agents self-extend from the marketplace or does not, and searching alone
	// has no effect worth a separate grant.
	CapMarket Capability = "market.install"

	// CapWorkflow gates the MUTATING ops of the `workflow` tool (M802): an
	// agent saving, running, or arming durable workflows. Saving installs
	// standing automation (a cron/event trigger keeps firing after the run
	// that wrote it ends) — but every tool node inside a run passes the
	// regular per-capability gate, and new workflows arrive disabled, so the
	// blast radius of a bad save is bounded. Ask-first by default: vet the
	// first use per session, then flow. list/show map to introspection.
	CapWorkflow Capability = "workflow.manage"

	// CapConfigRead / CapConfigWrite gate the `config` tool (M696): the agent
	// reading vs mutating Config Center settings. Reads (schema/get) are low-risk
	// and Allow by default. Writes (set/register/unregister) are Ask by default —
	// a write can reach built-in security fields (e.g. AGEZT_ALLOW_ALL) and a
	// register adds a new editable surface — so a confirmation is warranted;
	// operators can lower it in the policy center.
	CapConfigRead  Capability = "config.read"
	CapConfigWrite Capability = "config.write"
)

// TrustLevel encodes the trust ladder (DECISIONS F3).
type TrustLevel int

const (
	// LevelDeny — hard block (L0). Never overridable per-cap.
	LevelDeny TrustLevel = 0
	// LevelAsk — every call requires approval (L1).
	LevelAsk TrustLevel = 1
	// LevelAskFirst — ask on first use per session (L2).
	LevelAskFirst TrustLevel = 2
	// LevelAskScoped — ask once per (session, scope) (L3).
	LevelAskScoped TrustLevel = 3
	// LevelAllow — silent allow (L4).
	LevelAllow TrustLevel = 4
)

// String returns the conventional Lx label.
// Used by the control plane to power `agt edict show`.
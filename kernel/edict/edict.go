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
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"unicode"
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
	// CapSchedule gates the `schedule` tool: the agent arranging its OWN future
	// runs (one-shot / recurring / daily) in the daemon's cadence store. A
	// genuine autonomy grant — a scheduled intent fires later through the full
	// governed loop — so it is ask-first by default (M634).
	CapSchedule Capability = "schedule"
	// CapRunsRead gates the `runs` tool: the agent reading its OWN past runs from
	// the journal (recent runs / stats / search). A read of local activity the
	// operator already owns — low risk, Allow by default (M644).
	CapRunsRead Capability = "runs.read"
	// CapStanding gates the `standing` tool: the agent creating its OWN autonomous,
	// trigger-driven agents (Chronos standing orders that fire a plan on a cron
	// schedule or a matching event). The strongest autonomy grant — it sets up
	// unattended behaviour — so ask-first by default (M645).
	CapStanding Capability = "standing"
	// CapBoard gates the `board` tool: agents posting to and reading from the
	// shared, persistent message board so they can coordinate and talk to each
	// other. A local shared note-store like memory — low risk, Allow by
	// default (M647).
	CapBoard Capability = "board"
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
func (l TrustLevel) String() string {
	switch l {
	case LevelDeny:
		return "L0"
	case LevelAsk:
		return "L1"
	case LevelAskFirst:
		return "L2"
	case LevelAskScoped:
		return "L3"
	case LevelAllow:
		return "L4"
	default:
		return fmt.Sprintf("L?(%d)", int(l))
	}
}

// ParseTrustLevel parses an operator-facing trust-level string into a
// TrustLevel. It accepts the canonical "L0".."L4" labels (case-insensitive,
// the same vocabulary TrustLevel.String() emits) and the word aliases
// deny/ask/askfirst/askscoped/allow, so `agt edict level shell allow` and
// `... shell L4` are equivalent. Unknown input is an error, not a default —
// a typo must never silently land a capability at the wrong level.
func ParseTrustLevel(s string) (TrustLevel, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "l0", "deny":
		return LevelDeny, nil
	case "l1", "ask":
		return LevelAsk, nil
	case "l2", "askfirst", "ask-first":
		return LevelAskFirst, nil
	case "l3", "askscoped", "ask-scoped":
		return LevelAskScoped, nil
	case "l4", "allow":
		return LevelAllow, nil
	}
	return 0, fmt.Errorf("edict: unknown trust level %q (want L0..L4 or deny/ask/askfirst/askscoped/allow)", s)
}

// Decision is the engine's final operational decision.
type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
)

// Outcome is the full result of Decide. The runtime journals Outcomes
// as policy.decision events.
type Outcome struct {
	Decision   Decision
	Capability Capability
	Level      TrustLevel
	Reason     string
	// HardDenied is true iff a hard-deny rule matched, regardless of
	// trust level. A hard-denied Outcome always has Decision=Deny.
	HardDenied bool
	// HardDenyRule, when HardDenied, names the matching rule.
	HardDenyRule string
	// WouldAsk is true when the trust level was Ask-class (L1..L3) and
	// AskPolicy folded it. Used by the runtime to flag the journal entry
	// even though the operational decision was Allow.
	WouldAsk bool
	// RequiresApproval is true when AskPolicy=AskPrompt landed on an
	// Ask-class level. The runtime should pause the tool-loop and
	// submit an approval.Request; only after a grant should the call
	// proceed. Decision is left as DecisionDeny in this case so that
	// callers who ignore RequiresApproval still default-fail-closed.
	RequiresApproval bool
}

// AskPolicy controls how the engine resolves Ask-class levels (L1..L3): fold
// them into Allow (unattended runs), fold them into Deny (strict), or flag them
// for live human approval. The last is fully wired — AskPrompt sets
// RequiresApproval and the runtime routes the call through approval.Registry,
// blocking until an operator decides (see kernel/runtime policyHook).
type AskPolicy int

const (
	// AskAllow folds Ask into Allow with WouldAsk=true. Default; lets the
	// MVP build progress without an approver. The journal records every
	// would-have-asked moment so audits remain honest.
	AskAllow AskPolicy = iota
	// AskDeny folds Ask into Deny. Strict mode; only L4 calls pass.
	AskDeny
	// AskPrompt marks Ask-class verdicts with RequiresApproval=true so
	// the runtime can route a real prompt via approval.Registry. Until
	// the operator decides, the tool call must not proceed. Decision is
	// returned as Deny so any caller that ignores RequiresApproval will
	// fail closed.
	AskPrompt
)

// String returns the operator-facing label for an AskPolicy — the same
// vocabulary AGEZT_APPROVAL_MODE accepts and `agt edict show` prints.
func (p AskPolicy) String() string {
	switch p {
	case AskAllow:
		return "allow"
	case AskDeny:
		return "deny"
	case AskPrompt:
		return "prompt"
	default:
		return "unknown"
	}
}

// ParseAskPolicy parses an operator-facing approval-mode string into an
// AskPolicy. Accepts allow/deny/prompt (case-insensitive); unknown input
// is an error, never a silent default — a typo must not quietly flip the
// daemon into a different approval posture.
func ParseAskPolicy(s string) (AskPolicy, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "allow":
		return AskAllow, nil
	case "deny":
		return AskDeny, nil
	case "prompt":
		return AskPrompt, nil
	}
	return 0, fmt.Errorf("edict: unknown approval mode %q (want allow/deny/prompt)", s)
}

// HardDenyRule is a single hard-deny pattern.
type HardDenyRule struct {
	Name      string       // human label, included in the Outcome
	Substring string       // case-insensitive substring match against input
	AppliesTo []Capability // empty = applies to every capability
}

// matches reports whether r fires against the input under the named cap.
func (r HardDenyRule) matches(cap Capability, input string) bool {
	if len(r.AppliesTo) > 0 && !slices.Contains(r.AppliesTo, cap) {
		return false
	}
	return strings.Contains(strings.ToLower(input), strings.ToLower(r.Substring))
}

// denyCandidates returns the set of strings the hard-deny floor should be matched
// against for a tool input. It always includes the raw input (so prior behavior is
// preserved), plus — when the input is JSON — each decoded string VALUE with its
// whitespace collapsed. Decoding defeats JSON-escape evasion (`/` → `/`,
// `rm` → `rm`) and the whitespace collapse defeats padding (`rm  -rf /` →
// `rm -rf /`). Values are matched individually (not concatenated) so adjacent
// fields can't form a spurious match. Non-JSON input contributes its own
// whitespace-collapsed form (M173).
func denyCandidates(input string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	add := func(s string) {
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	add(input)
	// The base strings to derive whitespace-normalised variants from: each
	// decoded string value for JSON input (defeats JSON-escape evasion), else
	// the raw input itself.
	var bases []string
	var v any
	if err := json.Unmarshal([]byte(input), &v); err == nil {
		collectJSONStrings(v, &bases)
	} else {
		bases = []string{input}
	}
	for _, s := range bases {
		// collapsed: padding evasion — `rm  -rf  /` → `rm -rf /` (matches the
		// space-bearing floor rules like `rm -rf /`, `dd if=`).
		add(collapseWhitespace(s))
		// stripped: spacing evasion — `:(){ :|:& };:` → `:(){:|:&};:`. The
		// canonical fork bomb carries spaces that survive collapse but are
		// syntactically optional. Only whitespace ADJACENT TO PUNCTUATION is
		// removed (M426): stripping ALL whitespace collapsed ordinary prose onto an
		// alphabetic floor rule — `re boot the server` → `reboottheserver` matched
		// `reboot`, `mk fs` → `mkfs`, etc. — a permanent, un-overridable false
		// hard-deny. Punctuation-adjacent stripping still normalises the fork bomb
		// (its spaces sit next to `{ | & ;`) without ever merging two words.
		add(stripPunctAdjacentWhitespace(s))
	}
	return out
}

// collectJSONStrings appends every string value reachable in a decoded JSON value
// (objects, arrays, nested) to dst. Keys are ignored — only values carry the
// model-chosen action text.
func collectJSONStrings(v any, dst *[]string) {
	switch t := v.(type) {
	case string:
		*dst = append(*dst, t)
	case []any:
		for _, e := range t {
			collectJSONStrings(e, dst)
		}
	case map[string]any:
		for _, e := range t {
			collectJSONStrings(e, dst)
		}
	}
}

// collapseWhitespace replaces every run of whitespace (spaces, tabs, newlines)
// with a single space and trims the ends, so padded/typeset variants of a command
// normalize to the canonical spacing the floor rules use.
func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// stripPunctAdjacentWhitespace removes a whitespace character only when it borders
// a punctuation character (a non-space, non-alphanumeric rune) on either side, so a
// spacing-variant fork bomb (`:(){ :|:& };:`, whose spaces sit next to `{ | & ;`)
// normalizes to the no-space floor form WITHOUT ever merging two alphanumeric words.
// Stripping ALL whitespace (the previous behaviour) collapsed ordinary prose like
// `re boot the server` onto the `reboot` rule — an un-overridable false hard-deny
// (M426). A space between two alphanumerics is preserved (kept as a single space).
func stripPunctAdjacentWhitespace(s string) string {
	rs := []rune(s)
	isAlnum := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(rs); i++ {
		if !unicode.IsSpace(rs[i]) {
			b.WriteRune(rs[i])
			continue
		}
		var prev, next rune
		for j := i - 1; j >= 0; j-- {
			if !unicode.IsSpace(rs[j]) {
				prev = rs[j]
				break
			}
		}
		for j := i + 1; j < len(rs); j++ {
			if !unicode.IsSpace(rs[j]) {
				next = rs[j]
				break
			}
		}
		prevPunct := prev != 0 && !isAlnum(prev)
		nextPunct := next != 0 && !isAlnum(next)
		if prevPunct || nextPunct {
			continue // drop whitespace bordering punctuation
		}
		b.WriteRune(' ') // keep a single space between alphanumeric words
	}
	return b.String()
}

// Options seed a new Engine.
type Options struct {
	// Levels overrides per-capability defaults. Caps absent here use the
	// values from Defaults (or LevelDeny if absent entirely).
	Levels map[Capability]TrustLevel
	// HardDeny replaces the default hard-deny rule set (use append with
	// DefaultHardDeny to extend rather than replace).
	HardDeny []HardDenyRule
	// AskPolicy chooses how to fold L1..L3. Default: AskAllow.
	AskPolicy AskPolicy
	// UnknownAllow flips the default for a capability with no configured level
	// from default-DENY to allow (M613). Off by default (secure-default: an
	// unknown capability is refused). The daemon sets it under AGEZT_ALLOW_ALL so
	// "allow everything" truly covers capabilities not in DefaultLevels —
	// including ones a future plugin tool introduces — not just the known set.
	// Hard-deny rules still apply first, so the catastrophe rails hold.
	UnknownAllow bool
}

// Engine is the policy decision engine. Safe for concurrent use.
type Engine struct {
	mu           sync.RWMutex
	levels       map[Capability]TrustLevel
	hardDeny     []HardDenyRule
	askPolicy    AskPolicy
	unknownAllow bool // M613: treat unconfigured capabilities as allow, not deny
	rtSeq        int  // monotonic counter naming runtime-added hard-deny rules
}

// RuntimeRulePrefix names rules added at runtime via AddHardDeny. The
// prefix is the load-bearing security invariant of runtime management:
// RemoveHardDeny only removes rules whose name carries it, so the
// boot-time floor (DefaultHardDeny + AGEZT_EDICT_DENY's operator[N]
// rules) can be *tightened* at runtime but never *loosened*. You can add
// a deny without a restart; you cannot delete a kernel/operator deny.
const RuntimeRulePrefix = "runtime["

// IsRuntimeRule reports whether name belongs to a rule added at runtime
// (and is therefore removable via RemoveHardDeny). Built-in and
// AGEZT_EDICT_DENY rules return false — they are the immutable floor.
//
// The match is STRICT: exactly the shape AddHardDeny mints, `runtime[<digits>]`
// (M174). A bare-prefix check (`runtime[…anything…`) would let a crafted name —
// `runtime[`, `runtime[evil`, `runtime[]` — masquerade as removable and, should
// such a name ever reach a floor rule (a refactor, a forged durable event), strip
// it. Since this prefix is the load-bearing "tighten-but-never-loosen" invariant,
// it is validated to the full canonical shape, not just the opening bracket.
func IsRuntimeRule(name string) bool {
	rest, ok := strings.CutPrefix(name, RuntimeRulePrefix)
	if !ok {
		return false
	}
	digits, ok := strings.CutSuffix(rest, "]")
	if !ok || digits == "" {
		return false
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Levels returns a snapshot of the per-capability trust levels.
// Returned map is a copy — mutating it doesn't affect the engine.
// Used by the control plane to power `agt edict show`.
func (e *Engine) Levels() map[Capability]TrustLevel {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make(map[Capability]TrustLevel, len(e.levels))
	maps.Copy(out, e.levels)
	return out
}

// HardDenyRules returns a snapshot of the hard-deny rule set.
// Returned slice is a copy — same rationale as Levels().
func (e *Engine) HardDenyRules() []HardDenyRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]HardDenyRule, len(e.hardDeny))
	copy(out, e.hardDeny)
	return out
}

// AskPolicy returns the configured AskPolicy. Useful for the
// control plane's `agt edict show` so operators can confirm
// whether the daemon is currently in allow/deny/prompt mode.
func (e *Engine) AskPolicy() AskPolicy {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.askPolicy
}

// SetAskPolicy changes the engine-wide approval mode at runtime. The
// hard-deny floor is unaffected (it fires before AskPolicy is consulted),
// so even AskAllow can't relax a hard-deny. Safe for concurrent use.
func (e *Engine) SetAskPolicy(p AskPolicy) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.askPolicy = p
}

// New builds an Engine. Unset Options fall back to DefaultLevels +
// DefaultHardDeny + AskAllow.
func New(opt Options) *Engine {
	defaults := DefaultLevels()
	levels := make(map[Capability]TrustLevel, len(defaults)+len(opt.Levels))
	maps.Copy(levels, defaults)
	maps.Copy(levels, opt.Levels)
	hd := opt.HardDeny
	if hd == nil {
		hd = DefaultHardDeny()
	}
	return &Engine{
		levels:       levels,
		hardDeny:     hd,
		askPolicy:    opt.AskPolicy,
		unknownAllow: opt.UnknownAllow,
	}
}

// DefaultLevels is the MAX-AUTONOMY posture (M814, owner's law: "default
// olarak kapatmadıkça her şeye izni var" — everything is allowed unless the
// operator turns it off). Every capability defaults to LevelAllow;
// restriction is the operator's opt-OUT, applied per capability through the
// Policy center, `agt edict`, AGEZT_EDICT_DENY, or a durable overlay.
//
// What this deliberately does NOT relax (they are guards, not permissions):
//   - the F4 hard-deny strings (fork bombs, rm -rf /, raw-device writes);
//   - the http/browser SSRF guards (loopback/private-net egress);
//   - Governor budget ceilings and per-agent daily caps;
//   - EXPLICIT HITL surfaces the operator wires on purpose (the workflow
//     approval node, the forge promotion queue) — those block on the
//     approval registry regardless of capability levels.
//
// History: the pre-M814 ladder mixed Allow/AskFirst/Ask per DECISIONS F3,
// but the owner ran AskPolicy=AskAllow, which folded every ask to allow in
// practice — this makes the real posture the explicit one. New capabilities
// ship at LevelAllow unless the owner says otherwise.
func DefaultLevels() map[Capability]TrustLevel {
	levels := make(map[Capability]TrustLevel, len(AllCapabilities()))
	for _, c := range AllCapabilities() {
		levels[c] = LevelAllow
	}
	return levels
}

// DefaultHardDeny is the immutable hard-deny set from DECISIONS F4. The
// substrings are case-insensitive and only checked for the specified
// capabilities so they don't false-positive on unrelated tool input.
func DefaultHardDeny() []HardDenyRule {
	return []HardDenyRule{
		{Name: "fork-bomb", Substring: ":(){:|:&};:", AppliesTo: []Capability{CapShell}},
		{Name: "rm-rf-root", Substring: "rm -rf /", AppliesTo: []Capability{CapShell}},
		{Name: "rm-rf-root-flag", Substring: "rm -rf --no-preserve-root", AppliesTo: []Capability{CapShell}},
		{Name: "mkfs", Substring: "mkfs", AppliesTo: []Capability{CapShell}},
		{Name: "wipefs", Substring: "wipefs", AppliesTo: []Capability{CapShell}},    // wipe FS signatures
		{Name: "dd-of-dev", Substring: "dd if=", AppliesTo: []Capability{CapShell}}, // dd reading a source (usual disk-write shape)
		// dd/redirect writing to a RAW BLOCK DEVICE (M175). Keyed on of=/dev/<dev>
		// for the common device families so a `dd of=/dev/sdb` with no `if=` is also
		// caught — while the safe pseudo-devices (/dev/null, /dev/zero, /dev/random)
		// are deliberately NOT matched, so benign `dd of=/dev/null` stays allowed.
		{Name: "dd-of-sd", Substring: "of=/dev/sd", AppliesTo: []Capability{CapShell}},
		{Name: "dd-of-nvme", Substring: "of=/dev/nvme", AppliesTo: []Capability{CapShell}},
		{Name: "dd-of-vd", Substring: "of=/dev/vd", AppliesTo: []Capability{CapShell}},
		{Name: "dd-of-xvd", Substring: "of=/dev/xvd", AppliesTo: []Capability{CapShell}}, // AWS Xen
		{Name: "dd-of-mmcblk", Substring: "of=/dev/mmcblk", AppliesTo: []Capability{CapShell}},
		{Name: "shutdown", Substring: "shutdown -", AppliesTo: []Capability{CapShell}},
		{Name: "poweroff", Substring: "poweroff", AppliesTo: []Capability{CapShell}},
		{Name: "reboot", Substring: "reboot", AppliesTo: []Capability{CapShell}},
		{Name: "powershell-format", Substring: "format-volume", AppliesTo: []Capability{CapShell}},
	}
}

// AllCapabilities returns every governed capability, sorted, for validation and
// operator-facing listings.
func AllCapabilities() []Capability {
	caps := []Capability{
		CapShell, CapFileRead, CapFileWrite, CapFileDelete, CapFileList,
		CapHTTPGet, CapHTTPPost, CapProviderCall, CapDelegate, CapCoding,
		CapACPAgent, CapRemoteRun, CapNotify,
		CapHomeAssistantRead, CapHomeAssistantCall,
		CapBrowserRead, CapMemory, CapWorld, CapWebSearch, CapSchedule, CapRunsRead, CapStanding, CapBoard, CapSkill,
		CapIntrospect, CapCodeExec, CapToolForge, CapMCPInstall, CapMCP, CapConfigRead, CapConfigWrite,
		CapWorkflow,
	}
	slices.Sort(caps)
	return caps
}

// knownCapability reports whether s names a governed capability.
func knownCapability(s string) bool {
	return slices.Contains(AllCapabilities(), Capability(s))
}

// ParseDenyRules parses operator-supplied hard-deny rules from a ';'-separated
// spec, for AGEZT_EDICT_DENY. Each entry is either:
//
//   - "substring"               — denies that substring for EVERY capability
//   - "<capability>:substring"  — denies it only for that capability, when the
//     text before the first ':' is a known capability (e.g. "shell:rm -rf",
//     "http.post:169.254", "file.delete:/etc"). If the prefix is not a known
//     capability the whole entry is treated as an all-capability substring (so
//     "https://evil.example" works verbatim).
//
// A blank substring is rejected — a hard-deny rule matching the empty string
// would deny every action. Returned rules are meant to be appended to
// DefaultHardDeny. Entry order is preserved; rules are named "operator[N]".
func ParseDenyRules(spec string) ([]HardDenyRule, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	var out []HardDenyRule
	for raw := range strings.SplitSeq(spec, ";") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		var applies []Capability
		substr := entry
		if i := strings.IndexByte(entry, ':'); i > 0 {
			if prefix := entry[:i]; knownCapability(prefix) {
				applies = []Capability{Capability(prefix)}
				substr = strings.TrimSpace(entry[i+1:])
			}
		}
		if substr == "" {
			return nil, fmt.Errorf("edict: deny rule %q has an empty substring (would deny everything)", entry)
		}
		out = append(out, HardDenyRule{
			Name:      fmt.Sprintf("operator[%d]", len(out)+1),
			Substring: substr,
			AppliesTo: applies,
		})
	}
	return out, nil
}

// Decide returns the engine's verdict for one (capability, input) pair.
// input is the stringified tool input — the engine treats it as opaque
// text for hard-deny substring matching only. The runtime is responsible
// for journaling the Outcome.
func (e *Engine) Decide(cap Capability, input string) Outcome {
	// No ceiling: LevelAllow (the max) clamps nothing, so behaviour is unchanged.
	return e.DecideWithCeiling(cap, input, LevelAllow)
}

// DecideWithCeiling is Decide with a per-call trust ceiling (SPEC-16 §4
// initiative.max_trust): the looked-up capability level is clamped to at most
// `ceiling` before the level→decision mapping, so a normally auto-allowed (L4)
// capability is downgraded to Ask (or, at ceiling L0, Deny) within a bounded
// context like a standing order. The hard-deny floor and unknown-capability
// default-deny are unaffected — a ceiling can only TIGHTEN, never loosen.
func (e *Engine) DecideWithCeiling(cap Capability, input string, ceiling TrustLevel) Outcome {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// 1. Hard-deny always wins. Match against the decoded, normalized action — not
	// just the raw JSON tool-arg text. The model picks the command string, so it
	// could otherwise evade a floor rule by JSON-escaping a banned token
	// (`{"command":"rm -rf /"}`) or by padding whitespace (`rm  -rf /`); both
	// decode/normalize back to the banned form. denyCandidates returns the raw
	// input (no regression) plus each JSON string value with whitespace collapsed
	// (M173). A rule firing on ANY candidate denies.
	candidates := denyCandidates(input)
	for _, r := range e.hardDeny {
		for _, c := range candidates {
			if r.matches(cap, c) {
				return Outcome{
					Decision:     DecisionDeny,
					Capability:   cap,
					Level:        LevelDeny,
					Reason:       "hard-deny rule matched: " + r.Name,
					HardDenied:   true,
					HardDenyRule: r.Name,
				}
			}
		}
	}

	// 2. Look up the trust level for this capability.
	lvl, ok := e.levels[cap]
	if !ok {
		// Unknown capability. Default-deny is the strict, secure default (the
		// user must explicitly grant a level). But under UnknownAllow (M613, set
		// by AGEZT_ALLOW_ALL) an unconfigured capability is treated as L4 so
		// "allow everything" covers tools whose capability isn't in DefaultLevels
		// — including future plugin tools. Hard-deny already ran above, so the
		// catastrophe rails still hold.
		if !e.unknownAllow {
			return Outcome{
				Decision:   DecisionDeny,
				Capability: cap,
				Level:      LevelDeny,
				Reason:     fmt.Sprintf("no trust level configured for %q (default-deny)", cap),
			}
		}
		lvl = LevelAllow
	}

	// 2b. Clamp to the per-call ceiling (SPEC-16 §4): autonomy within this context
	// can be capped below the capability's configured level. Only ever tightens.
	ceilNote := ""
	if ceiling < lvl {
		lvl = ceiling
		ceilNote = fmt.Sprintf(" (clamped to ceiling %s)", ceiling)
	}

	// 3. Apply level → decision.
	switch lvl {
	case LevelDeny:
		return Outcome{
			Decision:   DecisionDeny,
			Capability: cap,
			Level:      lvl,
			Reason:     "capability set to L0 (deny)" + ceilNote,
		}
	case LevelAllow:
		return Outcome{
			Decision:   DecisionAllow,
			Capability: cap,
			Level:      lvl,
			Reason:     "capability set to L4 (allow)",
		}
	default: // L1..L3 — Ask
		switch e.askPolicy {
		case AskDeny:
			return Outcome{
				Decision:   DecisionDeny,
				Capability: cap,
				Level:      lvl,
				Reason:     fmt.Sprintf("level %s requires approval; AskPolicy=AskDeny", lvl) + ceilNote,
			}
		case AskPrompt:
			return Outcome{
				// Fail-closed default for callers that don't honour
				// RequiresApproval — the runtime overrides this after
				// a real grant.
				Decision:         DecisionDeny,
				Capability:       cap,
				Level:            lvl,
				Reason:           fmt.Sprintf("level %s; AskPolicy=AskPrompt → operator approval required", lvl) + ceilNote,
				WouldAsk:         true,
				RequiresApproval: true,
			}
		default: // AskAllow
			return Outcome{
				Decision:   DecisionAllow,
				Capability: cap,
				Level:      lvl,
				Reason:     fmt.Sprintf("level %s; AskPolicy=AskAllow (would prompt in MVP)", lvl) + ceilNote,
				WouldAsk:   true,
			}
		}
	}
}

// SetLevel changes the trust level for a capability at runtime. Useful
// for the future `agt trust` CLI; safe for concurrent use.
func (e *Engine) SetLevel(cap Capability, lvl TrustLevel) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.levels[cap] = lvl
}

// AddHardDeny appends a hard-deny rule at runtime and returns the stored
// rule (with its engine-assigned name). The caller supplies Substring and
// AppliesTo (e.g. from ParseDenyRules); the Name is always overwritten
// with a fresh "runtime[N]" so the rule is removable via RemoveHardDeny
// and can never be confused with a built-in or operator[N] floor rule.
//
// A blank substring is rejected for the same reason ParseDenyRules rejects
// it: a rule matching the empty string would deny every action. Safe for
// concurrent use; the change takes effect on the next Decide.
func (e *Engine) AddHardDeny(rule HardDenyRule) (HardDenyRule, error) {
	if strings.TrimSpace(rule.Substring) == "" {
		return HardDenyRule{}, fmt.Errorf("edict: hard-deny rule has an empty substring (would deny everything)")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rtSeq++
	rule.Name = fmt.Sprintf("%s%d]", RuntimeRulePrefix, e.rtSeq)
	e.hardDeny = append(e.hardDeny, rule)
	return rule, nil
}

// RemoveHardDeny removes the runtime-added hard-deny rule named name and
// reports whether a rule was removed. It refuses to touch the boot-time
// floor: removing a built-in or operator[N] rule returns an error, never a
// silent success — the floor stays put. Safe for concurrent use.
func (e *Engine) RemoveHardDeny(name string) (bool, error) {
	if !IsRuntimeRule(name) {
		return false, fmt.Errorf("edict: %q is not a runtime-added rule; the boot-time deny floor cannot be removed at runtime", name)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for i, r := range e.hardDeny {
		if r.Name == name {
			e.hardDeny = slices.Delete(e.hardDeny, i, i+1)
			return true, nil
		}
	}
	return false, nil
}

// PolicyChange is one decoded policy.changed event — the input to
// ProjectPolicyChanges. Field names mirror the control plane's
// policy.changed payloads (actions "level.set", "deny.add", "deny.rm"),
// so a journal payload unmarshals straight into this struct.
type PolicyChange struct {
	Action     string   `json:"action"`
	Capability string   `json:"capability"`
	To         string   `json:"to"`
	Name       string   `json:"name"`
	Substring  string   `json:"substring"`
	AppliesTo  []string `json:"applies_to"`
}

// PolicyOverlay is the net effect of a sequence of PolicyChanges: the
// per-capability level overrides plus the surviving runtime deny rules.
// It is what a daemon replays onto a freshly-booted engine to make
// runtime policy changes durable across a restart.
type PolicyOverlay struct {
	Levels    map[Capability]TrustLevel
	DenyRules []HardDenyRule
	// Mode, when non-nil, is the net runtime approval-mode override. A
	// pointer so "no mode change in history" is distinct from AskAllow (0).
	Mode *AskPolicy
}

// IsEmpty reports whether the overlay carries no changes — lets a caller
// skip the apply (and a banner line) when there's nothing to restore.
func (o PolicyOverlay) IsEmpty() bool {
	return len(o.Levels) == 0 && len(o.DenyRules) == 0 && o.Mode == nil
}

// ProjectPolicyChanges folds an ordered sequence of policy.changed events
// into the net overlay. It is pure (no engine, no I/O) so it is trivially
// testable and the daemon's only job is to decode the journal and apply
// the result.
//
// Semantics: level.set is last-wins per capability; deny.add/deny.rm are
// tracked by the rule's journaled name so an add later removed leaves no
// trace, and surviving rules keep their original add order. Malformed
// entries (blank capability/substring, unparseable level) are skipped
// rather than failing the whole replay — one bad historical event must
// not wedge a restart.
func ProjectPolicyChanges(changes []PolicyChange) PolicyOverlay {
	levels := map[Capability]TrustLevel{}
	denyByName := map[string]HardDenyRule{}
	var order []string // surviving deny names, in add order
	var mode *AskPolicy
	for _, ch := range changes {
		switch ch.Action {
		case "mode.set":
			if p, err := ParseAskPolicy(ch.To); err == nil {
				mode = &p // last-wins
			}
		case "level.set":
			if ch.Capability == "" {
				continue
			}
			lvl, err := ParseTrustLevel(ch.To)
			if err != nil {
				continue
			}
			levels[Capability(ch.Capability)] = lvl
		case "deny.add":
			if ch.Name == "" || strings.TrimSpace(ch.Substring) == "" {
				continue
			}
			var caps []Capability
			for _, c := range ch.AppliesTo {
				if c != "" {
					caps = append(caps, Capability(c))
				}
			}
			if _, seen := denyByName[ch.Name]; !seen {
				order = append(order, ch.Name)
			}
			denyByName[ch.Name] = HardDenyRule{Name: ch.Name, Substring: ch.Substring, AppliesTo: caps}
		case "deny.rm":
			if _, ok := denyByName[ch.Name]; ok {
				delete(denyByName, ch.Name)
				for i, n := range order {
					if n == ch.Name {
						order = slices.Delete(order, i, i+1)
						break
					}
				}
			}
		}
	}
	overlay := PolicyOverlay{Mode: mode}
	if len(levels) > 0 {
		overlay.Levels = levels
	}
	for _, n := range order {
		overlay.DenyRules = append(overlay.DenyRules, denyByName[n])
	}
	return overlay
}

// ApplyOverlay applies a projected overlay onto the engine: level
// overrides via SetLevel, surviving runtime deny rules via AddHardDeny
// (each re-assigned a fresh runtime[N] name). Returns the counts applied,
// for an operator banner. Used by the daemon when AGEZT_EDICT_DURABLE is
// on to restore runtime policy from the journal.
func (e *Engine) ApplyOverlay(o PolicyOverlay) (levels, rules int) {
	if o.Mode != nil {
		e.SetAskPolicy(*o.Mode)
	}
	for cap, lvl := range o.Levels {
		e.SetLevel(cap, lvl)
		levels++
	}
	for _, r := range o.DenyRules {
		if _, err := e.AddHardDeny(r); err == nil {
			rules++
		}
	}
	return levels, rules
}

// Level returns the current trust level for a capability, and a bool
// indicating whether it was explicitly configured (vs. default-deny).
func (e *Engine) Level(cap Capability) (TrustLevel, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	lvl, ok := e.levels[cap]
	return lvl, ok
}

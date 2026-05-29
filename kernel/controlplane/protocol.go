// SPDX-License-Identifier: MIT

// Package controlplane is the local control protocol between the agezt
// daemon and the agt CLI.
//
// Transport: TCP localhost, line-delimited JSON. The daemon binds to an
// ephemeral port; the client discovers it via two files under
// <BaseDir>/runtime/:
//
//	control.addr    text: "127.0.0.1:NNNNN\n"
//	control.token   text: hex token; clients must send it on every request
//
// Wire format — every line is one JSON object.
//
//	Request : {"id":"q1","cmd":"<name>","token":"<hex>","args":{...}}
//	Response: {"id":"q1","type":"event",  "event":{...}}        zero or more
//	Response: {"id":"q1","type":"result", "result":{...}}       exactly one
//	Response: {"id":"q1","type":"error",  "error":"reason"}     alternative final
//
// One request per connection: open, send, read until type=result|error,
// close. This keeps the protocol trivial and avoids multiplexing concerns.
//
// (Not JSON-RPC: that's the contract for the kernel↔plugin wire per
// DECISIONS B0. The control plane is process-local and uses a thinner,
// purpose-built shape so the CLI binary stays small.)
package controlplane

import "github.com/agezt/agezt/kernel/event"

// Command names supported by the control plane.
const (
	CmdVersion       = "version"
	CmdRun           = "run"
	CmdHalt          = "halt"
	CmdResume        = "resume"
	CmdWhy           = "why"
	CmdJournalVerify = "journal_verify"
	// HITL approval queue.
	CmdApprovals = "approvals" // list pending requests
	CmdDecide    = "decide"    // resolve one (args: id, decision="grant|deny", reason)
	// DAG scheduler.
	CmdPlan = "plan" // run a pre-built Plan (args: plan_json — see scheduler/PlanSpec)
	// Provider / model catalog (SPEC-15 §1; TASKS P1-CONDUIT-04).
	CmdCatalogSync     = "catalog_sync"     // (args: url? — defaults to AGEZT_CATALOG_URL or models.dev)
	CmdCatalogList     = "catalog_list"     // returns providers + models + pricing + credentials present
	CmdCatalogDiscover = "catalog_discover" // (args: endpoint? — Ollama /api/tags discovery)
	// Live reload of catalog + vault → rebuild primary provider in place
	// (M1.r). Replaces the "restart the daemon" friction printed by
	// `agt provider creds set` since M1.o.
	CmdProviderReload = "provider_reload"
	// Planner: ask the daemon's configured Provider to generate a
	// scheduler-shaped Plan JSON from a natural-language intent (M1.v).
	// Args: intent (string), optional model (string).
	// Returns: { "plan_json": "<the JSON string>", "node_count": N }.
	// The CLI can then forward plan_json to CmdPlan to execute, or
	// just print it for the operator to review.
	CmdPlanGenerate = "plan_generate"
	// Planner refinement (M1.uu): operator-driven re-plan with
	// feedback. Takes an existing plan JSON + free-text feedback,
	// returns a complete replacement plan in the same shape as
	// CmdPlanGenerate. Refinement is whole-replacement (not diff)
	// so re-validation catches any LLM mistakes the same way the
	// initial plan validators do.
	// Args: plan_json (string), feedback (string), optional model.
	// Returns: { "plan_json": "<new JSON>", "node_count": N }.
	CmdPlanRefine = "plan_refine"
	// Pulse: live operator observability (M1.u). Long-lived
	// subscription that streams bus events to the client until either
	// side closes the connection. Args: pattern (default ">"),
	// optional kinds filter ([]string). Never sends RespResult — the
	// client terminates the stream by closing the conn.
	CmdPulseSubscribe = "pulse_subscribe"

	// CmdBudget returns the governor's current spend snapshot.
	// Closes the M1.zz feedback loop: operators set per-task-type
	// daily caps but had no way to see how close they were to
	// hitting them. Returns:
	//   - utc_date         (string YYYY-MM-DD) — the day the
	//                      counters are scoped to.
	//   - spent_mc         (int)    — total spend today, microcents.
	//   - ceiling_mc       (int)    — daily ceiling (0 = unlimited).
	//   - per_task         (map[string]{spent_mc, ceiling_mc}) —
	//                      one entry per configured TaskBudget;
	//                      empty when no per-task caps configured.
	// No args.
	CmdBudget = "budget"

	// CmdToolList returns the in-process tool inventory the agent
	// loop will advertise to the model. Sister command to
	// CmdCatalogList (providers) — operators frequently want to
	// confirm a plugin actually registered its tool before
	// debugging "the model never called my tool" issues.
	// No args. Returns:
	//   - tools: [{name, description}, ...] sorted by name.
	//   - count: int
	// Source-of-tool ("in-process" vs "plugin: <name>") is not
	// distinguished at the kernel boundary — tools all satisfy
	// the same agent.Tool interface by the time they reach the
	// loop. The CLI surfaces the description so plugin tools that
	// follow the convention of prefixing their description ("[via
	// plugin foo] ...") remain self-identifying.
	CmdToolList = "tool_list"

	// CmdStatus is a one-shot daemon health overview. Existed as
	// scattered fields across CmdVersion + CmdBudget + CmdToolList;
	// this consolidates the operator-friendly "is my daemon
	// healthy, and what's it doing?" check into a single round-trip.
	// Also surfaces daemon version so the CLI can detect client/
	// daemon skew (agt version was previously client-only).
	// No args. Returns:
	//   - daemon         (string) — daemon binary version
	//   - protocol       (int)    — protocol version
	//   - uptime_seconds (int)    — seconds since Open()
	//   - halted         (bool)   — kernel halt flag
	//   - active_runs    (int)    — in-flight Run/RunPlan count
	//   - tools          (int)    — registered tool count
	//   - journal_head   (int)    — last journaled seq (0 = empty)
	CmdStatus = "status"

	// CmdPluginList enumerates the external plugins the daemon
	// spawned at startup. Sister to CmdToolList (which sees
	// tools by name but not the plugin they came from); operators
	// debugging "I configured plugin X but its tools aren't
	// available" use this to confirm the spawn actually happened
	// before chasing tool-registration issues.
	// No args. Returns:
	//   - plugins : [{prefix, path, args, tool_count,
	//                hash_pinned, allowed_tools}, ...]
	//             sorted by prefix.
	//   - count   : int
	CmdPluginList = "plugin_list"

	// CmdShutdown asks the daemon to exit gracefully. Same effect as
	// SIGTERM but reachable from any host that holds a valid control-
	// plane token — the gap that motivates this command is scripted
	// / CI workflows that need to stop the daemon without a shell on
	// the host (`pkill agezt` doesn't compose well in CI YAML; this
	// does). Handler writes `{ok:true}` first, then signals the
	// daemon's main loop to unblock and shut down after a short
	// delay so the client read completes before the process exits.
	// No args.
	CmdShutdown = "shutdown"

	// CmdJournalTail returns the last N events from the journal as a
	// one-shot historical read. Different from CmdPulseSubscribe with
	// --until: this never starts a subscription, never blocks, and
	// streams nothing — it's a synchronous snapshot for "show me what
	// just happened" use cases (postmortems, smoke tests, scrollback).
	// Args:
	//   - n : int (optional; default 20, clamped to 1..10000)
	// Returns:
	//   - events : [*event.Event, ...] in seq order, oldest→newest
	//   - count  : int — actual number returned (may be < n if the
	//              journal is shorter)
	//   - head   : int — current journal head seq (so the operator
	//              can compute "we showed events seq=(head-count+1)..head")
	CmdJournalTail = "journal_tail"
)

// Request is the wire shape sent by the client.
type Request struct {
	ID    string         `json:"id"`
	Cmd   string         `json:"cmd"`
	Token string         `json:"token"`
	Args  map[string]any `json:"args,omitempty"`
}

// Response types.
const (
	RespEvent  = "event"
	RespResult = "result"
	RespError  = "error"
)

// Response is the wire shape sent by the server. Exactly one of Event,
// Result, or Error is populated depending on Type.
type Response struct {
	ID     string         `json:"id"`
	Type   string         `json:"type"`
	Event  *event.Event   `json:"event,omitempty"`
	Result map[string]any `json:"result,omitempty"`
	Error  string         `json:"error,omitempty"`
}

// File names under <BaseDir>/runtime/.
const (
	addrFile  = "control.addr"
	tokenFile = "control.token"
)

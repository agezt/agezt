// SPDX-License-Identifier: MIT

package opapi

// Provenance: the Policy, state and artifact half of the control plane's command vocabulary,
// moved here from kernel/controlplane/protocol_commands_commands_edict.go when the
// operation vocabulary got one home (W3.79). The values are unchanged; the
// identifiers lost the `Cmd` prefix that named the transport's own alias.
//
// Every name below is declared exactly once, in this package. The application
// layer binds these constants into the operation's spec, and the transports
// refer to them by these names. A name that can be written in two places is a
// name that can drift, and a name that drifts is a route the daemon no longer
// answers to.

const (
	EdictShow = "edict_show"
	// EdictOverlay returns the NET durable policy overlay (M94) — every
	// runtime policy.changed folded via ProjectPolicyChanges (the boot replay).
	// No args. Returns: { levels: {cap: level}, deny_rules: [...], mode,
	// empty, changes_folded }
	EdictOverlay = "edict_overlay"
	// EdictCompact collapses the durable policy overlay into a snapshot (M95)
	// so boot replays {snapshot + post-snapshot changes} instead of all history.
	// No args. Returns: { folded, compacted, through_seq, empty }
	EdictCompact = "edict_compact"
	// EdictTest dry-runs a policy decision: "if I asked to do
	// <capability> with this <input>, what would the engine say?"
	// Read-only — does not register the call as having happened,
	// does not consume approval slots, does not journal as a
	// real decision event. Useful for:
	//   - CI preflight: assert "rm -rf /" is hard-denied before
	//     running an agent loop that might receive it.
	//   - Debugging: "why was this allowed but the next one
	//     denied?" — operators can probe variations interactively.
	//   - Composing hard-deny rules: confirm a new pattern
	//     actually catches the inputs the operator expects.
	// Args:
	//   - capability : string (required) — must match a known
	//                  edict.Capability value (shell, file_read, ...)
	//   - input      : string (optional, default "") — the input
	//                  text the runtime would be checking against
	// Returns the engine.Outcome shape:
	//   - decision         : "allow" | "deny"
	//   - level            : string — TrustLevel.String() (L0..L4)
	//   - reason           : string — human explanation from engine
	//   - hard_denied      : bool
	//   - hard_deny_rule   : string — present iff hard_denied=true
	//   - would_ask        : bool — Ask-class folded by AskPolicy
	//   - requires_approval: bool — AskPrompt mode + Ask-class hit
	EdictTest = "edict_test"
	// EdictDenyList enumerates the hard-deny rules currently loaded,
	// each tagged with whether it is removable at runtime. Same rows as
	// EdictShow's hard_deny block, plus a `removable` flag: built-in
	// and AGEZT_EDICT_DENY (operator[N]) rules are the immutable floor;
	// only runtime[N] rules added via EdictDenyAdd can be removed.
	// No args. Returns:
	//   - rules : [{name, substring, applies_to, removable}, ...]
	EdictDenyList = "edict_deny_list"
	// EdictDenyAdd appends a hard-deny rule at runtime — no restart.
	// The deny floor is security-critical, so the change is journaled as
	// a policy.changed event. Args:
	//   - rule : string (required) — one rule in AGEZT_EDICT_DENY syntax,
	//            "substring" (all caps) or "<capability>:substring"
	//            (scoped). Must parse to exactly one rule.
	// Returns: {name, substring, applies_to, count} — name is the
	// engine-assigned "runtime[N]" handle for a later remove.
	EdictDenyAdd = "edict_deny_add"
	// EdictDenyRemove removes a runtime-added hard-deny rule by name
	// and journals a policy.changed event. Refuses to remove a built-in
	// or operator[N] floor rule (error, not a silent no-op). Args:
	//   - name : string (required) — the "runtime[N]" handle.
	// Returns: {removed: bool, count}.
	EdictDenyRemove = "edict_deny_rm"
	// EdictSetLevel changes a capability's trust level at runtime — no
	// restart. The companion to CmdEdictDeny* for the other policy layer:
	// the trust ladder (L0 deny .. L4 allow). Loosening is safe by
	// construction — the hard-deny floor still fires regardless of level,
	// so even shell=L4 cannot pass `rm -rf /`. The change is journaled as
	// a policy.changed event. Args:
	//   - capability : string (required) — must be a known capability
	//                  (shell, file.read, http.post, ...). Unknown is an
	//                  error, never a silent default-deny entry.
	//   - level      : string (required) — "L0".."L4" or a word alias
	//                  (deny/ask/askfirst/askscoped/allow).
	// Returns: {capability, from, to} — the previous and new level labels.
	EdictSetLevel = "edict_set_level"
	// EdictSetMode changes the engine-wide approval mode (how Ask-class
	// levels L1..L3 are folded) at runtime — no restart. The third runtime
	// policy knob alongside CmdEdictDeny* and EdictSetLevel. The hard-deny
	// floor is unaffected (it fires before AskPolicy), so even `allow` mode
	// can't relax a hard-deny. Journaled as a policy.changed event. Args:
	//   - mode : string (required) — "allow" | "deny" | "prompt".
	// Returns: {from, to} — the previous and new mode labels.
	EdictSetMode = "edict_set_mode"
	// EdictLog lists recent policy decisions (M63) — a read-only audit of the
	// journal's policy.decision events (every tool-call gating). Args: limit
	// (optional), denied (optional bool — only denials). Returns: { decisions:
	// [ {ts_unix_ms, actor, correlation_id, tool, capability, allow, reason,
	// hard_denied} ], count }
	EdictLog = "edict_log"
	// EdictStats aggregates policy decisions (M64) — the security-dashboard
	// analogue of RunsStats. Args: since_ms (optional window). Returns:
	//   { total, allowed, denied, hard_denied, denial_rate,
	//     denied_by_capability: {cap → count}, window_ms }
	EdictStats = "edict_stats"
	// ToolLog lists recent tool invocations (M66) — a read-only audit of the
	// journal's tool.invoked + tool.result events (what the agent actually ran).
	// The execution analogue of EdictLog (which audits the policy gating of
	// those same calls). Args: limit (optional), errors (optional bool — only
	// failed calls), tool (optional name filter), since_ms (optional window).
	// Returns: { invocations: [ {ts_unix_ms, actor, correlation_id, tool,
	// call_id, input, output, error} ], count }
	ToolLog = "tool_log"
	// ToolStats aggregates tool invocations (M67) — the execution-dashboard
	// analogue of EdictStats. Args: tool (optional name filter), since_ms
	// (optional window). Returns: { total, errored, error_rate, by_tool: {tool →
	// {calls, errors}}, tools, window_ms }
	ToolStats = "tool_stats"
	// ExecutionProfiles returns the named execution-profile inventory:
	// local, warden, worktree-coding, browser-session, docker, ssh, and
	// remote-agezt, including requested vs effective isolation and routed tools.
	ExecutionProfiles = "execution_profiles"
	// ExecutionProfileShow returns one execution profile by id. Args: id.
	ExecutionProfileShow = "execution_profile_show"
	// ExecutionProfileCheck returns operator-facing health checks for execution
	// profiles: routing status, downgrade warnings, and docker/ssh backend probes.
	ExecutionProfileCheck = "execution_profile_check"
	// CacheStats aggregates prompt-cache usage + savings (M293) by folding
	// budget.consumed events. Args: since_ms (optional window). Returns:
	// { cached_input_tokens, cache_write_input_tokens, saved_microcents, calls,
	// window_ms } where saved_microcents is the difference between the no-cache
	// baseline (every input token at the full input rate) and the recorded
	// cache-aware cost, summed per call.
	CacheStats = "cache_stats"
	// StateList enumerates namespaces and (optionally) keys in
	// the kernel state store, which is normally invisible to
	// operators — agents and the scheduler write here but there's
	// no CLI path to read what's accumulated. Closing this gap
	// matters for debugging "why did the agent loop think X?" and
	// for postmortems on long-running runs.
	// Args:
	//   - namespace : string (optional) — if set, returns keys in
	//                 that namespace; otherwise returns the
	//                 sorted namespace list.
	// Returns:
	//   - namespaces : []string  (when namespace arg empty)
	//   - keys       : []string  (when namespace arg set)
	//   - namespace  : string    (echoed for context)
	StateList = "state_list"
	// StateGet reads a single (namespace, key) entry. Returns
	// the raw JSON value verbatim so jq pipelines can navigate it.
	// Args:
	//   - namespace : string (required)
	//   - key       : string (required)
	// Returns:
	//   - value : json.RawMessage — the stored value (any JSON shape)
	//   - found : bool — false when (ns, key) doesn't exist; value is null
	StateGet = "state_get"
	// JournalHead returns just the current journal head seq +
	// hash. The minimal-payload sibling of JournalTail (which
	// includes events). Useful when an operator just needs to
	// remember a checkpoint to pass as `pulse --since <seq>`
	// later, or to poll for journal growth in a tight loop
	// without parsing every event.
	//
	// No args. Returns:
	//   - head  : int    — current head seq (0 on empty journal)
	//   - hash  : string — current chain-tail hash, 64-hex. On an
	//                      empty journal this is the 64-zero
	//                      genesis (which is what any first
	//                      event will use as prev_hash).
	JournalHead = "journal_head"
	// JournalExport returns a complete, integrity-attested slice of
	// the journal for archival / compliance / disaster-recovery (M101).
	// Unlike JournalTail (count-windowed) and JournalGrep
	// (predicate-filtered for triage), export streams EVERY event —
	// optionally only those at/after a since_ms cutoff — with its hash
	// and prev_hash intact, plus the chain head at export time, so the
	// resulting bundle can be re-verified OFFLINE via
	// `agt journal verify --bundle <file>` (recompute each event's
	// BLAKE3 hash + check prev-hash continuity).
	//
	// Args:
	//   - since_ms : int (optional) — only events with ts >= now-since_ms.
	// Returns:
	//   - events    : []event — full events, ascending seq.
	//   - count     : int     — len(events).
	//   - first_seq / last_seq : int — seq bounds of the slice (-1 if empty).
	//   - head_seq / head_hash : int / string — chain head at export time.
	//   - truncated : bool    — true if the export hit the size cap.
	JournalExport = "journal_export"
	// ArtifactGet fetches a content-addressed artifact (SPEC-04 §3.6) — the
	// full bytes of a tool output the agent loop offloaded out of the journal
	// (the tool.result event carries a raw_ref). The store re-verifies the bytes
	// against the ref on read, so a corrupted blob is rejected.
	//
	// Args:
	//   - ref : string — the 64-hex BLAKE3 content address (from a raw_ref).
	// Returns:
	//   - ref   : string — echoed.
	//   - size  : int    — byte length.
	//   - data  : string — base64-encoded bytes.
	ArtifactGet = "artifact_get"
	// ArtifactList lists the artifact INDEX entries (M822) — browsable metadata
	// for stored artifacts (inbound images, tool outputs), newest first. No bytes.
	// Args (all optional, exact-match filters): kind, source, corr.
	// Returns: entries — []object{id,ref,name,mime,kind,source,sender,corr,size,created_ms,caption}.
	ArtifactList = "artifact_list"
	// ArtifactDelete removes an artifact index entry by id (M822); the blob is
	// garbage-collected when no other entry references it.
	// Args: id : string. Returns: deleted : bool.
	ArtifactDelete = "artifact_delete"
	// ArtifactCollect reaps STALE artifacts older than a threshold (M845) — the
	// dead-file collector. With dry_run (default) it only reports the candidates;
	// without it, they are deleted (blobs GC'd when unreferenced).
	// Args: older_than_days : number (default 30); dry_run : bool (default true).
	// Returns: dry_run, count, bytes, cutoff_ms, candidates ([]entry when dry_run).
	ArtifactCollect = "artifact_collect"
	// Personal Data Lake (M836) — the Web UI Data view + CLI read/manage the
	// agent-built structured collections (kernel/modules/artifacts/datalake, M834/M835).
	//
	// DataCollections lists every collection with its schema + record count.
	// Returns: collections — []object{name,title,icon,view,desc,fields,builtin,system,count,created_ms,created_by}.
	DataCollections = "data_collections"
	// DataRecords queries one collection's records.
	// Args: collection (req), search, sort, desc, limit, offset. Returns: records, count, schema.
	DataRecords = "data_records"
	// DataInsert adds a record. Args: collection, record(object). Returns: record.
	DataInsert = "data_insert"
	// DataUpdate merges fields into a record. Args: collection, id, record(object). Returns: record.
	DataUpdate = "data_update"
	// DataDelete removes a record. Args: collection, id. Returns: deleted, id.
	DataDelete = "data_delete"
	// DataCreateCollection creates a user collection. Args: collection(object schema). Returns: collection.
	DataCreateCollection = "data_create_collection"
	// DataDropCollection drops a non-system collection. Args: name. Returns: dropped.
	DataDropCollection = "data_drop_collection"
)

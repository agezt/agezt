# W2.50 typed impact cascade

The teardown **impact analysis** — what removing or retiring an agent would
touch — now derives its labels in the application layer, in
`kernel/app/roster/impact_cascade.go` beside the impact service that presents
them. The control plane keeps only the host's data access.

One native file is gone: `kernel/controlplane/roster_lifecycle_helpers.go`
(121 lines).

## What moved

The typed `agent_impact` and `agent_tombstone` operations were already in the
application layer (`kernel/app/roster/impact.go`): they own the preview's
shape and the sub-agent aggregation. What was still native was the
**derivation** behind the holdings — which labels belong to the agent, how a
mailbox message reads from its point of view, whether a workflow node
references it, and which agents are its sub-agents.

`kernel/app/roster/impact_cascade.go` (341 lines) owns that now:

```go
func ScheduleImpact(ports ImpactPorts, p core.Profile) []string
func MemoryImpact(ports ImpactPorts, p core.Profile) []string
func AuthoredSharedMemoryImpact(ports ImpactPorts, p core.Profile) []string
func SkillImpact(ports ImpactPorts, p core.Profile) []string
func ConfigImpact(ports ImpactPorts, p core.Profile) []string
func WorkspaceImpact(ports ImpactPorts, p core.Profile) []string
func WorkflowImpact(ports ImpactPorts, p core.Profile) []string
func MailboxImpact(ports ImpactPorts, p core.Profile) []string
func RemovalMailboxImpact(ports ImpactPorts, slug string, subagents []core.Profile, includeSubagents bool) []string
func Subagents(profiles []core.Profile, slug string) []core.Profile
```

plus the predicates they apply:

- `MailboxImpactLabel` — the topic and the direction (sent, received,
  broadcast, acked) with the message id, falling back to the timestamp;
- `WorkflowNodeConfigReferencesAgent` — the config walker that looks for the
  agent under the agent-reference keys (`agent`, `agent_slug`,
  `target_agent`, `owner_agent`, `parent_agent`, `delegate_to`,
  `source_agent`, `root_agent`), case-insensitively;
- `MemoryRecordLabel` — the record's subject, falling back to its id.

The host binds the stores:

```go
type ImpactPorts struct {
    Schedules     func() []cadence.Entry
    MemoryRecords func() ([]memory.Record, error)
    Skills        func() ([]skill.Skill, error)
    ConfigEntries func() []*configcenter.ConfigEntry
    WorkspaceInfo func(p core.Profile) (string, bool)
    Workflows     func() []workflow.Workflow
    Mailbox       func() []board.Message
}
```

The board's read limit stays in the host: the application asks for the
messages, the host reads them with the limit it uses everywhere else.

## The sub-agent tree

`agentSubagents` walked the roster in the control plane: the agents whose
owner or parent is the agent, and their own sub-agents, sorted by slug, with
the agent itself and any cycle excluded. The walk is now the application's
`Subagents`, over the roster the host lists.

## The semantics are unchanged

The derivation is moved verbatim:

- the schedules, the memory records, the skills (not archived), the config
  entries (with their rating) and the workflow nodes (with their label and
  type) that belong to the agent, each sorted;
- the mailbox messages the agent sent, received, was copied on by a broadcast
  or acknowledged;
- the removal payload's retained-message list, deduplicated by label — a
  message the agent sent and a message it received stay apart, because the
  direction is part of the label;
- the sub-agent tree, sorted by slug.

## The wire shape is unchanged

The impact preview's shape, its sections and the tombstone's footprint are
untouched: the application already owned the presentation, and the derivation
still answers the same questions with the same rules. The existing end-to-end
suite — which drives the real operation over a real kernel — passes unchanged.

## Runnable evidence

Permanent tests cover, at the application layer:

- the schedule labels (the intent and the id, and the id fallback);
- the memory labels (the scoped records, the subject and the id fallback);
- the shared memory labels (the unscoped authored records only);
- the skill labels (the agent's active skills, and the id fallback);
- the config labels (the agent's entry with its rating);
- the workspace (the workspace, and nothing when there is none);
- the workflow labels (the referencing node, with the workflow, the node, the
  label and the type);
- the mailbox labels (the four directions and the id fallback, and no label
  for another agent's message);
- the removal list (the deduplication and the sub-agent switch);
- the workflow config walker (ten cases: the agent-reference keys, the
  nesting, the case-insensitivity, a wrong agent, a key that cannot name an
  agent, an empty and a malformed config);
- the sub-agent tree (the owner and the parent links, the recursion, the
  sorting and the empty root).

At the control plane, the existing roster suite passes unchanged.

The mutations fail tests: another agent's schedules listed, the intent not
used as the label, the scope falling back to nothing, another agent's memory
listed, the subject not used as the label, the scoped records listed as
shared, the archived skills listed, the name not used as the label, another
agent's config entries listed, the rating dropped, the node label dropped,
the workflow reference ignored, another agent's messages listed, the sent
direction dropped, the acked direction dropped, the id fallback dropped, the
removal list not deduplicated, a key that cannot name an agent accepted, the
case-sensitive reference match, the sub-agent tree not walked, the parent
link not treated as a manager link and the tree not sorted. 22 mutations, 22
caught.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (268 packages, 129 allowlisted import violations and 13
allowlisted call sites) and `tools/structure-md`.

## Not done here

The workspace root helper (`roster_workspace.go`) is still native, and the
roster domain's remaining native bodies are the host bindings themselves.
This is the last of the roster domain's derived logic: what remains native
is data access and the host's own helpers.

The cloned-kernel harness of the earlier slices — the pre-slice handler and the
registered operation on two kernels, compared step by step — is not part of
this slice, as in W2.44b through W2.49. The roster's own end-to-end suite is
the parity proof here: it drives the real operation over a real kernel and
reads the journal back.

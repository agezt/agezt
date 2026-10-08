# W2.30k typed agent_add and agent_edit

Creating and editing roster profiles now runs on the shared dispatcher.
`approster.ProfileWriteService` works over four native ports — roster lookup, the
kernel's journaled `AddProfile` and `UpdateProfile`, and list cache invalidation —
and `ProfileWriteOperations` declares two primary-only specs that are not read-only
(`POST /api/agents/add` and `POST /api/agents/edit`, matching the Web UI write
routes), with unknown input allowed. Shared dispatch therefore requires a successful
operation audit admission before either write. The pure profile helpers moved with
them: `ApplyMutableProfilePatch`, `NormalizeProfileKind` and
`ValidateHierarchyRefs` (now taking a lookup port). The native handlers, their
manual `commandSpec` rows and the emptied `roster_crud.go` are removed;
`validateAgentHierarchyRefs` remains as a thin native wrapper for the lifecycle and
agent-tool callers. Both native wire entries are `AppOwned`, not `ReadOnly`, not
tenant-allowed or routed.

## Preserved behavior

Add requires `profile` (`args.profile required` when absent; a JSON null still
decodes to an empty profile and reaches the store's own validation), reports decode
failures as `args.profile: <decoder error>` with the same type names, maps
`kind: "subagent"` (trimmed, any case) to a managed profile, always clears `System`
(kernel-owned), validates owner/parent references and passes store errors through.
Edit requires `ref` and `profile`, applies only the top-level keys the caller sent
(so omitted fields and non-mutable fields such as slug, system and enabled stay),
treats `kind: "subagent"` as an explicit `direct_callable: false`, reports an
unknown agent before validating the patched candidate, and reports a store miss the
same way. Both return the legacy profile view and invalidate the list cache only
after success. Native and shared audits journal the same records, including
argument redaction.

One deliberate change: `ValidateHierarchyRefs` checks `owner_agent` before
`parent_agent`. The native helper ranged over a Go map, so a profile with two bad
references reported a randomly chosen one; it now always reports the owner first.
The other intended difference is the shared already-canceled admission, which now
rejects before audit and before any write (the legacy path ignored the context).

## Runnable comparison and regression evidence

The cloned-kernel harness from W2.30j runs the pre-slice handlers and helpers on one
copy of a closed base kernel and the registered operations on another, comparing raw
socket responses and the complete journal delta after normalizing only times,
durations, correlation ids and freshly generated profile/task ids and creation
times. 114 steps repeated twenty times cover eighteen sequences — add then edit,
missing/null/typed/invalid profiles, duplicate slugs, retired and self references,
unknown references with secret-named arguments, missing/typed refs, array profiles,
unknown agents, retired parents, kind changes, trimmed execution profiles, config
overrides and padded refs — under primary, wrong and tenant tokens and normal and
canceled contexts. All responses and journals are equal except the 19 primary-token
canceled steps, which return the canceled admission error and leave the shared
journal empty. Two-bad-reference profiles are excluded there because the legacy
message is random; the permanent tests pin the deterministic order.

Permanent tests pin every add/edit error and that failures neither write nor
invalidate, kind mapping, `System` clearing, the legacy profile wire and number
rounding, provided-key patch semantics including non-mutable fields, store misses
and errors, each of the twenty-four mutable fields copying exactly its own value
(and none when not provided), the validation order, spec metadata, audit admission
failure blocking both writes, non-primary and canceled admission, audited success
and failure records, and the native wire entries. Twenty-seven independent
mutations — including the native validation wrapper and registration — fail tests,
and sources are restored byte-for-byte. Fixtures use isolated temporary kernels and
do not touch the owner's home, wake agents, run a provider or send channel messages.

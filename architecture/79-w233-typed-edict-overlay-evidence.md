# W2.33d typed edict overlay and compaction

The last two edict operations now run on the shared dispatcher in
`kernel/app/edict`:

- `edict_overlay` shows the net runtime policy in effect: every journaled
  `policy.changed` folded by the same `edict.ProjectPolicyChanges` the daemon
  replays at boot.
- `edict_compact` writes that overlay's minimal change list as the boot
  snapshot, through the current journal head. It then journals the snapshot's
  content hash as `policy.compacted`, so a boot trusts only a snapshot the
  journal vouches for.

`Overlay` works over three ports:

- an `OverlayJournal` (`Range` and `Head`);
- a `SaveSnapshot` function, which writes the routed kernel's
  `runtime/edict_overlay_snapshot.json`;
- the shared `Publish` function.

All three are bound to the kernel the dispatcher routed to.

`OverlayOperations` declares two caller-tenant-routed specs with unknown input
allowed and no Web UI route. Both stay non-read-only, so every call is audited,
as before:

- the overlay view is tenant-owned;
- compaction is primary-only, since it rewrites durable boot state. An operator
  may still name any tenant.

`bind` now delegates to `bindAs`, which takes the authorization. `edict_overlay.go`
is removed. The whole edict family is typed.

## Preserved behavior

- Neither operation reads arguments. The tenant only routes, following the
  trimmed caller tenant.
- Both folds skip a payload that does not decode, exactly as the boot replay
  does. Other kinds are ignored.
- The overlay reports:
  - levels;
  - deny rules in fold order, with `applies_to` `[]` for a global rule;
  - the mode override, or `""` when there is none;
  - the empty flag and the number of changes folded.

  Empty levels and rules are `{}` and `[]`.
- Compaction reports folded, compacted, `through_seq` and empty. A failed
  snapshot write journals nothing.

The shared already-canceled admission rejects an authorized call before any
audit, snapshot or journal record. This is intended: legacy compaction ignored
the context. The routed audit record now carries the operator-selected tenant
label, the W2.1 refinement.

## Runnable comparison and regression evidence

The fixture journals a policy history of every action into the primary and
`acme` kernels:

- level, deny add/remove and mode changes;
- a removed and a surviving rule;
- scoped and global rules;
- a mistyped field, an unknown verb, an array payload, and a `policy.decision`
  that looks like a mode change.

The harness clones the fixture and runs the pre-slice handlers on one copy and
the registered operations on another.

102 steps, repeated twenty times, cover five sequences under primary, wrong and
tenant tokens, in normal and canceled contexts:

- repeated overlay and compaction;
- tenant-routed and padded tenants;
- tenant-token compaction (refused);
- foreign, `null`, non-string, blank and invalid tenants;
- secret-named arguments.

The comparison checks three things:

- responses are byte-exact;
- both kernels' journals, grouped by correlation, are equal: the operation audit
  records and `policy.compacted`;
- both kernels' snapshot files are equal byte-for-byte.

The harness asserts the fold, the mode and the surviving rule. It also asserts
the compaction, the refusal of tenant-token compaction, and an operator-named
compaction that lands on the tenant kernel only.

The exceptions are 17 canceled authorized steps. These return the admission
error and write no snapshot. The comparison also records four intended
operator-selected tenant audit labels.

Permanent tests cover:

- the fold, against `ProjectPolicyChanges` itself;
- fold order, malformed and foreign-kind skipping, and the empty and global-rule
  wires;
- journal read errors;
- the compaction snapshot, its `through_seq`, and the published envelope and
  content hash;
- a failed save journaling nothing, and no save after a read error;
- the specs, with primary-only compaction, and the output schemas.

Native tests cover the registry flags. They also cover routing: a tenant rule
is visible only through the tenant route, a tenant token is refused compaction,
and an operator-named compaction writes only the tenant's snapshot and journals
its audit record and `policy.compacted` there.

Twenty-nine independent mutations fail tests. These include the native
registration, the routed kernel, the snapshot path, the bus binding and the
`bind`/`bindAs` authorization. Mutation testing found two gaps, now closed:

- no test compared a surviving scoped rule's capabilities;
- no test proved that the snapshot keeps the minimal change list rather than
  the raw history.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
only.

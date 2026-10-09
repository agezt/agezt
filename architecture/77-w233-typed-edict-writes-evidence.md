# W2.33b typed edict writes

The four policy writes now run on the shared dispatcher in `kernel/app/edict`.

- `edict_deny_add` adds exactly one runtime hard-deny rule, in the boot-time deny
  syntax.
- `edict_deny_rm` removes a runtime rule. The boot-time floor refuses removal.
- `edict_set_level` sets one governed capability's trust level.
- `edict_set_mode` sets the ask policy.

`Writes` works over a `WriteEngine` port: `HardDenyRules`, `AskPolicy`, `Level`,
`SetLevel`, `SetAskPolicy`, `AddHardDeny` and `RemoveHardDeny`. A `Publish`
function journals each change. Both are bound to the policy engine and bus of the
kernel the dispatcher routed to.

`WriteOperations` declares four specs. Each is non-read-only, so the operation is
audited before the engine changes. Each is also tenant-owned and
caller-tenant-routed, and allows unknown input. The routes are
`POST /api/edict/deny_add`, `/deny_rm`, `/set_level` and `/set_mode`, matching
the Web UI routes.

Removed with the move:

- the native `edict.go`, `edict_deny.go` and `edict_set.go`;
- `edictFor` and `askPolicyLabel`;
- `registerEdictCommands`.

The config view reads `AskPolicy().String()` directly. The whole edict family is
now typed except `edict_log`/`edict_stats`, which stay in `policy_log.go`, and
`edict_overlay`/`edict_compact`.

## Preserved behavior

- Every change journals `policy.changed` on the routed kernel from `operator`,
  uncorrelated, with the same payloads: `deny.add`, `deny.rm`, `level.set` and
  `mode.set`. A refused change journals nothing.
- `deny_add` reads `rule` strictly and parses it before checking the strict
  tenant. More than one rule is
  `args.rule must specify exactly one deny rule (no ';' separators)`. The
  response gives the generated rule name, the substring, the `applies_to` list
  (`[]` for a global rule) and the new rule count.
- `deny_rm` requires a non-blank `name` but passes it untrimmed. It journals only
  an actual removal, and a floor rule's refusal is an error.
- `set_level` checks, in order:
  1. a required capability;
  2. that the capability is known (an unknown one is
     ``unknown capability X (see `edict show` for the governed set)``);
  3. a strict level, which is then parsed;
  4. the strict tenant.

  `from` is the previous level, or `unset`.
- `set_mode` reads `mode` strictly and parses it before the tenant. It reports
  the previous and new mode.

The shared already-canceled admission rejects an authorized write before any
audit or policy change. This is intended: the legacy handlers ignored the
context. The routed audit record now carries the operator-selected tenant label,
the W2.1 refinement.

## Runnable comparison and regression evidence

The harness clones fixture kernels. On each clone it gives the primary and
`acme` engines different starting levels and ask policies, so every change
reports the routed engine's own previous state. It runs the pre-slice handlers
and `edictFor` on one copy and the registered operations on another.

312 steps, repeated twenty times, cover nine sequences under primary, wrong and
tenant tokens, in normal and canceled contexts:

- adds of scoped and global rules, removal by generated name and a repeated
  removal;
- refusal to remove a floor rule;
- every codec error, in order;
- level changes, including aliases and an unknown capability;
- mode changes;
- tenant-routed writes;
- foreign, invalid, `null`, non-string and blank tenants;
- secret-named arguments.

The comparison checks four things:

- responses are byte-exact;
- both kernels' journals, grouped by correlation, are equal: the operation audit
  records and the `policy.changed` events;
- both engines end in the same full policy state;
- the changes, previous values, the floor refusal and the tenant-only effects
  are asserted.

The exceptions are 56 canceled authorized steps. These return the admission
error, journal nothing and leave the starting policy unchanged. The comparison
also records four intended operator-selected tenant audit labels.

Permanent tests cover:

- each write's codec order, with no engine call on error;
- the published envelope and payloads;
- removal-only journaling, `unset` and the previous level;
- engine refusals journaling nothing;
- the specs and output schemas;
- audit admission failure and an already-canceled request both blocking the
  policy change.

Native tests cover the registry flags, plus routing: a tenant rule landing only
on the tenant engine, and the tenant journal carrying each audit record around
its policy change.

Thirty-six independent mutations fail tests, including the native registration
and the routed kernel and bus binding. Mutation testing found one gap, now
closed: no test proved that a multi-rule spec is refused before the tenant
check. Sources are restored byte-for-byte. Fixtures use
isolated temporary kernels only.

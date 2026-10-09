# W2.33a typed edict reads

The three policy reads now run on the shared dispatcher, in the new package
`kernel/app/edict`:

- `edict_show`: the ask policy, every capability's trust level and the hard-deny
  rules;
- `edict_deny_list`: the hard-deny rules, marking the runtime-added ones as
  removable;
- `edict_test`: a dry-run decision in the vocabulary the runtime journals.

`Service` works over an `Engine` port (`Levels`, `HardDenyRules`, `AskPolicy`,
`Decide`). It is bound to the policy engine of the kernel the dispatcher routed
to. `Operations` declares three read-only, unaudited, tenant-owned and
caller-tenant-routed specs with unknown input allowed:

- `edict_show` on `GET /api/edict_show`;
- `edict_test` on `GET /api/edict/test`, both matching the Web UI read routes;
- `edict_deny_list`, with no route, as before.

The three native handlers and `denyRuleRows` are removed. `edictFor` and
`askPolicyLabel` stay for the four writes and the config view. Those writes move
in W2.33b.

## Preserved behavior

- `edict_show` and `edict_deny_list` reject a non-string `tenant` (including
  `null`) before anything else. Routing follows the trimmed tenant.
- Hard-deny rows are sorted by name. A rule without capabilities reports
  `applies_to: null`, meaning every capability. The removable marker follows the
  runtime-rule naming. Empty lists and level maps are `[]` and `{}`.
- `edict_test` reads `capability` leniently: a non-string is treated as empty,
  and an empty value is `args.capability required`. This is checked before the
  strict tenant, as before. The capability passes through untrimmed. A non-string
  `input` is an empty probe.
- The probe reports decision, capability, level, reason, hard-denied, rule,
  would-ask and requires-approval.

The only intended difference is the shared already-canceled admission, which
rejects an authorized read before any work.

## Runnable comparison and regression evidence

Policy state is per-kernel runtime state, so the harness applies identical
changes to the primary and `acme` engines of every clone after opening them:

- changed trust levels;
- a different ask policy per kernel;
- three runtime deny rules, capability-scoped and global, one of them
  kernel-specific.

It runs the pre-slice handlers and row builder on one copy and the registered
operations on another.

162 steps repeated twenty times cover seven sequences under primary, wrong and
tenant tokens and normal and canceled contexts:

- show and list;
- probes that hit and miss each rule kind;
- an unknown capability;
- a non-string input and a padded capability;
- every capability and tenant error, in order;
- tenant-routed, padded, foreign, `null`, non-string and invalid tenants;
- secret-named arguments.

Responses are byte-exact, and neither side journals anything. The harness asserts
that the runtime rule and both removable states were read, and that the probes
both hard-denied and allowed. The only exceptions are 29 canceled authorized
steps, which return the admission error.

Permanent tests cover:

- show with sorted rows, null `applies_to` and the empty wire;
- the deny list with removable markers and the empty wire;
- the strict tenant;
- the probe's error order with no decision on error, untrimmed capability and
  empty-input pass-through, and every outcome field, including would-ask without
  requires-approval;
- the specs, and the output schemas against real outputs;
- the native registry flags;
- native routing with a tenant-only deny rule visible only through the tenant
  route, the probe deciding against the routed engine, and the strict tenant
  error.

The existing native edict suites still pass through the new path.

Twenty-five independent mutations fail tests, including the native registration
and the routed-kernel binding. Mutation testing found one gap, now closed: no
outcome told would-ask apart from requires-approval. Sources are restored
byte-for-byte. Fixtures use isolated temporary kernels only.

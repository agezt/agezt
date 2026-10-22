# W2.58 one shared row type for the council and the plugins

The council's membership and the plugin manifest were carried across the
host/application boundary **twice**: once as the runtime's own value types
(`kernel/runtime/types`) and once as the application's row types
(`app/council.Member`, `app/plugins.Registration`). The two declarations had
the same fields, so the host copied the fields of every row it handed over —
and a copy is a place where the two sides can drift apart.

With this slice there is one type, declared once, and both sides name it.

## What moved

`kernel/contract/types` (new, L1) holds the three shared value types, moved
verbatim from `kernel/runtime/types`:

| type | what it is |
|---|---|
| `CouncilMember` | one seat of the Council of Elders (the seat and the model that speaks for it) |
| `PluginInfo` | one external plugin's manifest entry (prefix, path, args, tool count, pin flag, allowlist) |
| `SubAgentLimits` | the delegation-governance ceilings (M46–M48) |

The contract layer is where a shared **value** type belongs: a contract package
holds types and interfaces and no logic (P4), and a value type has no
behaviour to move.

`kernel/runtime/types` is **deleted**. The runtime keeps its public aliases
(`runtime.CouncilMember`, `runtime.PluginInfo`, `runtime.SubAgentLimits`), so
the host's API — and every caller of it (`cmd/agezt`, `kernel/toolreg`,
`plugins/tools/council`, `kernel/runtime/accessors`) — is unchanged.

The application's rows are now aliases of the contract types:

```go
// kernel/app/council
type Member = types.CouncilMember

// kernel/app/plugins
type Registration = types.PluginInfo
```

## The projections that are gone

Four places existed only because the two sides did not have the same type.

| where | before | after |
|---|---|---|
| `kernel/controlplane/app_operations.go` — council read | copied every seat and model into `appcouncil.Member` | `return s.k.CouncilDefaultMembers()` |
| `kernel/controlplane/app_operations.go` — council write | copied every seat and model into `runtime.CouncilMember` | `return members` |
| `kernel/controlplane/plugin.go` — manifest reader | copied the six manifest fields into `appplugins.Registration` | `return r.kernel.Plugins()` |
| `kernel/app/council/ask.go` — the answer's member list | rebuilt the rows field by field | copies the panel's rows |

The last one is in the application layer rather than the host, but it is the
same pattern: the application knew the row's fields and rebuilt them. The
copy keeps the answer's member list an empty array (never `null`) when the
panel is empty.

## Why the contract layer — and a correction

The runtime must not import the application layer: that edge points upward
and breaks the layer rule. The application layer must not reach into a
module's internals either. A shared value type therefore has no home in
either tree.

While measuring this, the W2.57 evidence doc's claim that "the application
layer cannot import `kernel/runtime`" was found to be **wrong**. The layer
rule allows L4 → L3 — the application layer's `app/system` already imports
`kernel/runtime` — and the prohibition that actually matters is the module
reach, not the layer order. The claim is corrected in place in
`architecture/124-w257-one-warden-branch-evidence.md`.

## The semantics are unchanged

- the wire shapes: `seat`/`model` for a council member, the six manifest
  fields for a plugin row;
- the plugin rows' prefix ordering;
- the defensive copies of the manifest's argument and allowlist slices (the
  listing still cannot write through to the kernel's configuration);
- the listing is a read: the kernel's own manifest keeps its order and its
  contents;
- an empty panel is an empty array, never `null`.

## Runnable evidence

Permanent tests cover:

- the contract types: the council member's wire shape (the seat and model
  keys, and the empty object), the manifest's six fields and its empty
  value, the delegation ceilings and their zero value;
- the application layer: the council row **is** the contract type (and so is
  a whole slice of it), the plugin registration **is** the contract type, the
  council answer's member list is the panel's own rows and an empty panel is
  `[]` and not `null`;
- the host: the membership the operator sets reaches the kernel unchanged (the
  seat stays the seat and the model stays the model) and comes back unchanged,
  the manifest reaches the wire unchanged and in the application's order, and
  the listing does not disturb the kernel's manifest.

The mutations fail tests: **18 mutations, 18 caught** —

1. the renamed council seat key, 2. the renamed council model key,
3. the renamed manifest tool-count field, 4. the narrowed delegation spend
ceiling, 5. the council member that gains a field,
6. the application's council row that becomes its own struct,
7. the application's plugin registration that becomes its own struct,
8. the runtime's council member that becomes its own struct,
9. the runtime's plugin info that becomes its own struct,
10. the council read that drops the model,
11. the council write that drops the model,
12. the manifest read that drops the tool count,
13. the manifest read that drops the pin flag,
14. the manifest args that are no longer copied,
15. the manifest allowlist that is no longer copied,
16. the plugin rows that are no longer sorted,
17. the council answer that rebuilds the member rows field by field,
18. the council answer that hands the panel's slice over directly (an empty
panel would answer `null`).

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (270 packages, 129 allowlisted import violations and 13
allowlisted call sites — the ratchet is unchanged) and `tools/structure-md`
(the new package; 161 kernel packages). The affected suites — the contract
types, the council, the plugins, the runtime accessors and the control plane
— pass at count=20.

One observation is recorded rather than dismissed. An earlier `count=20` run
of the control plane suite failed once, and **the failing test's name was not
captured** — that run's output had been piped through the tail of the log, so
only the operator-repair panic lines (which the suite prints on purpose) and
the `FAIL` summary survived. The same command then passed twice in a row
(`ok ... 1134.521s` and `ok ... 1227.726s`), and the exact head is green in
CI, including the two race jobs. The slice carries no timing, concurrency or
I/O change — it is a type unification and the removal of four field copies —
so the failure is not attributed to it; it is recorded because its name is
still unknown.

## Not done here

The remote backends' list — ssh, k8s, modal, daytona and remote-agezt — still
appears in two places, but with **opposite** meanings: the application
supports them for a direct run, and the host refuses them for a dispatched
seat. Unifying that list would mean moving the dispatched seat's policy into
the application, which is a separate decision.

The dead `boardReadDefaultLimit` constant in `kernel/controlplane/board.go` is
still there, and the wider run/tool adapter convergence, the W3 modules, the
journal raw-ref GC, the W4 triggers/channel supervision/config/signing and the
W5 generated SDK/frontend convergence remain open.

The cloned-kernel harness of the earlier slices — the pre-slice handler and
the registered operation on two kernels, compared step by step — is not part
of this slice, as in W2.44b through W2.57. The council, plugin and control
plane suites are the parity proof here.

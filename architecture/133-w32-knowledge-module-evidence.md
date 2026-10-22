# W3.2 the knowledge module

The knowledge module was **a module without a home**. The architecture's
layer map already named it — `kernel/memory`, `kernel/worldmodel` and
`kernel/taste` are all layer 3 in the module `knowledge` — but the packages
sat at the top of `kernel/`, outside the `kernel/modules/` tree that W3
establishes as the target home of the bounded contexts. The roadmap's W3 step
lists `modules/knowledge` (memory, world, taste, profile distill) as its second
entry, and asks for the profile-facet supersede bug to be fixed in the rewrite
PR.

This slice does both.

## What moved

| the package | before | after |
|---|---|---|
| the memory store | `kernel/memory` | `kernel/modules/knowledge/memory` |
| the world model | `kernel/worldmodel` | `kernel/modules/knowledge/worldmodel` |
| the taste store | `kernel/taste` | `kernel/modules/knowledge/taste` |

The **package names do not change** (`memory`, `worldmodel`, `taste`), so the
move is a change of import path and nothing else: 68 files that import these
packages were repointed, and no call site, type name or behaviour moved with
them.

The module's placement is unchanged and now *derived* rather than declared:
the layer map's `kernel/modules/*/...` rule (layer 3, module `@1`) places
`kernel/modules/knowledge/memory` in the module `knowledge` — exactly where the
three hand-written rules used to place `kernel/memory`. Those three rules are
gone; the spot-check in `tools/archcheck/main_test.go` now asserts the new
path's placement.

The allowlist moved with the edges: 13 entries changed their path (the same
13 violations, relocated), so the ratchet is unchanged at 129 import
violations and 13 call sites.

## The profile-facet supersede bug

The operator profile is a set of **stable subjects** — `operator profile:
preferences`, `operator profile: communication style`, and three more — each
carrying a synthesized sentence about the operator. The package doc promises
that a facet is "updated across passes (never duplicates)".

It did not.

A memory record's identity is its **content**: `ScopedID(type, subject,
content, scope)`. The facet's subject is stable, but its content is
synthesized afresh on every pass. So a pass whose wording changed wrote a
**second** record under the same subject and left the previous one active
forever. The operator then got two conflicting versions of the same facet
injected into every run — and `ProfileText()` (which lists the active facets)
showed both.

The existing test only proved the *same-content* case, where the
content-addressed id happens to be identical and the write reinforces the
record. The failing case needed a proof:

```
the profile carries 2 records for one facet, want 1 (the changed facet
supersedes the old one)
```

**The fix.** `DistillProfile` now retires the previous version of a facet
before writing the new one: it computes the new record's id
(`ScopedID(TypePreference, subject, content, scope)`), supersedes the active
records of that subject with it, and only then writes the new facet. The
superseded record keeps its audit trail (`memory.superseded`, and the
`SupersededBy` pointer), so the operator can see which record the new one
replaced — nothing is deleted, exactly like every other memory mutation.

The scope invariant is enforced where the facets are collected: only the
**shared** facets are candidates (a private-scope record with the same subject
belongs to another scope and is never retired by the shared write).

**A named error.** `supersedeExisting` already refused to supersede a record
the store does not hold — but it reported that as a bare error, so a caller
could not tell "the record was deleted concurrently" from a real failure. It
now reports `memory.ErrRecordNotFound`, and the profile pass treats that one
case as benign (the vanished record needs no superseding) while still failing
the pass on anything else. The old `supersede_bug_test.go` — a proof-of-failure
that had outlived its bug — is now a regression test that asserts the named
error.

## What did not change

The move is a path change: the same packages, the same exported API, the same
records on disk (`<dir>/memory.json`, `<dir>/worldmodel.json`,
`<dir>/taste.json`), the same events, the same journal, the same retention and
hygiene rules, the same injection order in the run's prompt.

The fix changes exactly one thing: a facet whose content changed between two
profile passes now **replaces** its predecessor instead of joining it. A facet
whose content did not change still reinforces the same record, as before.

## Runnable evidence

Permanent tests:

- `kernel/modules/knowledge/memory/profile_test.go` — the proof of the bug
  (`TestDistillProfile_SupersedesTheChangedFacet`: one facet, one record, the
  newer content, and the retired record pointing at its successor), the
  untouched-facet case (`TestDistillProfile_SupersedesOnlyTheChangedFacet`), the
  private-scope case
  (`TestDistillProfile_KeepsAPrivateFacetOfTheSameName`) and the concurrent
  deletion case
  (`TestDistillProfile_ToleratesARecordThatVanishedConcurrently`);
- `kernel/modules/knowledge/memory/supersede_bug_test.go` — the absent record
  now reports `ErrRecordNotFound` instead of silently succeeding;
- the pre-existing memory, world-model and taste suites, unchanged apart from
  the import path.

The mutations fail tests: **8 mutations, 8 caught** — the previous facet is
never retired, a facet retires itself, a facet of another name is retired, the
retired facet points at the wrong record, a vanished record aborts the pass, an
absent record is silently accepted, the absent record has no named error, and a
private facet enters the shared profile.

The run also produced an honest finding: the scope guard in the supersede loop
was **unreachable** — the private facets are already filtered out where the
candidates are collected, so removing that guard changed nothing. It was
deleted rather than left as an untestable branch, and the invariant it was
supposed to protect is now asserted by a test at the place that enforces it
(the mutation that breaks that filter is caught).

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (273 packages, 129 import violations and 13 call sites — the
ratchet is unchanged), `tools/structure-md`, `tools/changelog-lint`,
`tools/docclaimscheck` and staticcheck. The affected suites — the knowledge
module's three packages, the runtime, the application layer, the control plane
and the commands — pass.

## Not done here

**The runtime keeps its thin accessors.** `Kernel.DistillBrain`,
`Kernel.DistillProfile`, `Kernel.Taste()` and `Kernel.Seats()` stay in
`kernel/runtime`: they are the host's accessors over the module's stores
(they need the live config, the halt guard and the kernel's own wiring), and
the implementation behind them is the module's.

**The next modules are untouched.** `modules/skills` (with the builtin seeding
and the operator-quarantined built-ins), `modules/artifacts` (artifact +
datalake, with the journal `raw_ref` GC), `modules/board`, `modules/work`
(workboard, OKR, proof, assure, seats — with the `"container"` seat mapping),
`modules/workflows`, `modules/agents` (with the guardian reconcile cap bug),
`modules/extensions`, `modules/providers` and `modules/runs` last.

# W3.3 the skills module

The Forge was a **module without a home**. The layer map already named it —
`kernel/skill` is layer 3 in the module `skills` — but the package sat at the
top of `kernel/`, outside the `kernel/modules/` tree that W3 establishes as
the target home of the bounded contexts. Its built-in bundles were worse: they
were a *plugin* (`plugins/builtinskills`), so the module's own seed data
lived outside the module, and the plugin had to reach back into the module to
seed it.

The roadmap's W3 step 3 is `modules/skills` (+ builtin seeding; stop
re-promoting operator-quarantined built-ins). This slice does both.

## What moved

| the package | before | after |
|---|---|---|
| the Forge | `kernel/skill` | `kernel/modules/skills/skill` |
| the built-in bundles | `plugins/builtinskills` | `kernel/modules/skills/builtins` |

The **package name of the Forge does not change** (`skill`), so the move is
an import-path change and nothing else: the 59 Go files that imported it were
repointed, and no call site, type name or behaviour moved with them.

The built-in bundles changed shape: the old plugin was three Go files plus 16
embedded bundle directories, and it imported the Forge to seed it. Now the
**bytes** live in `kernel/modules/skills/builtins` (an `embed.FS`, the bundle
list, `Names()` and `Bundle(name)`) and the **seeding** lives in the module
(`kernel/modules/skills/skill/builtins.go`). The `builtins` package does not
import the `skill` package — that would be a cycle — so the two meet through
the module's own seam:

```go
// skill/builtins.go
type ForgeAPI interface {
    Create(corr string, spec CreateSpec) (Skill, bool, error)
    Promote(corr, id string) (Status, error)
    Get(id string) (Skill, bool, error)
}

func SeedBuiltins(f ForgeAPI, corr string) ([]Seeded, error)
func BuiltinNames() []string
func BuiltinBundle(name string) (skillMD []byte, resources map[string][]byte, err error)
```

`ForgeAPI` is an interface, so the seeder can be driven by the real Forge or
by a fake — which is how the failure path is tested.

The placement is now *derived* rather than declared: the layer map's
`kernel/modules/*/...` rule (layer 3, module `@1`) places
`kernel/modules/skills/skill` in the module `skills`, exactly where the
hand-written `kernel/skill` rule used to place it. That rule is gone.

The allowlist moved with the edges: nine entries changed their path (the same
nine violations, relocated) and one entry was **deleted** — the plugin
`plugins/builtinskills` no longer exists, so its `plugin-reach` edge is gone.
The ratchet therefore **drops from 129 to 128** import violations; the 13
allowlisted call sites are unchanged.

## The quarantined built-in came back

The seeder's job is to make the built-ins *available*: it installs every
embedded bundle into the Forge and promotes it to `active`, so capabilities
like full browser automation work out of the box, with no operator install and
no agent having to author the skill first.

It promoted them **unconditionally**.

The lifecycle has five states, and `quarantined → active` is a legal edge — a
promote is exactly how an operator brings a skill back. So when an operator
pulled a built-in out of production (`agt skill quarantine`, or the automatic
quarantine that fires when a skill crosses the failure threshold), the daemon's
own seeding undid the decision on the **next boot**: the built-in was back in
the retrieval pool, injected into runs, with the operator's reason nowhere in
sight.

The proof:

```
the quarantined built-in came back as "active" — the seeding resurrected it
```

**The fix.** The seeding makes the built-ins available; it does not re-open a
lifecycle decision. `seedOne` now stops the promote loop when the skill is
`quarantined` or `archived`:

```go
status := sk.Status
for i := 0; i < 3 && status != StatusActive; i++ {
    if status == StatusQuarantined || status == StatusArchived {
        break
    }
    ...
}
```

The seeding still does its job — a built-in that was never pulled out of
production is still promoted into the retrieval pool — and the report the
seeding returns names the status the skill was actually left in, so the boot
log can say why a built-in is not in the pool.

The automatic quarantine benefits from the same guard: a built-in the
regression guard detected no longer returns on the next boot.

## What the move broke, and what it fixed

Two live references pointed at the old home and were **silently** broken by
the move:

1. **`browser.ResolveActionDriverPath`** looked for
   `plugins/builtinskills/browseruse/scripts/browse.mjs` relative to the
   working directory's ancestors and the executable. The bundle moved, so the
   lookup resolved to nothing — and the browser action's e2e test *skips*
   when the driver is missing, so nothing would have failed. The lookup now
   points at the module's bundle, and a permanent test asserts that the
   driver is found and points at the module's copy.
2. **`.gitattributes`** forced LF on `plugins/builtinskills/**` — the bundles
   are `go:embed`-ded into the binary, and the shell setup script has to run
   on Linux, so a CRLF-embedded script breaks there. The rule follows the
   bundles to `kernel/modules/skills/builtins/**`.

Both are the kind of breakage a path move hides: the first behind a `Skip`,
the second on a Windows checkout only.

## What did not change

The move is a path change: the same package name, the same exported API, the
same records on disk (`<home>/skills/skills.json`, `<home>/skills/bundles/`),
the same events (`skill.created`, `skill.promoted`, `skill.quarantined`, …),
the same journal, the same retrieval rules, and the same injection into a
run's prompt. The 16 bundles are the same 16 bundles, with the same bytes.

The fix changes exactly one thing: a built-in that was pulled out of
production is **not** promoted back by the seeding.

## Runnable evidence

Permanent tests:

- `kernel/modules/skills/skill/builtins_quarantine_test.go` — the proof of
  the bug (`TestSeedBuiltins_DoesNotResurrectAQuarantinedBuiltin`: the
  built-in is quarantined, the daemon seeds again, and it is still
  quarantined), the report case
  (`TestSeedBuiltins_ReportsTheQuarantinedStatus`: the seeding says
  *quarantined*, so the boot log can explain the absence) and the
  counter-case (`TestSeedBuiltins_StillActivatesAFreshBuiltIn`: the fix does
  not stop the seeding from doing its job);
- `kernel/modules/skills/skill/builtins_failure_test.go` — the seeding is
  best-effort **per bundle**
  (`TestSeedBuiltins_OneFailingBundleDoesNotStopTheOthers`) and the returned
  error names the **first** failure
  (`TestSeedBuiltins_ReportsTheFirstFailure`);
- `kernel/modules/skills/skill/builtins_public_test.go` — the module's seam
  for the plugin layer (`BuiltinNames`/`BuiltinBundle` expose exactly the
  bundles the module seeds, and the bytes parse);
- `kernel/modules/skills/builtins/builtins_bundle_test.go` — the SKILL.md is
  the skill's **body**, not one of its resources;
- `plugins/tools/browser/action_driver_path_test.go` — the browser driver is
  found, and it is the module's copy;
- the pre-existing Forge suites, unchanged apart from the import path.

The mutations fail tests: **17 mutations, 16 caught** — the quarantine guard
is gone, only the archived state is guarded, the promote loop stops at no
progress, the promote loop runs once, the skill is never promoted, the report
names the bundle directory, the bundle is never materialized on disk, a
malformed SKILL.md is ignored, one failing bundle stops the rest, the last
failure overwrites the first, the SKILL.md ships as a resource, the bundle
list is shared, the resource key keeps the prefix, the SKILL.md is read from
the root, the seam's bundle list is empty, the seam's bundle bytes are empty.

The seventeenth mutation is recorded as an **equivalent**, not hidden: it
drops the `archived` half of the guard, and nothing changes — the transition
matrix already refuses `archived → active`, so the promote fails and the loop
stops either way. The `archived` branch is defensive; the `quarantined` one
is the load-bearing half, and it is the half the roadmap asked for.

The run also made three test gaps **visible**, and they are now fixed and
named: the failure path of the seeding was untested (one bundle failing
stopped nothing, and the last failure overwrote the first), the module's
public seam was untested, and the SKILL.md could have leaked into the bundle
resources unnoticed. Three test files carry them now, and the mutations
confirm that each of them is **load-bearing**: without them five of the
seventeen mutations — the failing bundle that stops the rest, the last
failure that overwrites the first, the SKILL.md that ships as a resource, and
the two that empty the module's public seam — would have escaped.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (273 packages, **128** import violations and 13 call sites
— the ratchet *dropped* by one, because the plugin that reached into the
module is gone), `tools/structure-md`, `tools/changelog-lint`,
`tools/docclaimscheck` and staticcheck. The affected suites — the skills
module's two packages, the browser tool, the runtime, the control plane and
the commands — pass.

## Not done here

**The plugins still reach into the module's implementation package.**
`plugins/builtinmarket` reads the embedded bundles through
`skill.BuiltinBundle`/`skill.BuiltinNames`, and `plugins/tools/skilltool`
drives the Forge — two `plugin-reach` edges that are allowlisted and whose
count is unchanged by this slice. The skills module has no `api` package yet
(unlike the reasoning module), so the plugin cannot talk to it through a
vocabulary boundary; that is a separate slice.

**The Forge's own API is unchanged.** `kernel/runtime` and
`kernel/controlplane` keep using the module through the same names they used
before (`adapter-bypass` and `cross-module` edges, relocated). The skills
module is a **move** in this slice, not a port — the port (an `api` package,
ports and an injected service) is the shape W3.1 proved, and it can follow.

**The next modules are untouched.** `modules/artifacts` (artifact +
datalake, with the journal `raw_ref` GC), `modules/board`, `modules/work`
(workboard, OKR, proof, assure, seats — with the `"container"` seat mapping),
`modules/workflows` (move `runtime/workflowrun*` + draft + test-node; fix the
trigger `source` labels), `modules/agents` (roster, guardians, selfrepair,
reaper, overseer ops — this removes the
`controlplane/selfrepair` → `plugins/tools/overseertool` edge),
`modules/extensions` (mcp, plugin host, toolforge, toolbox, acpcatalog),
`modules/providers` (catalog, chatgptauth, providerboot) and `modules/runs`
last (what remains of `kernel/runtime` + `runexec` + the agent loop + resume
+ delegation + intervention; `kernel/runtime` is deleted when empty).

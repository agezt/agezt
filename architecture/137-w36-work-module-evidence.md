# W3.6 the work module

Five packages were already one module — the layer map named `kernel/workboard`,
`kernel/okr`, `kernel/proof`, `kernel/assure` and `kernel/seat` as layer 3 in
the module `work` — but all five sat at the top of `kernel/`, outside the
`kernel/modules/` tree W3 establishes as the target home of the bounded
contexts.

The roadmap's W3 step 6 is `modules/work` (workboard, OKR, proof, assure,
seats; fix `"container"` seat → profile mapping). This slice does both.

## What moved

| the package | before | after |
|---|---|---|
| the durable task queue | `kernel/workboard` | `kernel/modules/work/workboard` |
| the objectives | `kernel/okr` | `kernel/modules/work/okr` |
| the proof | `kernel/proof` | `kernel/modules/work/proof` |
| the assurance verdicts | `kernel/assure` | `kernel/modules/work/assure` |
| the execution seats | `kernel/seat` | `kernel/modules/work/seat` |

The **package names do not change**, so the move is an import-path change and
nothing else: 54 Go files were repointed, and no call site, type name or
behaviour moved with them.

The placement is now *derived* rather than declared: the layer map's
`kernel/modules/*/...` rule (layer 3, module `@1`) places all five packages in
the module `work`, exactly where the five hand-written rules used to place
them. Those rules are gone.

The allowlist moved with the edges: eight entries changed their path (the same
eight violations, relocated). The ratchet therefore stays at **128 import
violations and 13 call sites**, and the package count stays at 273.

## A seat pinned to "container" was never routed

A seat is the operator's way to say *how* a task runs: the model tier, the
tool tier, and the isolation surface. The seat store accepts exactly three
isolation surfaces (plus "inherit"):

```go
case "", "local", "warden", "container":
```

A **run profile** names the same thing from the other side — the
*implementation* that provides the surface. The warden-family run profiles are
`local`, `warden` and **`docker`**; there is no `container` run profile.

The dispatch path took the seat's value and handed it straight to the profile
resolver:

```go
if st.ExecutionProfile != "" {
    isoProfile = st.ExecutionProfile   // "container"
}
...
s.context.Isolation(ctx, isoProfile)  // → "container" is not a run profile
```

So a seat pinned to `container` — a value the operator can set, the seat store
accepts and the console offers — reached the resolver as a profile that does
not exist. The resolver refused it ("execution profile "container" is not
routable for run tools yet"), and the dispatch **degraded**: the task ran with
the agent's own isolation instead of the seat's, and the task's comment said
so. The seat was quietly ignored.

The proof:

```
the seat's container surface was not mapped to the docker profile:
  [agent:reason:owned seat:selected isolation:container review:answer publish:completed]
```

**The fix.** The two vocabularies are translated, once, where the seat's value
becomes a run profile. The translation table lives with the run profiles (the
module that owns their ids), and the dispatch applies it:

```go
// kernel/executionprofile/profile.go
func SeatProfileID(surface string) string {
	switch strings.ToLower(strings.TrimSpace(surface)) {
	case "container":
		return IDDocker
	default:
		return strings.TrimSpace(surface)
	}
}
```

Every other value is passed through unchanged — the seats' `local` and
`warden` are already run profile ids.

## What did not change

- the same package names, the same exported APIs, the same records on disk,
  the same tasks, the same proof gate, the same seat catalogue and the same
  custom-seat overlay;
- the same **custom-first** seat resolution (a custom seat shadows the
  built-in with the same id) and the same built-in seats are still
  non-deletable;
- the same degradation behaviour when an isolation profile is genuinely
  unavailable (the task still runs with the agent's defaults and says why) —
  the fix removes a case where that degradation was the *wrong* answer;
- the same wire and the same events.

## Runnable evidence

Permanent tests:

- `kernel/app/workboard/execution_seat_profile_test.go` — the proof
  (`TestWorkboardExecutionMapsTheSeatContainerSurfaceToTheDockerProfile`: a
  seat pinned to `container` reaches the resolver as `docker`, and the
  unrouted value never appears);
- the moved packages' own suites, unchanged apart from the import path — the
  workboard's proof gate, the seat store's custom-first resolution, the OKR
  lifecycle, the proof criteria and the assurance verdicts.

The mutations fail tests: **6 mutations, 6 caught** — the seat surface is not
translated, the container surface is dropped from the translation, the docker
profile is not routable, the seat store stops accepting the container surface,
the workboard's proof gate is bypassed, and a custom seat no longer wins over
the built-in one.

The mutations on the moved packages matter as much as the ones on the fix:
they prove the suites still test the module after the move — a move that
quietly stopped testing anything would leave the ratchet green.

The gates pass: the build, `gofmt`, `go vet`, `tools/deadcodecheck`,
`tools/archcheck` (273 packages, 128/13 — the ratchet is unchanged, the edges
moved with their packages), `tools/structure-md`, `tools/changelog-lint`,
`tools/docclaimscheck` and staticcheck. The full `go test ./...` passes.

## Not done here

**The module has no `api` package yet.** The five packages' types are
imported by the runtime, the control plane, the app layer and the
workboard tool — those are the eight relocated allowlist entries. The
reasoning module's shape (an `api` package that owns the vocabulary, the host
keeping its public names as aliases) would shrink that surface; it is a
separate slice.

**The other vocabularies are untouched.** The roster profile's
`execution_profile` already speaks the *run profile* vocabulary (`docker`,
`ssh`, `k8s`, …) and was left alone; only the seat's surface vocabulary needed
the translation.

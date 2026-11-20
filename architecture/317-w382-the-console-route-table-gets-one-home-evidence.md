# W3.82 — the console's route table gets one home

> **Slice:** W3.82 · **Date:** 2026-11-19 · **Base:** `07f4829c` (main) + W3.79 + W3.80 + W3.81
> **Kind:** a vocabulary moves; no behaviour changes.
> **Follows:** W3.79 (the daemon's operation names → `kernel/contract/opapi`),
> W3.80 and W3.81 (the CLI's 311 sub-command and 71 top-level names). This
> slice gives the **console's route table** one home: the same package, beside
> the operation names it binds.

---

## 1. What was wrong

The console's route table was written **twice**, and the daemon's operation
specs repeated a third of it:

| Surface | What it held |
|---|---|
| `kernel/webui/webui_read_routes.go` (202 lines) | `apiRoutes` (36 parameterless GET → operation), `readArgsRoutes` (41 GET with an argument allowlist), the `writeRoute{cmd, args}` type |
| `kernel/webui/webui_write_routes.go` (307 lines) | `writeRoutes` (64 POST, args from the query string), `jsonRoutes` (57 POST, args from the JSON body), `planRoute` |
| `opapi.Spec.HTTP` | a "HTTP hint" — method + path — on **118** operation specs across 58 application-layer files |

Three findings from the measurement changed the shape of the slice:

1. **`Spec.HTTP` was dead.** No production code read it. About thirty test
   assertions read it, so the field looked load-bearing while nothing outside
   the tests consumed it. It was a copy of the console's paths, written a third
   time, that the daemon never used.
2. **The two console tables agreed with each other — and the hint agreed with
   them.** 112 of the 198 routes had a spec that declared the same path; the
   other **86** declared none at all. No path was bound to the wrong operation
   and no method disagreed. The duplication was not lying — it was *silent*.
   A table that cannot drift is still a table somebody maintains.
3. **The catalogue could not have been the source.** Of the 198 routed
   operations, **70 have no `opapi.Spec` literal in the application layer at
   all** — their specs are built by helpers (`bindControl(&ops, opapi.PulsePause, …)`),
   not written out. Only the *name* catalogue sees every operation. A
   generator that read the specs could never have covered them.

Point 3 is the reason the home is `kernel/contract/opapi` and not the
application layer: the console (layer 5) routes operations **by name** over the
control-plane socket, so the binding it needs has to live where the names
already live — in the contract package, below every layer that could read it.

## 2. What moved

**`kernel/contract/opapi/http.go`** (558 lines) is the console's route table:

```
203 routes · 5 kinds · one entry per operation
```

```go
{Op: Status, Method: "GET", Path: "/api/status", Kind: HTTPRouteRead},
{Op: AgentWake, Method: "POST", Path: "/api/agents/wake", Kind: HTTPRouteWrite, Args: []string{"ref", "intent", "reason", "incident_id", "root_incident_id", "parent_incident_id"}},
```

- `Op` — the operation, named by its **constant** (the catalogue sits next to
  the names, so a route cannot spell an operation name by hand).
- `Method` — the method the console answers with.
- `Path` — the path the console answers at.
- `Kind` — which proxy the console binds and where the arguments come from:
  `HTTPRouteRead` (36, no arguments), `HTTPRouteReadArgs` (41, query string),
  `HTTPRouteWrite` (64, query string), `HTTPRouteJSON` (57, JSON body),
  `HTTPRouteStream` (5, a streamed response).
- `Args` — the argument allowlist the browser may set. A fixed list: the
  browser can only ever invoke the operation with these arguments.

Every comment the old tables carried came across — the M-numbers, the "why" of
each route, the notes about the routes that were deliberately never wired.

**The console derives its routes from it:**

```go
for _, route := range opapi.ConsoleRoutes() {
	switch route.Kind {
	case opapi.HTTPRouteRead:
		router.Handle(route.Path, proxyRead, s.proxy(route.Op))
	case opapi.HTTPRouteReadArgs:
		router.Handle(route.Path, proxyRead, s.readArgsProxy(route.Op, route.Args))
	case opapi.HTTPRouteWrite:
		router.Handle(route.Path, protectedMutation, s.writeProxy(route.Op, route.Args))
	case opapi.HTTPRouteJSON:
		router.Handle(route.Path, jsonProtected, s.jsonProxy(route.Op, route.Args))
	}
}
```

The five streamed operations bind a handler of their own — the frames are the
payload — but their path and their forwarded keys come from the catalogue too
(`consolePath(opapi.Plan)`, `opapi.ConsoleArgs(opapi.Run)`).

## 3. What is gone

- **`kernel/webui/webui_read_routes.go`** (202 lines) and
  **`kernel/webui/webui_write_routes.go`** (307 lines) — the four tables and the
  `writeRoute` type.
- **`opapi.HTTP`** — the type, and with it `Spec.HTTP`, the field that
  production code never read.
- **118 HTTP hints** across 58 application-layer files. Three of them were
  written dynamically and are gone too: the `execprofile` table's path column,
  the routing package's `spec(name, method, path, …)` helper, and the channel
  OAuth package's two `startSpec.HTTP = …` assignments.
- **86 test clauses** that compared a spec's HTTP hint. The tests that
  mattered — "this operation has **no** public route" (the OAuth callback, the
  webhook stats, the agent tombstone) — were repointed at the catalogue and
  still assert the same promise.

## 4. The guard

Eight tests, in three files, each in the package that owns the fact:

**`kernel/contract/opapi/http_test.go`** (5) — the catalogue's own invariants:
every route binds a **declared** operation name; no path is bound twice; the
method matches the kind (a read cannot answer POST); a parameterless read
forwards nothing and no allowlist names an argument twice; the index the
console reads is the table the catalogue declares.

**`kernel/webui/webui_routes_ratchet_test.go`** (2) — the console's line: it
serves the catalogue and nothing else (every catalogue route registered, none
registered twice, no other path served), and **no file in the console may
declare a path of its own** — a path written in the console is a second copy
of the catalogue.

**`kernel/controlplane/console_routes_test.go`** (1) — the strongest one: every
operation the console binds is one **the daemon registers**. The console
routes by name, so a name the daemon does not know is a route that answers 502
in production, and the browser's panel would silently break. This test reads
the daemon's own registry (`commandRegistry` + the application operations) and
compares it with the catalogue. It passes today for all 203 routes — and the
probe that measured it found that **all 325 declared operations are registered
by the daemon** (the only undeclared strings in the package are the four
`PrincipalKind` values).

**Proved to fire** by two reverted mutations:

| Mutation | What went red |
|---|---|
| A route registered in the console outside the catalogue (`router.Handle("/api/drift", …)`) | `TestConsoleRegistersTheCatalogueAndNothingElse`: *"/api/drift is served but is neither a catalogue route nor a console-native endpoint"*; `TestConsolePathLiteralsLiveInTheCatalogue`: *"kernel/webui/webui_routes.go declares the console path \"/api/drift\"…"* |
| A catalogue entry bound to a hand-written operation name (`Op: "operator"`) | `TestConsoleRoutesBindDeclaredOperations`: *"/api/status binds \"operator\", which is not a declared operation"*; `TestEveryConsoleRouteOperationIsRegistered`: *"the console serves operator at /api/status, but the daemon registers no \"operator\" operation"* |

Both mutations were reverted; the tree is green again.

## 5. The measurement

| | Before | After |
|---|---|---|
| Places a console route is written | **3** (two console tables + the spec hint) | **1** (the catalogue) |
| Files holding the console's route table | 2 (509 lines) | 1 (558 lines, 203 routes) |
| Route entries | 198 + 5 streamed | 203 (one table) |
| Operation specs carrying an HTTP path | 118 (in 58 files) | **0** |
| Production readers of a spec's HTTP path | 0 | 0 |
| Hand-written read-command list in the console's tests | 60 names | **0** (derived from the catalogue) |
| Console routes the browser calls that the catalogue does not know | — | **0** of 198 |

The last row is a separate measurement, worth stating because it is the
strongest evidence that the move was faithful: the browser's own copy of the
paths (`frontend/src`, 218 distinct `/api/…` literals) was compared with the
catalogue. Every generic route the browser calls is in the catalogue; the paths
the browser calls that the catalogue does not know are the **21 console-native
endpoints** (the shell, the auth surface, the event feed, the file manager,
the voice surface, the rollback surface, the workflow hook, the OAuth
callback) and four test fixtures (`/api/foo`, `/api/x`, …).

**Files touched:** 157 tracked (+420 / −934), 5 new (909 lines), 2 deleted.

## 6. Not moved, on purpose

- **The console's 21 native endpoints** stay registered one by one in
  `webui_routes.go`. They are not operation bindings — the file manager walks
  the filesystem, the rollback surface restores checkpoints, the shell serves
  the SPA. The ratchet names them explicitly, so they are a *declared* list
  rather than an unnoticed second table.
- **The browser's copy of the paths** (`frontend/src`) is still hand-written.
  Pinning it to the catalogue is a real, feasible ratchet — the measurement
  above shows the two agree exactly — but the fixture paths in the frontend's
  own tests (`/api/foo`, `/api/x`) would need an allowlist, and a permanent
  allowlist is the thing this programme has been removing. The honest place
  for it is W5.1 ("OpenAPI from op registry; generated TS client"), where the
  client stops being hand-written at all.
- **`numericQueryArgs`** — the seven argument names the query-string proxies
  coerce to numbers — stayed in the console. It is a property of the *value*,
  not of the route, and the console owns the coercion.
- **The application layer keeps no HTTP surface at all.** The operations are
  transport-independent; the console's binding is a transport concern, and it
  now lives with the transport-independent vocabulary, next to the names.

## 7. Gates

`gofmt`, `go build ./...`, `go vet ./...`, `go test ./...`,
`archcheck` (339 packages, 1 allowlisted violation — unchanged),
`deadcodecheck`, `depscheck` (24 dependencies), `sdkparity -check`,
`structure-md -check`, `changelog-lint`, `docclaimscheck -base origin/main`,
`docsindex`, `staticcheck` (`GOTOOLCHAIN=go1.27.1`).

**No behaviour changed.** The console answers at the same paths, with the same
methods, the same argument allowlists and the same timeouts. The operation
names, the wire protocol and the browser's contract are untouched.

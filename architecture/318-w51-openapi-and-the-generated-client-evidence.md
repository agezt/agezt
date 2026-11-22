# W5.1 — the API document and the browser client get one home

> **Slice:** W5.1 · **Date:** 2026-11-19 · **Base:** `b4e83c9e` (main) + W3.82
> **Kind:** a surface is derived instead of written; three real bugs die on the way.
> **Follows:** W3.82, which gave the console's route table one home
> (`kernel/contract/opapi/http.go`). This slice gives the **API document** and
> the **browser's client** the same home, and makes the browser's paths a
> compile-time checked copy of the console's surface.

---

## 1. What was wrong

The console's HTTP surface was written **three** times, and the copies did not
agree:

| Surface | How the paths were written | Count |
|---|---|---|
| `kernel/contract/opapi/http.go` | the route catalogue — path, method, kind, forwarded arguments | 203 routes |
| `frontend/src/app/api/api.ts` | the browser's wrapper — `getJSON`/`postJSON`/`postAction` over a `string` path | 460 call sites, 179 distinct paths, **1 082 path literals** in 132 files |
| the daemon's operations | the registry — 324 operations, each with an input and an output schema | 324 |

The browser's wrapper took `path: string`. Nothing stopped a call site from
naming a path the console does not answer, from using the wrong verb for a
route, or from passing a parameter to a route that forwards none. The
measurement for this slice found **all three** in the tree:

### 1.1 Eight calls whose body went nowhere

The console binds a proxy per route kind: a `Write` route reads its arguments
from the **query string**, a `JSON` route reads them from the **body**. Eight
routes were catalogued as `Write` while the browser sent a JSON body:

| Path | Route kind | Browser sends |
|---|---|---|
| `/api/agents/wake` | Write | body |
| `/api/send` | Write | body |
| `/api/provider/reload` | Write | body |
| `/api/channel/account/remove` | Write | body |
| `/api/market/source/add` | Write | body |
| `/api/market/source/remove` | Write | body |
| `/api/market/sync` | Write | body |
| `/api/memory/bulk_forget` | Write | body |

`writeProxy` copies the route's allowlist out of `r.URL.Query()`. A body is
never read. **Every one of those eight calls reached the daemon with no
arguments at all.** `/api/memory/bulk_forget` is the clearest case: its one
argument is `ids`, a JSON *array*, which a query string cannot carry at all.

### 1.2 Three reads whose parameters went nowhere

`/api/approvals`, `/api/autonomy` and `/api/workflows` were catalogued as
parameterless reads, while the browser sent `pending`, `limit` and `with_runs`
respectively. A parameterless read's proxy forwards nothing, so the parameters
were dropped. Two of the three are real losses: `autonomy_feed` and
`workflow_list` both declare the argument the browser sends.

### 1.3 Paths that were never written down

Three browser calls built their URL by hand — `"/api/suggestions" + qs`,
`` `/api/journal?limit=${limit}` ``, `` `/api/rollback/checkpoints${qs}` `` —
so the path was a runtime string, and the console's own route guard could not
see it. One of them (`/api/memory/audit?limit=120`) sent a `limit` to an
operation that declares no input at all.

## 2. What the sources are

The document and the client are **derived** from two sources that already
existed and already had one home each:

- **the route catalogue** (`kernel/contract/opapi/http.go`) — which path
  answers which operation, with which verb and which forwarded arguments;
- **the operation registry** (`kernel/controlplane`) — what each operation
  accepts and returns, as JSON Schema.

The join of the two is the API's contract. The measurement confirmed the join
is complete: **all 203 catalogue routes name a registered operation, and all
203 have both an input and an output schema** (`missing=0`, `noInputSchema=0`,
`noOutputSchema=0` over the 324 registered operations).

The registry is readable **at build time** — the operations are bound in package
initialisers, so no daemon, no socket and no running kernel is needed to ask
what an operation's contract is. That is what makes the document a derived
artifact instead of a second copy.

## 3. What was built

### 3.1 `kernel/contract/opapi/openapi.go` — the document producer (L1)

A pure OpenAPI 3.1 producer. It takes the catalogue and a **schema source**
(callback) and returns the document. It does not import the registry: the
contract package stays free of the daemon, and the bridge that reads the
registry lives in the layer that owns it.

The route kind decides the HTTP shape:

| Kind | Method | Shape |
|---|---|---|
| `Read` | GET | no parameters, no body |
| `ReadArgs` | GET | query parameters — the console's allowlist |
| `Write` | POST | query parameters — the console's allowlist |
| `JSON` | POST | request body — the operation's input schema |
| `Stream` | POST | request body; the console binds a bespoke handler |

A route that names an operation the registry cannot answer **stops the
document**: a missing contract is said out loud rather than published as a
route with no shape.

### 3.2 `kernel/controlplane/openapi.go` — the registry bridge (L5)

`AppOperationSchemas()` returns every registered operation's input and output
schema, keyed by operation name; `AppOperationSchemaSource()` hands it over as
the producer's callback. The map is a copy — a caller that rewrote a schema
would rewrite the daemon's own contract.

### 3.3 `tools/openapi` — the generator (L7)

```
go run ./tools/openapi           # regenerate
go run ./tools/openapi -check    # fail if the committed files are stale
```

It writes two artifacts:

- **`docs/openapi.json`** — the OpenAPI 3.1 document, 203 paths, every
  operation carrying the registry's own input and output schema.
- **`frontend/src/app/api/client.ts`** — the browser's client.

### 3.4 The generated client

The client's path unions **are** the console's surface:

```ts
export type ReadPath = | "/api/acp/agents" | … ;   // 34
export type ReadArgsPath = | … ;                    // 43
export type WritePath = | … ;                       // 56
export type JsonPath = | … ;                        // 65
export type StreamPath = | … ;                      // 5
export type NativePath = | … ;                      // 15
```

The verb carries the kind rule **in its type**, so the three mismatches above
cannot be written any more:

```ts
export async function getJSON<T = any>(path: ReadPath, options?: RequestOptions): Promise<T>;
export async function getJSON<T = any>(
  path: ReadArgsPath, params?: Record<string, string>, options?: RequestOptions,
): Promise<T>;
export async function getJSON<T = any>(
  path: NativePath, params?: Record<string, string>, options?: RequestOptions,
): Promise<T>;
export async function getJSON<T = any>(
  path: RuntimePath, params?: Record<string, string>, options?: RequestOptions,
): Promise<T>;
```

- `getJSON` takes a **query bag** for a route that takes one, and **request
  options** for a parameterless read. So a parameter on a parameterless read
  does not type-check (`getJSON("/api/status", { limit: "5" })` is an error).
- `postJSON` takes a `JsonPath` — a body route. A `Write` route is not in that
  union, so `postJSON` on a query route does not type-check.
- `postAction` takes a `WritePath` — a query route.

**The runtime path.** A handful of components are handed a path as a prop
(`usePanel`, `cursorPager`, `LogDetail`). Their prop type is pinned to the
catalogue, but the *call* cannot check the literal again — there is no literal
to check. That gap is closed with a **brand**:

```ts
declare const runtimePathBrand: unique symbol;
export type RuntimePath = { readonly [runtimePathBrand]: true } & (ReadPath | ReadArgsPath | NativePath);
```

A path literal cannot be a `RuntimePath` (a string is not an object with that
property), so a literal is always matched against the catalogue's overloads,
and only a value the caller deliberately marked as runtime takes the last
overload. Exactly **two** call sites use it — `usePanel` and nothing else —
and both are marked with a comment saying why.

This was not the first design. A generic parameter
(`P extends ReadPath | ReadArgsPath | NativePath`) looked tidier, but an
explicit type argument at the call site (`getJSON<Payload>(…)` — 214 call
sites use one) makes `P` fall back to its default, and the literal's type is
never inferred. The hole was real and was found by mutation C; the brand
closes it.

### 3.5 `kernel/contract/opapi/native.go` — the console's own endpoints

The console answers 21 paths that are not operation bindings (the SPA shell,
login, the event feed, the file manager, voice, rollback, the workflow hook).
Their **names** lived in a test file, which is a place nobody reads. They now
live next to the catalogue, and the generated client publishes the 15 of them
under `/api/` as `NativePath`.

The console's route guard reads the same list and now holds it in **both**
directions: every native path is served, and nothing outside the catalogue and
the list is served.

## 4. What the browser looks like now

`frontend/src/app/api/api.ts` is **gone**. In its place:

- **`frontend/src/app/api/client.ts`** — GENERATED. The paths and the verbs.
- **`frontend/src/app/api/transport.ts`** — hand-written, and deliberately
  small: the bearer token, the error shape and the event feed's URL. Those are
  how the browser talks at all, not what it asks for.

The migration was mostly a **typing** exercise: 38 files changed, and the call
sites kept their shape. What moved:

| Change | Where | Why |
|---|---|---|
| 6 calls `postAction` → `postJSON` | Fleet, AgentDetail, Roster, ChannelSessions, Setup ×2 | the route is a body route |
| 2 template URLs → parameter bags | SuggestionsBar, Backups, Analyst, Reflect, market.ts | a hand-built URL cannot be checked |
| `path: string` → the catalogue's union | 12 components and tabs | the prop is now pinned to the console's surface |
| the dead `pending` parameter | MissionControl | the operation declares no input for it |
| the fake `/api/x` and `/api/foo` paths | two test files | a test may not name a path the console does not answer |

## 5. The catalogue's corrections

The catalogue was the source, so where the browser and the catalogue disagreed,
the **catalogue was measured against the registry's schema** and corrected
where the registry agreed with the browser:

| Path | Before | After | Why |
|---|---|---|---|
| 8 paths (§1.1) | `Write` | `JSON` | the browser sends a body; the operation's schema declares those fields; `bulk_forget`'s `ids` is an array and cannot travel in a query string |
| `/api/autonomy` | `Read` | `ReadArgs` (`limit`) | `autonomy_feed` declares `limit` |
| `/api/workflows` | `Read` | `ReadArgs` (`with_runs`) | `workflow_list` declares `with_runs` |

`/api/approvals` stays a parameterless read: `approvals` declares no input, so
the browser's `pending` parameter was dead and was removed rather than
honoured.

**The allowlist stays narrow on purpose.** The catalogue's argument list is
what the *browser* may set, not what the operation accepts; several routes
forward a subset of the operation's optional fields. That is the console's
posture, not a drift, and this slice did not widen it.

## 6. The guards

| Guard | Where | What it holds |
|---|---|---|
| `TestTheGeneratedArtifactsAreInSync` | `tools/openapi` | the committed document and client are exactly what the sources produce |
| `TestTheCatalogueAndTheRegistryAgree` | `tools/openapi` | every catalogue route names a registered operation **with** an input and an output contract |
| `TestTheBrowserOnlyNamesPathsTheConsoleAnswers` | `tools/openapi` | no file in `frontend/src` writes down a path the console does not answer (query strings and template suffixes trimmed) |
| `TestTheClientNamesEveryPathItServes` | `tools/openapi` | the client names every catalogue path |
| `TestTheDocumentIsValidJSON` | `tools/openapi` | the committed document parses and declares 203 paths |
| `TestConsoleServesEveryNativeEndpoint` | `kernel/webui` | the console's own endpoint list is served, in both directions |
| the **type checker** | `frontend` | the path, the verb and the parameter shape of every call |

Seven guards, and the last one is the strongest: it fires on the call site,
before the code runs.

## 7. The mutation evidence

Three mutations were applied and reverted. Each one was expected to turn a
gate red, and each was checked to do so.

**Mutation A — the generator drops a path.** The renderer was changed to leave
the last catalogue route out of the client's unions.

```
--- FAIL: TestTheGeneratedArtifactsAreInSync
    main_test.go:53: frontend/src/app/api/client.ts is stale — run `go run ./tools/openapi`
```

and, after regenerating with the mutation in place, the second gate:

```
--- FAIL: TestTheClientNamesEveryPathItServes
    main_test.go:187: the generated client does not name /api/world_log
```

**Mutation B — the catalogue's kind moves back.** `/api/agents/wake` was put
back to `Write` and the client regenerated. The browser's `postJSON` calls on
it must stop type-checking, because the route is a query route again:

```
src/components/Fleet.tsx(324,34): error TS2345:
  Argument of type '"/api/agents/wake"' is not assignable to parameter of type 'JsonPath | NativePath'.
src/features/agents/components/AgentDetail.tsx(403,51): error TS2345: …
src/features/agents/components/Roster.tsx(405,63): error TS2345: …
src/features/incidents/components/IncidentPage.tsx(572,18): error TS2345: …
```

**Mutation C — a parameter on a parameterless read.** A call
`getJSON<{ pending?: number }>("/api/status", { limit: "5" })` was added to a
browser file; `tsc` rejects it, because `status` is a parameterless read and
the second argument there is the request options:

```
src/features/observe/components/MissionControl.tsx(50,41): error TS2769: No overload matches this call.
  The last overload gave the following error.
    Argument of type '"/api/status"' is not assignable to parameter of type 'RuntimePath'.
```

**Mutation C is the one that changed the design.** With the first version of
the client — a generic parameter instead of overloads — mutation C did **not**
fire: an explicit type argument at the call site made the path's type fall
back to its default, so the literal was never checked. The hole was closed by
the branded `RuntimePath` (§3.4), and mutation C was re-run against it to prove
the hole is shut. A mutation that does not fire is a finding, not a nuisance.

All three mutations were reverted, and the tree was re-verified green.

## 8. Verification

| Gate | Result |
|---|---|
| `gofmt` | clean |
| `go build ./...` | clean |
| `go vet ./...` | clean |
| `go test ./...` | green — no package failed |
| `archcheck` | green — `opapi` (L1) imports nothing above it; `controlplane` (L5) reads L1; `tools/openapi` (L7) reads both |
| `deadcodecheck` | green — the new exported surface is used, not merely published |
| `depscheck` | green |
| `sdkparity` | green |
| `structure-md` | green — `.project/STRUCTURE.generated` regenerated for the new tool package |
| `changelog-lint` | green |
| `docclaimscheck` | green |
| `docsindex` | green — `docs/openapi.json` is not a document, so the index is untouched |
| `staticcheck` (`GOTOOLCHAIN=go1.27.1`) | green on the slice's packages; `S1016` (the options struct duplicating the info block) was the only finding and is fixed — the producer takes the `OpenAPIInfo` block directly now |
| `npm run typecheck` | green |
| `npm run deadcode` (knip) | green |
| `npm test` | the frontend suite's failures are unchanged from the pre-slice baseline (see §9) |

## 9. What did not change

- The console's behaviour for every route that was already correct. The
  catalogue's corrections change **ten** routes' argument sources; the other
  193 are byte-identical in behaviour.
- The daemon's operations, the registry, and the 324 operation contracts.
- The browser's request shape: the same verbs, the same bodies, the same
  query strings — except for the ten corrected routes and the two dead
  parameters that were removed.
- The console's native endpoints (login, files, voice, rollback, the event
  feed). They are still the console's own; they are now **named** in one place
  instead of a test file, and the browser's client publishes them as a type.

### The frontend suite's baseline

The frontend's vitest suite was already red before this slice: the machine's
Node build has no `localStorage` (`ExperimentalWarning: localStorage is not
available because --localstorage-file was not provided`), and the suite's
`beforeEach` hooks call `localStorage.clear()`. A clean `HEAD` worktree
(`b4e83c9e`, same `node_modules`) was measured to separate that from this
slice's effect:

| Run | Tests | Failed | Failing files |
|---|---|---|---|
| baseline (`b4e83c9e`, clean worktree) | 1 495 | 83 | 24 |
| this slice | 1 495 | **77** | **20** |

The slice's number is **lower** than the baseline's, and the difference is
where it should be: the four files whose expectations named the *wrong verb*
(`postAction("/api/agents/wake", …)` → `postJSON(…)`, and the same for
`/api/send` and `/api/provider/reload`) now pass. What remains is the same
`localStorage` class (69 of the 77 failures are
`Cannot read properties of undefined (reading 'clear')`) plus load-induced
`STACK_TRACE_ERROR`s.

The suite is also **flaky under parallel load**, which is why the numbers move
between runs: seven files that failed in the full run
(`cursorPager`, `Roster`, `Setup`, `IncidentPage`, `Schedules`, `Memory`,
`World`) pass **189/189** when the same seven are run on their own. That is
the pre-existing behaviour of this machine, not this slice's — the baseline
worktree shows it too.

## 10. What comes next

The document and the client are derived, but the browser's **payload types**
are still hand-written: 160 frontend interfaces describe what the registry
already describes. The next slice would generate the request and response
types from the same schema and let the frontend's own interfaces extend them
— which would make a renamed field a compile error in the browser too, the way
it is already a compile error in the daemon.

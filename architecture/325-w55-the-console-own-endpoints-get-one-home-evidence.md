# W5.5 — the console's own endpoints get one home

**Status:** delivered
**Date:** 2026-11-22
**Closes:** the open end W5.1 (both halves) and W5.4 left in the handoff —
*"the console's native endpoints are **named** in one place, but the console
still registers each one by hand."*

---

## 1. What was wrong

The console's HTTP surface has two halves. The first is **operation-bound**:
`kernel/contract/opapi/http.go` binds each path to the operation it answers,
and the console derives its routes from that catalogue (W3.82). The second half
is the console's **own** endpoints — the SPA shell, the auth surface, the event
feed, the file manager, the voice surface, the rollback surface and the
workflow hook. W5.1 moved their **names** into the same package
(`kernel/contract/opapi/native.go`), so the generated client and the console's
route guard read one source.

But "named in one place" was the whole of it. The measurement:

| What | Before |
|---|---|
| Console-native endpoints | 21 |
| Where the **path** was written | **twice** — the contract's list, and 21 `router.Handle("/api/…")` literals in `kernel/webui/webui_routes.go` |
| Where the **method** was written | once — in the console, in a local `RouteOpts` variable |
| Where the **tier** (public / protected) was written | once — in the console, as a local `RouteOpts` variable |
| Where the **mutation flag** was written | once — in the console, as a local `RouteOpts` variable |
| Console files holding a console path literal | **3** (`webui_routes.go`, `webui_security.go` — `"/hooks/"`, `webui_security_auth.go` — `"/events"`) |
| What the generated client published | one flat `NativePath` union of 15 paths, accepted by `getJSON` **and** `postJSON` **and** `postAction` |

The last row is the defect this slice closes. The published client said
*"you may GET `/api/login`"* and *"you may POST `/api/authmeta`"* — both false.
The console's router enforces the method policy it was given, so such a call
answers **405** at runtime. Nothing in the browser could catch it: the path was
in the union, the verb was not.

The other half of the gap was structural. The console's route guard checked
**paths** in both directions, but the path was the only thing it could check,
because the method lived in a local variable nobody else could see.

## 2. What was built

### 2.1 The contract states the whole promise

`kernel/contract/opapi/native.go` now carries a **structured** list — one
entry per console-native endpoint, with the four facts that make it a promise:

```go
type NativeRoute struct {
	Name     NativeRouteName // the console binds its handler by this name
	Path     string
	Method   string
	Public   bool
	Mutation bool
}
```

`Mutation` is **not** derived from the method, and that is not decoration: the
OAuth callback is a `GET` that stores a channel token. It has to be stated.

The names are constants (`opapi.NativeLogin`, `opapi.NativeWorkflowHook`, …),
so a handler cannot be bound to a name the contract does not carry, and the
console cannot write a path at all.

### 2.2 The console binds handlers by NAME

`kernel/webui/webui_routes.go` no longer registers anything by hand. It builds
a map keyed by the contract's **name** — never by a path — and registers the
contract's paths:

```go
for _, route := range opapi.ConsoleNativeRoutes() {
	binding, ok := bindings[route.Name]
	if !ok {
		panic("webui: no handler for the console-native endpoint " + string(route.Name))
	}
	…
	router.Handle(route.Path, opts, binding.handler)
}
```

The map says only what the contract cannot: **which handler answers** and what
the console allows the request to carry (a timeout, a body cap). The path, the
method, the tier and the mutation flag come from the contract.

Both halves of the promise are enforced **at start-up**: a name the contract
states with no handler, and a handler the contract does not name, both panic
when the router is built. A route nobody routes is a programmer error, and the
daemon refuses to start rather than serving a 404 the browser discovers later.

### 2.3 The client publishes the VERB

`tools/openapi/client.go` splits the console-native paths by the verb the
contract states:

```ts
export type NativeReadPath  = "/api/artifact/raw" | "/api/authmeta" | …;   // 7
export type NativeWritePath = "/api/files/delete" | "/api/login" | …;       // 8
```

`getJSON` now takes `NativeReadPath`, `postJSON` and `postAction` take
`NativeWritePath`. A wrong verb is a **compile error**, not a 405.

That tightened the browser: **15 files** carried `NativePath` where only one
verb was possible — 13 that POST (`ActionButton`, `AppNav`, `RunDetail`, the
skills / schedules / mcp / policy / standing pages, `AgentDetail` and the four
agent-detail tabs) and 2 that GET (`usePanel`, `Panel`). All 15 now name the
verb they can actually carry.

## 3. The guards

| Guard | What it holds |
|---|---|
| `TestConsoleNativeRoutesCarryTheContractPolicy` | the console answers each native endpoint with the method, the tier and the mutation flag the contract states |
| `TestConsoleNativeHandlersAreNamedByTheContract` | every handler the console holds is named by the contract, and every name the contract states has a handler |
| `TestConsoleDeclaresNoPathOfItsOwn` | **no file in the console may declare a console path at all** — not a catalogue route, not a native endpoint |
| `TestTheConsoleNativeListIsWellFormed` | no two native endpoints share a name or a path; every method is one the router can enforce |
| `TestConsoleNativePathIsTheRoutesPath` | the lookup the console uses to name its own endpoints answers the path the list states |
| `TestConsoleNativeWritesAreThePostOnes` | "is this endpoint a write?" has exactly one answer |
| `start-up panic` | a name with no handler, or a handler with no name, stops the daemon |

## 4. The mutation evidence

Four mutations, four catches, each reverted (the script restores the file
byte-for-byte).

**M1 — a console-native handler the contract does not name.**

```
opapi.NativeFavicon: {handler: handleFavicon},
+       opapi.NativeRouteName("ghost"): {handler: handleFavicon},
```

```
webui_routes_ratchet_test.go:153: the console holds a handler for "ghost",
which the contract does not name
```

**M2 — a console-native endpoint with no handler.**

```
-       opapi.NativeTTS: {handler: s.handleTTS, bodyMax: ttsTextMaxBytes},
```

```
webui_routes_ratchet_test.go:148: the contract names the console-native
endpoint "tts", but the console has no handler for it
```

(and, at start-up, the router panics: *"no handler for the console-native
endpoint tts"*)

**M3 — the console overrides the contract's method.**

```
-       Method:   route.Method,
+       Method:   "GET",
```

```
webui_routes_ratchet_test.go:121: the console answers /api/login with GET,
but the contract says POST            (and nine more)
```

**M4 — the browser calls a native write endpoint with GET.**

```diff
- await postJSON("/api/login", { password: pw });
+ await getJSON("/api/login", { password: pw });
```

```
src/features/auth/components/Login.tsx(51,21): error TS2769: No overload
matches this call.  Argument of type '"/api/login"' is not assignable to
parameter of type 'RuntimePath'.
```

Before this slice the same edit compiled: the client published every native
path in one union and every verb accepted it.

## 5. Verification

| Gate | Result |
|---|---|
| `gofmt -l kernel cmd tools internal` | empty |
| `go build ./...` | 0 |
| `go vet ./...` | 0 |
| `go test ./...` | 0 |
| `go run ./tools/archcheck -summary` | 340 packages, **0** violations, 0 allowlisted imports, 0 allowlisted calls |
| `go run ./tools/openapi -check` | in sync (document, client, types) |
| `go run ./tools/deadcodecheck` | 0 |
| `go run ./tools/depscheck` | 0 |
| `go run ./tools/sdkgen -check` | 0 |
| `go run ./tools/structure-md -check` | 0 |
| `go run ./tools/changelog-lint` | 0 |
| `go run ./tools/docclaimscheck` | 0 |
| `go run ./tools/docsindex` | 0 |
| frontend `npm run typecheck` | 0 |
| frontend `npx knip` | 0 |
| `cargo fmt --check` / `cargo test` (sdk/rust) | 0 / OK |
| `staticcheck` | **not run locally** — CI-only prebuilt binary (see 324 §7) |

The frontend suite's baseline is unchanged from 324: this machine's Node build
has no `localStorage`, so the suite is already red here, and the comparison is
by failing test id against the clean baseline, not by count.

Measured on this slice:

| | suites | tests | failing tests |
|---|---|---|---|
| baseline (`vitest_w51b_base.json`) | 540 | 1443 | 77 |
| this slice (`vitest_w55.json`) | 540 | 1443 | 78 |

Failing in both: **73**. Failing only in the baseline: **4**. Failing only in
this tree: **5** — `ConfigCenter`, `AgentActivity`, `Channels`, `IncidentPage`
and `Standing`, none of which this slice touches behaviourally (the change is
type-only: the TypeScript types are erased, and the committed bundle is
byte-identical). Run on their own, those eight files pass **91/92**, and the
one failure is `Data.test.tsx` — a file that was **already failing in the
baseline**. The suite fails a different set on each run on this machine; that
is the same flake 324 recorded, not a regression.

## 6. What did not change

- The console answers at the **same paths**, with the **same methods**, the
  **same tiers** and the **same mutation flags**. Every one of the 21
  endpoints was moved, not rewritten: the timeout and body cap each carried
  are the ones it carried before (the SPA shell 0, the login body cap
  `loginBodyLimit`, rollback apply 120 s / `jsonBodyMax`, artifact raw 15 s,
  the workflow hook 130 s / `webhookBodyCap`, the OAuth callback 30 s, the
  file routes `defaultFileCap`, transcribe `audioMaxBytes`, TTS
  `ttsTextMaxBytes`).
- The generated client publishes the same **15** console-native paths; only
  their verbs are now stated.
- The browser's behaviour is unchanged. Nothing in the browser called a native
  endpoint with the wrong verb today — the compiler just could not say so.

## 7. What comes next

- The browser's **view types** are still the browser's own names
  (`AgentProfile`, `ReaperReport`, `ApiOrder`). The daemon's rows are the
  truth; a browser that wants a narrower view should say so with `Pick`.
- `docs/openapi.json` carries the daemon's version, so a version bump makes it
  stale and the `-check` gate fails. That is intended, but the release step
  has to regenerate it.
- **W4** is still untouched: `modules/triggers` (W4.1), `modules/pulse`
  absorbing anomaly + alerter (W4.2), the channels supervisor and conversation
  store (W4.3), email DKIM/SPF (W4.4) and signed self-update (W4.5). Those are
  behaviour-changing waves, not cleanups, and they need the owner's decisions.

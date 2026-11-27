# W5.33 — the console's operation arguments are checked against the registry's declared request rows

## 1. What was wrong

The console is a surface that **sends** as well as one that answers. It sends
**407 arguments** to the daemon:

- **388** of them through the operation catalogue's route table — every route
  names the arguments it forwards, so the browser can only ever invoke an
  operation with these arguments;
- **10** more from the console's own handlers, which spell their argument maps
  out by hand (`map[string]any{"path": body.Path, "parents": body.Parents}`).

And the daemon's operation registry — the thing that says what each operation
**accepts**, with a JSON Schema that names the fields and marks the required
ones — was never compared with either list.

That is two homes for one row, and the row is the **request**: the console's
list of arguments and the operation's list of accepted fields. A row with two
homes drifts silently, because nothing says they must agree.

**One had already drifted.** The chat route (`POST /api/run`) forwarded
**`history`**:

```go
{Op: Run, Method: "POST", Path: "/api/run", Kind: opapi.HTTPRouteStream,
 Args: []string{"intent", "model", "history", "system", ...}},
```

but the `run` operation's request row declares **no** `history`:

```json
{"properties": {"agent": …, "assure": …, "auto_approve_caps": …, "dry_run": …,
                 "execution_profile": …, "images": …, "intent": …, "max_cost": …,
                 "model": …, "prompt_injection_trust": …, "remote_peer": …,
                 "system": …, "tenant": …, "timeout": …, "tools": …}}
```

Because `history` is the chat's prior turns, the console **consumed** it
itself: it folded the turns into the intent and dropped the key before the
call (`webui_security.go`). The daemon therefore never read it — it only
tolerated it, because the operation's schema allows unknown input. The
catalogue claimed the console forwarded it; the published OpenAPI document
repeated the claim ("forwards intent, model, history, system, …"); the browser
was told the body had a field the operation does not have.

The same blind spot covered the other direction: **no** check said that an
operation's **required** field must actually be sent. `skill_restore` requires
`id` and `status`; the console sends them today — by hand, by eye.

## 2. What was built

### 2.1 The route row says two things

`opapi.HTTPRoute` now separates the two lists that were conflated:

```go
type HTTPRoute struct {
	Op     string
	Method string
	Path   string
	Kind   HTTPRouteKind
	Args   []string
	// Consumed are the arguments the browser may send that the console
	// consumes itself and does not forward.
	Consumed []string
}
```

`Args` are the **operation's** arguments — every one is forwarded to the
daemon, so every one must be a field the operation's request row declares.
`Consumed` are the **console's own** inputs: the browser may send them, the
console reads them, and the daemon never sees them.

The chat route now says the truth:

```go
{Op: Run, Method: "POST", Path: "/api/run", Kind: opapi.HTTPRouteStream,
 Args: []string{"intent", "model", "system", "agent", "execution_profile", "auto_approve_caps"},
 Consumed: []string{"history"}},
```

`opapi.ConsoleArgs(op)` still returns the forwarded list; a handler that reads
the browser's body gets **both** lists from the route row itself, through
`decodeRouteBody`.

### 2.2 The console reads the body in two halves

The split is **structural**, not a convention:

```go
func (s *Server) decodeRouteBody(w http.ResponseWriter, r *http.Request, route opapi.HTTPRoute) (map[string]any, map[string]any, bool)
```

The forwarded arguments and the consumed ones come back in **separate maps**, so
a consumed argument cannot be forwarded by accident — the chat handler no
longer has to remember to `delete(args, "history")`. The `run` handler reads
the prior turns from the consumed half and folds them into the intent.

The console's own behaviour test already says so, and still passes:
`TestRunStreamFoldsHistoryIntoTranscript` sends the prior turns, asserts the
folded transcript reaches the daemon as the intent — and asserts that
`history` does **not** reach it.

### 2.3 The guard

`kernel/webui/operation_args_test.go` holds the console to the registry. It
reads the registry at build time (the operations are bound in package
initialisers, so no daemon and no socket are needed to ask what an operation
accepts) and it reads the console's **own sources** for the argument maps it
spells by hand — with `go/parser`, so the call sites are read as syntax, not as
text.

Three rules, one per way a request row drifts:

1. **an argument must be declared** by the operation's request row;
2. **a required field must be sent**;
3. **a consumed argument must not be the operation's field** — that would be one
   row with two homes (the console's own input and the operation's request).

The check is the console's own: a catalogue route is checked against its
operation's row, and a hand-written argument map is checked against the row of
the operation it names. The operation constants are resolved by reading the
contract's own source, so the guard never carries a second list of operation
names.

The guard's core (`checkArguments`) is kept apart from the test bodies so that
the **guard itself** can be measured: a check that is quietly dropped must
fail the guard's own test.

### 2.4 The document tells the truth

`docs/openapi.json` is derived, so it moved with the catalogue:

```
run — POST /api/run (forwards intent, model, system, agent, execution_profile, auto_approve_caps) (consumes history)
```

## 3. What was measured

| | before | after |
|---|---|---|
| arguments checked against the registry's request rows | **0** (of 407) | **407** (of 407) |
| forwarded arguments the request row does not name | **1** (`/api/run` → `history`) | **0** |
| required fields the console never sends | **0** | **0** |
| arguments with two homes (consumed **and** the operation's field) | **0** | **0** |
| catalogue routes checked | **0** (of 203) | **203** |
| hand-written argument maps checked | **0** (of 10) | **10** |

The measurement script (`.temp_files/w533_measure.py`) reads the catalogue
from `HEAD` and from the working tree and compares both with the registry.

## 4. The mutations

Five reverted mutations prove the slice (`.temp_files/w533_mutations.py`; every
mutation was reverted and the tree is back to green).

**M1 — the console's hand-written argument key is misspelled.**

```
--- FAIL: TestTheConsoleSendsOnlyDeclaredArguments (0.01s)
```

**M2 — the catalogue forwards an argument the operation never declared.**

```
--- FAIL: TestTheConsoleRoutesForwardOnlyDeclaredArguments (0.00s)
```

**M3 — the operation's request row loses a field the console sends.**

```
--- FAIL: TestTheConsoleSendsOnlyDeclaredArguments (0.01s)
```

**M4 — the guard loses its eye for a required field.**

```
--- FAIL: TestTheGuardReportsBothWaysOfDrifting (0.00s)
```

**M5 — the catalogue consumes an argument that is also the operation's field.**

```
--- FAIL: TestTheConsoleRoutesForwardOnlyDeclaredArguments (0.00s)
```

## 5. What is untouched

- **No daemon behaviour changed.** The same arguments reach the same
  operations; `history` still reaches the console, and it still never reaches
  the daemon.
- **The published OpenAPI document moved in one line** — the chat route's
  summary — and says the truth now.
- **No layer rule moved.** `archcheck` still reports 0 violations: the guard
  reads the registry from the console's own layer, and the route row grew a
  field in the contract the catalogue already lives in.
- **The gate is green**: `gofmt`, `go build ./...`, `go vet ./...`,
  `go test ./...`, `depscheck`, `sdkgen -check`, `deadcodecheck`, `archcheck`,
  `bodydoor`, `structure-md -check`, `openapi -check`, `docsindex`,
  `changelog-lint`, `docclaimscheck`, `gitleaks`.

## 6. What remains open

- **The console's ten hand-written argument maps are still hand-written.** The
  guard holds them to the registry; the request row itself (the operation's
  input schema) is still a JSON literal in the app layer, and the app layer's
  own request structs are a second spelling of the same fields. The next step
  is a contract-owned request row per operation, derived by both sides.
- **The catalogue still does not cover every operation the console binds.** Ten
  operations the console calls (the file mutations, the restores, the OAuth
  callback, the workflow hook) are not in the route table — their paths live
  in the console's native route list, and their argument maps in the console's
  handlers.
- **The catalogue's `Args` and the operation's request row are still two
  lists.** The guard compares them at build time; deriving one from the other
  would make the comparison unnecessary.
- **The console's route policy answers a wrong verb in plain text**
  ("method not allowed"), not with the envelope — a second refusal shape on
  the wire, still a decision to take.

# W5.34 — the console's ten operations take their request row from the contract, and the schema is derived from it

## 1. What was wrong

W5.33 gave the console's **407 arguments** a second home: the catalogue's route
table said what each route forwards, and the registry said what each operation
accepts — and a new guard compared the two. The comparison found one drift
(`/api/run` forwarded `history`) and closed it.

But the comparison was against a row that itself had **two homes**. The
operation's request shape was declared **twice**:

- in the **app layer**, as a JSON Schema literal — the thing the registry
  publishes and the daemon's door validates against;
- in the **surface**, as a second literal — the console's own request struct, or
  its hand-written argument map.

Ten operations the console calls were in that state:

| operation | the app layer's literal | the console's own spelling |
|---|---|---|
| `artifact_get` | `getRequestSchema` | `map[string]any{"ref": ref}` |
| `file_mkdir` | `json.RawMessage(d.input)` | `map[string]any{"path": …, "parents": …}` |
| `file_rename` | `json.RawMessage(d.input)` | `map[string]any{"from": …, "to": …}` |
| `file_delete` | `json.RawMessage(d.input)` | `map[string]any{"path": …, "recursive": …}` |
| `file_restore` | `{"id":{"type":"string"}}` | `map[string]any{"id": …}` |
| `skill_restore` | *already derived* | `map[string]any{"id": …, "status": …, "reason": …}` |
| `workflow_restore` | the shared 14-field `requestSchema` | `map[string]any{"workflow": …, "reason": …}` |
| `workflow_webhook` | the shared 14-field `requestSchema` | `map[string]any{"ref": …, "secret": …, "payload": …}` |
| `config_set` | the settings family's `settingsInputSchema` | `map[string]any{"name": …, "value": …}` |
| `channel_oauth_callback` | `{"code":{"type":"string"},"state":{"type":"string"}}` | `map[string]any{"code": …, "state": …}` |

**Nine of the ten** carried a hand-written schema literal in the app layer — the
tenth (`skill_restore`) was derived from the app's own struct, which is the
accident of that slice, not a decision.

Two spellings of one row drift. The guard could not see it happen: it compared
the console's list with the **registry's** list, and both lists could move
together, or the app's literal could say one thing while the console's map said
another.

One had **already** drifted, and silently: `config_set`'s published schema was
the settings **family's** schema (name, value, section, id, force) while the
operation is only ever asked for **name** and **value** — and the browser's
generated type for it was an **alias of the wrong row**
(`ConfigSchemaRegisterRequest`), because the SDK generator read the app layer's
literal rather than the row.

## 2. What was built

### 2.1 The request row lives in the contract

`kernel/contract/opapi/requests.go` declares the ten request rows, next to the
operation names they belong to:

```go
type FileMkdirRequest struct {
	Path    string `json:"path,omitempty"`
	Parents bool   `json:"parents,omitempty"`
}
```

and binds them to their operations in one ordered table:

```go
var RequestRows = []RequestRow{
	{Operation: ArtifactGet, Row: ArtifactGetRequest{}},
	{Operation: FileMkdir, Row: FileMkdirRequest{}},
	…
}
```

The rules are the tree's own:

- a field **without** `omitempty` is **required** — the door refuses a request
  that does not carry it (`skill_restore`'s `id` and `status`);
- a field **with** `omitempty` may be absent, and `Args` leaves it out when it
  holds its zero value — exactly as the wire has always carried it.

`opapi.Args(row)` turns a row into the operation's arguments, reading the field
names from the row's own json tags — so a caller **cannot** spell an argument
the row does not carry.

`AnyValue` is the row's way of saying "any JSON value": the webhook's payload.
It rides the wire as the value itself, and the schema derived from the row
names the union of the JSON types rather than an empty property that says
nothing.

### 2.2 The app layer no longer writes the schema

The framework already derived an operation's input schema when the spec does
not carry one (`app.operationSchema` → `schema.FromType`). The app layer's job
was therefore only to **stop writing it by hand**:

- `artifacts`: `GetRequestInput` and `getRequestSchema` are gone;
- `files`: the shared `MutationRequest`/`args()` and the `RestoreRequest`
  literal are gone — the four file operations each take their own row;
- `skill`: `RestoreInput` is gone, the row is the contract's;
- `settings`: `SetRequest` is gone — the family's literal still serves
  `config_schema`/`config_schema_register`/`config_schema_unregister`, but
  `config_set` is **derived** now;
- `channels`: the callback's literal is gone — the callback's schema is derived
  from the row;
- `workflow`: `workflow_restore` and `workflow_webhook` bind through a new
  `bindRow`, which derives the schema from the row; the family's other eight
  operations keep the shared `requestSchema`.

### 2.3 The console sends rows, not maps

The console's ten call sites now pass the operation's **own row**:

```go
res, err := s.client.Call(ctx, opapi.ArtifactGet, opapi.Args(opapi.ArtifactGetRequest{Ref: ref}))
```

and the console's own **body** structs became **aliases** of the contract's rows
(`fileMkdirBody` is `opapi.FileMkdirRequest`), so the console's content and the
daemon's request are one declaration.

### 2.4 The guard now reads three rules instead of comparing two lists

`kernel/webui/operation_args_test.go` was rewritten. W5.33's rules (an argument
must be declared, a required field must be sent, a consumed argument must not
be the operation's field) stay — and three more rules replace the comparison of
two spellings:

1. **an operation call must pass the operation's row through `opapi.Args`** — a
   hand-written argument map, direct or through a variable, is **refused**;
2. **the row a call passes must be the operation's own row** — and every
   operation the contract gives a row to, the console calls with that row;
3. **the registry's declared input for an operation with a contract row must BE
   the schema derived from that row** — so the published contract and the row
   cannot drift.

The third rule is the one that closes the loop: with it, a hand-written schema
beside a row is not a second spelling any more — it is a **failure**.

Two kinds of call are told apart, because they forward different things: a call
that **builds** an operation's request forwards the row (the console's ten), and
a call that **relays** what the browser sent forwards the catalogue's declared
arguments (the streaming proxies) and names no row at all. The guard reads the
console's own **type aliases** too — the file handlers' body structs ARE the
contract's rows (`fileMkdirBody = opapi.FileMkdirRequest`), so a call that
forwards the body forwards the row, and the guard has to see through the alias.
A row it cannot read is a call it **refuses**, not one it skips: a guard that
quietly stopped looking would pass while the console drifted.

The guard's core is kept apart from the test bodies (`checkArguments`,
`declaredInputIsTheRow`, `sameSchema`), and the guard has **its own tests**
(`TestTheGuardSeesASchemaOfItsOwn`, `TestTheGuardSeesASecondSpelling`), so a
comparison that quietly stops comparing fails the guard rather than passing
silently.

## 3. What was measured

| | before | after |
|---|---|---|
| hand-written operation argument maps in the console | **10** | **0** |
| arguments the console spells out by hand | **20** | **0** |
| request rows held by the contract | **0** | **10** |
| operations whose input schema is written by hand in the app layer | **9** (of 10) | **0** |

The measurement script (`.temp_files/w534_measure.py`) reads `HEAD` and the
working tree and counts the same four things in both.

## 4. The mutations

Five reverted mutations prove the slice (`.temp_files/w534_mutations.py`; every
mutation was reverted and the tree is back to green).

**M1 — the console spells an operation's arguments by hand.**

```
--- FAIL: TestTheConsolePassesTheOperationsOwnRow (0.01s)
```

**M2 — the console calls an operation with the wrong row.**

```
--- FAIL: TestTheConsolePassesTheOperationsOwnRow (0.01s)
```

**M3 — the app layer puts a hand-written schema beside the row.**

```
--- FAIL: TestTheRegistrysDeclaredInputIsTheRow (0.00s)
```

**M4 — the contract's row drops a field the route forwards.**

```
--- FAIL: TestTheConsoleRoutesForwardOnlyDeclaredArguments (0.00s)
```

**M5 — the guard's schema comparison quietly stops comparing.**

```
--- FAIL: TestTheGuardSeesASchemaOfItsOwn (0.00s)
```

## 5. What is untouched

- **No daemon behaviour changed.** The same arguments reach the same
  operations; `skill_restore`'s required `id` and `status` are still required.
- **The published OpenAPI document moved in one place**: `config_set`'s body is
  now the derived `{name, value}` instead of the settings family's five-field
  schema. The browser's generated type for it stopped being an alias of the
  wrong row (`ConfigSchemaRegisterRequest`) and became its own interface.
- **The workflow family's other eight operations keep their shared
  `requestSchema`** — this slice covered the operations the console calls, not
  the whole workflow family.
- **No layer rule moved.** `archcheck` reports 0 violations: `opapi` is a
  contract (L1) and imports only another contract (`skillapi`); the app layer
  and the console read the rows from there.
- **The gate is green**: `gofmt`, `go build ./...`, `go vet ./...`,
  `go test ./...`, `depscheck`, `sdkgen -check`, `deadcodecheck`, `archcheck`,
  `bodydoor`, `structure-md -check`, `openapi -check`, `docsindex`,
  `changelog-lint`, `docclaimscheck`, `gitleaks`, `staticcheck`.

## 6. What remains open

- **The workflow family is still half-spelled.** Eight of its operations share
  one hand-written 14-field schema; only `workflow_restore` and
  `workflow_webhook` have rows. The next slice can give the family one row per
  operation and retire `requestSchema`.
- **The other families keep their literals too.** `config_schema_register`,
  `skill_hygiene`, `skill_import`, the artifact family's list/delete/collect
  rows — each still writes its own schema in the app layer. The rule that
  stopped them is now in place; the rows are the remaining work.
- **The catalogue's `Args` and the request row are still two lists.** The guard
  compares them at build time; deriving the route's forwarded list from the row
  would make the comparison unnecessary — the same move this slice made for the
  schema.
- **The console's ten native routes are not in the catalogue's route table.**
  The file mutations, the restores, the OAuth callback and the workflow hook
  are served by the console's own native route list, so their paths are not
  declared in one place yet.
- **Type mismatches now fail at the door, in the schema's words.** The OAuth
  callback used to treat a non-string value as an empty one; it is refused by
  the schema now. The result is the same, the wording is not.

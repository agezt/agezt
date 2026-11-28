# W5.35 — the workflow family has one request row per operation, and the shared schema is retired

## 1. What was wrong

W5.34 gave the console's ten typed calls their request rows and left the
**workflow family** half-migrated: thirteen operations, **two** of them with a
row (`workflow_restore`, `workflow_webhook`), the other **eleven** sharing two
hand-written declarations:

```go
// one schema for eleven operations
var requestSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{
  "ref":{"type":"string"},"workflow":{"type":"object"},"reason":{"type":"string"},
  "enabled":{"type":["boolean","string"]},"async":{"type":"boolean"},
  "payload":{"type":["object","array","string","number","boolean","null"]},
  "with_runs":{"type":["boolean","string"]},"limit":{"type":"number"},
  "name":{"type":"string"},"description":{"type":"string"},"instruction":{"type":"string"},
  "secret":{"type":"string"},"node":{"type":"string"},"data":{"type":"object"}}}`)

// one struct for eleven operations
type RequestInput struct { /* all fourteen fields */ }
```

Every operation in the family therefore declared **every** field: the eleven
operations that shared the schema declared **154** fields between them, while
the family's real vocabulary is **25**. `workflow_list` published a `secret`, a
`node` and a `payload`; `workflow_test_node` published a `name` and an
`instruction`. Nothing said so, because the shape was one shape.

The readings were hand-written too, in the app layer, on top of
`json.RawMessage`:

- `requestFlag` — the strict reading: `true`, `"true"`, `"1"`, `"yes"` are on,
  a typo is **refused** (`args.with_runs must be a boolean`);
- `requestEnabled` — the lenient reading: `true`, `"true"`, `"1"` are on,
  anything else reads as **false**;
- `requestText`, `requiredText`, `requestAsync`, `requestLimit`, `requestGraph`.

Two flag readings, two spellings of "a boolean a query string carries", and a
limit that could not tell "no limit asked" from "zero" without a
`json.RawMessage` to hold the difference.

## 2. What was built

### 2.1 One row per operation, in the contract

Eleven rows join the two W5.34 already had:

```go
type WorkflowShowRequest struct {
	Ref string `json:"ref,omitempty"`
}
```

The family is now **thirteen rows**, one per operation, bound to their
operations in the contract's own table. The shared `RequestInput` and the
shared `requestSchema` are **gone**; the app layer binds each operation
through `bindRow`, and the declared input is derived from the row.

### 2.2 The row carries the reading, not just the shape

Three field types say what the wire may carry, and the schema derived from
them says exactly that:

- **`BoolFlag`** — a boolean, or the **string** a browser's query carries for
  one. The schema derives the union `["boolean","string"]` — the same shape
  the old literal had — and the two readings the family always had now live
  with the row: `BoolFlagValue` (strict: a typo is refused) and
  `BoolFlagOrFalse` (lenient: anything it does not know is off).
- **`Object`** — a JSON object, and **not** "an object or null": a missing
  object and a null one are different answers. The schema derives
  `{"type":"object"}` — exactly what the old literal said.
- **`Limit`** — a number, and it knows whether it was sent at all, so "no
  limit asked" and "zero" stay different answers.

The schema package learned three **structural markers** for this — the same
shape as W5.34's `AnyJSONValue`: a method is enough, so the package needs no
import from the contracts.

```go
type booleanOrString interface{ BooleanOrString() }
type jsonObject       interface{ JSONObject() }
type jsonNumber       interface{ JSONNumber() }
```

### 2.3 The published document narrowed, on purpose

Each operation now publishes **its own** body: `workflow_list` publishes
`with_runs`, `workflow_show` publishes `ref`, `workflow_test_node` publishes
`workflow`, `node`, `data` and `payload`. The family's declared request fields
went from **170** to **25** — every field an operation actually reads, and
nothing else.

The door still refuses what it refused: a mistyped `data`, a null `limit`, a
`reason` that is a boolean — the same bodies, in the same words, at the same
moment (before the graph is read).

### 2.4 The guard's forward rule was replaced, with a reason

W5.34's guard held the console's typed calls to their rows, and it also
required that **every** row in the contract's table be an operation the
console calls with a typed row. That was true when the table held the
console's ten, and it is false now that the table holds the whole family's
rows: the console calls `workflow_show` through the **catalogue's proxy**,
which forwards the catalogue's declared arguments.

The rule was replaced by the one that carries its meaning: **every row in the
contract's table must be the operation's own declared input** (W5.34's third
rule), which now covers **twenty-one** rows. A row nobody calls with is still
caught — not by the console's call list, but by the registry: a row whose
operation the registry does not declare, or whose declared input is not the
row, fails.

## 3. What was measured

| | before | after |
|---|---|---|
| request rows held by the contract | **10** | **21** |
| workflow operations with a row | **2** (of 13) | **13** (of 13) |
| hand-written input schemas in the workflow family | **1** | **0** |
| the family's shared request struct | **1** | **0** |
| request fields the family declares | **170** | **25** |

The measurement script (`.temp_files/w535_measure.py`) reads `HEAD` (W5.34's
merge) and the working tree and counts the same five things in both.

## 4. The mutations

Five reverted mutations prove the slice (`.temp_files/w535_mutations.py`; every
mutation was reverted and the tree is back to green).

**M1 — the app puts a hand-written schema back beside the family's rows.**

```
--- FAIL: TestTheRegistrysDeclaredInputIsTheRow (0.00s)
```

**M2 — a contract row drops a field the catalogue's route forwards.**

```
--- FAIL: TestTheConsoleRoutesForwardOnlyDeclaredArguments (0.00s)
```

**M3 — the flag reader stops refusing a typo.**

```
--- FAIL: TestBoolFlagValueRejectsATypoRatherThanReadingItAsFalse (0.00s)
```

**M4 — the guard's schema comparison quietly stops comparing.**

```
--- FAIL: TestTheGuardSeesASchemaOfItsOwn (0.00s)
```

**M5 — the console calls `workflow_restore` with another operation's row.**

```
--- FAIL: TestTheConsolePassesTheOperationsOwnRow (0.01s)
```

## 5. What is untouched

- **No daemon behaviour changed.** The door refuses the same mistyped bodies
  in the same words; the `enabled` flag's lenient reading and the `with_runs`
  flag's strict one are both kept, because they were different on purpose.
- **The SDK catalogs did not move** — the workflow operations are not in them.
- **The browser's generated types moved only in the workflow request bodies**,
  and its typecheck is green.
- **No layer rule moved.** `archcheck` reports 0 violations: `opapi` is a
  contract (L1) and imports only another contract; the schema package's
  markers are structural.
- **The gate is green**: `gofmt`, `go build ./...`, `go vet ./...`,
  `go test ./...`, `depscheck`, `sdkgen -check`, `deadcodecheck`, `archcheck`,
  `bodydoor`, `structure-md -check`, `openapi -check`, `docsindex`,
  `changelog-lint`, `docclaimscheck`, `gitleaks`, `staticcheck`.

## 6. What remains open

- **The other families still write their own schemas.** The app layer carries
  **161** hand-written input schemas: the board's seven, the workboard's
  nineteen, the artifacts' three, the taste family's three, the skill family's
  two, the standing and okr families, the world's, the settings' remaining
  two. The rule that stopped them is in place; their rows are the work.
- **The catalogue's `Args` and the request row are still two lists.** The guard
  compares them at build time; deriving the route's forwarded list from the row
  would make the comparison unnecessary — the same move this slice made for the
  schema.
- **The console's ten native routes are not in the catalogue's route table.**
- **The console's route policy answers a wrong verb in plain text**
  ("method not allowed"), not with the envelope — a second refusal shape on the
  wire.
- **`workflow_templates`' row names no field**, which the contract's own test
  holds against the catalogue: an operation the console forwards arguments to
  must have a row that names them.

# W5.36 — the board family has one request row per operation, and the page size keeps its rules

## 1. What was wrong

The **board** is the surface where agents talk to each other, and its seven
operations had their request shape declared **twice**: as a JSON Schema
literal in the app layer, and as a request struct beside it.

```go
// the app layer's copy
type ReadRequestInput struct {
	Topic  string `json:"topic,omitempty"`
	Cursor string `json:"cursor,omitempty"`
	Limit  any    `json:"limit,omitempty"`
}

var readRequestSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"topic":{"type":"string"},"cursor":{"type":"string"},"limit":{"type":"number"}}}`)
```

Six request structs and **four** hand-written schemas served seven operations —
and the board's **send** row was already a contract row (`boardapi.Draft`),
which made the family look finished when it was not: the send row had a home
in the contract, and the other six had theirs in the application layer.

The page size was the sharpest edge. The board reads it as a **number that may
be a fraction**, and the rules are not obvious:

| the row says | the board reads |
|---|---|
| nothing | **50** (the family's default) |
| `0` or below | **50** — no page size was asked for |
| `0.5` | **0** — a fraction is not a page, so the store reads every message |
| `1.9` | **1** |
| `9999` | **500** (the cap) |

That rule was spread over three places: the literal (which said "number"), the
struct (which said `any`), and the reader (`admittedLimit`, which knew that a
fraction is not a page but that a zero is no answer at all).

## 2. What was built

### 2.1 Six rows join the send row

The board's six remaining rows live in the contract now, next to the send row
the board already had:

```go
type BoardReadRequest struct {
	Topic  string `json:"topic,omitempty"`
	Cursor string `json:"cursor,omitempty"`
	Limit  Limit  `json:"limit,omitempty"`
}
```

The app layer keeps no copy: the six request structs and the four schema
literals are **gone**, and each operation binds through the contract's row.
The board's send row stays where it was — `boardapi.Draft`, the wire row the
REST surface decodes — and it joins the contract's row table, so the guard
holds it too.

### 2.2 The page size is a number that knows whether it came

`opapi.Limit` (W5.35's type) carries the number **and** whether it was sent at
all, so "no limit asked" and "a limit of zero" stay different answers. The
board's own rules — the default, the fraction, the cap — stay in the board:
they are the board's vocabulary, not the contract's.

```go
func admittedLimit(limit opapi.Limit) int {
	size := 50
	if limit.Present() && limit.Number() > 0 {
		size = int(limit.Number())
	}
	if size > 500 {
		size = 500
	}
	return size
}
```

### 2.3 The published document did not move

The board's four literals already said what the rows now say — the same
fields, the same types — so `docs/openapi.json` and the SDK catalogs are
**byte-identical**. This slice moved where the shape lives, not what it says.

## 3. What was measured

| | before | after |
|---|---|---|
| request rows held by the contract | **21** | **28** |
| board operations with a row | **0** (of 7) | **7** (of 7) |
| hand-written input schemas in the board family | **4** | **0** |
| hand-written request structs in the board family | **6** | **0** |
| hand-written input schemas in the app layer | **161** | **157** |

The measurement script (`.temp_files/w536_measure.py`) reads `HEAD` (W5.35's
merge) and the working tree and counts the same five things in both.

## 4. The mutations

Five reverted mutations prove the slice (`.temp_files/w536_mutations.py`; every
mutation was reverted and the tree is back to green).

**M1 — the app puts a hand-written schema back beside the board's rows.**

```
--- FAIL: TestTheRegistrysDeclaredInputIsTheRow (0.00s)
```

**M2 — a board row drops a field the catalogue's route forwards.**

```
--- FAIL: TestTheConsoleRoutesForwardOnlyDeclaredArguments (0.01s)
```

**M3 — the board's page-size rule changes.**

```
--- FAIL: TestBoardTypedAdmissionRetainsLimitsAndRefusesMistypedText (1.07s)
```

**M4 — the guard's schema comparison quietly stops comparing.**

```
--- FAIL: TestTheGuardSeesASchemaOfItsOwn (0.00s)
```

**M5 — the limit type loses "not asked" against "zero".**

```
--- FAIL: TestTheLimitKnowsWhetherItCame (0.00s)
```

## 5. What is untouched

- **No daemon behaviour changed.** The board's page sizes, its defaults, its
  cap and its refusals are the same; the door still names a mistyped limit
  (`$.limit has wrong type: got string, want number`).
- **The published document and the SDK catalogs did not move** — the board's
  rows already said what the board's rows say.
- **The REST surface is untouched**: it decodes the board's send row
  (`boardapi.Draft`) and posts the same fields.
- **No layer rule moved.** `archcheck` reports 0 violations: `opapi` and
  `boardapi` are both contracts (L1), and a contract may read another.
- **The gate is green**: `gofmt`, `go build ./...`, `go vet ./...`,
  `go test ./...`, `depscheck`, `sdkgen -check`, `deadcodecheck`, `archcheck`,
  `bodydoor`, `structure-md -check`, `openapi -check`, `docsindex`,
  `changelog-lint`, `docclaimscheck`, `gitleaks`, `staticcheck`.

## 6. What remains open

- **The app layer still carries 157 hand-written input schemas.** The biggest
  families left are the **workboard** (21), the **roster** (19), the **edict**
  (11), the **runs** (8), the **okr** (7), the **memory** (7), the **tenants**
  (6), the **steer** (6), the **journal** (5), the **audit** (5), the
  **channels** (6), the **tools** (5), the **approvals** (4), the **system**
  (4), the **taste** (3), the **edict**'s writes (4) and reads (3), the
  **artifacts**' three, the **skill**'s two, the **settings**' remaining two.
  The rule is in place; their rows are the work.

  **Done in W5.37** — the workboard family has one row per operation now; see
  [357](357-w537-the-workboard-family-has-one-request-row-per-operation-evidence.md).
- **Those families read their fields loosely** — `any` and `json.RawMessage`
  fields with hand-written readers that tolerate what the door would refuse.
  Their rows will need the same reading-rules contract the board's page size
  just got.
- **The catalogue's `Args` and the request row are still two lists.**
- **The console's ten native routes are not in the catalogue's route table.**
- **The console's route policy answers a wrong verb in plain text**
  ("method not allowed"), not with the envelope — a second refusal shape on the
  wire.

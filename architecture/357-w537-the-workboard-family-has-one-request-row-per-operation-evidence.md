# W5.37 — the workboard family has one request row per operation, and the counts and lists keep their rules

## 1. What was wrong

The **workboard** is the board of work — the tasks an agent claims, proves and
finishes — and its **twenty-one** operations declared their request shape
**twice**: as a JSON Schema literal in the app layer, and as a request struct
beside it.

```go
// the app layer's copy
type SweepRequestInput struct {
	Actor         any `json:"actor,omitempty"`
	StaleAfterMS  any `json:"stale_after_ms,omitempty"`
	Limit         any `json:"limit,omitempty"`
	CorrelationID any `json:"correlation_id,omitempty"`
}

var sweepRequestSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"actor":{"type":"string"},"stale_after_ms":{"type":"number"},"limit":{"type":"number"},"correlation_id":{"type":"string"}}}`)
```

**Fifteen** request structs and **fifteen** hand-written literals served the
twenty-one operations, and every field was `any` — so the shape was declared by
a literal while the handler read it with hand-written readers that tolerated
whatever arrived:

```go
func requestText(raw any) string { s, _ := raw.(string); return strings.TrimSpace(s) }
func requestLimit(raw any, def int) int { /* float64, int, int64 — and a default */ }
func requestStrings(raw any) []string { /* a list — or ONE comma-separated string */ }
```

The rules those readers carried were the family's own, and they lived in
exactly one place — which is why the family could not say them out loud:

| the row says | the workboard reads |
|---|---|
| a count that was not sent | the family's default (100 for a listing, 500 for the lanes, 10 minutes for a stale claim) |
| a count of `0`, a negative count, a fraction | the family's default — a count that is not a count is no count |
| `1.9` | **1** |
| `9999` on a sweep | **1000** (the family's cap) |
| `9999` on a watch | **200** (the family's cap) |
| a list of tags | the list, trimmed, and the members that say nothing are dropped |
| **one** comma-separated string | the string, split on commas |

## 2. What was built

### 2.1 Fifteen rows serve twenty-one operations

The workboard's rows live in the contract now (`kernel/contract/opapi/requests.go`),
beside the workflow and board rows:

```go
// WorkboardListRequest is the workboard_list request row: which tasks to
// list, and how many.
type WorkboardListRequest struct {
	Status          string `json:"status,omitempty"`
	Tenant          string `json:"tenant,omitempty"`
	Assignee        string `json:"assignee,omitempty"`
	IncludeArchived bool   `json:"include_archived,omitempty"`
	Limit           Limit  `json:"limit,omitempty"`
}
```

The twenty-one operations share **fifteen** rows, and the sharing is the
family's own vocabulary:

- the **lanes** read what the listing reads — one listing row, two operations;
- the **heartbeat** carries what the **claim** carries — one claim row, two
  operations;
- the five **reasoned mutations** — block, fail, unblock, complete, archive —
  are one reasoned mutation: the task, who acts, and why.

The app layer keeps no copy: the fifteen request structs and the fifteen
literals are **gone**, and every operation binds through the contract's row.
All twenty-one operations are held by the guard, and the contract's row table
grows from **28** to **49** entries.

### 2.2 The counts keep the family's rules — and say "not asked" apart from "zero"

`opapi.Limit` (W5.35's type) carries the number **and** whether it was sent at
all, so "no count asked" and "a count of zero" stay different answers. The
family's own rules stay in the family:

```go
// admittedLimit reads a count the row asked for: the family's default when the
// row did not ask for one — or asked for one that is not a count.
func admittedLimit(limit opapi.Limit, fallback int) int {
	if !limit.Present() {
		return fallback
	}
	size := int(limit.Number())
	if size < 1 {
		return fallback
	}
	return size
}
```

The retry policy is where the distinction earns its keep: a policy that asks
for **no** attempts and a policy that asks for **zero** attempts are different
answers, and the family answers them differently — the first is refused
("workboard_policy requires max_attempts or clear"), the second is refused too
("workboard_policy max_attempts must be positive"), and a policy that asks for
three is a policy.

### 2.3 The list that may be one string

The workboard's tag lists are **lists of texts — or the ONE comma-separated
string a caller sends instead of a list**. That shape now has a home in the
contract:

```go
// TextList is a request field that carries a list of texts — or the ONE
// comma-separated string a caller sends instead of a list.
//
// The members stay unconstrained on purpose: the handler filters the ones it
// cannot use rather than refusing the whole body, so the list says "texts" and
// the reading says which of them count.
type TextList struct { … }
```

and the schema package learned the marker that derives it — a method is
enough, so it needs no import from the contracts:

```go
// textList is the marker a request field carries when it holds a list of
// texts — or the ONE comma-separated string a caller sends instead of a list.
type textList interface{ TextList() }

var textListTypes = []string{"array", "string"}
```

The derived property is exactly the one the old literal said —
`{"type":["array","string"]}` — and the members stay unconstrained, because
the family **filters** the ones it cannot use instead of refusing the body.

The reading is the family's own, and it is deliberately not symmetrical:

- a **list**'s members are trimmed, and the members that say nothing are
  dropped;
- a **string** is split on commas **as it came** — its parts are not trimmed,
  because that is how the family has always read it.

### 2.4 The published document did not move

The workboard's fifteen literals already said what the fifteen rows now say —
the same fields, the same types — so `docs/openapi.json`, the generated
client and the browser's generated types are **byte-identical**. This slice
moved where the shape lives, not what it says.

## 3. What was measured

| | before | after |
|---|---|---|
| request rows held by the contract | **28** | **49** |
| workboard operations with a row | **0** (of 21) | **21** (of 21) |
| workboard row types living in the contract | **0** | **15** |
| hand-written input schemas in the workboard family | **21** | **0** |
| hand-written schema literals in the workboard family | **15** | **0** |
| hand-written request structs in the workboard family | **15** | **0** |
| hand-written input schemas in the app layer | **157** | **136** |

The measurement script (`.temp_files/w537_measure.py`) reads `HEAD` (W5.36's
merge, `62dd6681`) and the working tree and counts the same things in both.

## 4. The mutations

Five reverted mutations prove the slice (`.temp_files/w537_mutations.py`; every
mutation was reverted and the tree is back to green).

**M1 — the app puts a hand-written schema back beside the workboard's rows.**

```
--- FAIL: TestTheRegistrysDeclaredInputIsTheRow (0.00s)
```

**M2 — the contract's list reading breaks.**

```
--- FAIL: TestTheTextListReadsAListOrTheOneStringThatCarriesIt (0.00s)
```

**M3 — the family's page-size rule changes.**

```
--- FAIL: TestWorkboardTypedAdmissionRetainsLimitsAndRefusesMistypedText (0.01s)
```

**M4 — the guard's schema comparison quietly stops comparing.**

```
--- FAIL: TestTheGuardSeesASchemaOfItsOwn (0.00s)
```

**M5 — the schema package loses the list marker.**

```
FAIL	github.com/agezt/agezt/kernel/webui	0.873s
```

## 5. What is untouched

- **No daemon behaviour changed.** The workboard's defaults, its caps, its
  refusals and its readings are the same: the listing reads **100** rows when
  nothing was asked for, the lanes **500**, a sweep is capped at **1000**, a
  watch at **200**, a stale claim at **ten minutes**, and the tag list is still
  trimmed when it is a list and still split as it came when it is a string.
- **The published document and the SDK catalogs did not move** — the family's
  rows already said what the rows now say.
- **The REST surface is untouched**: the workboard is not a REST row family.
- **No layer rule moved.** `archcheck` reports 0 violations: `opapi` is a
  contract (L1) and the schema package's markers are structural.
- **The gate is green**: `gofmt`, `go build ./...`, `go vet ./...`,
  `go test ./...`, `depscheck`, `sdkgen -check`, `deadcodecheck`, `archcheck`,
  `bodydoor`, `structure-md -check`, `openapi -check`, `docsindex`,
  `changelog-lint`, `docclaimscheck`, `gitleaks`, `staticcheck`, and the
  browser's `typecheck`, `build` and `deadcode`.

## 6. What remains open

- **The app layer still carries 136 hand-written input schemas.** The biggest
  families left are the **roster** (19), the **edict** (11), the **runs** (8),
  the **memory** (7), the **okr** (7), the **channels** (6), the **journal** (6),
  the **steer** (6), the **tenants** (6), the **audit** (5), the **tools** (5),
  the **approvals** (4), the **system** (4), the **artifacts**' three, the
  **council**'s three, the **taste**'s three, the **world**'s three, the
  **skill**'s two, the **settings**' one, and thirty more families of one to
  two. The rule is in place; their rows are the work.

  **Done in W5.38 for the roster's read half** — see
  [358](358-w538-the-roster-familys-read-half-has-one-request-row-per-operation-evidence.md);
  the roster's write half is the next slice's work.
- **Those families read their fields loosely** — `any` and `json.RawMessage`
  fields with hand-written readers that tolerate what the door would refuse.
  Their rows need the same **reading rules** the workboard's counts and lists
  just got.
- **The catalogue's `Args` and the request row are still two lists.**
- **The console's ten native routes are not in the catalogue's route table.**
- **The console's route policy answers a wrong verb in plain text**
  ("method not allowed"), not with the envelope — a second refusal shape on the
  wire.

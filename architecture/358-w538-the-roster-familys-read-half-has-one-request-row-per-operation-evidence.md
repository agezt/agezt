# W5.38 — the roster family's read half has one request row per operation

## 1. What was wrong

The **roster** is the board of agents — the profiles an operator lists, reads,
inspects and repairs — and its **nineteen** operations declared their request
shape **twice**: as a JSON Schema literal in the app layer, and as a request
struct beside it. The family is spread over **fifteen files**, so the two
spellings sat far apart from each other:

```go
// the app layer's copy
type RefPageRequest struct {
	Ref    json.RawMessage `json:"ref,omitempty"`
	Limit  json.RawMessage `json:"limit,omitempty"`
	Cursor json.RawMessage `json:"cursor,omitempty"`
}

op, err := app.NewOperation(opapi.Spec{Name: opapi.AgentActivity, ReadOnly: true, /* … */ AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"ref":{"type":"string"},"limit":{"type":"number"},"cursor":{"type":"string"}}}`)}, func(ctx context.Context, in ActivityRequest) (ActivityOutput, error) {
```

Every field was `any` — a **raw** message read by hand-written readers
(`rawValue`, `ref()`, `limit()`, `cursor()`, `days()`, `escalationCursor()`),
and the family's rules lived only in those readers:

| the row says | the roster reads |
|---|---|
| a ref that was not sent, or a blank one | **"args.ref required"** |
| a ref with spaces | the **untrimmed** name — the agent's name is the name it was given |
| a page size that was not sent | the family's default (**50** for the activity, **20** for the escalations and the repair status) |
| a page size of `0`, a negative one | the family's default — no page size was asked for |
| a page size of `0.5` | **0** — a fraction truncates to no rows at all |
| a page size above the family's cap | the cap (**500** for the activity, **100** for the escalations and the repair status) |
| a cursor that is not a sequence number | no cursor — the page starts at the beginning |
| an age that is a number, or the string that carries one | the age — a string that does not parse is no age at all |

The door already refused the mistyped bodies (the literals said `string`,
`number` and `["number","string"]`), so those app-level type readings were
**unreachable in the daemon** — the shape was declared twice, and only one of
the two declarations was ever spoken.

This slice gives the family's **read half** — eight operations — its rows. The
**write half** (eleven operations: the capabilities, the profile writes, the
removal, the repair, the resolve, the enable, the retirement, the task update
and the wake) is W5.39's work.

## 2. What was built

### 2.1 Four rows serve eight operations

The read half's rows live in the contract now
(`kernel/contract/opapi/requests.go`), and the sharing is the family's own
vocabulary:

| the row | the operations | the fields |
|---|---|---|
| `AgentRefRequest` | `agent_impact`, `agent_tombstone`, `agent_permissions` | the agent's ref |
| `AgentPageRequest` | `agent_activity`, `agent_escalations`, `agent_repair_status` | the ref, the page size, the cursor |
| `AgentListRequest` | `agent_list` | the page size, the cursor |
| `AgentGraveyardRequest` | `agent_graveyard` | the age |

The app layer keeps no copy of the read half: the seven request structs and
the eight literals are **gone**, and the operations bind through the contract's
rows. The contract's row table grows from **49** to **57** entries.

### 2.2 The age is a number — or the string that carries one

The graveyard's age was the family's one shape that was not a plain number:
a query string carries `"30"`, not `30`. The row now says so, and the schema
package learned the marker that derives the union:

```go
// NumberFlag is a request field that carries a number — or the STRING a
// query string carries for one.
type NumberFlag struct { … }

// NumberOrString marks the type as carrying a number or the string a query
// string carries for one, so the schema derives the union of the two.
func (NumberFlag) NumberOrString() {}
```

The reading is the family's own, and it keeps its leniency: a string that does
not parse is no age at all, and the parse error is ignored — its value (a
range error's ±Inf) is kept.

### 2.3 The family's rules stay in the family

The rules live in one place now (`kernel/app/roster/rows.go`), and they are the
roster's vocabulary, not the contract's:

```go
// pageLimit reads the page size a row asked for: the family's default when the
// row did not ask for one, and the family's cap when it asked for too many.
func pageLimit(limit opapi.Limit, fallback, max int) int {
	size := fallback
	if limit.Present() && limit.Number() > 0 {
		size = int(limit.Number())
	}
	if size > max {
		size = max
	}
	return size
}
```

Note the rule the board and the workboard do **not** have: a fraction
truncates to **no rows at all** here, while the board reads a fraction as no
page and the workboard reads it as its default. The families differ, and the
difference is now visible in one place instead of scattered over three
readers.

### 2.4 The published document did not move

The roster's endpoints are **REST** paths, and the published document renders
the REST surface's own rows — which already said what the app's rows now say.
`docs/openapi.json`, the generated client and the browser's generated types
are **byte-identical** after the regeneration. This slice moved where the
shape lives, not what it says.

## 3. What was measured

| | before | after |
|---|---|---|
| request rows held by the contract | **49** | **57** |
| roster operations with a row | **0** (of 19) | **8** (of 19 — the read half) |
| roster row types living in the contract | **0** | **4** |
| hand-written input schemas in the roster family | **19** | **11** |
| hand-written request structs in the roster family | **14** | **10** |
| hand-written input schemas in the app layer | **136** | **128** |

The measurement script (`.temp_files/w538_measure.py`) reads `HEAD` (W5.37's
merge, `a0106421`) and the working tree and counts the same things in both.

## 4. The mutations

Five reverted mutations prove the slice (`.temp_files/w538_mutations.py`; every
mutation was reverted and the tree is back to green).

**M1 — the app puts a hand-written schema back beside the roster's rows.**

```
--- FAIL: TestTheRegistrysDeclaredInputIsTheRow (0.00s)
```

**M2 — the contract's number reading breaks.**

```
--- FAIL: TestTheNumberFlagReadsANumberOrTheStringThatCarriesIt (0.00s)
```

**M3 — the family's page-size rule changes.**

```
--- FAIL: TestTheRosterRowsCarryTheFamiliesOwnRules (0.00s)
```

**M4 — the guard's schema comparison quietly stops comparing.**

```
--- FAIL: TestTheGuardSeesASchemaOfItsOwn (0.00s)
```

**M5 — the family's ref rule changes.**

```
--- FAIL: TestTheRosterRowsCarryTheFamiliesOwnRules (0.00s)
```

## 5. What is untouched

- **No daemon behaviour changed.** The door already refused the mistyped
  bodies, with the same words (`$.limit has wrong type: got string, want
  number`); the page sizes, the defaults, the caps, the ref's refusal and the
  graveyard's leniency are the same.
- **The published document and the SDK catalogs did not move** — the REST
  surface's own rows already said what the roster's rows now say.
- **The write half is untouched** — it still reads its fields raw, and its
  rows are W5.39's work.
- **No layer rule moved.** `archcheck` reports 0 violations: `opapi` is a
  contract (L1) and the schema package's markers are structural.
- **The gate is green**: `gofmt`, `go build ./...`, `go vet ./...`,
  `go test ./...`, `depscheck`, `sdkgen -check`, `deadcodecheck`, `archcheck`,
  `bodydoor`, `structure-md -check`, `openapi -check`, `docsindex`,
  `changelog-lint`, `docclaimscheck`, `gitleaks`, `staticcheck`, and the
  browser's `typecheck`, `build` and `deadcode`.

## 6. What remains open

- **The roster's write half still writes its own schemas** — eleven
  operations, eleven literals and ten request structs. Their rows are the next
  slice's work, and they carry the shapes the read half did not have: objects
  (`profile`, `cascade`, `noise_policy`, `config_overrides`, `task`), strict
  lists of texts (`tool_allow`, `tool_deny`, `task_model_chain`) and a flag
  that is a boolean or the string a query carries for one (`enabled`).

  **Done in W5.39 for the roster's write half** — see
  [359](359-w539-the-roster-familys-write-half-has-one-request-row-per-operation-evidence.md);
  the contract grew the missing vocabulary (a text, a list of texts and an
  integer that know whether they were sent) and all **nineteen** of the
  roster's operations now have a row.
- **The app layer still carries 128 hand-written input schemas.** The biggest
  families left are the roster's write half (**11**), the **edict** (11), the
  **runs** (8), the **memory** (7), the **okr** (7), the **channels** (6), the
  **journal** (6), the **steer** (6), the **tenants** (6), the **audit** (5),
  the **tools** (5), the **approvals** (4), the **system** (4), and thirty more
  families of one to three.
- **Those families read their fields loosely** — `any` and `json.RawMessage`
  fields with hand-written readers that tolerate what the door would refuse.
- **The catalogue's `Args` and the request row are still two lists.**
- **The console's ten native routes are not in the catalogue's route table.**
- **The console's route policy answers a wrong verb in plain text**
  ("method not allowed"), not with the envelope — a second refusal shape on the
  wire.

# W5.25 — the REST surface's own rows have one home: the surface spells no row by hand

## 1. What was wrong

W5.22 gave the board's message row one home, W5.23 the envelopes around it,
W5.24 the artifact and update answers. The **REST surface's own rows** were
still written by hand — one `map[string]any` per answer:

| answer | fields | written as |
|---|---|---|
| the health answer | status, version, default_model, model_count | a hand-written map |
| the models answer | default, models | a hand-written map |
| the readiness probe (ready) | status | a hand-written map |
| the readiness probe (not ready) | status, reason | a hand-written map |
| the error body | type, message | a hand-written map inside a map |
| the run answer (completed) | correlation_id, model, status, answer | a hand-written map |
| the run answer (failed) | correlation_id, model, status, error | a hand-written map |
| the run frame (start) | correlation_id, model | a hand-written map |
| the run frame (done) | correlation_id, status, answer | a hand-written map |
| the run frame (error) | correlation_id, error | a hand-written map |
| the run arc | correlation_id, count, events | a hand-written map |

**Eleven** hand-written views and **28** field-name spellings (**15** distinct
names), for rows the contract already declared — `RowHealth`, `RowModels`,
`RowProbeStatus`, `RowAPIError`, `RowRunResult`, `RowRunArc`.

The run family is the worst of them: **one** row spelled **six** times, and the
run result's *failed* answer spelled the row's names with `error` where the row
says `answer` — a reader could not tell which of the two was the row.

## 2. What was built

### 2.1 Where the rows live

The rows a **domain** contract owns live in that domain's contract — the board's
in `boardapi`, the artifact index's in `remoteapi`, the update surface's in
`updateapi` — because another surface shares them. The REST surface's **own**
rows have no other home: they live in `kernel/contract/restapiapi`, the
surface's own contract, in a new file `served.go`:

```go
type Health struct {
	DefaultModel string `json:"default_model"`
	ModelCount   int    `json:"model_count"`
	Status       string `json:"status"`
	Version      string `json:"version"`
}
```

beside the vocabulary the SDKs speak (`RowHealth` and friends). The two lists
are two things — the **published vocabulary** (with its docs, its optionality
and the order the catalogs render it in) and the **served bytes** — and the
ratchet is what holds them to each other.

### 2.2 One row, six answers

The run row is now **one** struct, and it serves six answers:

```go
type RunResult struct {
	Answer        *string `json:"answer,omitempty"`
	CorrelationID string  `json:"correlation_id"`
	Error         *string `json:"error,omitempty"`
	Model         *string `json:"model,omitempty"`
	Status        *string `json:"status,omitempty"`
}
```

The optional fields are **pointers**, and that is the point: the pointer is how
the row says *absent*. The successful answer carries an answer **even when the
model returned an empty string** (the vocabulary names `answer` as required);
the failed answer carries an error and no answer; the stream's frames carry
only the part they have. One row, six answers, one spelling of the names.

The ratchet grew a rule to match: **a field the contract calls a string may be
carried as a string or as a pointer to one** — the pointer is how the row says
"absent", and the wire cannot tell the two apart.

### 2.3 The run arc's events

```go
type RunArc struct {
	CorrelationID string          `json:"correlation_id"`
	Count         int             `json:"count"`
	Events        json.RawMessage `json:"events"`
}
```

The events are carried as **the bytes the daemon already marshalled**: the
contract may not name the platform's event type (archcheck's principle P4), and
the wire does not care how the daemon held them. The rule that covers it was
already there: a free-form array (`any[]`) is served by **any** slice — and
`json.RawMessage` is one.

### 2.4 The two guards

**A — the stable-subset ratchet** (`kernel/contract/restapiapi/servedrows_test.go`).
Every field the contract names must be on the row the daemon serves, under the
same name and a type the contract allows; the served row may carry more. It now
covers **15** rows: the board's **7**, the domain contracts' **2**, and the
surface's own **6**.

**B — the surface's own guard** (`kernel/restapi/served_rows_test.go`). It reads
the surface's own sources and fails when a map literal spells the field names of
a contract-owned row. It now watches **18** rows across **11** files.

## 3. What was measured

| | before | after |
|---|---|---|
| hand-written views of a contract row in the REST surface | **11** | **1** |
| …of which are a declared exception | 0 | **1** (the watch's fallback frame) |
| hand-spelled field names in the surface (this slice) | **28** | **0** |
| rows the stable-subset ratchet covers | **9** | **15** |
| contract-owned rows the surface guard watches | **12** | **18** |
| the served bytes | — | **unchanged** (the golden fixtures are byte-identical) |

The **before** side is not a different tree: it is the *same* tree with the same
eleven hand-written views, and a guard that watched twelve rows.

## 4. The mutations

Five reverted mutations prove the slice (`.temp_files/w525_mutations.py`; every
mutation was reverted and the tree is back to green).

**M1 — the contract renames a field of the health row.** `model_count` becomes
`models`:

```
the contract's "health" row names "models", which the row the daemon serves does not carry
```

**M2 — the contract calls a field a type the served row does not carry.** The
health row's `status` becomes a number:

```
the contract's "health" row names "status" as "int", the row the daemon serves carries it as string
```

**M3 — the served row drops a field the contract names.** `restapiapi.Health`
loses its `model_count` (and the call site with it):

```
the contract's "health" row names "model_count", which the row the daemon serves does not carry
```

**M4 — the surface writes the health answer by hand again.** The health answer
goes back to a map literal:

```
restapi_routes.go writes a hand-written view of the "health" row: it spells default_model,model_count,status,version
```

**M5 — the surface writes the run answer by hand again.** The successful run
answer goes back to a map literal:

```
restapi_routes_runs.go writes a hand-written view of the "run_result" row: it spells answer,correlation_id,model,status
```

## 5. What is untouched

- **No published artifact moved.** The SDK catalogs and the generated OpenAPI
  catalog are byte-identical (`sdkgen -check`, `openapi -check`), and the
  golden fixtures are byte-identical to `main`'s.
- **No layer rule moved.** `archcheck` still reports 0 violations; the REST
  contract gained a file of rows and imported nothing but the standard
  library.
- **The daemon's behaviour did not change.** The run's empty answer is still
  carried (the pointer says "present, and it is empty"), and the frames still
  carry what they carried.
- **The gate is green**: `gofmt`, `go build ./...`, `go vet ./...`,
  `go test ./...`, `depscheck`, `sdkgen -check`, `deadcodecheck`, `archcheck`,
  `bodydoor`, `structure-md -check`, `openapi -check`, `docsindex`,
  `changelog-lint`, `docclaimscheck`.

## 6. What remains open

- **One hand-written view is left on the surface, and it is declared**: the
  watch stream's fallback frame — the event's metadata view for a message
  evicted from the store. It is deliberately not a row, and the guard says so
  in writing.
- **The error body's envelope is still a map** (`{"error": {...}}`): the
  contract's vocabulary cannot say "a row inside a row", so the envelope has no
  row and only the error object inside it is checked.
- **The console's rollback endpoints are still outside the generated
  catalog**, so the browser's `Checkpoint` view is still in W5.21's ledger.
  Bringing them in is a real decision (an operation, a row, a contract).
- **The rows the REST surface shares with the OpenAI-compatible surface** (the
  chat answers) and the **console's own rows** have not been measured — the
  guard reads the REST surface's own files, and the console's answers are
  served by `kernel/webui`.

# W5.26 — the console's rollback answers have one home: the console spells no rollback row by hand

## 1. What was wrong

W5.22 gave the board's mail row one home, W5.23 the envelopes around it,
W5.24 the artifact and update answers, W5.25 the REST surface's own rows. The
**console's rollback answers** were still written by hand — one
`map[string]any` per answer:

| answer | fields | written as |
|---|---|---|
| the rollback list | checkpoints, count, (run_id) | a hand-written map |
| the apply answer (already applied) | checkpoint, applied, reason | a hand-written map |
| the apply answer (applied) | checkpoint, applied, result | a hand-written map |

**Three** hand-written views and **8** field-name spellings (**6** distinct
names), in an *implementation* package, for a row the **platform** owned.

The row itself — the checkpoint, with its eleven fields — lived in the
platform's store (`kernel/platform/rollbackstore`), and the store is the wrong
place for it: the **console** serves that row on the wire, the store merely
writes it. A row two packages name must be declared once, and the row that two
packages name was declared in the one that writes it.

So the checkpoint had two homes the moment a second surface wanted it: the
store's copy (the console read it through a type alias of its own) and, for
every answer around it, the console's hand-written maps. Nothing said the
console's `{"checkpoint": …, "applied": …}` was *the* checkpoint.

## 2. What was built

### 2.1 Where the rows live

A new contract, `kernel/contract/rollbackapi`, holds the rows the rollback
engine and everyone who asks "what can I roll back?" agree on:

```go
type Checkpoint struct {
	ID           string         `json:"id"`
	Kind         string         `json:"kind"`
	Action       string         `json:"action"`
	RunID        string         `json:"run_id,omitempty"`
	SubjectID    string         `json:"subject_id"`
	SubjectName  string         `json:"subject_name,omitempty"`
	BeforeStatus string         `json:"before_status,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	Before       map[string]any `json:"before,omitempty"`
	CreatedMS    int64          `json:"created_ms"`
	AppliedMS    int64          `json:"applied_ms,omitempty"`
}

type List struct {
	Checkpoints []Checkpoint `json:"checkpoints"`
	Count       int          `json:"count"`
	RunID       string       `json:"run_id,omitempty"`
}

type Applied struct {
	Applied    bool           `json:"applied"`
	Checkpoint Checkpoint     `json:"checkpoint"`
	Reason     string         `json:"reason,omitempty"`
	Result     map[string]any `json:"result,omitempty"`
}
```

The checkpoint's fields are declared in the order the wire speaks them,
because the field order **is** the member order of the JSON the console
serves. The contract is **pure**: the standard library and other contract
packages only (archcheck's principle P4).

`Applied.Result` carries the restore's own answer — another operation's row,
which this contract cannot name — and does so as the bytes the daemon
already produced, the same way `restapiapi.RunArc.Events` does.

### 2.2 The platform aliases, it does not copy

The store's own type is now an **alias**:

```go
type Checkpoint = rollbackapi.Checkpoint
```

An alias cannot drift: the platform writes the contract's row, by
construction. A **copy** could drift, and would compile everywhere else — so
the guard against a copy is written as a **function type**, because an
assignment would accept a copy:

```go
var _ func(rollbackapi.Checkpoint) rollbackapi.Checkpoint = func(c rollbackapi.Checkpoint) rollbackapi.Checkpoint { return c }
```

`kernel/platform/rollbackstore/contract_rows_test.go`.

### 2.3 The console serves the contract's rows

`kernel/webui/rollback.go` answers with `rollbackapi.List` and
`rollbackapi.Applied`. The served bytes did not move: Go's map encoder sorted
the old answer's keys (`applied`, `checkpoint`, `reason`) and the struct's
member order is the same, and the checkpoint's member order is the platform's
own, byte for byte. `kernel/webui/rollback_rows_test.go` **pins** the bytes
so a future member order cannot drift silently.

### 2.4 The console's guard

`kernel/webui/served_rows_test.go` reads the console's own sources and fails
when a map literal spells the field names of a contract-owned row — the same
shape as the REST surface's guard, and the same floor: **two** shared field
names. It watches **3** rows (`rollback_checkpoint`, `rollback_list`,
`rollback_applied`).

The floor is two because the console's **operation arguments** share two
names with the checkpoint row (`id`, `reason`) — the skill restore's
arguments are forwarded to the daemon's `skill_restore` operation and are not
an answer. That literal is the guard's **one declared exception**, and the
declaration is two-way: an exception that no longer matches the code **fails**
the check, because a stale exception is a lie about the code.

## 3. What was measured

| | before | after |
|---|---|---|
| hand-written views of a contract row in the console (this slice) | **3** | **0** |
| …of which are a declared exception | 0 | **1** (the skill restore's operation arguments) |
| hand-spelled field names in the console's answers (this slice) | **8** | **0** |
| contract-owned rows the console's guard watches | 0 | **3** |
| the served bytes | — | **unchanged** (pinned by three tests) |

The **before** side is not a different tree: it is the *same* tree with the
same three hand-written views, and a console with no guard at all.

## 4. The mutations

Five reverted mutations prove the slice (`.temp_files/w526_mutations.py`;
every mutation was reverted and the tree is back to green).

**M1 — the platform declares its own copy of the checkpoint row.** The alias
becomes a struct:

```
kernel/platform/rollbackstore/contract_rows_test.go:12:28: cannot use func(rollbackapi.Checkpoint) rollbackapi.Checkpoint as func(rollbackapi.Checkpoint) rollbackapi.Checkpoint value in variable declaration
```

**M2 — the contract's checkpoint row loses a field the console reads.** The
`kind` field is removed:

```
kernel/app/files/restore.go:50:16: checkpoint.Kind undefined (type *rollbackstore.Checkpoint has no field or method Kind)
kernel/app/files/restore.go:51:127: checkpoint.Kind undefined (type *rollbackstore.Checkpoint has no field or method Kind)
```

**M3 — the console writes the rollback list by hand again.** The list answer
goes back to a map literal:

```
rollback.go writes a hand-written view of the "rollback_list" row: it spells checkpoints,count
```

**M4 — the console writes the rollback apply answer by hand again.** The
"already applied" answer goes back to a map literal:

```
rollback.go writes a hand-written view of the "rollback_applied" row: it spells applied,checkpoint,reason
```

**M5 — the console's declared exception stops matching the code.** The skill
restore's arguments rename `reason` to `why`:

```
rollback_helpers.go declares an exception for "id,reason,status" that no longer spells a row by hand — the exception is stale and must be deleted in the same change
```

## 5. What is untouched

- **No published artifact moved.** The SDK catalogs and the generated OpenAPI
  catalog are byte-identical (`sdkgen -check`, `openapi -check`).
- **No layer rule moved.** `archcheck` still reports 0 violations; the new
  contract imported nothing but the standard library, and the platform
  (L2) and the console (L5) both point **up** at it.
- **The daemon's behaviour did not change.** The rollback list still filters
  by run, the apply still refuses an already-applied checkpoint, and the
  console's answers are the same bytes.
- **The gate is green**: `gofmt`, `go build ./...`, `go vet ./...`,
  `go test ./...`, `depscheck`, `sdkgen -check`, `deadcodecheck`, `archcheck`,
  `bodydoor`, `structure-md -check`, `openapi -check`, `docsindex`,
  `changelog-lint`, `docclaimscheck`, `gitleaks`.

## 6. What remains open

- **The console's other answers have no home yet**: the auth meta
  (`authed`, `password_required`), the voice status (`configured`, `stt`), the
  workflow hook (`correlation_id`, `executed`, `ok`, `outputs`, `workflow`),
  the file tree (`parents`, `path`) and the OAuth pair (`code`, `state`). They
  are hand-written today, and the guard does not watch them because the
  contract does not own them yet.
- **The console's native routes still declare no answer.** `opapi.NativeRoute`
  carries `Body json.RawMessage` and nothing for the response, so the
  generated browser catalogue has **no** type for those answers — which is
  why the browser's `Checkpoint` view is still in W5.21's ledger.
- **The browser's view is still a ledger entry, not a pin.** Closing it needs
  the native route to name its answer row and `tools/openapi` to render that
  row into the catalogue.
- The next slice takes these three together: the console's remaining answers
  become structs, the native route names the row it answers with, and the
  browser's `Checkpoint` view becomes a **pin**.

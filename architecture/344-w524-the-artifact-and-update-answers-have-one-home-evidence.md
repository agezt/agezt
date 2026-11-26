# W5.24 — the artifact and update answers have one home, and the surface guard covers the whole surface

## 1. What was wrong

W5.22 gave the board's **message row** one home and W5.23 gave the **envelopes
around it** one home. Two more surfaces were still writing their answers by
hand — one `map[string]any` per answer, spelling the row's field names a second
time, in an *implementation* package, while the contract declared the same
shapes for the SDKs:

| home | answer | fields | written as |
|---|---|---|---|
| `kernel/restapi/artifacts.go` | the artifacts list | count, total_count, limit, truncated, entries | a hand-written map |
| `kernel/restapi/update_handlers.go` | the update check (disabled) | current, up_to_date, status | a hand-written map |
| `kernel/restapi/update_handlers.go` | the update check (up to date) | current, up_to_date | a hand-written map |
| `kernel/restapi/update_handlers.go` | the update check (a release) | current, up_to_date, update{version, sha256, url, notes} | a hand-written map |
| `kernel/restapi/update_handlers.go` | the update apply (staged) | applied, version | a hand-written map |
| `kernel/restapi/update_handlers.go` | the update apply (refused) | applied, error | a hand-written map |

**Six** hand-written views, **15** field names, in a package that is not the
contract — and the update surface spelled the **same four release fields**
(version, sha256, url, notes) inside a nested map of its own.

The domain contracts were already there, holding the rows: `remoteapi` owns the
artifact row (`Artifact`) and `updateapi` owns the release (`Manifest`) and the
check outcome. The surface simply did not use them for its answers.

W5.23's guard measured **three files** — the mailbox's. The rest of the surface
was not measured at all, so "the surface spells no row by hand" was a claim
about three files, not about the surface.

## 2. What was built

### 2.1 The answers live in the domain contracts

```go
// kernel/contract/remoteapi/artifact.go
type ArtifactList struct {
	Count      int        `json:"count"`
	Entries    []Artifact `json:"entries"`
	Limit      int        `json:"limit"`
	TotalCount int        `json:"total_count"`
	Truncated  bool       `json:"truncated"`
}
```

```go
// kernel/contract/updateapi/updateapi.go
type Release struct {
	Notes   string `json:"notes"`
	SHA256  string `json:"sha256"`
	URL     string `json:"url"`
	Version string `json:"version"`
}

type Check struct {
	Current  string   `json:"current"`
	Status   string   `json:"status,omitempty"`
	UpToDate bool     `json:"up_to_date"`
	Update   *Release `json:"update,omitempty"`
}

type Applied struct {
	Applied bool   `json:"applied"`
	Error   string `json:"error,omitempty"`
	Version string `json:"version,omitempty"`
}
```

The fields are declared in the order the wire speaks them (alphabetically),
because the field order **is** the member order of the served JSON — so the
served bytes did not move.

### 2.2 The guards

**A — the stable-subset ratchet** (`kernel/contract/restapiapi/servedrows_test.go`).
The contract's `artifacts` row is the stable subset of the row the daemon
serves; the contract's `update_apply` row (the release the client sends) is the
stable subset of the release the daemon reports back. Names **and** types.

One rule was added for it: a free-form array (`any[]`) is served by **any**
slice — the contract says the values are free-form, not that the array holds
one particular Go type. That is what lets the artifacts row's `entries` be
`[]remoteapi.Artifact` rather than `[]any`.

**B — the surface's own guard**, now covering the **whole surface**
(`kernel/restapi/served_rows_test.go`). It reads the package's own sources —
**11** files — and fails when a map literal spells the field names of a
**contract-owned row**. The rows it watches are the ones the contracts own
(**12** of them: the board's message row and its six envelopes, the artifact
row and its list, the release, the check and the apply).

The guard is a ratchet in both directions: a declared exception that no longer
matches anything fails too. There is exactly one exception, and it is a frame
that is deliberately **not** a row — the watch stream's fallback for a message
evicted from the store.

## 3. What was measured

| | before | after |
|---|---|---|
| hand-written views of a contract-owned row in the surface | **6** | **0** |
| hand-spelled field names in the surface (this slice) | **15** | **0** |
| files the surface guard measures | **3** (the mailbox's) | **11** (the whole surface) |
| contract-owned rows the guard watches | **9** | **12** |
| a renamed row field | the SDKs keep the old name | the stable-subset ratchet fails |
| the surface writes a row by hand again | nothing says so | the surface's guard fails |
| the served bytes | — | **unchanged** |

The **before** side is not a different tree: it is the *same* tree with the same
six hand-written views, and a guard that watched three files.

## 4. The mutations

Five reverted mutations prove the slice (`.temp_files/w524_mutations.py`; every
mutation was reverted and the tree is back to green).

**M1 — the contract renames a field of the artifacts row.** `total_count`
becomes `total`:

```
the contract's "artifacts" row names "total", which the row the daemon serves does not carry
```

**M2 — the contract calls a field a type the served row does not carry.**
`truncated` becomes a string:

```
the contract's "artifacts" row names "truncated" as "string", the row the daemon serves carries it as bool
```

**M3 — the served row drops a field the contract names.** `updateapi.Release`
loses its `version` field (and the call site with it):

```
the contract's "update_apply" row names "version", which the row the daemon serves does not carry
```

**M4 — the surface writes the artifacts answer by hand again.** The list answer
goes back to a map literal:

```
artifacts.go writes a hand-written view of the "artifact_list" row: it spells count,entries,limit,total_count,truncated
```

**M5 — the surface writes the update answer by hand again.** The check answer
goes back to a map literal:

```
update_handlers.go writes a hand-written view of the "update_check" row: it spells current,up_to_date
```

## 5. What is untouched

- **No published artifact moved.** The SDK catalogs and the generated OpenAPI
  catalog are byte-identical (`sdkgen -check`, `openapi -check`).
- **No layer rule moved.** `archcheck` still reports 0 violations; the two
  contract packages gained rows and imported nothing new.
- **The board's rows did not move.** `boardapi`'s rows are exactly as W5.23
  left them.
- **The gate is green**: `gofmt`, `go build ./...`, `go vet ./...`,
  `go test ./...`, `depscheck`, `sdkgen -check`, `deadcodecheck`, `archcheck`,
  `bodydoor`, `structure-md -check`, `openapi -check`, `docsindex`,
  `changelog-lint`, `docclaimscheck`.

## 6. What remains open

- **The REST surface still writes four rows by hand**: the health answer, the
  run result (in three places) and the run arc. They are the same disease and
  the same cure — the row moves into the contract and the surface serves it —
  but each has a wrinkle the rows above did not have: the run result's
  **failure** answer carries an `error` and no `answer` (so it is not the run
  result row at all), the run arc carries the daemon's own event type, and the
  health row has **no domain contract** to live in. Those are decisions, not
  transcriptions.
- **The update check's answer has no SDK-facing row.** The self-update surface
  is deliberately out of the SDKs' vocabulary, so the contract declares no row
  for the answer as a whole — only for the release inside it. The contract's
  vocabulary has no way to say "a row inside a row", so the check answer's
  envelope (`current`, `up_to_date`) is checked by the type system alone.
- **The watch stream's fallback frame stays hand-written** — it is the event's
  metadata view, not a row, and it is declared as such in the guard.

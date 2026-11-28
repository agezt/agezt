# W5.41 — the runs family has one request row per operation

## 1. What was wrong

The **runs** family is the daemon's governed run: the run itself, its two
reads, the plan surface the run answers with, and the plan authoring. Its
**eight** operations declared their request shape **twice**, across **seven**
files:

```go
// the app layer's copy
type RunRequest struct {
	Intent      json.RawMessage `json:"intent,omitempty"`
	Tenant      json.RawMessage `json:"tenant,omitempty"`
	Model       json.RawMessage `json:"model,omitempty"`
	Agent       json.RawMessage `json:"agent,omitempty"`
	Images      json.RawMessage `json:"images,omitempty"`
	System      json.RawMessage `json:"system,omitempty"`
	Timeout     json.RawMessage `json:"timeout,omitempty"`
	Tools       json.RawMessage `json:"tools,omitempty"`
	MaxCost     json.RawMessage `json:"max_cost,omitempty"`
	ExecProfile json.RawMessage `json:"execution_profile,omitempty"`
	RemotePeer  json.RawMessage `json:"remote_peer,omitempty"`
	ApproveCaps json.RawMessage `json:"auto_approve_caps,omitempty"`
	Trust       json.RawMessage `json:"prompt_injection_trust,omitempty"`
	Assure      json.RawMessage `json:"assure,omitempty"`
	DryRun      json.RawMessage `json:"dry_run,omitempty"`
}
```

```go
// and, beside it, the shape spelled again as a schema literal
opapi.Spec{Name: opapi.Run, /* … */ InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"intent":{"type":"string"},"tenant":{"type":"string"},…,"auto_approve_caps":{"type":["string","array"],"items":{"type":"string"}},"prompt_injection_trust":{"type":["boolean","string"]},…}`)}
```

Every field was **raw** — a `json.RawMessage` read by hand-written readers
(`rawValue`, `optionalString`, `requiredString`, `lenientInt64`, `argString`,
`argInt64`, `argBool`, `argStringList`, `argText`, `argTruthy`, `parseCapList`)
— and the family's rules lived only in those readers:

| the family says | the app said |
|---|---|
| the intent is **required**, and is trimmed | `args.intent required` |
| a mistyped flag is a **usage error**, never a silent default | `args.dry_run must be a boolean` |
| a list of tool names drops its **blank** members | `args.tools[i] must be a string` |
| a list that was sent is a list **even when it is empty** | — (the reader's own `set` flag) |
| the auto-approve grant is **one string or a list** | `args.auto_approve_caps` |
| the trust flag is **lenient**: "on", "true", "yes", "1" | — (a silent false) |
| the page is **twenty** rows, never fewer than one, never more than a thousand | `min(max(limit, 1), maxListLimit)` |
| a window is **truncated** toward zero | `int64(n)` |

The door already refused every mistyped body those literals described, so the
app-level type readings were **unreachable in the daemon**: the shape was
declared twice, and only one of the two declarations was ever spoken.

## 2. What was built

### 2.1 Eight rows serve eight operations

The rows live in the contract now (`kernel/contract/opapi/requests.go`):

| the row | the operations | the fields |
|---|---|---|
| `RunRequest` | `run` | the intent, the tenant, the model, the agent, the images, the system prompt, the timeout, the tools, the cost cap, the execution profile, the remote peer, the auto-approve grant, the injection trust, the assurance attempts, the dry-run flag |
| `RunListRequest` | `runs_list` | the page, the cursor, the status, the intent, the model, the cost window |
| `RunStatsRequest` | `runs_stats` | the window, the intent |
| `PlanRunRequest` | `plan` | the plan to run |
| `PlanHistoryRequest` | `plan_history` | the page, the cursor, the status |
| `PlanStatsRequest` | `plan_stats` | nothing |
| `PlanGenerateRequest` | `plan_generate` | the intent, the model |
| `PlanRefineRequest` | `plan_refine` | the plan, the feedback, the model |

The app layer keeps no copy of the family: the **eight** request structs and
the **eight** literals are **gone**, and the operations bind through the
contract's rows. The contract's row table grows from **79** to **87**.

### 2.2 The contract grew ONE new word, and it was needed

The family speaks in types the contract already had — a **text** for the
intent, the tenant, the model, the agent, the system prompt, the timeout, the
execution profile, the remote peer, the plan, the feedback; a **`Limit`** for
the page, the cost cap, the assurance attempts and the window; a **`TextArray`**
for the images and the tools; a **`BoolFlag`** for the injection trust — with
**one** exception.

The **auto-approve grant** is a list of texts **or** the ONE comma-separated
string a caller sends instead of a list, and its members must be texts: the
old literal said so (`"auto_approve_caps":{"type":["string","array"],"items":{"type":"string"}}`).
The contract's `TextList` leaves its members **open** on purpose — a handler
filters the ones it cannot use — so the contract grew the strict twin:

```go
// StrictTextList is a request field that carries a list of texts — or the ONE
// comma-separated string a caller sends instead of a list — and whose members
// ARE texts.
//
// The TextList leaves its members open on purpose: a handler filters the ones
// it cannot use rather than refusing the whole body. This one does not: a
// member that is not a text is refused, the way the door refuses it.
type StrictTextList struct {
	present bool
	isText  bool
	text    string
	members []string
}
```

The schema package learned **one** more structural marker for it — a method is
enough, so it needs no import from the contracts:

```go
// strictTextList is the marker a request field carries when it holds a list
// of texts — or the ONE comma-separated string a caller sends instead of a
// list — and whose members ARE texts.
type strictTextList interface{ StrictTextList() }
```

The **trust flag** needed one thing more: the family's own spelling ("on" is
on) had to survive the move, so `BoolFlag` grew a reader for the string it
was sent as — the flag's **reading** is the family's, not the contract's.

### 2.3 The family's rules stayed in the family

The rules live in the family (`kernel/app/runs/`), because they are the run's
vocabulary, not the contract's:

```go
// approveCaps parses an auto-approve grant — the ONE comma/space-separated
// string a caller sends OR a list of texts — into a capability set.
func approveCaps(list opapi.StrictTextList) map[string]bool {
	if !list.Present() {
		return nil
	}
	parts := list.Members()
	if list.IsText() {
		parts = []string{list.Text()}
	}
```

```go
// truthy reads the trust flag the lenient way the family has always read it:
// a boolean, or the strings "on", "true", "yes" and "1".
func truthy(flag opapi.BoolFlag) bool {
	if b, ok := flag.Value().(bool); ok && b {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(flag.Text())) {
	case "on", "true", "yes", "1":
		return true
	}
	return false
}
```

The **tools list** keeps its two rules — a list that was sent is a list, even
an empty one, and its blank members are dropped — and the **page** keeps its
own clamp.

### 2.4 The published document moved in shape, not in meaning

The run's and the plan's endpoints are the daemon's own **stream** routes, and
the published document renders their bodies. `/api/run` and `/api/plan/run`
now carry the **derived** form — the property keys are sorted and the schema
says `"required": null` — so `docs/openapi.json` changed by **96** lines.

**Nothing changed in meaning.** A semantic comparison of the whole document
(old against new, every path, ignoring the key order and the empty `required`)
finds **zero** changed paths, and the generated client and the browser's
generated types are **byte-identical**.

## 3. What was measured

| | before | after |
|---|---|---|
| request rows held by the contract | **79** | **87** |
| runs operations with a row | **0** (of 8) | **8** (of 8) |
| runs row types living in the contract | **0** | **8** |
| hand-written input schemas in the runs family | **8** | **0** |
| hand-written request structs in the runs family | **8** | **0** |
| hand-written input schemas in the app layer | **106** | **98** |

The measurement script (`.temp_files/w541_measure.py`) reads `HEAD` (W5.40's
merge, `99c5a194`) and the working tree and counts the same things in both.

## 4. The mutations

Five reverted mutations prove the slice (`.temp_files/w541_mutations.py`;
every mutation was reverted and the tree is back to green).

**M1 — the app puts a hand-written schema back beside the runs' rows.**

```
--- FAIL: TestTheRegistrysDeclaredInputIsTheRow (0.00s)
```

**M2 — the schema package stops naming the members of a list of texts.**

```
--- FAIL: TestTheRunsRowsCarryTheFamiliesOwnRules (0.00s)
```

**M3 — the app stops asking whether a limit was asked for.**

```
--- FAIL: TestListLimitClamp (0.00s)
```

**M4 — the family's trust-flag spellings narrow.**

```
--- FAIL: TestTruthy (0.00s)
```

**M5 — the app splits the auto-approve grant on commas alone.**

```
--- FAIL: TestApproveCaps (0.00s)
```

## 5. What is untouched

- **No daemon behaviour changed.** The door already refused the mistyped and
  the null bodies, with the same words and the same types: a number where a
  string belongs, a null where a string belongs, a string where a number
  belongs, a number where a list belongs — all of them were refused by the old
  literals, and the derived schemas say the same thing. The app-level readers
  that repeated the check (`args.tenant must be a string`, `args.dry_run must
  be a boolean`, `args.tools[i] must be a string`) were unreachable in the
  daemon; the rows now carry the strictness.
- **Three app-level answers changed, and none of them was reachable through
  the door**: a **null** argument used to be a type error and now reads as the
  argument's empty text (so a null intent is "args.intent required", where it
  used to be "args.intent must be a string"); a **null** list used to be a
  type error and now reads as an empty list; and a **null** plan used to be
  "args.plan_json must be a JSON string" and now reaches the plan reader,
  which says "plan_json parse: …". The door refused all three before.
- **The published document moved in shape only** — the run's and the plan's
  bodies carry the derived form, and a semantic comparison finds **zero**
  changed paths. The generated client and the browser's generated types are
  byte-identical.
- **No layer rule moved.** `archcheck` reports 0 violations: `opapi` is a
  contract (L1) and the schema package's markers are structural.
- **The gate is green**: `gofmt`, `go build ./...`, `go vet ./...`,
  `go test ./...`, `depscheck`, `sdkgen -check`, `deadcodecheck`, `archcheck`,
  `bodydoor`, `structure-md -check`, `openapi -check`, `docsindex`,
  `changelog-lint`, `docclaimscheck`, `gitleaks`, `staticcheck`, and the
  browser's `typecheck`, `build` and `deadcode`.

## 6. What remains open

- **The app layer still carries 98 hand-written input schemas.** The biggest
  families left are the **memory** (7), the **okr** (7), the **channels** (6),
  the **journal** (6), the **steer** (6), the **tenants** (6), the **audit**
  (5), the **tools** (5), the **approvals** (4), the **system** (4), and
  thirty more families of one to three.
- **Those families read their fields loosely** — `any` and `json.RawMessage`
  fields with hand-written readers that tolerate what the door would refuse.
- **The catalogue's `Args` and the request row are still two lists.**
- **The console's ten native routes are not in the catalogue's route table.**
- **The console's route policy answers a wrong verb in plain text**
  ("method not allowed"), not with the envelope — a second refusal shape on the
  wire.

---

*Done in W5.41.*

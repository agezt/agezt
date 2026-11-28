# W5.39 — the roster family's write half has one request row per operation

## 1. What was wrong

The **roster** is the board of agents. Its **read half** got its rows in
W5.38; its **write half** — eleven operations: the capability patch, the
profile writes, the removal, the repair, the resolve, the enable, the
retirement, the task update and the wake — still declared its request shape
**twice**, across **eight** files:

```go
// the app layer's copy
type CapabilitiesRequest struct {
	Ref             json.RawMessage `json:"ref,omitempty"`
	TrustCeiling    json.RawMessage `json:"trust_ceiling,omitempty"`
	ToolAllow       json.RawMessage `json:"tool_allow,omitempty"`
	ToolDeny        json.RawMessage `json:"tool_deny,omitempty"`
	NoisePolicy     json.RawMessage `json:"noise_policy,omitempty"`
	ConfigOverrides json.RawMessage `json:"config_overrides,omitempty"`
	MemoryScope     json.RawMessage `json:"memory_scope,omitempty"`
	Workdir         json.RawMessage `json:"workdir,omitempty"`
	MaxCostMc       json.RawMessage `json:"max_cost_mc,omitempty"`
	MaxDailyMc      json.RawMessage `json:"max_daily_mc,omitempty"`
}

op, err := app.NewOperation(opapi.Spec{Name: opapi.AgentCapabilities, /* … */ AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"ref":{"type":"string"},"trust_ceiling":{"type":"string"},…}`)}, func(ctx context.Context, in CapabilitiesRequest) (CapabilitiesOutput, error) {
```

Every field was `any` — a **raw** message read by hand-written readers
(`rawRefOf`, `rawValue`, `lenientString`, `optionalString`, `decodeField`,
`rawChain`), and the family's rules lived only in those readers.

The write half was also the family's hardest case, because it carries three
states the contract could not yet say:

| the row says | the roster reads |
|---|---|
| a field that was **not sent** | **left alone** — the patch does not touch it |
| a field that was sent as **null** | the field is **patched** — it clears |
| a field that was sent **empty** | the field is **patched** — it clears |
| a list of **texts** (`tool_allow`, `tool_deny`, `task_model_chain`) | a list — a member that is not a text is refused |
| an **object** (`profile`, `cascade`, `noise_policy`, `config_overrides`, `task`) | an object — not "an object or null" |
| a **count** (`max_cost_mc`, `max_daily_mc`) | an integer — and a count that was not sent is not a count of zero |
| a **flag** (`enabled`) | a boolean — or the string a query string carries for one |

The door already refused the mistyped bodies (the literals said `string`,
`integer`, `object` and `array`), so those app-level type readings were
**unreachable in the daemon** — the shape was declared twice, and only one of
the two declarations was ever spoken.

## 2. What was built

### 2.1 The contract grew the vocabulary the write half needed

Three new field types live in the contract now
(`kernel/contract/opapi/requests.go`), beside the ones the read half and the
workboard already had:

```go
// Text is a request field that carries a text — and knows whether it was sent
// at all.
//
// A field that was not sent and a field that was sent as an empty text are
// different answers: one says "leave it alone", the other says "clear it".
type Text struct {
	present bool
	value   string
}

// TextArray is a request field that carries a list of texts — a list, and
// nothing else — and knows whether it was sent at all.
type TextArray struct {
	present bool
	members []string
}

// Integer is a request field that carries an integer — and knows whether it
// was sent at all.
type Integer struct {
	present bool
	value   int64
}
```

`TextArray` is deliberately **not** the `TextList` the workboard uses: a
`TextList` accepts the ONE comma-separated string a caller sends instead of a
list, and the roster's lists are strict — a member that is not a text is
refused, the way the door refuses it. A plain `[]string` was not usable
either: it derives a **nullable** array, and the old literals said `"array"`.

`Object` grew a **present** flag too, so an object that was not sent and an
empty one are different answers — and its numbers are read **as they were
written** (`json.Decoder.UseNumber`), so a count that does not fit in a float
still reads as itself. The repo already had a test that said so: a profile's
`max_cost_mc` of `9007199254740993` must survive the crossing.

The schema package learned **three** more structural markers for the new
shapes — a method is enough, so it needs no import from the contracts:

```go
// jsonText is the marker a request field carries when it holds a text.
type jsonText interface{ JSONText() }

// textArray is the marker a request field carries when it holds a list of
// texts — a list, and nothing else.
type textArray interface{ TextArray() }

// jsonInteger is the marker a request field carries when it holds an integer.
type jsonInteger interface{ JSONInteger() }
```

### 2.2 Ten rows serve eleven operations

| the row | the operations | the fields |
|---|---|---|
| `AgentCapabilitiesRequest` | `agent_capabilities` | the ref, and the nine capability fields the operator patches |
| `AgentProfileAddRequest` | `agent_add` | the profile to create |
| `AgentProfileEditRequest` | `agent_edit` | the ref, and the profile to write |
| `AgentRemoveRequest` | `agent_remove` | the ref, and the cascade |
| `AgentRepairRequest` | `agent_repair` | the ref, the reason, the incident chain |
| `AgentResolveRequest` | `agent_resolve` | the ref, the resolution, the routing |
| `AgentSetEnabledRequest` | `agent_set_enabled` | the ref, and the flag |
| `AgentSetRetiredRequest` | `agent_retire`, `agent_revive` | the ref, and the reason |
| `AgentTaskUpdateRequest` | `agent_task_update` | the ref, the op, the task, the task's own fields |
| `AgentWakeRequest` | `agent_wake` | the ref, the intent, the reason, the incident chain |

The retirement and the revive share a row because they are the same
mutation — one of them is the other with the flag turned around.

The app layer keeps no copy of the write half: the **ten** request structs
and the **eleven** literals are **gone**, and the operations bind through the
contract's rows. The contract's row table grows from **57** to **68**.

### 2.3 The family's rules stayed in the family

The rules live in the family (`kernel/app/roster/rows.go` and the families
themselves), because they are the roster's vocabulary, not the contract's:

```go
// The capability patch reads only the fields the operator sent: a field that
// was not sent is left alone; a field that was sent is patched — even when it
// was sent empty, and even when it was sent as null.
if in.TrustCeiling.Present() {
	value := in.TrustCeiling.String()
	patch.trustCeiling = &value
	touched = true
}
```

The **task update** reads **presence**, not value — and the row's `Text`
fields are what carry that difference:

```go
// taskFieldPresent reports whether the task object carried a field at all:
// presence, not value, decides which fields an update touches.
func taskFieldPresent(task opapi.Object, key string) bool {
	_, ok := task.Map()[key]
	return ok
}
```

The **enabled flag** keeps the lenient spelling it has always had: the
contract's `BoolFlagOrFalse` reads a boolean, or the string `"true"`/`"1"`,
and anything else as false — the same sentence the app's own reader said.

### 2.4 The published document moved in shape, not in meaning

The roster's endpoints are **REST** paths, and the published document renders
the **operations' own** declared input schemas. The write half's eight
endpoints now carry the **derived** form the read half already had — the
property keys are sorted and the schema says `"required": null` — so
`docs/openapi.json` changed by **82** lines.

**Nothing changed in meaning.** A semantic comparison of the whole document
(old against new, every path, ignoring the key order and the empty
`required`) finds **zero** changed paths, and the generated client and the
browser's generated types are **byte-identical**. The document says the same
thing it said before; it says it in the family's own voice now.

## 3. What was measured

| | before | after |
|---|---|---|
| request rows held by the contract | **57** | **68** |
| roster operations with a row | **8** (of 19) | **19** (of 19) |
| roster row types living in the contract | **4** | **14** |
| hand-written input schemas in the roster family | **11** | **0** |
| hand-written request structs in the roster family | **10** | **0** |
| hand-written input schemas in the app layer | **128** | **117** |

The measurement script (`.temp_files/w539_measure.py`) reads `HEAD` (W5.38's
merge, `e007162d`) and the working tree and counts the same things in both.

## 4. The mutations

Five reverted mutations prove the slice (`.temp_files/w539_mutations.py`; every
mutation was reverted and the tree is back to green).

**M1 — the app puts a hand-written schema back beside the roster's rows.**

```
--- FAIL: TestTheRegistrysDeclaredInputIsTheRow (0.00s)
```

**M2 — the contract's "was it sent" knowledge breaks.**

```
--- FAIL: TestTheTextKnowsWhetherItWasSent (0.00s)
```

**M3 — the schema package stops deriving the list of texts itself.**

```
--- FAIL: TestFromTypeDerivesTheTextAndTheListOfTextsAndTheInteger (0.00s)
```

**M4 — the guard's schema comparison quietly stops comparing.**

```
--- FAIL: TestTheGuardSeesASchemaOfItsOwn (0.00s)
```

**M5 — the family's "sent but empty" rule breaks.**

```
--- FAIL: TestTheRosterWriteRowsCarryTheFamiliesOwnRules (0.00s)
```

## 5. What is untouched

- **No daemon behaviour changed.** The door already refused the mistyped
  bodies, with the same words (`$.enabled has wrong type: got number, want
  boolean|string`); the patch's presence rules, the task update's presence
  rules, the retirement's reason, the wake's default intent and the enabled
  flag's lenient spelling are the same.
- **The published document moved in shape only** — the eight roster write-half
  endpoints carry the derived form the read half already had, and a semantic
  comparison finds **zero** changed paths. The generated client and the
  browser's generated types are byte-identical.
- **No layer rule moved.** `archcheck` reports 0 violations: `opapi` is a
  contract (L1) and the schema package's markers are structural.
- **The gate is green**: `gofmt`, `go build ./...`, `go vet ./...`,
  `go test ./...`, `depscheck`, `sdkgen -check`, `deadcodecheck`, `archcheck`,
  `bodydoor`, `structure-md -check`, `openapi -check`, `docsindex`,
  `changelog-lint`, `docclaimscheck`, `gitleaks`, `staticcheck`, and the
  browser's `typecheck`, `build` and `deadcode`.

## 6. What remains open

- **The app layer still carries 117 hand-written input schemas.** The biggest
  families left are the **edict** (11), the **runs** (8), the **memory** (7),
  the **okr** (7), the **channels** (6), the **journal** (6), the **steer** (6),
  the **tenants** (6), the **audit** (5), the **tools** (5), the **approvals**
  (4), the **system** (4), and thirty more families of one to three.
- **Those families read their fields loosely** — `any` and `json.RawMessage`
  fields with hand-written readers that tolerate what the door would refuse.
- **The catalogue's `Args` and the request row are still two lists.**
- **The console's ten native routes are not in the catalogue's route table.**
- **The console's route policy answers a wrong verb in plain text**
  ("method not allowed"), not with the envelope — a second refusal shape on the
  wire.

---

*Done in W5.39.*

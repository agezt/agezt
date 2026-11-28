# W5.40 — the edict family has one request row per operation

## 1. What was wrong

The **edict** family is the daemon's policy: the ask policy, the trust levels,
the hard-deny rules, the overlay and the decision log. Its **eleven**
operations declared their request shape **twice**, across **four** files:

```go
// the app layer's copy
type TenantRequest struct {
	Tenant json.RawMessage `json:"tenant,omitempty"`
}

type TestRequest struct {
	Capability json.RawMessage `json:"capability,omitempty"`
	Input      json.RawMessage `json:"input,omitempty"`
	Tenant     json.RawMessage `json:"tenant,omitempty"`
}
```

```go
// and, beside it, the shape spelled again as a schema literal
opapi.Spec{Name: opapi.EdictTest, ReadOnly: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"capability":{"type":"string"},"input":{"type":"string"},"tenant":{"type":"string"}}}`)}
```

Every field was **raw** — a `json.RawMessage` read by hand-written readers
(`rawValue`, `checkTenant`, `optionalString`, `requiredString`,
`lenientInt64`, `scope`), and the family's rules lived only in those readers:

| the family says | the app said |
|---|---|
| the routing tenant is a **strict** string | `args.tenant must be a string` |
| a capability is **required**, and is not trimmed | `args.capability required` |
| a rule must name **exactly one** deny rule | `args.rule must specify exactly one deny rule (no ';' separators)` |
| a page is **twenty** rows, never fewer than one, never more than a thousand | `min(max(limit, 1), 1000)` |
| a window is **truncated** toward zero | `int64(n)` |
| the denied flag is a **strict** boolean | `args.denied must be a boolean` |

The family carried **nine** request structs and **eleven** schema literals —
and the door already refused every mistyped body those literals described, so
the app-level type readings were **unreachable in the daemon**: the shape was
declared twice, and only one of the two declarations was ever spoken.

## 2. What was built

### 2.1 Nine rows serve eleven operations

The rows live in the contract now (`kernel/contract/opapi/requests.go`):

| the row | the operations | the fields |
|---|---|---|
| `EdictTenantRequest` | `edict_show`, `edict_deny_list` | the routing tenant |
| `EdictTestRequest` | `edict_test` | the capability, the input, the tenant |
| `EdictDenyAddRequest` | `edict_deny_add` | the rule, the tenant |
| `EdictDenyRemoveRequest` | `edict_deny_rm` | the rule's name, the tenant |
| `EdictSetLevelRequest` | `edict_set_level` | the capability, the level, the tenant |
| `EdictSetModeRequest` | `edict_set_mode` | the mode, the tenant |
| `EdictOverlayRequest` | `edict_overlay`, `edict_compact` | the tenant, and nothing else |
| `EdictDecisionLogRequest` | `edict_log` | the denied flag, the tool, the capability, the page, the window, the cursor |
| `EdictDecisionStatsRequest` | `edict_stats` | the window, the tool, the capability |

The overlay's two operations share a row because they are the same read: the
tenant, and nothing else.

**No new contract vocabulary was needed.** The family speaks in types the
contract already had — a **boolean** for the denied flag, a **text** for the
tenant, the tool, the capability, the rule, the name, the level and the mode,
and a **`Limit`** for the page and the window (the convention the workboard
already set: `WorkboardSweepRequest.StaleAfterMS Limit`). A `Limit` knows
**whether it was sent at all**, so "no page was asked for" and "a page of
zero" are different answers.

The app layer keeps no copy of the family: the **nine** request structs and
the **eleven** literals are **gone**, and the operations bind through the
contract's rows. The contract's row table grows from **68** to **79**.

### 2.2 The family's rules stayed in the family

The rules live in the family (`kernel/app/edict/`), because they are the
edict's vocabulary, not the contract's:

```go
// Log lists policy decisions newest first: a strict denied flag, then the
// tool and capability filters, then the lenient page.
func (d *Decisions) Log(_ context.Context, in opapi.EdictDecisionLogRequest) (DecisionsOutput, error) {
	tool, capability := in.Tool, in.Capability
	limit := defaultDecisionLimit
	if in.Limit.Present() {
		limit = int(in.Limit.Number())
	}
	page := journalview.Input{Limit: min(max(limit, 1), maxDecisionLimit), CutoffMS: d.cutoff(int64(in.SinceMS.Number())), Cursor: in.Cursor}
```

The **capability is not trimmed**, the **blank rule never reaches the
engine**, the **level** and the **mode** are parsed by the edict's own
parsers, and the **window** is truncated toward zero — all of them in the
family, where they were before.

### 2.3 The door says the same words

The derived schemas are the old literals in the family's own voice:

```
edict_test -> {"additionalProperties":true,"properties":{"capability":{"type":"string"},"input":{"type":"string"},"tenant":{"type":"string"}},"required":null,"type":"object"}
edict_log  -> {"additionalProperties":true,"properties":{"capability":{"type":"string"},"cursor":{"type":"string"},"denied":{"type":"boolean"},"limit":{"type":"number"},"since_ms":{"type":"number"},"tool":{"type":"string"}},"required":null,"type":"object"}
```

The properties, the types and the order of the old literals are all there —
so the door's refusal (`$.tenant has wrong type: got number, want string`) is
the refusal the caller saw before, and the **published document did not move
at all**: the edict's routes are the daemon's own, not REST paths whose
bodies are rendered, and `openapi -check` reports the document **in sync**
with no change.

## 3. What was measured

| | before | after |
|---|---|---|
| request rows held by the contract | **68** | **79** |
| edict operations with a row | **0** (of 11) | **11** (of 11) |
| edict row types living in the contract | **0** | **9** |
| hand-written input schemas in the edict family | **11** | **0** |
| hand-written request structs in the edict family | **9** | **0** |
| hand-written input schemas in the app layer | **117** | **106** |

The measurement script (`.temp_files/w540_measure.py`) reads `HEAD` (W5.39's
merge, `6d403b1b`) and the working tree and counts the same things in both.

## 4. The mutations

Five reverted mutations prove the slice (`.temp_files/w540_mutations.py`;
every mutation was reverted and the tree is back to green).

**M1 — the app puts a hand-written schema back beside the edict's rows.**

```
--- FAIL: TestTheRegistrysDeclaredInputIsTheRow (0.00s)
```

**M2 — the contract's edict row forgets an argument.**

```
--- FAIL: TestTenantStrict (0.00s)
```

**M3 — the contract's edict row renames a flag.**

```
--- FAIL: TestDecisionLog (0.00s)
```

**M4 — the app stops asking whether a limit was asked for.**

```
--- FAIL: TestDecisionLogLimits (0.00s)
```

**M5 — the app trims the capability.**

```
--- FAIL: TestTest (0.00s)
```

## 5. What is untouched

- **No daemon behaviour changed.** The door already refused the mistyped and
  the null bodies, with the same words and the same types: a number where a
  string belongs, a null where a string belongs, a string where a number
  belongs — all of them were refused by the old literals, and the derived
  schemas say the same thing. The app-level readers that repeated the check
  (`args.tenant must be a string`, `args.denied must be a boolean`,
  `args.rule must be a string`) were unreachable in the daemon; the rows now
  carry the strictness, and the row's own refusal replaces them.
- **Two app-level answers changed, and neither was reachable through the
  door**: a **null** limit used to read as the default of twenty and now reads
  as a limit of zero — the floor of **one**; and a mistyped body used to be
  answered with the family's own wording and is now refused by the row
  itself. The door refused both before, so no caller saw either.
- **The published document did not move** — `openapi -check` reports the
  document, the generated client and the browser's generated types **in sync**.
- **No layer rule moved.** `archcheck` reports 0 violations: `opapi` is a
  contract (L1) and the edict family is an app layer (L4).
- **The gate is green**: `gofmt`, `go build ./...`, `go vet ./...`,
  `go test ./...`, `depscheck`, `sdkgen -check`, `deadcodecheck`, `archcheck`,
  `bodydoor`, `structure-md -check`, `openapi -check`, `docsindex`,
  `changelog-lint`, `docclaimscheck`, `gitleaks`, `staticcheck`, and the
  browser's `typecheck`, `build` and `deadcode`.

## 6. What remains open

- **The app layer still carries 106 hand-written input schemas.** The biggest
  families left are the **runs** (8), the **memory** (7), the **okr** (7), the
  **channels** (6), the **journal** (6), the **steer** (6), the **tenants**
  (6), the **audit** (5), the **tools** (5), the **approvals** (4), the
  **system** (4), and thirty more families of one to three.
- **Those families read their fields loosely** — `any` and `json.RawMessage`
  fields with hand-written readers that tolerate what the door would refuse.
- **The catalogue's `Args` and the request row are still two lists.**
- **The console's ten native routes are not in the catalogue's route table.**
- **The console's route policy answers a wrong verb in plain text**
  ("method not allowed"), not with the envelope — a second refusal shape on the
  wire.

---

*Done in W5.40.*

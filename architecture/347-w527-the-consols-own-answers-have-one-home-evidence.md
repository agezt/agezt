# W5.27 — the console's own answers have one home: the console spells no answer by hand

## 1. What was wrong

W5.26 gave the console's **rollback** answers one home. The console's **other**
answers were still written by hand — one `map[string]any` per answer:

| answer | fields | written as |
|---|---|---|
| the auth meta | authed, password_required | a hand-written map |
| the login (no gate) | ok, password_required | a hand-written map |
| the logout | ok | a hand-written map |
| the SSE token | token | a hand-written map |
| the voice status | stt{configured}, tts{configured} | two nested hand-written maps |
| the workflow hook (reply) | correlation_id, executed, ok, outputs, workflow | a hand-written map |
| the workflow hook (accepted) | accepted, correlation_id, workflow | a hand-written map |
| the stream's event frame | kind, payload, subject (+ correlation_id in the run stream) | three hand-written maps, one per proxy |
| the stream's error frame | error, kind | three hand-written maps |
| the stream's result frame | kind, result | three hand-written maps |

**Fourteen** hand-written views of a contract-owned row and **34** field-name
spellings, in an *implementation* package, for rows **nobody had declared**.

The stream frames were the worst of them: **one** frame shape spelled **three**
times, once per proxy (the run stream, the toolbox install stream, the
marketplace stream), and the run stream's frame carried a **fourth** field the
others did not — nothing said which was which.

And the browser reads all of it. `Login.tsx` reads `password_required` and
`authed` by hand; the console's answers had no name the browser could pin to.

## 2. What was built

### 2.1 Where the rows live

A new contract, `kernel/contract/consoleapi`, holds the console's **own**
answers — the rows a surface writes itself, for the routes that are not
operation bindings:

```go
type AuthMeta struct {
	Authed           bool `json:"authed"`
	PasswordRequired bool `json:"password_required"`
}

type Login struct {
	OK               bool  `json:"ok"`
	PasswordRequired *bool `json:"password_required,omitempty"`
}

type VoiceStatus struct {
	STT VoiceHalf `json:"stt"`
	TTS VoiceHalf `json:"tts"`
}
```

The REST surface's own rows live in `kernel/contract/restapiapi`; the console's
own rows live here, for the same reason: **a surface's own rows have no other
home**, and a row two packages name must be declared once. The contract is
**pure**: the standard library and other contract packages only (archcheck's
principle P4).

### 2.2 Two rows, one meaning: the pointer

`Login.PasswordRequired` is a **pointer**, because it says two different
things: *absent* means "the gate is there and you are through it", and
*present* means "there is no gate at all". The pointer is how a row says
*absent* — the same rule `restapiapi.RunResult` uses.

The same is true of the workflow hook's `CorrelationID` and `Workflow`: the
daemon's row omits them when it has nothing to say, and the console forwards
that absence instead of inventing an empty string.

### 2.3 One frame, three answers

The stream's frames are **one** row family, served by all three proxies:

```go
type StreamEvent struct {
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
	Subject string          `json:"subject"`
}

type RunStreamEvent struct {
	CorrelationID string          `json:"correlation_id"`
	Kind          string          `json:"kind"`
	Payload       json.RawMessage `json:"payload"`
	Subject       string          `json:"subject"`
}
```

The run stream's frame carries the correlation that ties an event to its run,
so it has a row of its own; the install and marketplace streams do not. The
payload rides as the bytes the daemon already marshalled (the contract may not
name the platform's event type — archcheck's P4), exactly as
`restapiapi.RunArc.Events` does.

### 2.4 The two guards

**A — the console's guard** (`kernel/webui/served_rows_test.go`). It reads the
console's own sources and fails when a map literal spells the field names of a
contract-owned row. It now watches **16** rows: the rollback family (**3**) and
the console's own (**13**). The floor is **two** shared names — the same floor
the REST surface's guard uses.

The console's **operation arguments** are not answers, and the guard says so:
the skill restore's arguments share `id` and `reason` with the checkpoint row,
and that one literal is the guard's **declared exception** — two-way, so an
exception that stops matching the code fails too.

**B — the pinned bytes** (`kernel/webui/console_rows_test.go`). A struct's
member order **is** the member order of the JSON, so a row that reorders its
fields moves the wire. Five tests hold the console's own answers exactly: the
auth meta, the two login answers, the logout, the voice status and the two
workflow-hook answers.

## 3. What was measured

| | before | after |
|---|---|---|
| hand-written views of a contract-owned row in the console | **14** | **1** |
| …of which are a declared exception | 0 | **1** (the skill restore's operation arguments) |
| hand-spelled field names in those views | **34** | **2** |
| map literals with two or more keys in the console | **23** | **9** (all of them operation arguments) |
| contract-owned rows the console's guard watches | **3** | **16** |
| the served bytes | — | **unchanged** (pinned by five tests) |

The **before** side is not a different tree: it is the *same* tree with the
same fourteen hand-written views.

## 4. The mutations

Five reverted mutations prove the slice (`.temp_files/w527_mutations.py`; every
mutation was reverted and the tree is back to green).

**M1 — the console writes the auth meta by hand again.**

```
session.go writes a hand-written view of the "console_auth_meta" row: it spells authed,password_required
```

**M2 — the console writes the workflow reply answer by hand again.**

```
webui_security.go writes a hand-written view of the "console_workflow_reply" row: it spells correlation_id,executed,ok,outputs,workflow
```

**M3 — the console writes the stream's event frame by hand again.**

```
webui_security.go writes a hand-written view of the "console_stream_event" row: it spells kind,payload,subject
```

**M4 — the contract's auth meta row drops a field the console reads.** The
`authed` field is removed:

```
kernel/webui/session.go:119:17: unknown field Authed in struct literal of type consoleapi.AuthMeta
```

**M5 — the contract's auth meta row changes its member order.** The two fields
are swapped — the compiler is happy, and the wire moves:

```
the auth meta answer's bytes moved:
 got {"password_required":false,"authed":false}
want {"authed":false,"password_required":false}
```

M5 is the mutation that says why the bytes are pinned: a row's member order is
part of its contract, and nothing else would have noticed.

## 5. What is untouched

- **No published artifact moved.** The SDK catalogs and the generated OpenAPI
  catalog are byte-identical (`sdkgen -check`, `openapi -check`).
- **No layer rule moved.** `archcheck` still reports 0 violations; the new
  contract imported nothing but the standard library, and the console (L5)
  points **up** at it.
- **The console's behaviour did not change.** The auth meta still leaks the
  existence of a gate and nothing else; the login still mints a session
  cookie; the voice status still exposes no endpoint, model or credential; the
  webhook's refusals are still uniform; the streams still open, carry events
  and close with the operation's own answer.
- **The gate is green**: `gofmt`, `go build ./...`, `go vet ./...`,
  `go test ./...`, `depscheck`, `sdkgen -check`, `deadcodecheck`, `archcheck`,
  `bodydoor`, `structure-md -check`, `openapi -check`, `docsindex`,
  `changelog-lint`, `docclaimscheck`, `gitleaks`.

## 6. What remains open

- **The console's native routes still declare no answer.** `opapi.NativeRoute`
  carries `Body json.RawMessage` and nothing for the response, so the generated
  browser catalogue has **no** type for these answers — which is why the
  browser's `Checkpoint` view is still in W5.21's ledger.
- **The browser's views of these answers are still hand-written.** The browser
  reads `password_required` and `authed` in `Login.tsx` by name; now that the
  console declares them, they can become **pins** — a compile-time assertion
  that the browser still reads the fields the console serves.
- **The console's error envelopes are still maps** (`{"error": …}`): the error
  body is a shape, not a row, and the guard does not watch it.
- The next slice takes the first two together: the native route names the row
  it answers with, `tools/openapi` renders that row into the browser's
  catalogue, and the browser's views become pins.

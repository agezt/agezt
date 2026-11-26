# W5.19 — the ACP surface gets a contract and a door

## 1. What was wrong

The daemon speaks the **Agent Client Protocol** (SPEC-15 §3) to IDEs: the editor
spawns the process and calls `initialize` → `session/new` → `session/prompt`
over JSON-RPC 2.0 on stdio. Every one of those calls carries a `params`
object — and until this slice, **nothing said what that object may contain**.

The params were read into whatever struct the handler happened to use:

```go
type newSessionParams struct{ Cwd string `json:"cwd"` }

type promptParams struct {
	SessionID string         `json:"sessionId"`
	Prompt    []contentBlock `json:"prompt"`
}
```

so what happened to a mistyped body was decided by Go's `encoding/json`, and the
answer named a **Go struct** — a private type of an implementation detail:

```
{"cwd":42}          → "invalid params: json: cannot unmarshal number into Go struct field newSessionParams.cwd of type string"
{"prompt":"hello"}  → "invalid params: json: cannot unmarshal string into Go struct field promptParams.prompt of type []acp.contentBlock"
```

Three things were wrong with that answer at once:

1. **It named a Go type.** `newSessionParams` and `contentBlock` are private
   details of one implementation; a caller can act on neither.
2. **It was not the registry's sentence.** Everywhere else in the daemon, a
   mistyped request body is refused as
   `operation <op> input: $.<field> has wrong type: got <got>, want <want>`.
3. **A null was accepted.** `{"cwd":null}` became the empty string, and a null
   prompt block became an empty block — a null is not a value, and it is not a
   default either.

This slice is the **last open end** of the "every surface's body is named in a
contract and measured by a door" programme: the REST surface, the console, the
OpenAI-compatible surface, the agent gateway and the whole channel webhook
family already had one. The ACP surface did not.

## 2. What was built

### 2.1 One table of the surface

The contract (`kernel/contract/acpserverapi`) holds the surface as data — the
**wire method**, the **operation name** the refusal carries, and the **JSON
Schema** of the params:

| wire method | operation | params |
|---|---|---|
| `initialize` | `initialize` | none — the server reads none of them |
| `session/new` | `session_new` | `cwd` (string) |
| `session/prompt` | `session_prompt` | `sessionId` (string), `prompt` (array of `{type, text}`) |
| `session/cancel` | `session_cancel` | none — a notification |
| `$/cancelNotification` | `cancel_notification` | none — a notification |

Two decisions are worth naming:

- **The operation name is an identifier, not a path.** The refusal reads
  `operation session_new input: …`, not `operation session/new input: …` —
  the registry names operations, and every other operation in the daemon is
  named that way.
- **A method that reads nothing names no shape.** `initialize` answers with
  the daemon's own protocol version and capabilities and reads **none** of the
  client's params; the cancellation notifications carry a session id the
  server does not read at this version (the per-prompt context does the
  cancelling). So the contract says so, and the door has nothing to measure
  there — instead of pretending the method takes a body it never reads.

### 2.2 The door, in the registry's words

The door (`kernel/acp/params_door.go`) measures an arriving params object
against the contract's shape **before the handler reads it**, with the **same
validator the registry uses** (the shared core, `kernel/httpserver`):

```
operation session_new input: $.cwd has wrong type: got number, want string
operation session_prompt input: $.prompt[0].text has wrong type: got null, want string
```

and the surface keeps its own **voice**: a JSON-RPC error object with the
`invalid params` code (`-32602`), which is what an ACP client already reads.

Two rules keep the door the registry's door:

1. **It checks types, not presence.** A missing field is the handler's
   business — `session/new` without params still opens a session with no
   working directory (as it always did), and `session/prompt` without params is
   refused by the *handler* ("invalid params: no params"), not with a schema's
   words.
2. **It measures the contract's schema**, not the handler's struct — the
   contract is the one home for the request vocabulary.

### 2.3 The contract and the server are held together at boot

The server asks the contract, when it is built, whether the two still agree on
the shape of the params it reads — the same fields, with the same types, in
**both** directions:

```go
if err := checkContract(reflect.TypeOf(newSessionParams{}), reflect.TypeOf(promptParams{})); err != nil {
	panic("acp: the contract and the server disagree: " + err.Error())
}
```

So a contract that mistyped a field, dropped one, or named one the server does
not read **stops the daemon** instead of quietly disagreeing with the code
that reads it.

### 2.4 The shape vocabulary is shared, not copied

The ACP surface asks the very same questions the channel webhook family asks
("which fields does this schema name?", "does it agree with the Go type the
daemon reads?", "which word does the registry use for this type?"). Two copies
of that machinery would be two copies to drift, so it now lives **once**, in
`kernel/contract/jsonshape` — and the channel webhook family's contract
delegates to it. The family's public vocabulary (`channelhookapi.Fields`,
`channelhookapi.CheckShape`) is unchanged; what changed is where the
comparison lives.

The words themselves are the registry's: `string`, `number`, `boolean`,
`object`, `array` — and `any`, for a field read as raw JSON, which the ACP
surface does not have.

## 3. What was measured

The probe (`.temp_files/w519probe`) drives the ACP server the way an editor
does — JSON-RPC over stdio — and classifies every answer. It imports nothing
but the surface itself, so the **same program** runs on `origin/main`
(`f23fbd4b`) and on this branch.

| | before (`f23fbd4b`) | after |
|---|---|---|
| probes | 19 | 19 |
| refused with the registry's sentence | **0** | **16** |
| refused with a Go type's sentence | **11** | **0** |
| refused with some other words | 3 | 0 |
| a null refused | **0** | **5** |
| a null accepted | 2 | **0** |
| a null refused with other words | 3 | 0 |
| answers naming a private Go type | **11** | **0** |
| a body that fits the contract accepted | 3 | 3 |

And the surface itself:

| | before | after |
|---|---|---|
| methods the contract names | **0** (no contract) | **5** |
| methods the contract names a shape for | 0 | 2 |
| request fields measured before the handler reads them | 0 | **5** |
| methods behind a door | 0 | **2** |
| methods whose contract and daemon are held together at boot | 0 | **2** |

## 4. The guards

**Fourteen** guards hold the line — six in the contract, eight in the surface:

*the contract* — the method table is well-formed (an identifier operation, no
duplicates); every params schema is a schema the door can check (no `required`
list, an undeclared field allowed, a type at every level); the lookups answer
the table and answer none for a method the contract does not know; the methods
that read no params are named as such; **the shape guard catches a schema that
is wrong on purpose**; the guard answers nothing for a method that reads no
params.

*the surface* — the contract and the server name the same fields with the same
types; the boot-time ratchet refuses a shape the server does not read; a
mistyped field is refused with the registry's words; a null field is refused; a
params object that is not an object is refused; **the refusal no longer names a
Go type**; a body that fits the contract gets through and an undeclared field
stays allowed; presence is left to the handler; the door measures **before**
the handler reads; the door reads the contract's own shape.

## 5. The mutations

**Ten** reverted mutations prove them. Each breaks ONE thing; each is caught;
each is put back.

| | mutation | caught by |
|---|---|---|
| M1 | the shared door core stops measuring the body | the mistyped-field guard — and the surface answers with the Go type's sentence again |
| M2 | the ACP door stops measuring the params | the mistyped-field guard |
| M3 | the contract calls the working directory a number | the contract-and-server guard |
| M4 | the contract drops the block's text | the contract-and-server guard |
| M5 | the contract names a field the server does not read | the contract-and-server guard |
| M6 | the operation name becomes the wire name | the mistyped-field guard |
| M7 | the boot-time ratchet asks about the wrong method | the server stops answering at all — the panic is the guard |
| M8 | a schema stops allowing an undeclared field | the schema guard |
| M9 | a schema asks for presence | the schema guard |
| M10 | the shared shape vocabulary stops reading the daemon's fields | the shape guard — in **both** contracts that use it |

M1 and M10 are the two that prove the **shared** parts are load-bearing: the
first breaks the door core every surface uses, the second breaks the shape
vocabulary two contracts hold themselves to.

## 6. What is untouched

- The **protocol** is unchanged: the same methods, the same answers, the same
  `session/update` notifications, the same `stopReason`.
- The **voice** is the surface's own: a JSON-RPC error with the
  `invalid params` code, as before. The **wording** of a type refusal is the
  registry's now.
- The **client half** (`kernel/platform/acpclient`), the agent catalogue and
  the bridge tool are untouched.
- The **session** semantics are untouched: an unknown session is still
  "unknown sessionId: …", an empty prompt is still "empty prompt".
- The **SDKs** do not speak ACP, so no SDK moved (`sdkgen -check` is in sync).

## 7. What remains open

The programme's last loose end is not a surface but a **habit**: nothing stops
a **new** surface from reading a body without a contract. The inventory this
slice is built on says what exists today:

| surface | the door |
|---|---|
| `kernel/restapi` | the REST contract's request rows |
| `kernel/webui` | the console-native bodies (`kernel/contract/opapi`) |
| `kernel/openaiapi` | `kernel/contract/openaiapi` |
| `kernel/agentgw` | `kernel/contract/agentgwapi` |
| `plugins/channels/*` (15) | `kernel/contract/channelhookapi` |
| `kernel/acp` | `kernel/contract/acpserverapi` (this slice) |

but the inventory is a **document**, and a document does not fail a build. The
next slice is an **inventory guard** — a repository-wide check that every
handler which reads a request body is behind a door, or has declared itself an
exception — so the table above cannot silently become shorter than the tree.

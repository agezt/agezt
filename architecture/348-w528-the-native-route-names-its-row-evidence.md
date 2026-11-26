# W5.28 — the native route names the row it answers with: the console's answer side is declared

## 1. What was wrong

The console's own endpoints — the **native** routes, the ones that are not
operation bindings — declared their **request** side and said nothing about
their **answer** side:

```go
type NativeRoute struct {
	Name     NativeRouteName
	Path     string
	Method   string
	Public   bool
	Mutation bool
	Body     json.RawMessage   // the request body's schema
	// …and nothing for the answer
}
```

So the console's answer rows existed (W5.26 and W5.27 put them in
`kernel/contract/rollbackapi` and `kernel/contract/consoleapi`), the console
served them — and **no route said which row it answers with**. The route
table knew the path, the verb, whether the route is token-free, and the shape
of the body it takes; the browser had to guess the shape of the answer.

The guess is where it went wrong. The browser's rollback view
(`frontend/src/features/admin/components/Backups.tsx`) reads the daemon's
rollback row through its **own** hand-written interface:

```ts
interface Checkpoint {
  id: string;
  created_at?: number;   // the daemon sends created_ms
  run_id?: string;
  kind?: string;
  description?: string;      // the daemon never sends this
  affected_paths?: string[];  // the daemon never sends this
  size_bytes?: number;        // the daemon never sends this
  reversible?: boolean;       // the daemon never sends this
}
```

Six of its nine members are fields **the daemon does not send** — and one of
them decides whether the operator can act at all:

```tsx
<Button … disabled={!cp.reversible}>Restore</Button>
```

`reversible` never arrives, so the **Restore button is permanently disabled**,
the timestamps render as "—", and the "files affected" column is empty. The
daemon's rollback apply endpoint works; the browser cannot reach it. This is
exactly the drift the programme exists to stop — and it stopped nothing,
because nothing said the browser was reading a daemon row.

## 2. What was built

### 2.1 The route names its row

```go
type NativeRoute struct {
	…
	Body   json.RawMessage
	Answer any   // the ROW this endpoint answers with; nil when it answers with no row
}
```

`Answer` holds the **zero value of the contract's row** — the row itself, not
a name of it, so the field names live in exactly one place. Eight routes name
their row:

| route | path | the row it answers with |
|---|---|---|
| `authmeta` | `/api/authmeta` | `consoleapi.AuthMeta` |
| `login` | `/api/login` | `consoleapi.Login` |
| `logout` | `/api/logout` | `consoleapi.Logout` |
| `sse_token` | `/api/sse-token` | `consoleapi.SSEToken` |
| `voice_status` | `/api/voice/status` | `consoleapi.VoiceStatus` |
| `rollback_list` | `/api/rollback/checkpoints` | `rollbackapi.List` |
| `rollback_apply` | `/api/rollback/apply` | `rollbackapi.Applied` |
| `workflow_hook` | `/hooks/` | `consoleapi.WorkflowHook` |

The other thirteen native routes answer with the **daemon's** rows (the
proxied ones), with **bytes** (the raw file, the audio, the artifact) or with
**HTML** (the shell) — those rows are named where they belong, and an
endpoint that answers with no row at all says so by naming none.

### 2.2 One row, two hook answers

The workflow hook answers in one of two shapes: it either RAN the workflow and
finished while the caller waited, or it ACCEPTED the run and will finish it
in the background. W5.27 declared those as two rows — but a route can name
**one** row, and the route is the thing that says what it answers.

So the two rows became **one**, the way `restapiapi.RunResult` serves six
answers with one struct:

```go
type WorkflowHook struct {
	Accepted      bool    `json:"accepted,omitempty"`
	CorrelationID *string `json:"correlation_id"`
	Executed      any     `json:"executed,omitempty"`
	OK            bool    `json:"ok,omitempty"`
	Outputs       any     `json:"outputs,omitempty"`
	Workflow      *string `json:"workflow"`
}
```

The members the other shape does not carry are absent, and the **bytes did not
move** — the map encoder's alphabetical order and the struct's member order
are the same order.

### 2.3 The ratchet: the console answers the row the route names

`kernel/webui/native_rows_test.go` drives every named route and checks the
answer against the row, in **both** directions:

- a member the console sends that the row does not carry is a violation (a
  second spelling of the row);
- a member the row always carries that the console never sends is a violation
  (a row that promises more than the wire keeps).

And it is closed on both ends: a route that names a row but has no driver
fails ("the route is unverified"), and a route that has a driver but names no
row fails ("the answer side of the route is declared in the contract").

## 3. What was measured

| | before | after |
|---|---|---|
| native routes that name the row they answer with | **0** | **8** |
| native routes whose answer is checked against that row | **0** | **8** |
| contract-owned rows the console's guard watches | **16** | **15** (the hook's two rows became one) |
| the served bytes | — | **unchanged** (pinned by the byte tests) |

## 4. The mutations

Five reverted mutations prove the slice (`.temp_files/w528_mutations.py`; every
mutation was reverted and the tree is back to green).

**M1 — the console answers a member the row does not carry.** The auth meta
grows a `gate` member:

```
the native route "authmeta" answers the member "gate", which the row it names does not carry
```

**M2 — the console drops a member the row always carries.** The auth meta
loses its `authed`:

```
the native route "authmeta" does not answer the member "authed", which the row it names always carries
```

**M3 — the route names the wrong row.** The voice status names the auth meta:

```
the native route "voice_status" answers the member "tts", which the row it names does not carry
the native route "voice_status" does not answer the member "authed", which the row it names always carries
```

**M4 — the route names no row at all.** The auth meta's `Answer` is deleted:

```
the native route "authmeta" answers with the console's own row, but it names none — the answer side of the route is declared in the contract
```

**M5 — the contract's auth meta row changes its member order.** The compiler
is happy and the wire moves:

```
the auth meta answer's bytes moved:
 got {"password_required":false,"authed":false}
want {"authed":false,"password_required":false}
```

## 5. What is untouched

- **No published artifact moved.** The SDK catalogs and the generated OpenAPI
  catalog are byte-identical (`sdkgen -check`, `openapi -check`).
- **No layer rule moved.** `archcheck` still reports 0 violations; the
  contract package grew two contract imports and nothing else.
- **The console's behaviour did not change.** The same bytes, the same
  statuses, the same refusals.
- **The gate is green**: `gofmt`, `go build ./...`, `go vet ./...`,
  `go test ./...`, `depscheck`, `sdkgen -check`, `deadcodecheck`, `archcheck`,
  `bodydoor`, `structure-md -check`, `openapi -check`, `docsindex`,
  `changelog-lint`, `docclaimscheck`, `gitleaks`.

## 6. What remains open

- **The browser's catalogue does not know these rows yet.** `tools/openapi`
  renders the operation registry's schemas; the native routes' answers are not
  in it, so `ResponseOf<"/api/rollback/checkpoints">` still resolves to `any`
  and the browser still writes its own view by hand. The next slice renders
  the named rows into the catalogue.
- **The browser's stale rollback view is still stale** — its `Checkpoint`
  interface names six members the daemon does not send, and the Restore button
  it disables is the operator's way back. Pinning the view to the catalogue's
  row fixes it; that is the next slice's work, and it is a **behaviour** fix,
  not a type fix.
- **The ledger entry** — "the daemon's rollback op is not in the generated
  catalog yet" — stays until the catalogue carries the row.
- **The route table cannot name the daemon's rows.** A contract package may
  not import an application package (archcheck's P4), so a proxied native
  route names nothing — its row belongs to the daemon's registry.

# W5.32 — the console's error envelope has one home: one row, one helper, and the browser reads it from the catalogue

## 1. What was wrong

Every refusal the console writes carried the same envelope — and the console
spelled it **by hand, thirty-seven times**:

```go
writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body: " + err.Error()})
writeJSON(w, http.StatusNotFound, map[string]any{"error": "checkpoint not found"})
// … 34 more, across 7 files
```

The envelope is a **shape**, not a row: it had no declaration, no name and no
home. Nothing said that the thirty-seven spellings were the same shape, so
nothing could say that one of them had drifted — that one refusal wrote
`{"message": …}` instead of `{"error": …}`, say, and the browser would have
read `undefined` and shown "HTTP 500" instead of the console's sentence.

And the **browser** read the shape by hand too — in four places, none of them
typed:

```ts
const j = await res.json();
if (j?.error) msg = String(j.error);
```

## 2. What was built

### 2.1 The envelope is a row

`kernel/contract/consoleapi` declares it, beside the console's other rows:

```go
// Error is the console's error envelope: every refusal the console writes
// carries this one-member row.
type Error struct {
	Error string `json:"error"`
}
```

It is not a route's **answer** — it is the answer of a **refusal** — but it is
a row all the same, and the browser reads it, so it needs a home like every
other row the console writes.

### 2.2 The console writes it through one helper

```go
// writeError writes the console's error envelope: the row every refusal
// carries (kernel/contract/consoleapi.Error).
func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, consoleapi.Error{Error: message})
}
```

**Thirty-seven** hand-written envelopes became **thirty-seven** calls to one
helper. The bytes did not move: the row has one member, and the map had one
member.

### 2.3 The catalogue carries it, and the browser names it

The console declares the envelope in the operation contract
(`opapi.ConsoleNativeErrorRow()`), so the catalogue renders it the way it
renders every other console row:

```ts
/** ConsoleErrorResponse */
export interface ConsoleErrorResponse {
  error: string;
}
```

The browser's four reads now name the catalogue's row instead of guessing:

```ts
const j = (await res.json()) as ConsoleErrorResponse;
if (j?.error) msg = String(j.error);
```

in `app/api/transport.ts`, `lib/chat.ts`, `features/market/lib/market.ts` and
`lib/voice.ts`.

### 2.4 The guards

**A — the console's guard** (`kernel/webui/served_rows_test.go`). The
envelope joins the contract-owned rows the console watches — and because it
carries **one** member, its floor is **one**, not two: a floor of two would
never have seen it, and a refusal that spelled its own map would have been a
second spelling of the row.

```
rollback.go writes a hand-written view of the "console_error" row: it spells error
```

**B — the browser's ratchet** (`viewrows.ratchet.test.ts`). A hand-written
copy of the envelope is a copy of a daemon row, and the one-member arm (W5.30)
catches it:

```
app/api/transport.ts:ConsoleErrorResponse copies the daemon's ConsoleErrorResponse by hand — take the members' types from the daemon
```

**C — the console's byte pin** (`kernel/webui/console_rows_test.go`). The
envelope's bytes are pinned: `{"error":"checkpoint id required"}`.

**D — the browser's catalogue test** (`app/api/console_rows.test.ts`). The
catalogue carries the envelope, with its one member.

**E — the compiler.** The browser may not name a field the envelope does not
carry:

```
src/app/api/transport.ts(100,12): error TS2339: Property 'message' does not exist on type 'ConsoleErrorResponse'.
```

## 3. What was measured

| | before | after |
|---|---|---|
| the console's hand-written error envelopes | **37** (in 7 files) | **0** (one helper) |
| error envelopes in the browser's catalogue | **0** | **1** |
| contract-owned rows the console's guard watches | **15** | **16** |
| browser reads of the envelope bound to the catalogue's row | **0** (of 4) | **4** |
| the served bytes | — | **unchanged** |

## 4. The mutations

Five reverted mutations prove the slice (`.temp_files/w532_mutations.py`; every
mutation was reverted and the tree is back to green).

**M1 — a refusal spells the error envelope by hand again.**

```
served_rows_test.go:172: rollback.go writes a hand-written view of the "console_error" row: it spells error
```

**M2 — the browser spells the error envelope by hand again.**

```
app/api/transport.ts:ConsoleErrorResponse copies the daemon's ConsoleErrorResponse by hand — take the members' types from the daemon
```

**M3 — the contract renames the envelope's member.**

```
console_rows_test.go:90: the error envelope's bytes moved:
```

**M4 — the console stops declaring the error envelope.**

```
src/app/api/console_rows.test.ts > carries the console's error envelope
AssertionError: expected '…' to contain 'export interface ConsoleErrorResponse…'
```

**M5 — the browser names a field the envelope does not carry.**

```
src/app/api/transport.ts(100,12): error TS2339: Property 'message' does not exist on type 'ConsoleErrorResponse'.
```

## 5. What is untouched

- **No daemon behaviour changed.** Every refusal serves the same bytes; the
  byte pin says so.
- **The published OpenAPI document did not move** (`docs/openapi.json`): it
  describes the operation-bound surface.
- **No layer rule moved.** `archcheck` still reports 0 violations; the row is
  in the console's own contract, and the console imports it.
- **The gate is green**: `gofmt`, `go build ./...`, `go vet ./...`,
  `go test ./...`, `depscheck`, `sdkgen -check`, `deadcodecheck`, `archcheck`,
  `bodydoor`, `structure-md -check`, `openapi -check`, `docsindex`,
  `changelog-lint`, `docclaimscheck`, `gitleaks`, and the frontend's
  `npm test`, `npm run typecheck`, `npm run build`, `npm run deadcode`.

## 6. What remains open

- **The daemon's own error payloads are still maps** — the *event* payloads
  the daemon publishes ("the run failed", "the tool errored") are a different
  family from the console's HTTP envelope, and they have their own homes.
- **`docs/openapi.json` does not carry the console's answers**, and the
  document says so. A caller outside the browser still has no published shape
  for `/api/authmeta` or `/api/files/tree`.
- **The `/hooks/` path is a prefix**, not a path the browser calls: the hook's
  row is rendered, but the browser never fetches it.
- **The console's route policy answers a wrong verb in plain text**
  ("method not allowed"), not with the envelope. That is a second refusal
  shape on the wire — a decision to take, not an oversight.

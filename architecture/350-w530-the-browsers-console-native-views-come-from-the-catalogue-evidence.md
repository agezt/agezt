# W5.30 — the browser's console-native views come from the catalogue: the auth gate, the voice card and the SSE token name no field of their own

## 1. What was wrong

W5.29 put the console's own rows into the browser's catalogue, so
`ResponseOf<"/api/authmeta">` finally names the console's answer instead of
`any`. The **browser had not moved**: it still spelled the console's rows by
hand, in four places.

```ts
// features/auth/components/Login.tsx — the auth gate
interface AuthMeta {
  password_required: boolean;
  authed: boolean;
}

// features/jarvis/components/Jarvis.tsx — the voice card
interface ServerVoiceStatus {
  stt?: { configured?: boolean; provider?: string };
  tts?: { configured?: boolean; provider?: string };
}

// features/voice/lib/voiceStatus.ts — the voice readiness
interface ServerVoiceStatus {
  stt?: { configured?: boolean };
  tts?: { configured?: boolean };
}

// app/api/transport.ts — the event feed's token
const body = (await res.json()) as { token?: string };
```

**Seven** member names, spelled by hand, for rows the contract already
declares and the catalogue already carries.

And one of them was **fiction**. The voice card rendered the provider:

```tsx
{voice?.stt?.provider && <span className="ml-1 text-muted">({voice.stt.provider})</span>}
```

The console's voice row carries **no provider at all**:

```go
// kernel/contract/consoleapi
type VoiceHalf struct {
	Configured bool `json:"configured"`
}
```

The daemon says so in as many words — *"It intentionally exposes no endpoint,
model, or credential detail"* — and it has said so since before the browser
asked. So the card promised a field nobody sends, and **two of its own tests
asserted the fiction**: they fed the card an answer carrying `provider` and
expected `(whisper.cpp)` on the screen. The field never arrives from the
daemon, so the operator never saw it — the browser was maintaining a promise
the console had never made.

Nothing failed. A hand-written shape is not a contract, so nothing could say
that it had drifted.

## 2. What was built

### 2.1 The auth gate names no field

`Login.tsx` does not declare the auth meta; it asks the catalogue:

```ts
getJSON("/api/authmeta").then((m) => setState(m.password_required && !m.authed ? "need" : "ok"))
```

`m` is the catalogue's `AuthmetaResponse` — the console's own row — so the
gate reads `password_required` and `authed` because the **console says those
are the members**, not because this file guessed.

### 2.2 The voice card renders what the row says

`Jarvis.tsx` and `voiceStatus.ts` bind to the catalogue's row and **stop
promising a provider**. The provider render and the two tests that asserted
it are gone; a new test pins the truth instead:

```
renders no provider name even when the answer carries one
```

An old daemon — or a hand-edited answer — that offers a provider name must
not put it on screen: the view's vocabulary is the console's row, and the row
says only whether each half is wired.

### 2.3 The event feed's token binds to the path

`transport.ts` cannot use the generated client (the client imports the
transport), so it fetches the token itself. It no longer guesses the shape:

```ts
type SSETokenRow = ResponseOf<"/api/sse-token">;
…
const body = (await res.json()) as SSETokenRow;
```

The alias binds the **path** to the row, so the browser asks the catalogue
what `/api/sse-token` answers with.

### 2.4 The ratchets

**B — the browser's own** (`viewrows.ratchet.test.ts`). The rule that counts a
copy grew a second arm: a type that names a row's **whole** vocabulary is that
row, whatever its size. The floor is **one** member — the console's own rows
are that small (the SSE token carries one, the auth meta two), and a
hand-written copy of one of them is a second contract all the same. The
coincidence that the rule now counts is listed in the **ledger**: a hook's own
option bag (`UseApiKeySubmitOptions`) carries a `provider` member, and that is
its own vocabulary, not the daemon's oauth-start row.

**C — the compiler.** A view that names a field the console's row does not
carry does not compile:

```
src/features/auth/components/Login.tsx(24,55): error TS2551: Property 'is_authed' does not exist on type 'AuthmetaResponse'. Did you mean 'authed'?
```

**D — the browser's own tests.** The voice card renders what the row says and
only that.

**E — the browser's catalogue test** (`app/api/console_rows.test.ts`, new).
Every console-native path the catalogue binds resolves to a row the catalogue
declares, and the paths the browser's console-native views read are bound to
**their** rows. The compile-time half does not run; it **compiles**: if the
catalogue stopped binding a path to its row, `ResponseOf` would fall back to
`any` and the typecheck gate would say so.

## 3. What was measured

| | before | after |
|---|---|---|
| browser copies of a console-native row, spelled by hand | **4** | **0** |
| members of the console's rows spelled by hand in the browser | **7** | **0** |
| browser fields the console never sends (the voice card) | **1** | **0** |
| tests asserting the console sends a provider | **2** | **0** |

The four copies were the auth gate's `AuthMeta`, the voice card's
`ServerVoiceStatus`, the voice readiness's `ServerVoiceStatus`, and the event
feed's inline cast.

## 4. The mutations

Five reverted mutations prove the slice (`.temp_files/w530_mutations.py`; every
mutation was reverted and the tree is back to green).

**M1 — the auth gate spells the console's auth meta by hand again.**

```
features/auth/components/Login.tsx:AuthMeta copies the daemon's AuthmetaResponse by hand — take the members' types from the daemon
```

**M2 — the voice card spells the console's voice status by hand again.**

```
features/jarvis/components/Jarvis.tsx:ServerVoiceStatus copies the daemon's VoiceStatusResponse by hand — take the members' types from the daemon
```

**M3 — the auth gate names a field the console's row does not carry.**

```
src/features/auth/components/Login.tsx(24,55): error TS2551: Property 'is_authed' does not exist on type 'AuthmetaResponse'. Did you mean 'authed'?
```

**M4 — the voice card renders the provider the console never sends.**

```
src/features/jarvis/components/Jarvis.test.tsx > renders no provider name even when the answer carries one
AssertionError: expected <span class="ml-1 text-muted"></span> to be null
```

**M5 — the catalogue stops binding a console-native path to its row.**

```
src/app/api/console_rows.test.ts > binds the paths the browser's console-native views read
AssertionError: expected undefined to be 'SseTokenResponse'
```

## 5. What is untouched

- **No daemon behaviour changed.** The console serves the same bytes; the
  browser reads them. The contract is untouched — the rows were already
  declared; the browser had simply not read them.
- **The published OpenAPI document did not move** (`docs/openapi.json`): it
  describes the operation-bound surface, and the console's own endpoints are
  not operation bindings.
- **No layer rule moved.** `archcheck` still reports 0 violations; the new
  browser test lives with the browser's own ratchets.
- **The gate is green**: `gofmt`, `go build ./...`, `go vet ./...`,
  `go test ./...`, `depscheck`, `sdkgen -check`, `deadcodecheck`, `archcheck`,
  `bodydoor`, `structure-md -check`, `openapi -check`, `docsindex`,
  `changelog-lint`, `docclaimscheck`, `gitleaks`, and the frontend's
  `npm test`, `npm run typecheck`, `npm run build`, `npm run deadcode`.

## 6. What remains open

- **The console's file-tree answer has no row at all.** The browser still
  spells it by hand (`lib/files.ts`: `FileNode`, `FileTreeResponse`) — and the
  daemon's `fileTreeResponse` carries no `children`, which the browser's copy
  promises. The console's own contract does not declare that row yet, so the
  browser has nothing to bind to. That is the next slice: name the row in
  `kernel/contract/consoleapi`, let the native route name it, and let the
  browser bind to it.
- **The console's error envelopes are still maps** — a shape, not a row.
- **The console's own answers are not in `docs/openapi.json`**, and the
  document says so. A caller outside the browser still has no published shape
  for `/api/authmeta` or `/api/rollback/checkpoints`.

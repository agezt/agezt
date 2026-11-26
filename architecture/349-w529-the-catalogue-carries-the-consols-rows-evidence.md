# W5.29 — the browser's catalogue carries the console's rows: the browser's rollback view is pinned, and it works again

## 1. What was wrong

W5.28 gave every native route a name for the row it answers with. The row was
declared — but the **browser** could not see it: `tools/openapi` rendered the
operation registry's schemas, and the console's own answers were not operation
bindings, so they were not in the catalogue at all.

So `ResponseOf<"/api/rollback/checkpoints">` resolved to `any`, and the browser
wrote its own view by hand. It had been doing so for a long time, and the view
had **drifted**:

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
them decided whether the operator could act at all:

```tsx
<Button … disabled={!cp.reversible}>Restore</Button>
```

`reversible` never arrives, so the **Restore button was permanently disabled**:
the timestamps rendered as "—", the "files affected" column was empty, and the
daemon's rollback apply endpoint — which works — could not be reached from the
browser. Nothing failed, because nothing said the browser was reading a daemon
row.

There was a **second** copy of the same row, in `components/RunDetail.tsx` —
this one spelled the daemon's fields *correctly*, and it was pinned to the
**artifact** row (`ArtifactCollectResponseCandidatesItem`) because the rollback
row was not in the catalogue to be pinned to.

## 2. What was built

### 2.1 The struct becomes a schema

The console's answers are Go **structs**; the browser's catalogue is rendered
from **schemas**. `tools/openapi/structschema.go` is the step between the two:

```go
func schemaOf(value any) (json.RawMessage, error)
```

It keeps the contract's own rules: a member without `omitempty` is required,
one with it may be absent, and a **pointer** says *may be null*. A free-form
value (`any`, a raw JSON body) says nothing — the schema is empty and the
browser reads it as `any`.

### 2.2 The catalogue carries the rows

`renderTypes` now takes the console's native routes and renders every route
that names a row — the row's **struct** is the source, so the field names live
in the contract and nowhere else:

```ts
export interface RollbackCheckpointsResponseCheckpointsItem {
  action: string;
  applied_ms?: number;
  before?: Record<string, any>;
  before_status?: string;
  created_ms: number;
  id: string;
  kind: string;
  reason?: string;
  run_id?: string;
  subject_id: string;
  subject_name?: string;
}
```

and a **native response map** binds the paths to them:

```ts
export interface NativeResponseMap {
  "/api/authmeta": AuthmetaResponse;
  "/api/login": LoginResponse;
  "/api/logout": LogoutResponse;
  "/api/rollback/checkpoints": RollbackCheckpointsResponse;
  "/api/rollback/apply": RollbackApplyResponse;
  "/api/sse-token": SseTokenResponse;
  "/api/voice/status": VoiceStatusResponse;
  "/hooks/": WorkflowHookResponse;
}
```

`ResponseOf` consults it, so the browser's `getJSON("/api/rollback/checkpoints")`
returns the row's type instead of `any`.

### 2.3 The browser's view is the daemon's row

`Backups.tsx` no longer names a field of its own:

```ts
type Checkpoint = RollbackCheckpointsResponse["checkpoints"][number];
```

and it reads the daemon's real fields: `created_ms` (not `created_at`),
`subject_name` (not `description`), the file path from the snapshot's `before`,
and `before_status`. The **Restore button** now follows the **daemon's** rule —
a checkpoint that carries `applied_ms` has already been restored, and the
daemon answers "already applied" if it is asked again, so the button is
disabled for exactly those.

`components/RunDetail.tsx`'s copy of the row became an alias of the catalogue's
row too, and its **pin** — the one that bound it to the artifact row — was
deleted: the view names no field by hand, so it needs no pin.

### 2.4 The ratchets

**A — the catalogue's own** (`tools/openapi/native_rows_test.go`). Every native
route that names a row must be rendered into the catalogue and bound to its
path; and the schema renderer must keep the contract's rules.

**B — the browser's own** (`viewrows.ratchet.test.ts`). It caught the second
copy the moment the row entered the catalogue:

```
components/RunDetail.tsx:RollbackCheckpoint copies the daemon's RollbackCheckpointsResponseCheckpointsItem by hand — take the members' types from the daemon
```

**C — the compiler.** A view that names a field the daemon does not send does
not compile:

```
src/features/admin/components/Backups.tsx(193,37): error TS2551: Property 'created_at' does not exist on type 'RollbackCheckpointsResponseCheckpointsItem'. Did you mean 'created_ms'?
```

## 3. What was measured

| | before | after |
|---|---|---|
| console rows in the browser's catalogue | **0** | **8** |
| native paths bound to a row in the catalogue | **0** | **8** |
| browser views that copy the daemon's rollback row by hand | **2** | **0** |
| browser members the daemon does not send (the rollback view) | **6** | **0** |
| the Restore button | **permanently disabled** | **enabled unless the daemon says it is applied** |

## 4. The mutations

Five reverted mutations prove the slice (`.temp_files/w529_mutations.py`; every
mutation was reverted and the tree is back to green).

**M1 — the catalogue stops rendering the console's own rows.**

```
the native route "authmeta" names the row it answers with, but the browser's catalogue declares no type for it
```

**M2 — the schema renderer requires every member.**

```
the row's schema requires a member the contract may omit: {"properties":{"created_ms":…,"run_id":…
```

**M3 — the browser's view copies the daemon's row by hand again.**

```
features/admin/components/Backups.tsx:Checkpoint copies the daemon's RollbackCheckpointsResponseCheckpointsItem by hand — take the members' types from the daemon
```

**M4 — the browser's view names a field the daemon does not send.**

```
src/features/admin/components/Backups.tsx(193,37): error TS2551: Property 'created_at' does not exist on type 'RollbackCheckpointsResponseCheckpointsItem'. Did you mean 'created_ms'?
```

**M5 — the catalogue's native map binds no path to a row.**

```
the native route "authmeta" names the row it answers with, but the catalogue's native response map binds no path to it
```

## 5. What is untouched

- **No daemon behaviour changed.** The console serves the same bytes; the
  browser reads them.
- **The published OpenAPI document did not move** (`docs/openapi.json`): it
  describes the operation-bound surface, and the console's own endpoints are
  not operation bindings — that is stated in the document's own description.
- **No layer rule moved.** `archcheck` still reports 0 violations; the schema
  renderer lives in the tool, not in the kernel.
- **The gate is green**: `gofmt`, `go build ./...`, `go vet ./...`,
  `go test ./...`, `depscheck`, `sdkgen -check`, `deadcodecheck`, `archcheck`,
  `bodydoor`, `structure-md -check`, `openapi -check`, `docsindex`,
  `changelog-lint`, `docclaimscheck`, `gitleaks`, and the frontend's
  `npm test`, `npm run typecheck`, `npm run build`, `npm run deadcode`.

## 6. What remains open

- **The browser's other console-native views are still hand-written**: the auth
  meta (`Login.tsx` reads `password_required` and `authed` by hand), the voice
  status, the SSE token. The rows are now in the catalogue, so they can become
  aliases — one family at a time.
- **The console's own answers are not in `docs/openapi.json`**, and the
  document says so. A caller outside the browser still has no published shape
  for `/api/authmeta` or `/api/rollback/checkpoints`.
- **The `/hooks/` path is a prefix**, not a path the browser calls: the hook's
  row is rendered (the route names it), but the browser never fetches it.
- **The console's error envelopes are still maps** — a shape, not a row.

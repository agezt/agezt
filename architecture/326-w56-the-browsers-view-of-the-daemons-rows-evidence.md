# W5.6 — the browser's view of the daemon's rows

## 1. What was wrong

W5.1 gave the browser its **payload** types from the daemon's registry: the
paths and their JSON shapes are rendered, not written. The browser still wrote
its own **view** of every daemon row by hand — the shape it reads after the
payload arrives — and those views were a second contract.

The measurement (`.temp_files/w56_scope.py`, `w56_scope2.py`,
`w56_direction.py`):

- The browser held **470** hand-written types and interfaces.
- **126** of them copied a daemon row field for field.
- Of those, **19** were convertible to a plain narrowing of the daemon's row,
  **4** were built locally on purpose (form models the browser owns), and
  **91** were blocked by an **optionality conflict**: the browser declared a
  field optional (or narrower) where the daemon declares it otherwise.
- The conflicts covered **570** fields. **568** were *defensive*: the browser
  marked a field the daemon ALWAYS sends as optional. **2** were *false*: the
  browser promised a field the daemon may not send at all
  (`ScheduleWorkflow.name`, `QueuedMsg.id`).

The renderer also had a rule that made the conflict invisible: `null` was
folded into "absent" for the whole catalog. A field the daemon may send as
`null` looked like a field the daemon may omit, so the browser's `?` was
"defensive" and nobody could tell a real conflict from a harmless one.

## 2. What was built

### 2.1 The null arm survives where the browser has no other warning

`tools/openapi/types.go` — the renderer keeps the registry's null arm **only
on a member the daemon always sends**. Such a member carries no `?`, so
without the arm the browser would read a `null` as a value (`row.halted.length`
on a null). A member the daemon may omit carries the `?` already, and that
`?` forces the guard a null needs — the arm would only make the type noisier.

The rule is not a promise the browser breaks silently: it is the **cost** of
the rule that the compiler charged. Restoring the null arm where the browser
needs it produced **56** type errors — the compiler naming, file by file, the
places where the browser's view and the daemon's row disagreed. (W5.1 measured the blunt versions: keeping the null arm **everywhere** broke the
browser in **149** places, and dropping it **everywhere** left **38** call sites
reading a null as a value. The narrow rule costs **56** compile errors across
**25** files — and every one of them names a place where the browser's view
and the daemon's row disagreed.)

The rule also exposed a **precedence bug in the renderer**: a nullable array
item was rendered `X | null[]` — an array of nulls, not an array of
possibly-null elements. The compiler found it through
`InboxResponse.threads`; the renderer now parenthesises the element union
(`(X | null)[]`).

### 2.2 The browser's view types are the daemon's rows, narrowed

The browser names the fields it reads; the daemon says what those fields are.
Every view type in the families the compiler pointed at is now written as a
narrowing of the daemon's row — `Pick<Row, …>` or a field list whose types are
indexed accesses into the row (`Row["field"]`). The **optionality** stays the
browser's (a field it reads only sometimes may still be optional); the **type**
is the daemon's.

The families the slice bound (the compiler's list): setup, chains, catalog /
model picker, ACP, reaper + repair, permissions + wake access, fleet orders,
autonomy feed, channel sessions, channels, connections, council, data
collections + records, execution profiles, models, policy (edict + redaction),
sandbox, standing orders + history, workflows (draft, runs, templates),
config centre and routing.

Two habits carried the work:

- **A nullable list is normalised at the boundary.** The page reads a list,
  never a null: `setRows(d.channels ?? [])`. The normalisation is where the
  daemon's null arm is answered for, and the compiler insists on it — a view
  that promises `T[]` where the daemon sends `T[] | null` does not compile
  until the boundary says where the null goes.
- **A test fixture is a row the daemon can produce.** The setup and catalog
  fixtures now build the daemon's row (every field the daemon sends), not a
  partial object cast into place. A fixture that leaves a field out is a
  fixture the daemon cannot produce, and the type system now says so.

### 2.3 The daemon was dropping a field the browser read

The browser's workflow node event read `output_truncated` — the flag the
workflow engine writes into the journal when it snips a node's output. The
daemon's `workflow.NodeEvent` row had **no such field**: the journal carried
the flag, the run list dropped it, and the canvas's "output (truncated)" hint
could never fire. The daemon's row now carries it
(`kernel/app/workflow/runs.go`), the registry's schema (derived by reflection)
declares it, the generated type has it, and the browser's field is the
daemon's.

## 3. The guards

- **`tools/openapi/types_ratchet_test.go`** — three tests on the renderer's
  rules, because the browser's types are generated from them: a required
  nullable member keeps its null arm; an optional nullable member drops it; a
  nullable array item is parenthesised.
- **`frontend/src/app/api/viewtypes.ratchet.test.ts`** — **35** compile-time
  assertions across the bound families: the daemon's field type must be
  assignable to the browser's view of it. A browser that narrows the daemon's
  contract fails the typecheck gate, and the failure names the field. One
  runtime test echoes the rule on the generated catalog (the null arms are
  still there, and no array element wears the array's null).

## 4. The mutation evidence

Four mutations, four caught, all reverted:

| # | Mutation | Caught by |
|---|---|---|
| M1 | The renderer stops keeping the null arm (`renderMode.null = false`) | `TestARequiredNullableMemberKeepsTheNullArm` — *"a required nullable member lost its null arm: a: string;"* |
| M2 | The renderer drops the array element's parentheses | `TestANullableArrayItemIsParenthesised` — *"the element's null arm is not parenthesised: RowItem[]"* |
| M3 | The browser's setup view narrows the daemon's `env` to `string[]` | the frontend ratchet — `viewtypes.ratchet.test.ts(79,3): Type 'false' does not satisfy the constraint 'true'` — and the setup fixtures (`Setup.test.tsx`: the daemon's row is not assignable to the narrowed view) |
| M4 | The browser's workflow view drops the daemon's `output_truncated` | the typecheck — `page.tsx(721,36): error TS2339: Property 'output_truncated' does not exist on type 'WfRunNodeEvent'` |

## 5. Verification

- `go run ./tools/openapi -check` — the three generated artifacts are in sync.
- `npx tsc --noEmit` — **0** errors (was **56** after the null arm was
  restored).
- `npx knip` — clean (four view types that the slice made local again are no
  longer exported).
- `go test ./...` — green, including the new renderer ratchet.
- `go vet ./...`, `gofmt -l` — clean.
- `archcheck` (340 packages, 0 violations, 0 allowlisted edges),
  `deadcodecheck` (0 unexpected), `depscheck`, `sdkgen -check`,
  `structure-md -check`, `changelog-lint`, `docclaimscheck`, `docsindex` —
  green.
- `staticcheck` is CI-only (no prebuilt binary on this machine); it did not
  run here.

## 6. What did not change

- The daemon's wire protocol. One field was **added** to the workflow run
  list (`output_truncated`), and it was already in the journal; every other
  response is byte-identical.
- The browser's behaviour, except where a null the daemon may send would have
  crashed it: those are now answered for at the boundary (`?? []`, `?? {}`).
- The paths, verbs, payloads and operation names.
- The **4** locally built browser types (form models the browser owns) and the
  **2** false promises (`ScheduleWorkflow.name`, `QueuedMsg.id`) are still
  there — they are not daemon rows, so they have no daemon row to bind to.

## 7. What comes next

The **91** types the first measurement blocked on optionality are now
unblocked: the optionality conflict was measured against a catalog that had
folded null into absent. A fresh measurement over the **470** browser types
will say how many copies of a daemon row are left, and the remaining
hand-written views (the ones that are not daemon rows at all — the browser's
own form models, the run-gallery rows, the board rows) can be classified
against that number.

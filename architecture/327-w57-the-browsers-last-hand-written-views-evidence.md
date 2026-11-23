# W5.7 — the browser's last hand-written views

## 1. What was wrong

W5.6 gave the browser its **payload** types from the daemon's registry and
bound the views the compiler pointed at. What it left behind was the rest of
the browser's own vocabulary: the browser still wrote **358** hand-written
types of its own, and **153** of them copied a daemon row field for field —
**98** of them still by hand, in **44** files.

Those copies were a second contract. Measured field by field against the
daemon's rows (the same tool before and after — `.temp_files/w57_conflicts.py`):

| axis | fields | what it means |
|---|---|---|
| defensive | **318** | the daemon always sends the field; the browser said optional |
| false promise | **7** | the daemon may omit the field; the browser said required |
| narrowing | **6** | the browser's type is a strict subset of the daemon's |
| null-blind | **10** | the daemon may send `null`; the browser's type does not admit it |

**341** fields over **80** types. Two more kinds did not show up as a field
conflict at all: the browser named fields the daemon's row **does not have**
(phantoms), and it read the daemon's rows through **64 casts** onto 30
hand-written types — the compiler's blind spot, where a second contract can
hide anything.

The false promises were the interesting ones. Two of them (`ScheduleWorkflow.name`,
`QueuedMsg.id`) were left by W5.6 as "no daemon row to bind to"; this slice
found what they really are. `ScheduleWorkflow` is the browser's own pick-list
for a schedule, and the daemon's *request* schema types every field as `any` —
so there was never a daemon row to promise. `QueuedMsg` is the browser's own
**message queue** (what the operator typed, waiting to send): the name
coincided with a board message's `id`, and the daemon's row was never in the
picture. Neither was a broken promise; both were a measurement that matched
names instead of dataflow.

## 2. What was built

### 2.1 The views take the daemon's types

**44** browser files. Every view that copies a daemon row now names its
fields and takes their types from that row — `Row["field"]`, `Pick<Row, …>`,
or a type that does the same. The optionality stays the browser's (a field it
reads only sometimes may still be optional); the type is the daemon's.

The typecheck was the oracle: the binding produced **40** compile errors, and
every one of them named a place where the browser's view and the daemon's row
disagreed. All **40** are resolved, and the typecheck is back to **0**.

### 2.2 The browser's live status IS the daemon's status row

The browser folds the event feed into a live status for the roster. That type
(`AgentRuntimeStatus`) named **69** fields — and the measurement found that
**every one of them** is a field of the daemon's own status row
(`AgentListResponseProfilesItemStatus`, 81 fields). It had **no field of its
own**: it was a hand-written copy of the daemon's row, with four literal
unions and one record narrowed. It is now a partial view of that row
(`extends Omit<Partial<AgentListResponseProfilesItemStatus>, …>`), and the two
members the daemon sends untyped stay the browser's own shape.

### 2.3 Two fields the daemon never sends

- **`Sched.once_at_unix`** — the daemon's schedule **row** has no such field
  (only the *request* schema does), so the browser's view promised a field
  that was always `undefined`. The one-shot's fire time is `next_run_unix`,
  which the UI already fell back to. The field is gone.
- **`ChannelProbe.note`** — the daemon's channel probe row carries no `note`,
  so the badge's tooltip fell through to its own words every time. Gone.

### 2.4 The daemon's terminal cursor is an empty string

The daemon sends `next_cursor: ""` when a list has no next page (and omits
the field on some routes). The pager read `""` as a cursor, so `hasMore`
stayed **true** on the last page: the footer kept offering "load more" and
clicking it did nothing, because the request was never sent. The pager now
reads both shapes — `""` and absent — as "no more pages". The runs pager test
had the same fiction (a `null` cursor the daemon never sends) and now uses the
daemon's shape.

### 2.5 The views that the daemon always fills

The channel field view called `env_pinned`, `help`, `required`, `secret` and
`set` optional; the daemon always sends them. The reload boundary's `envs`
list is the browser's **own** (it groups the schema's sections), so the view
says "a list" and the daemon's nullable row stays nullable.

## 3. The guard

**`frontend/src/app/api/viewrows.ratchet.test.ts`** — the browser may not
spell a daemon row by hand. The test reads the generated catalog, walks the
browser's sources, and finds every hand-written type that copies a daemon
row (three or more members, all of them fields of one row). Such a type must
take its members' types from the daemon's vocabulary; the ones that do not
are the browser's own and are listed.

The ledger has **5** entries and **two teeth**: a copy that is not on the
list fails, and a listed type that no longer copies (or has been bound)
fails too — a stale allowance is a lie about the code. The floor is three
members on purpose: two field names in common ("id", "label") are a
coincidence of English, not a copy, and a ratchet that cries wolf gets
ignored. The five entries are the browser's own one-line badge summaries
(label + detail + a tone), which the browser **builds** — they are not the
daemon's `next_action` row.

The existing compile-time guard
(`frontend/src/app/api/viewtypes.ratchet.test.ts`, 35 assertions) still holds
the families W5.6 bound; this slice's guard holds the *shape* of every view
in the tree.

## 4. The mutation evidence

Three mutations, three caught, all reverted:

| # | Mutation | Caught by |
|---|---|---|
| M1 | A hand-written copy of the daemon's run row appears (`SneakyRun`) | the ratchet — *"features/runs/components/Runs.tsx:SneakyRun copies the daemon's RunsListResponseRunsItem by hand"* |
| M2 | A bound view loses the daemon's types (three members of `PolicyDecision` go back to plain `string`/`number`) | the ratchet — *"…/shared.tsx:PolicyDecision copies the daemon's EdictLogResponseDecisionsItem by hand"* |
| M3 | A stale allowance on the ledger (`Run` listed, but it no longer copies) | the ratchet — *"features/runs/components/Runs.tsx:Run is on the ledger but no longer copies a daemon row"* |

## 5. Verification

- `npx tsc --noEmit` — **0** errors (was **40** right after the binding).
- `npx knip` — clean.
- `npx vitest run` (whole suite, same machine, same conditions):
  **before** 89 failed / 1355 passed, **after** 73 failed / 1373 passed — the
  slice *fixed* 19 and the failing set moved. The three that failed only
  after the slice are **load-dependent timeouts** (one of them ran 5380 ms
  against the suite's 5000 ms limit); each of those files passes on its own
  (30/30). The tree's baseline on this machine is unstable under a full
  suite (89 failures against the 77 recorded earlier), which is why the
  comparison is by failing test id and not by count.
- `go test ./...`, `go vet ./...`, `gofmt -l` — clean (no Go file changed).
- `archcheck` (340 packages, 0 violations, 0 allowlisted edges),
  `deadcodecheck`, `depscheck`, `sdkgen -check`, `structure-md -check`,
  `changelog-lint`, `docclaimscheck`, `docsindex` — green.
- `staticcheck` is CI-only (no prebuilt binary on this machine); it did not
  run here.

## 6. What did not change

- The daemon's wire protocol. **No** Go file changed in this slice: the
  daemon's rows were already right, and the browser was the one that
  disagreed.
- The browser's behaviour, except where a field the daemon never sends was
  read (both were already dead) and where the terminal cursor was read as a
  cursor (the "load more" button now disappears on the last page).
- The **2** false promises W5.6 left: they are not false promises, and the
  evidence above says why.

## 7. What comes next

The browser's views are now the daemon's rows, and a guard holds that line.
What is left is the daemon's own side of the same dictionary: the request
schemas are still typed as `any` field by field (the daemon's
`ScheduleAddRequest`, `StandingAddRequest`, `ChannelAccountSetRequest` and
friends), so a request row is a shape the browser can only promise against.
The same treatment — the registry's schema for the **request** side, and a
browser that sends the request row the daemon declares — is the next end of
"the dictionary gets one home".

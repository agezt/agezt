# W5.21 — the browser pins its payload views: a renamed daemon field is a compile error in the browser too

## 1. What was wrong

W5.1's sentence — *the daemon's contract is one document, and the browser reads
it* — was true for the **request** side and false for the **response** side.

The daemon's rows live in one place: the generated catalog
(`frontend/src/app/api/types.generated.ts`, rendered from the operation
registry by `tools/openapi`). The browser's own **payload views** — the
interfaces that say "this is what one agent profile looks like", "this is what
one repair status looks like" — were, by and large, **hand-written**. A view
that names the daemon's fields by hand is a **second contract**: it can promise
a field the daemon never sends, and — the way that bites — it can keep
**reading a field the daemon has renamed**.

The failure is silent in the worst way. The daemon renames `status` to `state`
in the profile row; the browser's `AgentProfile.status` still compiles, still
type-checks, still reads `undefined` at runtime, and the panel that renders it
shows a blank where a state used to be. Nothing fails. There is no build to
break, because **nothing in the browser ever said it was reading a daemon
field** — it said it was reading *a* `status`.

The programme's own hands-off note named this as its last open end:

> the **frontend's payload types are still hand-written**: about 160 frontend
> interfaces describe what the operation registry already describes … This is
> W5.1's open end, and it is the last place a rename can still pass silently.

## 2. What was built

### 2.1 The rule

A browser view of a daemon row must not **name** a daemon field without
**pinning** it. A field is pinned in one of two ways:

- **derived** — the browser writes the field's type as an access into the
  catalog (`AgentListResponseProfilesItem["slug"]`). A rename in the daemon is
  already a compile error there, because the access stops resolving.
- **pinned** — the field is the browser's own shape of a daemon field (the
  daemon's `status` row is folded into the browser's `AgentRuntimeStatus`; the
  daemon's `history` is a list of the browser's `AgentRepairEvent`), so the
  browser cannot simply write the catalog's type. Those fields are pinned in
  one place: `frontend/src/app/api/payloadtypes.ratchet.test.ts`.

### 2.2 What a pin is

A pin is a **compile-time assertion** that `PICK`s the field out of the
daemon's row:

```ts
// @pin lib/agentdetail.ts:AgentRepairStatus | AgentRepairStatusResponse | pinned=history,inflight,latest,contract,next_action | own=
type _pin_AgentRepairStatus = Expect<
  AllTrue<
    FieldsComparable<
      Pick<AgentRepairStatusResponse, "history" | "inflight" | "latest" | "contract" | "next_action">,
      Pick<AgentRepairStatus, "history" | "inflight" | "latest" | "contract" | "next_action">
    >
  >
>;
```

Three things happen in that one line:

1. **The name is pinned.** `Pick<Row, "history">` is a compile error if the
   daemon's row no longer has a `history` — *"Type '"history"' is not assignable
   to type 'keyof AgentRepairStatusResponse'"*. This is the whole point of the
   slice: **a renamed daemon field is a compile error in the browser.**
2. **The type is compared.** `FieldsComparable` holds the daemon's field type
   and the browser's to each other. The browser may **narrow** (promise a
   subset of what the daemon sends — the daemon's `status` is a `string`, the
   browser reads a union of states), and the comparison ignores the `null` and
   `undefined` arms, because the daemon may send `null` and the browser may
   read the field as absent. It may **not** be a different shape: `string[]`
   and `ChatTool[]` are not comparable, and the pin fails.
3. **The marker is the pin.** The `// @pin …` line above the assertion is
   what the runtime ratchet reads, so the data the runtime checks and the data
   the compiler checks cannot drift apart.

### 2.3 The ratchet

The runtime half of the file walks the browser's sources and the generated
catalog, and asks, per hand-written view:

> does this view share three or more field names with a daemon row, and does it
> **name** any of them by hand?

Three fields is the floor, for the same reason `viewrows.ratchet.test.ts` uses
it: two field names in common (*"id"*, *"name"*) are a coincidence of English,
not a copy, and a ratchet that cries wolf gets ignored.

Every view that answers "yes" must be **pinned** or **declared**. And both
lists are **ratchets in both directions**:

- a view that names a daemon field with no pin **fails** — new debt cannot
  land;
- a pin whose fields are no longer named by hand **fails** — a stale pin is a
  lie about the code;
- a **ledger** entry — a view that is entirely the browser's own vocabulary —
  that stops being a payload view at all **fails** too, so the browser's own
  shapes cannot be quietly re-listed as daemon views.

The ledger is where the browser's own vocabulary is named, with the reason:

```
// @own features/agents/components/agentdetail/capability.tsx:WorkflowToolAccessSummary | the browser's badge summary, not the daemon's next_action row
// @own lib/fleet.ts:FleetEntity | the browser's fleet model, not a daemon row
// @own nav.tsx:NavItem | the browser's navigation, not a daemon row
```

**Twenty-three** entries live there: the badge summaries the browser builds
(`label`, `detail`, `tone`), its fleet model, its navigation, its tabs, its
MCP preset catalogue, its voice catalogue, its quick-action card and its
learned-memory record. These are not daemon rows and must not be pinned as
such — and one of them, `Backups.Checkpoint`, is named for a **real** gap: the
daemon's rollback operation is not in the generated catalog yet.

### 2.4 The browser's own types now say their names

Sixteen payload views were **file-local**, so a pin could not name them. They
became named exports — one word each, no behaviour change, no new surface of
consequence (`knip` reports no dead export, because the pin file imports
every one of them).

Three views were also **re-derived** while the slice was measured, because
their types were simply the daemon's:

- `RepairProposal` (`lib/agentrepair.ts`) — `soul`, `model`, `fallbacks`,
  `task_type` and `config_overrides` now come from
  `AgentListResponseProfilesItem`;
- `EffectiveToolPermission` (`…/agentdetail/capability.tsx`) — its six
  daemon fields come from `AgentCapabilitiesResponsePermissionsItem`;
- `PolicyStats` (`…/agentdetail/shared.tsx`) — its five daemon fields come from
  `EdictStatsResponse`.

## 3. What was measured

The browser's own tree, measured by the gate itself:

| | before | after |
|---|---|---|
| hand-written browser type declarations with an object body | 341 | 341 |
| their members | 2 318 | 2 318 |
| **payload views** of a daemon row (≥3 shared fields, ≥1 named by hand) | **68** | **68** |
| their shared fields, **derived** from the catalog | 148 | 148 |
| their shared fields, **named by hand** | **246** | **246** |
| …of those, **pinned** to the daemon's row | **0** | **164** (in **45** pins) |
| …of those, **declared the browser's own** | **prose in a document** | **82** (in **23** ledger entries) + **1** inside a pin (`ChatTurn.tools`) |
| daemon rows the browser is held to | **0** | **28** |
| a daemon field renamed behind the browser's back | **silent** | **a compile error in the browser** |

The **before** side is not a different tree: it is the *same* tree with the
same 68 views and the same 246 hand-named fields, and nothing that checked
them. The daemon's catalog already said what those fields were; the browser
simply did not say so.

## 4. The mutations

Four reverted mutations prove the slice. Two are **the daemon's** rename — the
case the slice exists for — and two are the **browser's** drift.

**M1 — the daemon renames the repair status's `history`.** In
`kernel/app/roster/repair_status.go` the wire tag becomes `events`, the
catalog is regenerated (`go run ./tools/openapi`), and the browser's
typecheck says:

```
src/app/api/payloadtypes.ratchet.test.ts(423,39): error TS2344: Type '"contract" | "history" | "inflight" | "latest" | "next_action"' does not satisfy the constraint 'keyof AgentRepairStatusResponse'.
  Type '"history"' is not assignable to type 'keyof AgentRepairStatusResponse'.
```

and the runtime ratchet says, in its own words:

```
"lib/agentdetail.ts:AgentRepairStatus pins history, which it no longer names by hand",
```

**M2 — the daemon renames the profile row's `status`.** The tag in
`kernel/app/roster/list_output.go` becomes `state`. The browser's typecheck
fails in **five** new pins at once — `AgentLivePatch`, `AgentLiveTarget`,
`AgentProfile`, `ApiProfile` and `IncidentProfileLite` all read that row's
`status` — and the runtime ratchet names four of them:

```
"features/agents/lib/agentlive.ts:AgentLiveTarget pins status, which it no longer names by hand",
"lib/agentshared.ts:AgentProfile pins status, which it no longer names by hand",
"lib/fleet.ts:ApiProfile pins status, which it no longer names by hand",
"lib/incidents.ts:IncidentProfileLite pins status, which it no longer names by hand",
```

This mutation is the slice's proof of worth: **no earlier ratchet caught it.**
The old `viewtypes.ratchet.test.ts` pinned a handful of flows by hand; the
profile row's `status` was not among them, because the browser's `status` is
its own `AgentRuntimeStatus` and never named the daemon's field.

**M3 — the browser names a daemon field with no pin.** One line added to
`ChatTool` (`seq?: number;` — `seq` is a field of the daemon's tool-log row):

```
"lib/chat.ts:ChatTool names the daemon's seq by hand and does not pin it",
```

**M4 — a pin goes stale.** `AgentProfile.lifecycle` re-derived to the
catalog's own type, which is the *right* thing to do and therefore the pin's
death:

```
"lib/agentshared.ts:AgentProfile pins lifecycle, which it no longer names by hand",
```

All four mutations were reverted; the tree is back to green.

## 5. The guards

Three guards hold the slice to its own question, all in
`frontend/src/app/api/payloadtypes.ratchet.test.ts`:

- a view that names a daemon field with no pin **fails**;
- a pin that no longer matches the view's hand-named fields **fails**, in both
  directions (a field named but not pinned, and a field pinned but no longer
  named);
- a ledger entry that is no longer a payload view **fails** — and so does a
  pin with no compile-time assertion behind it, because a pin without teeth is
  a comment.

The compile-time half is guarded by `tsc --noEmit`, which CI already runs.

## 6. What is untouched

- **No daemon code changed.** The slice is the browser's; the daemon's rows
  were read, not moved.
- **No contract changed**, so no layer rule moved (`archcheck` still reports 0
  violations).
- **No SDK moved** (`sdkgen -check` is in sync) and the **generated catalog**
  is byte-identical to `main`'s — the daemon's wire format did not change.
- The only production-source changes are **sixteen `export` keywords** and
  **three re-derived view types**; the browser's behaviour is unchanged.

## 7. What remains open

- **The browser's `null`-arm leniency stays.** The comparison deliberately
  ignores `null` and `undefined`, so a browser that reads a nullable daemon
  field as absent is not a violation. That is the browser's own leniency, and
  the daemon's catalog still says the field may be `null` — the pin does not
  force the browser to handle it, and does not stop it.
- **The narrowing the browser already does stays.** The daemon's `status` is a
  `string`; the browser reads a union of states. The pin allows a narrowing
  (the browser may promise a subset) but cannot prove the daemon only ever
  sends those states — the catalog does not say so, and making it say so is a
  daemon-side decision, not this slice's.
- **One daemon operation is not in the catalog at all**: the console's
  rollback checkpoints (`/api/rollback/checkpoints`). The browser's
  `Checkpoint` view is therefore named in the ledger as the browser's own
  shape, with the reason. When that operation joins the registry, the view
  becomes a pin.
- **The SDKs do not carry the browser's views.** The Rust SDK speaks
  `/api/v1`; the browser's payload views are the console's own, and the two
  are not in one catalogue.

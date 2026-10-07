# W2.14 standing native exit evidence

Prepared on the shared main checkout. Protected delivery remains local while Git
metadata is read-only and GitHub authentication returns HTTP 401. This closes the
seven-command native standing migration; resident execution and wider module,
trigger and generated-surface work remain open.

| Operations | Typed ownership and policy |
|---|---|
| standing_list, standing_why | Primary-only unaudited unary reads over actual order and journal ports. |
| standing_add, standing_edit, standing_set_enabled, standing_remove | Primary-only unary mutations through the existing runtime facade; durable operation audit before state effects. |
| standing_fire | Primary-only unary dispatch admission; durable operation audit before the injected resident callback. |

## Registry and source ownership

TestStandingNativeMetadataComesFromSevenTypedAppSpecs checks the exact aggregate,
actual Go input/output types and schemas, unary framing, AppOwned compatibility
metadata, primary authorization/routing, read/write policy and unknown-arg
compatibility. Renaming only fire or why independently fails this exit test.
CRUD/why/fire native business handlers, their registration and the last agent
validation forward are removed. The selected service factory and public
SetStandingFire bridge remain; callback injection is resolved per dispatch.

App services own projections, list annotations/counts/warnings, agent admission,
CRUD presence/conversions, history filters and manual fire admission. Runtime
facade writes retain durable store identity/timestamps/rollback and existing
best-effort standing.created/updated/removed publication. No direct store bypass
replaces that facade. The injected callback remains the resident runner boundary.

## Compatibility evidence

Order projection preserves the embedded core wire, required zero/empty/false
fields and optional omissions. Empty lists remain arrays. Enabled count includes
blocked orders; blocked/ready annotations stay list-only. Cron warning wins over
low event cooldown. Agent validation keeps trimmed lookup, retired-before-paused
and managed-call refusal. Add retains raw agent; Edit trims only requested agent.
Edit decodes all patch fields before validation or writes, retaining first type
error, fractional assure/cooldown truncation and unknown updated:false omission.
Resume validates before the facade; pause bypasses the agent gate.

Raw request fields preserve absent/null/wrong types, required untrimmed IDs and
order re-encoding. Observed non-bool enabled inputs continue to become false.
Why retains standing.* prefix, exact string payload ID, malformed/foreign skips,
journal order, six required event fields (including zero sequence/timestamp),
events:null when empty and legacy ignored Range errors/partial results. Fire
checks callback availability before lookup, returns missing fired:false/request
ID, validates found agent before callback and retains callback bool/request ID.

Old/new native foundation parity covers five CRUD handlers x fifteen inputs and
two why/fire handlers x eight inputs x three callback modes, count=20. Typed native
binding repeats both comparisons through app dispatch, preserving response,
actual store state and callback effects. Only generated IDs, clocks bounded by
two seconds and generic framework error-code boundaries are normalized.
Owned actual stores/journals and runtime lifecycle fixtures complement port tests.

## Audit and tenant evidence

Actual closed-journal tests block all five mutation commands before state,
provider or callback effects. List/Why add no operation audit. Fire unavailable
and declined cases settle once with failed/completed events joined to invoked
by the owned nonempty correlation. Raw correlation spoof cannot choose identity.
Runtime lifecycle events keep their original empty correlation; correlated domain
publication is not claimed. Actual socket tenant credentials are denied all seven
commands without changing primary orders or calling the provider/callback.

## Validation and limits

35 valid mutations across a-d: CRUD/projection 12, history/fire 9, binding/audit
12 and exact exit 2. All fail regression assertions, not compilation. Exit
mutations restore exact production bytes from c. Full Go tests, build/vet and
complete controlplane race count=1 passed on that unchanged c production state.
Final scoped native/CLI and service/core/journal race count=20 pass, followed by
staticcheck, archcheck, deadcodecheck, depscheck, official structure, gofmt,
doc/changelog, owned source secret scan and patch-chain checks.

Counts: 233 placed packages, 139 import/13 call exceptions, 28 SDK findings plus
one test seam, 24 justified dependencies, 124 generated kernel packages.

BenchmarkDispatchWithoutAuditIO, three 100ms runs: 6355/6785/6483 ns/op,
7608/7607/7607 B/op, 89/89/89 allocs/op. All meet the <50us framework budget.
This mock-host benchmark excludes audit/journal I/O and standing/provider latency.
Owned fixtures do not start a resident runner or certify a live provider, browser,
restarted daemon, other adapter, extracted module or generated transport/SDK.

Why best-effort read errors remain separate repair work. W2.2 runs.Start, W3
modules, W4 triggers and W5 generated surfaces remain open. Protected delivery
requires authenticated final-head CI and normal merge. Next: workflow, pulse and
autonomy in roadmap order 4.

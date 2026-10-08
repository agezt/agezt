# W2.29b typed operator update native exit evidence

Both update commands now use typed application operations and shared native dispatch.
The old handlers, map callback codec and manual registry rows are removed. The native
adapter retains the current backend/current version, primary drain, sentinel writer
and delayed shutdown ports. Lower download/checksum/signature/provenance/lock/drain
verification and W4.5 release-signing work remain distinct.

| Operation | Native contract |
|---|---|
| update_check | Primary ReadOnly unary; CheckRequest/CheckOutput; no public HTTP metadata |
| update_apply | Primary mandatory-audit unary; ApplyRequest/ApplyOutput; no public HTTP metadata |

## DTOs, codec and admission

Check always includes current/update/up_to_date; no update is JSON null. Disabled
adds status; an available release has four required public version/sha256/url/notes
strings, including empty notes. Lower provenance/signature are not presentation
fields. Apply always includes applied, with failure-only error and success-only
version. Pointer fields preserve present zero/empty values versus absent members.
Nested output schemas and both exact unique AppOwned registrations are pinned.

RawMessage apply fields preserve strict version→sha256→url→notes errors, raw string
lexemes/whitespace and unknown fields. Service-disabled remains earlier than codec
errors. The two primary operations reject tenant/system/agent callers before the
provider; actual tenant sockets independently reject both commands. Pre-canceled
requests stop before backend effects, journal audit or sentinel. Failed mandatory
audit admission prevents Apply; Check remains readable with a closed journal.

## Two different terminal ownership contracts

The stdlib-only TerminalCleanup port owns cancellation until terminal completion,
including panic. Direct service callbacks retain their original lifetime; absent
or rejected ownership leaves cleanup with the service. Transfer occurs only when
presentation is ready. Backend panic and nil successful Check result cancel before
the opaque internal-error response writer is entered.

[TerminalWrite](../kernel/contract/opapi/terminal_write.go) separately owns callbacks
until the response writer returns, including a returned write error. A writer panic
discards them. [The native owner](../kernel/controlplane/app_terminal_write.go) closes
under its ownership mutex and executes accepted callbacks outside it, exactly once
in LIFO order. Reentry, concurrent acceptance/finalization, late rejection and a
callback panic are tested. The remaining callbacks still run when another panics.

Successful Apply writes sentinel before response framing, then transfers a100ms
restart schedule to TerminalWrite. Every result/error writer-return path finishes
this ownership; panic unwind discards it and still releases cancellation cleanup.
The unchanged paused success/error expectations pass through the shared dispatcher.
Returned write failures still restart; writer panics do not. Actual terminal audit
failure after successful Apply returns an error response, retains sentinel/effects,
keeps the call context alive through that error write and schedules restart only
after it returns. Direct service fallback and panic behavior remain tested.

## Runnable evidence

480 native cases repeated twenty times cover six selected backend states, twenty
inputs, both commands and normal/canceled contexts.240 normal cases preserve complete
raw response bytes, backend effects, unverified manifest fields and sentinel presence.
240 canceled cases assert the explicit admission change with no effects. Sentinel
clock, audit pair/kind/correlation/actor/subject and read non-auditing are checked
independently. No raw response normalization is used.

Twenty-seven service/DTO/codec/spec/terminal/registration mutations produce permanent
test failures and restore production sources byte-for-byte. Both registration omissions,
early/missing finalization, wrong panic behavior, missing ownership and opaque error
boundaries are detected. Durable full-path backups support interrupted-proof recovery.
The original naive return bridge was red3 on early cancellation and restart while
the socket was blocked; the corrected native binding passes20.

Direct DTO golden member order was updated for struct results; native bytes remain
independently exact. A temporary parity source was removed during a parallel vet
enumeration; subsequent package checks were serialized and passed. An audit fixture
initially expected nil output, while the existing dispatcher retains output beside
its joined terminal-audit error; its assertion was corrected. Those harness issues
are excluded from product findings.

No-I/O Windows/amd64 dispatch, GOMAXPROCS=4,200ms x3 per operation, measures
9.0–13.1us/op, below50us. It includes schema/admission/service/presentation and fake
audit; excludes journal/network/storage, real drain and restart I/O. Fixtures do not
download/swap a release, expose a tunnel or restart a real daemon.

Source/update/opapi/native/lifecycle/tenant/admission/send suites pass20 and their
race variants pass20. Whole controlplane race and all repository Go tests pass once.
Build, vet, scoped staticcheck, archcheck, deadcodecheck, depscheck, gofmt and official
structure generation/check pass. Archcheck retains244 packages/135 import exceptions/
13 call exceptions; no allowance grows. Gofmt covers2,909 current Go files.2,914
source/build/architecture input hashes remain unchanged across closure. Documentation,
payload/commit-history secret gates and protected exact-head CI/main delivery close
as separate publication steps.

Order7 roster/steer/runs/journal/edict/tenant/shutdown/remote, broader adapter/run/tool
convergence, W3 modules/raw-ref GC, W4 triggers/channels/signing and W5 generated
surfaces/zero allowances remain open.

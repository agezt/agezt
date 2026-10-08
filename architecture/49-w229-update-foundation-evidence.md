# W2.29a operator update service foundation and tunnel premise

This move gives operator update business ordering an application owner. The two
native commands still use their manual registrations and callback terminal codec.
Concrete operation DTOs, shared admission and native exit remain the next slice.

## Measured tunnel boundary

No native tunnel command/handler/protocol/client operation exists in the current
control plane. Daemon boot calls buildTunnel after constructing HTTP surfaces;
target selection, public URL/password presentation and host allowlisting belong to
that composition adapter. The existing layer5 tunnel supervisor already launches
through platform/sandbox. No operation is invented to satisfy a stale domain-order
premise. Broader lifecycle/module/public-exposure work remains open.

Boot target/URL helper tests pass twenty repetitions; the full tunnel source race
suite passes twenty repetitions. Its tests use owned fake supervisors and harmless
process stand-ins. An initial name filter selected no tunnel-package tests and is
excluded from the evidence. No public tunnel provider is started.

## Update ownership and error ordering

[The application service](../kernel/app/update/service.go) owns disabled behavior,
check presentation, ordered required fields, manifest construction, background call
contexts, apply failure translation and sentinel/response/restart sequencing.
[Its backend port](../kernel/app/update/backend.go) preserves the lower verified
engine's existing Check/Apply/DrainResult contract. Native adapters select the
current Server backend/current version, primary-kernel drain, sentinel writer and
delayed graceful shutdown. The public concrete SetUpdateService API remains; nil
is explicitly canonicalized before assigning the interface field.

Check retains current/update/up_to_date plus disabled-only status, null update and
four release fields including empty notes. Lower CheckResult.Err remains ignored,
as before. Backend failures keep the original response text; direct service errors
also retain their cause. Nil successful backend results and backend panics preserve
legacy panic behavior and cleanup.

Apply checks disabled before native DecodeError and required fields. Strict raw
strings remain untrimmed, including accepted whitespace values. Missing version,
sha256 and url errors retain order and separator; notes remains optional and strict
when present. Caller fields build an unverified manifest with no signature: this
move adds no caller-controlled trust or signing path. Wrapped drain timeout becomes
the exact applied:false/error result; other backend failures keep update-failed
presentation. Neither error path schedules restart or writes the sentinel.

## Terminal context and restart contract

Check uses a background60s timeout; Apply uses a background cancel context. Both
exclude caller values/cancellation and remain alive through the synchronous native
callback. Their contexts cancel after callback return, callback panic, backend panic
or failed write. Successful Apply writes its best-effort sentinel before terminal
delivery, then schedules shutdown100ms after the writer returns.

A returned write error still schedules shutdown. A writer panic skips scheduling.
[Actual paused/failed/panicking writer tests](../kernel/controlplane/update_lifetime_test.go)
pin those different outcomes, immediate backend-panic cleanup and raw manifest
identity. A naive callback-capturing return bridge fails three times: all four
check/apply success/error contexts cancel too early, and restart happens while the
successful response is blocked. Exact source restoration follows the proof.
The callback bridge preserves the measured contract and passes twenty repetitions.

Cleanup ownership alone cannot represent this restart boundary: its release also
runs during panic unwind. The next typed binding needs explicit after-writer-return
ownership as well as cancel cleanup; failed writes and writer panics must remain
separate, independently verified outcomes.

## Runnable evidence and scope

480 legacy/current actual native cases repeated twenty times cover two commands,
six backend states, twenty argument variants and normal/pre-canceled contexts.
Complete raw response bytes, backend effects/manifest fields and sentinel presence
match. The sentinel's RFC3339 clock is independently bounded; writer audit kind,
actor/subject and joined correlation are asserted separately, and reads remain
unaudited. The foundation deliberately preserves legacy in-progress/background
cancellation behavior; shared pre-canceled admission is still future work.

Nineteen service/codec/error/context/manifest/port/restart/nil-backend mutations
produce permanent test failures and restore sources byte-for-byte. The initial
typed-nil fixture panicked in a bare test goroutine; it now uses the same native
panic containment as the real socket path, then asserts the disabled response.
That harness correction is excluded from product findings.

The lower cancellation test's old10-second sleeping server blocked httptest.Close
on every repetition and exhausted the local three-minute package budget. Its owned
handler now waits for explicit release; after request entry, caller cancellation
must return context.Canceled before the distinct five-second client timeout. The
caller-context-removal mutation fails three times; source and race suites pass
twenty repetitions with the original test counts/CI timeout unchanged. Lower
production verification code restores exactly after that extra mutation.

Fixtures use fake backends, callbacks and owned temporary state. They do not
download a release, swap a real binary, restart a daemon or alter lower checksum,
signature, provenance, download, lock and drain verification. W4.5 release signing,
typed update exit, order7, wider adapters and W3–W5 remain open.

## Local closure

Source packages and focused native/source suites pass twenty repetitions, with
race variants also passing twenty. Whole controlplane race and all repository Go
tests pass once. Build, vet, scoped staticcheck, archcheck, deadcodecheck, depscheck,
gofmt and official structure generation/check pass. Archcheck places244 packages
with unchanged135 import and13 call exceptions. Gofmt covers2,901 current Go files;
the official kernel structure contains135 packages. The recorded test-harness change
is the only input change after the initial snapshot;2,906 source/build/architecture
hashes are verified for current closure. Documentation/payload/history secret gates
and exact-head protected CI/main delivery remain separate publication steps.

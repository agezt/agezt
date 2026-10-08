# W2.27 channel eleven-operation native exit evidence

This closes the channel operations' native application binding on shared main.
It does not close transport supervision, conversation storage, run ingress,
webhook/tunnel/update, remaining order7 domains, or W3–W5.

| Operation | Native policy and result |
|---|---|
| channel_list | Primary read; typed channel inventory and media probes |
| channel_account_set | Primary audited writer; typed account result |
| channel_account_remove | Primary audited writer; typed removal result |
| channel_oauth_start | Primary audited writer; typed authorization start |
| channel_oauth_callback | Primary audited writer; typed exchange result |
| channel_oauth_status | Primary read; typed presence-preserving status |
| whatsappgw_status | Primary read; typed gateway status |
| whatsappgw_qr | Primary read; typed gateway QR |
| inbox | Primary read; typed threads/messages and exact int64 timestamps |
| send | Primary audited writer; required sent/channel/to result |
| acp_agents | Primary read; typed active/default agent discovery |

## Ownership and registration

[The native exit regression](../kernel/controlplane/channels_native_exit_test.go)
pins eleven unique AppOwned registrations, concrete input/output types, six reads
and five writers, primary authorization/tenancy, unary delivery, unknown-input
compatibility and absent emission schemas. All eleven individual registration
omissions make that regression fail; production source restores byte-for-byte.
The empty channel registrar and manual send registration/wrapper are removed.

Application services own validation, presentation and use-case ordering. Selected
Server ports remain at the native boundary: current sender, channel inventory,
account/OAuth stores, gateway HTTP, inbox journal and ACP source. These ports do
not imply that channel transports or lower stores have completed W3/W4 migration.
The existing Web UI POST send route is byte-for-byte unchanged. OAuth callback
retains its internal native operation and existing credential-forwarding route.

## Send terminal ownership

[Outbound.Send](../kernel/app/channels/send.go) is the production service entry.
It retains trimmed required fields, lowercased channel, validation before sender
availability, original error identity and a background 30-second sender context
that excludes caller values and cancellation. Its concrete result exposes only
sent/channel/to; permissive RawMessage fields retain wrong-type-as-empty behavior.

[TerminalCleanup](../kernel/contract/opapi/terminal.go) is a stdlib-only contextual
ownership port. Ownership transfers only after the sender returns. A rejected or
absent owner keeps cleanup with the caller; direct callback users retain cleanup
after their callback. Sender panic unwinds before transfer and cancels immediately.
The typed operation preserves the native opaque `internal error` response.

[The native terminal scope](../kernel/controlplane/app_terminal.go) releases after
result/error socket delivery, including failed writes and panics. It closes and
clears ownership under its mutex, then runs accepted callbacks outside the lock
in LIFO order. Reentry and concurrent release cannot execute a callback twice;
remaining callbacks still run when another callback panics.

A naive callback-capturing return bridge canceled the sender context before socket
delivery: the original paused-write regression failed three times. The corrected
shared binding passes that unchanged success/error lifetime expectation twenty
times. Permanent tests also cover failed/panicking writes, immediate sender-panic
cleanup, rejected/absent scopes, context isolation, reentry and concurrent ownership.

## Admission, parity and evidence boundaries

Shared dispatch rejects pre-canceled requests and failed mandatory audit admission
before the sender factory or effect. Tenant requests remain rejected. Writer audit
retains invocation/terminal pairing, correlation, actor/subject and text redaction.

The current native parity proof runs 160 cases twenty times: four sender states
(absent/success/error/panic), twenty argument variants and two context states.
Eighty normal cases preserve complete raw response bytes, owned effects and files,
including panic opacity. Eighty canceled cases independently assert the deliberate
admission change: no sender effects and no operation audit. Deadline/cleanup,
correlated audit/privacy and zero provider calls are asserted separately.

Thirty-four service/spec/terminal/native/contract mutations produce real test
failures and restore source exactly. An initial panic-text assertion gap was
strengthened; a compiler-only fixture error was repaired and excluded. Eleven
additional native registration omissions are independently detected.

## Local closure

Current scoped source tests and related package race tests pass twenty repetitions;
native send/terminal/exit race tests pass twenty repetitions. Full controlplane race
and all repository Go tests pass once. Build, vet, scoped staticcheck, archcheck,
deadcodecheck, depscheck, gofmt, official structure generation/check, docclaimscheck,
changelog lint and frozen-payload secret checks pass. Source/build/architecture
input hashes remain unchanged across closure. Archcheck retains 242 placed packages,
135 import exceptions and 13 call exceptions; none is added for this binding.
Gofmt covers 2,891 current Go files. Final commit-history secret scanning and
protected exact-head CI/main delivery remain separate publication steps.

These fixtures use owned temporary stores and fake senders. They do not send live
channel messages or certify deployed transports, gateway credentials, daemon
restart, installed binaries or every inbound adapter. Exit scope remains native.

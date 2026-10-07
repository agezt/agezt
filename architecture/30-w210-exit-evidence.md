# W2.10 workboard native exit evidence

Prepared locally: protected main delivery of W2.10g-i is pending. W2.10a-e was
verified merged as PR #700. Background execution was submitted as PR #701;
its final status is unverified since the session lost Git write/GitHub access.

Twenty-one workboard native commands now bind typed app/workboard operations
through the common app host. This completes the workboard native slice in roadmap
order 3; OKR/storage/artifacts, remaining native domains, broader adapters,
generated surfaces, runs.Start and W3-W5 remain open.

| Operations | Scope and ownership |
|---|---|
| workboard_list, workboard_lanes, workboard_show, workboard_watch | Four primary-only reads, unaudited unary snapshots over selected store/journal ports. |
| workboard_create, workboard_claim, workboard_heartbeat, workboard_comment, workboard_block, workboard_fail, workboard_unblock, workboard_complete, workboard_prove, workboard_seat, workboard_archive | Eleven lifecycle mutations; mandatory audit before actual kernel/store/seat effects. |
| workboard_link, workboard_policy, workboard_depend, workboard_reclaim, workboard_sweep | Five relation/maintenance mutations; mandatory audit before effects. |
| workboard_dispatch | Mandatory audited admission, dependency/agent checks, fresh claim/run link/publication and owned asynchronous execution. |

## Coverage and compatibility

TestWorkboardNativeExitCompleteTypedRegistry guards exact twenty-one-command
coverage in the common registry, actual input/output types and schemas, AppOwned
metadata, primary/read-only/unary scope and legacy unknown-input compatibility.
Removing the family or dispatch operation independently fails the exit regression.
Old socket business wrappers, registration, task/decision/dependency views and
response/run-selection/event-fold helpers are removed. Shared legacy correlation/
integer and string-list admission remains because OKR and seat still call it;
the selected native context/execution-profile bridge remains a host integration.

Typed task records preserve durable task JSON plus comment/link/attempt/failure
counts and conditional criteria/proof/retry fields. Reads retain filtering/lane
projection and wire shape. Actual lifecycle ports retain transitions, journaling,
idempotence, retry/escalation decisions, prove timeout and error causes. Relation
ports retain policy presence/clear, dependency alias/cycles, stale duration/default/
limit admission and empty sweep arrays.

Request DTOs retain lenient trimmed/nonstring fields, integer/limit defaults and
caps, mixed-list/scalar admission, maximum presence/null and the kernel's empty
policy normalization, strict booleans, seat catalog checks and unknown fields.
Prove keeps a 90-second deadline and inherits the admitted operation context.
Native parity covers twenty-one handlers x three inputs count=20; generated IDs,
bounded binding clocks and the common generic failure-code envelope are explicit
normalization boundaries. Domain errors and remaining result fields are checked.

Watch remains one snapshot, not a new stream. Explicit/claim/attempt/link run
selection, subject-or-run filtering, stable chronological newest tail, required
empty correlation, absent/null/empty-object payload shape, null empty events and
optional dependency fields remain. Actual empty/partial JSONL decode failures and
dependency errors previously returned success; W2.10h separately repairs them to
return the original read cause and zero output. Native tests require one terminal
error frame with no partial result/event/provider effect. Actual owned journal and
sentinel regressions are red against the original service, pass three verifier
runs count=20, and fail when either error gate is suppressed.

## Audit, identity and execution evidence

Permanent actual closed-journal fixtures independently enumerate all seventeen
mutations, ensuring audit failure prevents task, seat, claim, link, publication
and background effects. Four reads remain usable without mutation audit. Actual
primary/acme socket clients deny every workboard command to tenant credentials
without changing primary state. The tenant task field remains a primary task
label; it does not route these primary-only operations to a tenant kernel.

Default lifecycle/domain events now share the host-owned operation audit
correlation, with one invoked/domain/completed chain. Explicit inbound correlation
remains a native bridge contract. Dispatch generates a separate fresh run
correlation and returns it with the task claim/run link; a client cannot choose
that run identity. Actual mock-provider native dispatch acceptance and subsequent
review settlement are covered by an owned asynchronous integration test.

Transport-independent execution owns seat selection/precedence, degradation
comments, failure/reclaim/task retry recursion and current-claim proof/review
settlement. The selected bridge binds the entire agent profile, wake reason/
subject, cost ceiling, model/tools, agent retry policy and actual warden policy/
backend APIs. Actual store/mock-provider old/current execution parity covers eight
cases count=20: review, reader, missing/unavailable isolation, failure, task retry,
proof and proof fallback. Successful and failed settlement paths, same-correlation
reclaim, claim ownership and Unicode 240/300-byte summaries have permanent tests.

## Exit validation and boundaries

Sixty independent mutation proofs cover the six extraction slices, typed audit
binding, read-error repair and aggregate exit. Native/service/store/CLI focused
suites and workboard package race pass count=20. Complete controlplane race and
final full Go tests pass count=1; build/vet, scoped staticcheck, formatting,
architecture/deadcode/dependency/document/changelog and official structure gates
pass. Counts remain 228 packages, 141 import/13 call exceptions, 28 SDK findings
plus one test seam, 24 dependencies and 119 generated kernel packages. No
allowlist is expanded or removed ahead of remaining callers.

BenchmarkDispatchWithoutAuditIO, three 100 ms runs: 5932 / 5636 / 6374 ns/op;
7607 / 7608 / 7608 B/op; 89 / 89 / 89 allocs/op. All satisfy the <50 us framework
budget. Construction and audit/journal I/O are excluded; this does not certify
whole-operation roundtrip timing or provider latency.

Fixtures use owned TempDir stores/journals, mock providers and socket clients.
No live-provider, physical-browser or generated transport/SDK coverage is claimed.
Restricted-session validation uses fresh task-owned Go/staticcheck caches;
build exits successfully with a warning about the unavailable shared module stat
cache. Protected delivery still requires all final-head CI gates before merge.

Next: OKR, then storage/artifacts in roadmap order 3.

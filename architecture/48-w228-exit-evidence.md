# W2.28 webhook observability native exit evidence

This slice moves the two outbound delivery observability commands into the
application boundary. It does not replace the outbound dispatcher, webhook ingress,
transport supervision, trigger pipeline, tunnel or updater.

| Operation | Native contract |
|---|---|
| webhook_log | Typed LogRequest/LogOutput; ReadOnly, OwnTenant/CallerTenant, unary; existing GET /api/webhook_log |
| webhook_stats | Typed StatsRequest/StatsOutput; ReadOnly, OwnTenant/CallerTenant, unary; no new public HTTP route |

## Ownership and compatibility

[Application observability](../kernel/app/webhook/observability.go) owns delivery
decode, filtering and statistics. The native provider selects the routed kernel's
journal from appHost. The old webhook_log.go business handlers and manual registry
rows are removed. Both unique registrations derive native AppOwned/read/tenant
metadata from their operation specs; [the exit regression](../kernel/controlplane/webhook_typed_test.go)
pins concrete signatures and unary/non-emitting contracts.

The lower platform/journalview engine continues to own timestamp/sequence ordering,
cutoff, cursor filtering and page construction. Malformed or non-string cursors are
ignored, as before; count is returned page length. Numeric limit defaults to20,
truncates fractions and clamps1–1000. Non-numeric limit is ignored. Positive since_ms
sets the clock cutoff; nonpositive values mean all time. Stats echoes the original
truncated signed window, including negative values; non-numeric windows become0.

Log failed remains a strict optional boolean: present null/non-boolean fails before
the service provider. Unknown or irrelevant fields remain accepted. RawMessage
codecs retain the native JSON float64 input conversion, while typed int64 output
uses the shared exact terminal codec. A native regression proves sequence and
timestamp values above2^53 remain exact.

Delivered rows always include status, even0, and omit error. Failed rows always
include error, even empty, and omit status. Other row fields retain empty/false/zero
presence. Payload decode errors retain encoding/json's partial/zero behavior.
Empty log results contain[]; empty stats contain{} for by_url and zero failure rate.
Stats preserves per-URL delivered/failed counts, malformed URL grouping, ratio,
window and Range error identity. Failed reads return no partial success result.

## Admission and tenant isolation

The unchanged before-proof failed three times for both pre-canceled native reads:
the old handlers still returned results. Shared dispatch now rejects before provider
selection/journal iteration; the same regression passes twenty repetitions.
Read operations remain unaudited. No provider invocation or operation-audit append
is added to observability.

Actual socket regressions exercise primary, operator-selected tenant and tenant-token
reads against three independent initialized kernel journals. Both operations return
only the selected journal's private fixture URLs; a tenant token cannot read another
tenant. Routed context is also independently asserted at the application provider.

## Runnable evidence

240 native cases repeated twenty times cover two commands, thirty input variants,
empty/populated journals and normal/canceled contexts.120 normal cases preserve
complete raw response bytes, owned file hashes and journal head without normalization.
120 canceled cases assert the intended earlier admission error. Provider count and
operation-audit/file effects remain zero. Fixtures send no external webhook.

Twenty-seven independent service/codec/spec/provider/registration mutations make
permanent tests fail. All mutated sources restore byte-for-byte; durable full-path
backups support recovery. A fixture's initial expected Range-call count was corrected
from6 to5 and excluded from product findings. Both registration omissions are detected.

Scoped service/projection/native/tenant/schema/precision tests and race tests pass
twenty repetitions. A Windows/amd64 no-I/O dispatch benchmark, GOMAXPROCS=4,
200ms x3 per operation, measures24.6–27.8us/op, below the roadmap's50us budget.
It includes decoding, schema validation, routing, service selection and projection
over one owned fake event; it excludes journal/network/storage I/O and deployment.

Full controlplane race and all repository Go tests pass once; build, vet, scoped
staticcheck, archcheck, deadcodecheck, depscheck, gofmt and official structure
generation/check pass. Archcheck places243 packages with unchanged135 import and13
call exceptions; gofmt covers2,897 current Go files. Source/build/architecture hashes
remain unchanged across closure. Documentation/payload/commit-history secret gates
and protected exact-head CI/main delivery close separately. Remaining order6/7 and
W3–W5 stay open.

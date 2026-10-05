# W2.4 catalog/provider native exit evidence

The catalog/provider pilot now uses typed app operations through the native
control-plane adapter. This closes roadmap §3 order 1 alongside status/version;
the remaining domains, broader adapters and W3–W5 remain open.

| Operations | Metadata and ownership |
|---|---|
| catalog_sync, catalog_discover | Primary-only mutations; app/catalog owns fetch/persistence/reload and domain events. |
| catalog_list | Primary-only read; typed provider/model/pricing projection. |
| provider_connect, provider_reload, provider_key_add, provider_key_activate, provider_key_remove | Primary-only mutations; app/providers owns catalog/keyring changes and reload. |
| provider_key_list | Primary-only read; labels, active flags and last-four fingerprints only. |
| provider_oauth_start, provider_oauth_import, provider_oauth_logout | Primary-only mutations; per-server app/providers.OAuth owns login/token state. |
| provider_oauth_status | Primary-only read; preserves the authenticated account's reported models, including an empty result. |
| provider_log, provider_stats, provider_rejections | OwnTenant/CallerTenant reads; observations use the host-selected journal. |
| provider_probe | Primary-only read; typed URL/key admission and caller-context-aware guarded HTTP. |

All 17 operations declare actual input/output types and schemas, StreamNone and
the existing unknown-argument compatibility. `TestCatalogProviderNativeExitCompleteTypedRegistry`
checks the complete set, typed metadata, AppOwned binding and derived native
read-only/tenant/stream flags. The existing registry/protocol, tenant-routing,
authentication and operation-audit suites continue to check the production host.
Two independent mutations remove the probe or mark it mutating; the aggregate
test rejects each, and passes count=20 after exact source restoration.
Old `handleCatalog*`/`handleProvider*` business handlers are absent from the control
plane. Its common adapter decodes arguments, dispatches and encodes results.

## Behavior and lifetime evidence

Source and app suites retain wire field presence, optional prices, model catalog
authority, key privacy, native error framing and operation correlation. Mutating
operations require successful audit admission before effects; AppOwned binding
avoids duplicate native audit. Observation fixtures retain journal isolation,
sorting/cursors, filtering and the distinction between model-chain and provider
fallbacks. Earlier move/binding parity and mutation evidence is recorded in
[NEXT.md](NEXT.md), W2.4a–W2.4u.

OAuth callback business accepts context and fields independently of HTTP. The
platform/browsercallback adapter owns query projection, escaped HTML, TCP/mux
and HTTP server mechanics. App owns the prepared listener and publishes login
state before Serve. Permanent controlled fixtures cover Close-before-Serve port
release, concurrent/repeated close and cleanup causes, successful logout release,
expiry cancellation and current-login identity. Delayed token fetch fixtures
prove retired/logout/replaced/canceled callbacks cannot persist candidate tokens;
persistence and logout share session admission under the same mutex.

Typed probe fixtures prove caller cancellation reaches both the injected port and
guarded HTTP transport, including cancellation during response-body reads. Legacy
context-free getters retain their existing background/best-effort behavior.
Production app/providers imports neither net nor net/http.

## Exit validation

On Windows amd64, GOMAXPROCS=4:

- Related controlplane/catalog/providers/chatgptauth/browsercallback/netout source,
  native host, tenant, registry, callback and context suites passed count=20.
- Full `go test ./... -count=1 -timeout=10m`, `go build ./...`, `go vet ./...`
  and staticcheck for those six packages passed.
- Archcheck passed: 222 packages, 143 import and 13 call exceptions. Deadcodecheck
  passed with 28 public SDK findings and one test seam; depscheck passed with
  24 justified core dependencies. No exception growth.
- `BenchmarkDispatchWithoutAuditIO`, three 100 ms runs: 6,359 / 6,899 / 6,913
  ns/op, 7,608–7,611 B/op and 89 allocs/op. The measured 6.4–6.9 us/op is below
  the <50 us dispatch budget; construction and real audit/journal I/O are excluded.

Fixtures use isolated homes, fake token fetches and owned loopback HTTP/listeners.
These checks do not certify a live provider, paid model, browser login, generated
HTTP/OpenAPI/SDK surfaces or provider boot/runtime module dissolution. Protected
delivery still requires every final-head CI check before normal merge.

Next in roadmap order: memory, world, taste and skill app operations. Full
runs.Start convergence and the remaining architecture migration remain open.
